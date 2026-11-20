package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestTransaction(t *testing.T) {
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	app := application{store: repo}
	now := time.Now()
	day := now.Format(dateLayout)
	errs := make(chan error, 8)
	for range cap(errs) {
		go func() { _, err := app.execute(t.Context(), day, "add", []string{"test"}, now); errs <- err }()
	}
	for range cap(errs) {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(repo.path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := app.execute(t.Context(), day, "status", nil, now)
	if err != nil || len(s.Tasks) != cap(errs) {
		t.Fatal("lost update", err)
	}
	if _, err := app.execute(t.Context(), day, "delete", []string{s.Tasks[0].ID, "missing"}, now); err == nil {
		t.Fatal("expected invalid id")
	}
	after, err := os.ReadFile(repo.path)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed command changed data", err)
	}
}

func TestCanceledUpdateDiscardsMutations(t *testing.T) {
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, err := repo.update(ctx, func(s *state) error {
		s.Tasks = append(s.Tasks, &task{ID: "not-saved"})
		cancel()
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled update returned %v", err)
	}
	if _, err := os.Stat(repo.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled update wrote a snapshot: %v", err)
	}
	app := application{store: repo}
	now := time.Now()
	if _, err := app.execute(ctx, now.Format(dateLayout), "ls", nil, now); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read returned %v", err)
	}
}

func TestSuggestions(t *testing.T) {
	m := terminal{day: "2026-11-20", input: "add 15 re", data: &state{Tasks: []*task{
		{Title: "Read"}, {Title: "read"}, {Title: "Review"}, {Title: "Removed", Deleted: true}, {Title: "Reserved", Day: "2027-01-01"},
	}}}
	prefix, matches := m.addSuggestions()
	if prefix != "add 15 " || len(matches) != 2 || matches[0] != "Review" || matches[1] != "read" {
		t.Fatal(prefix, matches)
	}
	m.edit(tea.KeyPressMsg{Code: tea.KeyDown})
	m.edit(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.input != "add 15 read" {
		t.Fatal(m.input)
	}
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	m.app = application{store: repo}
	m.mode = "command"
	m.edit(tea.KeyPressMsg{Code: tea.KeyEnter})
	saved, err := repo.load(t.Context())
	if err != nil || len(saved.Tasks) != 1 {
		t.Fatal("registration failed", err)
	}
	task := saved.Tasks[0]
	if task.Title != "read" || task.Minutes == nil || *task.Minutes != 15 {
		t.Fatal(task)
	}

}

func TestCommandCursorEditsAndSavesMultibyteInput(t *testing.T) {
	now := time.Now()
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	m := terminal{app: application{store: repo}, data: &state{}, now: now, day: now.Format(dateLayout), width: 16}
	update := func(msg tea.Msg) {
		t.Helper()
		next, _ := m.Update(msg)
		m = next.(terminal)
	}
	update(tea.KeyPressMsg{Code: ':', Text: ":"})
	update(tea.PasteMsg{Content: "add 15 長いタスク"})
	cursor := m.View().Cursor
	if cursor == nil || cursor.Position.X >= m.width {
		t.Fatalf("end cursor outside command viewport: %+v", cursor)
	}
	update(tea.KeyPressMsg{Code: tea.KeyHome})
	update(tea.KeyPressMsg{Code: tea.KeyRight})
	cursor = m.View().Cursor
	if cursor == nil || cursor.Position.X != 5 {
		t.Fatalf("cursor didn't move within the command: %+v", cursor)
	}
	update(tea.KeyPressMsg{Code: tea.KeyEnd})
	update(tea.KeyPressMsg{Code: tea.KeyLeft})
	update(tea.KeyPressMsg{Code: tea.KeyDelete})
	update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	update(tea.PasteMsg{Content: "イトル"})
	update(tea.KeyPressMsg{Code: tea.KeyEnter})
	saved, err := repo.load(t.Context())
	if err != nil || len(saved.Tasks) != 1 || saved.Tasks[0].Title != "長いタイトル" {
		t.Fatalf("edited command was not saved: %+v, %v", saved, err)
	}
}

func TestInvalidSnapshot(t *testing.T) {
	for _, task := range []string{"null", `{"id":"a","state":"running"}`} {
		if _, err := decodeState([]byte(`{"schema":1,"tasks":[` + task + `]}`)); err == nil {
			t.Fatal("accepted", task)
		}
	}
}

func TestStartDoneTaskCreatesNewRun(t *testing.T) {
	now := time.Now()
	day := now.Format(dateLayout)
	started := now.Add(-time.Hour)
	ended := now.Add(-time.Minute)
	minutes := 30.0
	original := &task{
		ID: "original", Day: day, Title: "Read", State: "done", Minutes: &minutes,
		WorkCode: "PJ-042", Category: "work", SectionID: "morning", RoutineID: "routine-1",
		Sessions: []session{{Start: started, End: &ended}}, Notes: []note{{Text: "finished a chapter"}},
	}
	s := &state{Tasks: []*task{original}}
	if err := s.apply(day, "start", []string{original.ID}, now, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Tasks) != 2 {
		t.Fatalf("got %d tasks", len(s.Tasks))
	}
	restarted := s.running()
	if restarted == nil || restarted.ID == original.ID || restarted.Title != original.Title ||
		restarted.Day != day || restarted.WorkCode != original.WorkCode || restarted.Category != original.Category ||
		restarted.SectionID != original.SectionID ||
		restarted.RoutineID != original.RoutineID || restarted.Minutes == nil || *restarted.Minutes != minutes {
		t.Fatalf("incorrect task copy: %+v", restarted)
	}
	if len(restarted.Notes) != 0 || len(restarted.Sessions) != 1 ||
		!restarted.Sessions[0].Start.Equal(now) || restarted.Sessions[0].End != nil {
		t.Fatal("new run inherited old history")
	}
	if original.State != "done" || len(original.Notes) != 1 || len(original.Sessions) != 1 || original.Sessions[0].End == nil {
		t.Fatal("original history changed")
	}
}

func TestRoutineSectionDefaultsAndDailyAssignments(t *testing.T) {
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	app := application{store: repo, sections: []section{
		{ID: "morning", Name: "Morning", Start: "00:00", End: "12:00"},
		{ID: "afternoon", Name: "Afternoon", Start: "12:00", End: "24:00"},
	}}
	now := time.Date(2026, time.November, 20, 9, 0, 0, 0, time.UTC)
	execute := func(day, cmd string, args ...string) *state {
		t.Helper()
		s, err := app.execute(t.Context(), day, cmd, args, now)
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		return s
	}
	const first, second, third = "2026-11-20", "2026-11-21", "2026-11-22"
	taskID := execute(first, "add", "Read").Tasks[0].ID
	if _, err := app.execute(t.Context(), first, "section", []string{taskID, "missing"}, now); err == nil {
		t.Fatal("accepted an unknown section")
	}
	if s := execute(first, "ls"); s.Tasks[0].SectionID != "" {
		t.Fatal("failed assignment changed the task")
	}
	execute(first, "section", taskID, "morning")
	s := execute(first, "routine", taskID, "daily")
	routineID := s.Order[0]
	if s.Routines[routineID].SectionID != "morning" {
		t.Fatal("new routine did not inherit the source task's section")
	}
	secondTask := execute(second, "ls").rows(second)[0]
	if secondTask.SectionID != "morning" {
		t.Fatal("new occurrence did not inherit the routine's section")
	}
	s = execute(second, "section", secondTask.ID, "-")
	if s.rows(second)[0].SectionID != "" || s.Routines[routineID].SectionID != "morning" {
		t.Fatal("daily assignment did not clear independently of routine")
	}
	s = execute(third, "routine-section", routineID, "afternoon")
	if s.rows(third)[0].SectionID != "afternoon" {
		t.Fatal("new occurrence did not inherit the changed routine section")
	}
	if s.rows(first)[0].SectionID != "morning" || s.rows(second)[0].SectionID != "" {
		t.Fatal("routine assignment changed an existing task")
	}
	saved, err := repo.load(t.Context())
	if err != nil || len(saved.rows(third)) != 0 {
		t.Fatalf("routine edit must not persist generated occurrences: %v", err)
	}
	s = execute(third, "routine-section", routineID, "-")
	if s.Routines[routineID].SectionID != "" || s.rows(third)[0].SectionID != "" {
		t.Fatal("clearing the routine section did not carry over to a new occurrence")
	}
}

func TestRoutineWorkCodeIsInheritedByNewTasks(t *testing.T) {
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	app := application{store: repo}
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	const first, second, third, fourth = "2026-11-20", "2026-11-21", "2026-11-22", "2026-11-23"
	execute := func(day, cmd string, args ...string) *state {
		t.Helper()
		s, err := app.execute(t.Context(), day, cmd, args, now)
		if err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
		return s
	}
	id := execute(first, "add", "Read").Tasks[0].ID
	execute(first, "code", id, "PJ-042")
	routineID := execute(first, "routine", id, "daily").Order[0]
	next := execute(second, "ls").rows(second)[0]
	if next.WorkCode != "PJ-042" {
		t.Fatalf("generated task has work code %q", next.WorkCode)
	}
	execute(second, "rename", next.ID, "Read tomorrow")
	if _, err := app.execute(t.Context(), first, "routine-code", []string{routineID, "invalid code"}, now); err == nil {
		t.Fatal("accepted whitespace in a routine work code")
	}
	execute(first, "routine-code", routineID, "PJ-043")
	if got := execute(third, "ls").rows(third)[0].WorkCode; got != "PJ-043" {
		t.Fatalf("generated task did not inherit updated work code: %q", got)
	}
	if got := execute(second, "ls").rows(second)[0].WorkCode; got != "PJ-042" {
		t.Fatalf("updating routine code changed a saved task: %q", got)
	}
	execute(first, "routine-code", routineID, "-")
	if got := execute(fourth, "ls").rows(fourth)[0].WorkCode; got != "" {
		t.Fatalf("generated task did not inherit cleared work code: %q", got)
	}
}

func TestMoveSectionUpdatesPlacementAndOrderTogether(t *testing.T) {
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	app := application{store: repo, sections: []section{
		{ID: "morning", Name: "Morning", Start: "00:00", End: "12:00"},
		{ID: "afternoon", Name: "Afternoon", Start: "12:00", End: "18:00"},
		{ID: "evening", Name: "Evening", Start: "18:00", End: "24:00"},
	}}
	now := time.Date(2026, time.November, 20, 9, 0, 0, 0, time.UTC)
	day := now.Format(dateLayout)
	if err := repo.save(&state{Schema: 1, Tasks: []*task{
		{ID: "afternoon-task", Day: day, State: "pending", SectionID: "afternoon"},
		{ID: "movable", Day: day, State: "pending", SectionID: "morning"},
		{ID: "older-setting", Day: day, State: "pending", SectionID: "removed"},
	}}); err != nil {
		t.Fatal(err)
	}
	move := func(sectionID string, position ...string) *state {
		t.Helper()
		s, err := app.execute(t.Context(), day, "move-section", append([]string{"movable", sectionID}, position...), now)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	check := func(s *state, index int, sectionID string, order ...string) {
		t.Helper()
		rows := s.rows(day)
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		if !slices.Equal(ids, order) || rows[index].SectionID != sectionID {
			t.Fatalf("got order %v with placement %q", ids, rows[index].SectionID)
		}
	}
	check(move("afternoon", "before", "afternoon-task"), 0, "afternoon", "movable", "afternoon-task", "older-setting")
	before, err := os.ReadFile(repo.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.execute(t.Context(), day, "move-section", []string{"movable", "morning", "after", "afternoon-task"}, now); err == nil {
		t.Fatal("accepted an anchor from another section")
	}
	after, err := os.ReadFile(repo.path)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed move changed saved placement or order", err)
	}
	check(move("-", "after", "older-setting"), 2, "", "afternoon-task", "older-setting", "movable")
	check(move("evening"), 2, "evening", "afternoon-task", "older-setting", "movable")
}

func TestTaskMovesRejectEndedSections(t *testing.T) {
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	app := application{store: repo, sections: []section{
		{ID: "am", Name: "Morning", Start: "00:00", End: "12:00"},
		{ID: "pm", Name: "Afternoon", Start: "12:00", End: "24:00"},
	}}
	noon := time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local)
	day := noon.Format(dateLayout)
	tomorrow := noon.AddDate(0, 0, 1).Format(dateLayout)
	if err := repo.save(&state{Schema: 1, Tasks: []*task{
		{ID: "today", Day: day, State: "pending", SectionID: "pm"},
		{ID: "tomorrow", Day: tomorrow, State: "pending", SectionID: "pm"},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"section", "move-section"} {
		if _, err := app.execute(t.Context(), day, cmd, []string{"today", "am"}, noon); err == nil {
			t.Fatalf("%s accepted a section at its end time", cmd)
		}
	}
	s, err := repo.load(t.Context())
	if err != nil || s.rows(day)[0].SectionID != "pm" {
		t.Fatalf("blocked placement changed saved state: %v", err)
	}
	if _, err := app.execute(t.Context(), day, "move-section", []string{"today", "am"}, noon.Add(-time.Second)); err != nil {
		t.Fatalf("section before its end should accept movement: %v", err)
	}
	if _, err := app.execute(t.Context(), day, "move-section", []string{"today", "pm"}, noon); err != nil {
		t.Fatalf("should be able to move out of an ended section: %v", err)
	}
	if _, err := app.execute(t.Context(), tomorrow, "section", []string{"tomorrow", "am"}, noon); err != nil {
		t.Fatalf("tomorrow's section should accept placement: %v", err)
	}
}

func TestRoutinePlaceUpdatesDefaultAndOrderTogether(t *testing.T) {
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	app := application{store: repo, sections: []section{{ID: "morning"}, {ID: "afternoon"}, {ID: "evening"}}}
	now := time.Date(2026, time.November, 20, 9, 0, 0, 0, time.UTC)
	day := now.Format(dateLayout)
	if err := repo.save(&state{Schema: 1, Tasks: []*task{}, Order: []string{"movable", "anchor", "older-setting"}, Routines: map[string]routine{
		"movable":       {ID: "movable", SectionID: "morning", FirstDay: day, Days: []string{"fri"}},
		"anchor":        {ID: "anchor", SectionID: "afternoon", FirstDay: day, Days: []string{"fri"}},
		"older-setting": {ID: "older-setting", SectionID: "removed", FirstDay: day, Days: []string{"fri"}},
	}}); err != nil {
		t.Fatal(err)
	}
	place := func(sectionID string, position ...string) *state {
		t.Helper()
		s, err := app.execute(t.Context(), day, "routine-place", append([]string{"movable", sectionID}, position...), now)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	check := func(s *state, sectionID string, order ...string) {
		t.Helper()
		if !slices.Equal(s.Order, order) || s.Routines["movable"].SectionID != sectionID {
			t.Fatalf("got order %v with placement %q", s.Order, s.Routines["movable"].SectionID)
		}
		for _, task := range s.rows(day) {
			if task.RoutineID == "movable" && task.SectionID == sectionID {
				return
			}
		}
		t.Fatal("new occurrence did not inherit routine placement")
	}
	check(place("afternoon", "after", "anchor"), "afternoon", "anchor", "movable", "older-setting")
	before, err := os.ReadFile(repo.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.execute(t.Context(), day, "routine-place", []string{"movable", "morning", "after", "anchor"}, now); err == nil {
		t.Fatal("accepted an anchor from another section")
	}
	after, err := os.ReadFile(repo.path)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed move changed saved default or order", err)
	}
	check(place("-", "after", "older-setting"), "", "anchor", "older-setting", "movable")
	reordered, err := app.execute(t.Context(), day, "routine-place", []string{"older-setting", "-", "after", "movable"}, now)
	if err != nil || reordered.Routines["older-setting"].SectionID != "removed" {
		t.Fatalf("reordering unassigned routines changed the saved section ID: %v", err)
	}
	check(place("evening"), "evening", "anchor", "movable", "older-setting")
	saved, err := repo.load(t.Context())
	if err != nil || len(saved.rows(day)) != 0 {
		t.Fatalf("routine placement must not persist generated occurrences: %v", err)
	}
}
