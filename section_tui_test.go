package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func sectionPress(m terminal, keys ...rune) terminal {
	for _, code := range keys {
		key := tea.KeyPressMsg{Code: code}
		if code > 0 && code < 127 && code != tea.KeyEnter && code != tea.KeyEsc && code != tea.KeyTab {
			key.Text = string(code)
		}
		next, _ := m.Update(key)
		m = next.(terminal)
	}
	return m
}

func TestSectionListPinsAndDeletesVisibleTasks(t *testing.T) {
	today := time.Now()
	now := time.Date(today.Year(), today.Month(), today.Day(), 11, 0, 0, 0, today.Location())
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{
		{ID: "afternoon-1", Day: day, Title: "First afternoon task", State: "pending", SectionID: "afternoon"},
		{ID: "morning-1", Day: day, Title: "First morning task", State: "pending", SectionID: "morning"},
		{ID: "morning-2", Day: day, Title: "Second morning task", State: "pending", SectionID: "morning"},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{
		{ID: "morning", Name: "Morning", Start: "09:00", End: "12:00"},
		{ID: "afternoon", Name: "Afternoon", Start: "13:00", End: "18:00"},
	}}
	m := terminal{app: app, data: s, now: now, day: day}
	m = sectionPress(m, 'L')
	if got := m.render(100); !strings.Contains(got, "Morning  09:00 – 12:00") ||
		!strings.Contains(got, "Free 03:00:00 | 2 unestimated") ||
		strings.Contains(got, "First afternoon task") {
		t.Fatalf("incorrect section view:\n%s", got)
	}
	m = sectionPress(m, ':', 's', 't', 'a', 'r', 't', tea.KeyEnter)
	if running := m.data.running(); running == nil || running.ID != "morning-1" {
		t.Fatalf("start did not use the first pending task in the section: %+v", running)
	}
	if !m.execute("reset", "morning-1") {
		t.Fatalf("could not reset the task after verifying start: %s", m.notice)
	}
	m.selectTask("morning-1")
	next, _ := m.Update(clockMsg(now.Add(90 * time.Minute)))
	m = next.(terminal)
	for _, key := range []rune{'L', tea.KeyEsc} {
		closed := sectionPress(m, key)
		if closed.list || closed.sectionID != "" {
			t.Fatalf("key %q did not close the section list", key)
		}
		reopened := sectionPress(closed, 'L')
		if reopened.list || reopened.notice != "No section at the current time." {
			t.Fatal("opened a section during a gap")
		}
	}
	if m.sectionID != "morning" {
		t.Fatalf("section changed during a gap: %q, %q", m.sectionID, m.notice)
	}
	if got := m.render(100); !strings.Contains(got, "Morning  09:00 – 12:00") ||
		strings.Contains(got, "Free ") || strings.Contains(got, "unestimated") {
		t.Fatalf("ended section should show its heading without capacity:\n%s", got)
	}
	m = sectionPress(m, '2', 'd', 'd')
	m = sectionPress(m, 'l')
	if !m.list || m.sectionID != "" || len(m.visibleRows()) != 1 || m.target().ID != "afternoon-1" {
		t.Fatalf("counted delete did not use filtered rows: %+v", m.visibleRows())
	}
	background := "\x1b[48;2;36;44;56m"
	rule := background + "\x1b[38;5;245m" + strings.Repeat("─", 100) + "\x1b[0m"
	label := "  Afternoon  13:00 – 18:00  │  Free 05:00:00 | 1 unestimated"
	bar := background + "\x1b[38;5;252m" + label + strings.Repeat(" ", 100-ansi.StringWidth(label)) + "\x1b[0m"
	if got := m.render(100); !strings.Contains(got, rule+"\n"+bar+"\n"+rule) {
		t.Fatalf("full list omitted task placement:\n%s", got)
	}
	next, _ = m.Update(clockMsg(now.Add(2 * time.Hour)))
	m = sectionPress(next.(terminal), 'L', 'b', 'g', tea.KeyEnter)
	if len(m.visibleRows()) != 0 || m.target() != nil {
		t.Fatal("clearing a placement should remove its row from the current section")
	}
	m = sectionPress(m, ':', 's', 't', 'a', 'r', 't', tea.KeyEnter)
	if m.data.running() != nil || m.notice != "No pending task in this section." {
		t.Fatal("start from an empty section should leave hidden tasks pending")
	}
}

func TestGroupedListMovesThroughAdjacentSections(t *testing.T) {
	tomorrow := time.Now().AddDate(0, 0, 1)
	now := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 10, 0, 0, 0, tomorrow.Location())
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{
		{ID: "c", Day: day, Title: "Charlie", State: "pending", SectionID: "afternoon"},
		{ID: "d", Day: day, Title: "Delta", State: "pending", SectionID: "afternoon"},
		{ID: "e", Day: day, Title: "Echo", State: "pending"},
		{ID: "a", Day: day, Title: "Alpha", State: "pending", SectionID: "morning"},
		{ID: "b", Day: day, Title: "Bravo", State: "pending", SectionID: "morning"},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{
		{ID: "morning", Name: "Morning", Start: "09:00", End: "12:00"},
		{ID: "lunch", Name: "Lunch", Start: "12:00", End: "13:00"},
		{ID: "afternoon", Name: "Afternoon", Start: "13:00", End: "18:00"},
	}}
	m := terminal{app: app, data: s, now: now, day: day}
	assertOrder := func(want ...string) {
		t.Helper()
		got := []string{}
		for _, row := range m.visibleRows() {
			got = append(got, row.ID)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("task order = %v, want %v", got, want)
		}
	}
	assertTarget := func(id, section string) {
		t.Helper()
		task := m.target()
		if task == nil || task.ID != id || task.SectionID != section {
			t.Fatalf("selection = %+v, want %s in %q", task, id, section)
		}
	}
	m.height = 40
	m = sectionPress(m, 'l')
	assertOrder("a", "b", "c", "d", "e")
	text := ansi.Strip(m.render(100))
	if !strings.Contains(text, "Alpha / -") {
		t.Fatalf("Next did not use grouped task order:\n%s", text)
	}
	last := -1
	for _, part := range []string{"\n  Morning", "> Pending  Alpha", "Pending  Bravo", "\n  Lunch", "\n  Afternoon", "Pending  Charlie", "Pending  Delta", "\n  Unassigned", "Pending  Echo"} {
		index := strings.Index(text, part)
		if index <= last {
			t.Fatalf("missing or out-of-order %q in grouped view:\n%s", part, text)
		}
		last = index
	}
	m = sectionPress(m, 'L', 'G', 'J')
	assertOrder("a", "b")
	assertTarget("b", "morning")
	m = sectionPress(m, 'l', 'K')
	assertOrder("b", "a", "c", "d", "e")
	assertTarget("b", "morning")
	m = sectionPress(m, 'J', 'J')
	assertOrder("a", "b", "c", "d", "e")
	assertTarget("b", "lunch")
	m = sectionPress(m, 'J')
	assertOrder("a", "b", "c", "d", "e")
	assertTarget("b", "afternoon")
	m = sectionPress(m, 'K', 'K')
	assertOrder("a", "b", "c", "d", "e")
	assertTarget("b", "morning")
	m = sectionPress(m, 'G', 'K')
	assertOrder("a", "b", "c", "d", "e")
	assertTarget("e", "afternoon")
	m = sectionPress(m, 'J')
	assertTarget("e", "")
	m = sectionPress(m, 'g', '2', 'd', 'd')
	assertOrder("c", "d", "e")
}

func TestTaskKeysBlockMovingIntoEndedSection(t *testing.T) {
	now := time.Now()
	day := now.AddDate(0, 0, -1).Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{
		{ID: "task", Day: day, State: "pending", SectionID: "pm"},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{
		{ID: "am", Name: "Morning", Start: "00:00", End: "12:00"},
		{ID: "pm", Name: "Afternoon", Start: "12:00", End: "24:00"},
	}}
	m := terminal{app: app, data: s, now: now, day: day, list: true}
	m = sectionPress(m, 'K')
	if !strings.Contains(m.notice, "already ended") || m.target().SectionID != "pm" {
		t.Fatalf("K changed placement or skipped the notice: %q", m.notice)
	}
	m = sectionPress(m, 'b', 'k')
	if !strings.Contains(m.render(80), "Morning  00:00 – 12:00 (ended)") {
		t.Fatal("picker omitted the ended label")
	}
	m = sectionPress(m, tea.KeyEnter)
	if m.mode != "section-picker" || !strings.Contains(m.notice, "already ended") || m.target().SectionID != "pm" {
		t.Fatalf("picker accepted the ended section: %q", m.notice)
	}
}

func TestGroupedSelectionFollowsUnassignedTaskOnStartAndFinish(t *testing.T) {
	now := time.Now()
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{
		{ID: "planned", Day: day, Title: "Planned", State: "pending", SectionID: "today"},
		{ID: "unplanned", Day: day, Title: "Unplanned", State: "pending"},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{{ID: "today", Name: "Today", Start: "00:00", End: "24:00"}}}
	m := terminal{app: app, data: s, now: now, day: day}
	m = sectionPress(m, 'l', 'G', 's')
	if selected := m.target(); selected == nil || selected.ID != "unplanned" || selected.State != "running" {
		t.Fatalf("selection did not follow a task to its execution section: %+v", selected)
	}
	cursorElsewhere := sectionPress(m, 'j')
	if view := cursorElsewhere.render(100); !strings.Contains(view, "\x1b[1;97;48;5;24m  ▶ Unplanned") ||
		!strings.Contains(view, "\x1b[1;36m  Running") || !strings.Contains(view, "\n> Pending") ||
		strings.Contains(view, "\x1b[38;5;252;48;5;237m") {
		t.Fatalf("only the running header should have a background in the full list:\n%s", view)
	}
	m = sectionPress(m, 's')
	if selected := m.target(); selected == nil || selected.ID != "unplanned" || selected.State != "done" || selected.SectionID != "" {
		t.Fatalf("finish did not retain selection and original assignment: %+v", selected)
	}
	m = sectionPress(m, 'L')
	if rows := m.visibleRows(); len(rows) != 2 || rows[0].ID != "unplanned" {
		t.Fatalf("section list did not include execution-date fallback: %+v", rows)
	}
}

func TestGroupedListPagesThroughEmptySections(t *testing.T) {
	m := terminal{data: &state{}, list: true, height: 18,
		app: application{sections: []section{
			{ID: "a", Name: "First", Start: "06:00", End: "07:00"},
			{ID: "b", Name: "Second", Start: "07:00", End: "08:00"},
			{ID: "c", Name: "Third", Start: "08:00", End: "09:00"},
		}},
	}
	m = sectionPress(m, tea.KeyPgDown, tea.KeyPgDown)
	if view := ansi.Strip(m.render(100)); !strings.Contains(view, "\n  Third") ||
		!strings.Contains(view, "\n  Unassigned") || strings.Contains(view, "\n  First") {
		t.Fatalf("could not page through empty sections:\n%s", view)
	}
	m = sectionPress(m, tea.KeyPgUp, tea.KeyPgUp)
	if view := ansi.Strip(m.render(100)); !strings.Contains(view, "\n  First") ||
		!strings.Contains(view, "\n  Second") || strings.Contains(view, "\n  Third") {
		t.Fatalf("could not page back to first sections:\n%s", view)
	}
}

func TestTaskListOnlyShowsSelectedWorkCode(t *testing.T) {
	now := time.Now()
	day := now.Format(dateLayout)
	m := terminal{day: day, now: now, list: true, height: 14, data: &state{Tasks: []*task{
		{ID: "active", Day: day, Title: "Active", WorkCode: "PJ-ACTIVE", State: "running", Sessions: []session{{Start: now}}},
		{ID: "next", Day: day, Title: "Next task", WorkCode: "PJ-NEXT", State: "pending"},
		{ID: "plain", Day: day, Title: "Plain task", State: "pending"},
	}}}
	view := m.render(80)
	if strings.Count(view, "🔖") != 2 || !strings.Contains(view, "└ Code PJ-ACTIVE") ||
		strings.Contains(view, "[PJ-ACTIVE]") || strings.Contains(view, "PJ-NEXT") {
		t.Fatalf("unexpected work code display:\n%s", view)
	}
	m = sectionPress(m, 'j')
	view = m.render(80)
	if !strings.Contains(view, "└ Code PJ-NEXT") || strings.Contains(view, "PJ-ACTIVE") {
		t.Fatalf("work code details did not follow selection:\n%s", view)
	}
	m = sectionPress(m, 'j')
	view = m.render(80)
	if strings.Contains(view, "└ Code") || strings.Contains(view, "PJ-") {
		t.Fatalf("plain task has work code details:\n%s", view)
	}
}

func TestRoutineSectionPickerReturnsToRoutineView(t *testing.T) {
	day := "2026-11-20"
	now := time.Date(2026, time.November, 20, 11, 0, 0, 0, time.Local)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{}, Order: []string{"reading"},
		Routines: map[string]routine{"reading": {ID: "reading", Title: "Read", FirstDay: day}},
	}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{{ID: "evening", Name: "Evening", Start: "18:00", End: "22:00"}}}
	m := terminal{app: app, data: s, now: now, day: day}
	m = sectionPress(m, 'v', 'b', 'j', tea.KeyEnter)
	if m.mode != "routines" || m.data.Routines["reading"].SectionID != "evening" ||
		!strings.Contains(m.render(100), "Evening") {
		t.Fatalf("section picker did not save or return to routines:\n%s", m.render(100))
	}
	m = sectionPress(m, 'b', 'k', tea.KeyEsc)
	if m.mode != "routines" || m.data.Routines["reading"].SectionID != "evening" {
		t.Fatal("cancel should return to routines with the assignment intact")
	}
}

func TestCountedTaskFields(t *testing.T) {
	now := time.Now()
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{
		{ID: "one", Day: day, State: "pending", SectionID: "am"},
		{ID: "two", Day: day, State: "pending", SectionID: "pm"},
		{ID: "three", Day: day, State: "pending", SectionID: "am"},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	m := terminal{app: application{store: repo, sections: []section{{ID: "am"}, {ID: "pm"}}}, day: day, now: now, data: s, list: true, sectionID: "am"}
	m = sectionPress(m, '2', 'c', 'h', 'o', 'm', 'e', tea.KeyEnter)
	m = sectionPress(m, '9', 'C', 'P', ' ', '1', tea.KeyEnter)
	m = sectionPress(m, '2', 'C', '-', tea.KeyEsc)
	saved, err := repo.load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range saved.Tasks {
		if task.SectionID == "am" && (task.Category != "home" || task.WorkCode != "P 1") {
			t.Fatalf("selected task not updated: %+v", task)
		}
		if task.SectionID == "pm" && (task.Category != "" || task.WorkCode != "") {
			t.Fatal("changed filtered task")
		}
	}
	m = sectionPress(m, ':', 'c', 'o', 'd', 'e', ' ', '@', ' ', 'P', ' ', ' ', '2', tea.KeyEnter)
	if m.mode != "" || m.target().WorkCode != "P  2" {
		t.Fatalf("explicit target did not preserve spaces: %q", m.notice)
	}
	m = sectionPress(m, ':', 'c', 'o', 'd', 'e', ' ', 'P', '3', tea.KeyEnter)
	if m.mode != "" || m.target().WorkCode != "P3" {
		t.Fatalf("single-word shorthand no longer works: %q", m.notice)
	}
	m = sectionPress(m, 'C', '1', tea.KeyTab, tea.KeyEnter)
	if m.mode != "" || m.target().WorkCode != "P 1" {
		t.Fatalf("could not complete a previously used work code: %q", m.notice)
	}
}

func TestSearchKeepsSectionAndLimitsCountedActions(t *testing.T) {
	now := time.Now()
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	data := &state{Schema: 1, Tasks: []*task{
		{ID: "alpha", Day: day, Title: "Design Alpha", State: "pending", SectionID: "today"},
		{ID: "hidden", Day: day, Title: "Hidden", State: "pending", SectionID: "today"},
		{ID: "beta", Day: day, Title: "Beta", WorkCode: "DESIGN", State: "pending", SectionID: "today"},
		{ID: "outside", Day: day, Title: "Design Outside", State: "pending"},
	}}
	if err := repo.save(data); err != nil {
		t.Fatal(err)
	}
	m := terminal{app: application{store: repo, sections: []section{
		{ID: "today", Start: "00:00", End: "24:00"},
	}}, data: data, now: now, day: day, width: 100}
	m = sectionPress(m, 'L', '/', 'd', 'e', 's', 'i', 'g', 'n')
	if len(m.visibleRows()) != 2 || m.View().Cursor == nil {
		t.Fatal("search did not filter by title and code within the section")
	}
	m = sectionPress(m, tea.KeyEnter, '/', 'x', tea.KeyEsc)
	if m.mode != "" || m.filter != "design" || len(m.visibleRows()) != 2 {
		t.Fatal("cancel did not restore the applied filter")
	}
	m = sectionPress(m, '2', 'd', 'd')
	if !m.data.Tasks[0].Deleted || !m.data.Tasks[2].Deleted || m.data.Tasks[1].Deleted || m.data.Tasks[3].Deleted {
		t.Fatal("counted delete did not use the filtered rows")
	}
	m = sectionPress(m, tea.KeyEsc)
	if !m.list || m.sectionID != "today" || m.filter != "" || len(m.visibleRows()) != 1 {
		t.Fatal("clearing the filter should keep the section list open")
	}
}

func TestCountedSectionMoveToCurrentSection(t *testing.T) {
	now := time.Now()
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{
		{ID: "a", Day: day, State: "pending"},
		{ID: "b", Day: day, State: "pending"},
		{ID: "c", Day: day, State: "pending"},
		{ID: "existing", Day: day, State: "pending", SectionID: "current"},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{{ID: "current", Start: "00:00", End: "24:00"}}}
	m := terminal{app: app, data: s, now: now, day: day, list: true, selected: 1}
	m = sectionPress(m, '4', 'b')
	if m.mode == modeSectionPicker {
		t.Fatal("accepted a count larger than the source group")
	}
	m = sectionPress(m, '3', 'b', '.')
	var ids []string
	for _, task := range m.data.rows(day) {
		if task.SectionID != "current" {
			t.Fatalf("task %s did not move", task.ID)
		}
		ids = append(ids, task.ID)
	}
	if m.mode == modeSectionPicker || !slices.Equal(ids, []string{"existing", "a", "b", "c"}) {
		t.Fatalf("unexpected result: mode %q, order %v", m.mode, ids)
	}
}

func TestPlanFinishAndAdd(t *testing.T) {
	now := time.Now()
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	m := terminal{app: application{store: repo}, data: &state{}, now: now, day: now.Format(dateLayout)}
	if !m.execute("add", "First") || !m.execute("start") {
		t.Fatal(m.notice)
	}
	m = sectionPress(m, ':', 'p', 'l', 'a', 'n', tea.KeyEnter)
	if !m.planning || m.target() == nil || m.target().State != "running" {
		t.Fatal("missing running task in Plan")
	}
	m = sectionPress(m, 's', 'a', tea.KeyEsc)
	if !m.planning || m.target() != nil || !strings.Contains(m.render(100), "No remaining tasks") {
		t.Fatal("expected empty Plan after cancel")
	}
	m = sectionPress(m, 'a', 'N', 'e', 'w', tea.KeyEnter)
	if !m.planning || m.target() == nil || m.target().Title != "New" {
		t.Fatal("new task was not selected in Plan", m.notice)
	}
	viaLS := sectionPress(m, ':', 'l', 's', tea.KeyEnter)
	if viaLS.planning || viaLS.target() == nil || viaLS.target().ID != m.target().ID {
		t.Fatal("ls changed the selected task when revealing completed rows")
	}
	m = sectionPress(m, tea.KeyEsc)
	if m.planning || !m.list || len(m.visibleRows()) != 2 {
		t.Fatal("expected full list")
	}
}

func TestFocusJumpsWithinTaskList(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.Local)
	day := now.Format(dateLayout)
	m := terminal{
		app: application{sections: []section{
			{ID: "am", Start: "00:00", End: "12:00"},
			{ID: "pm", Start: "12:00", End: "24:00"},
		}},
		data: &state{Tasks: []*task{
			{ID: "morning", Day: day, State: "pending", SectionID: "am", Title: "Match morning"},
			{ID: "running", Day: day, State: "running", Sessions: []session{{Start: now.Add(-time.Minute)}}},
			{ID: "first", Day: day, State: "pending", SectionID: "pm", Title: "Match first"},
			{ID: "second", Day: day, State: "pending", SectionID: "pm"},
		}},
		day: day, now: now, list: true,
	}
	for _, tc := range []struct {
		filter string
		want   string
	}{
		{"", "running"},
		{"match", "first"},
		{"morning", "morning"},
	} {
		view := m
		view.filter = tc.filter
		view = sectionPress(view, 'g', 'f')
		if target := view.target(); target == nil || target.ID != tc.want {
			t.Errorf("filter %q: wanted %s, got %+v", tc.filter, tc.want, target)
		}
	}
}
