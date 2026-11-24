package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func typeText(m terminal, text string) terminal {
	for _, r := range text {
		next, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(terminal)
	}
	return m
}

func reviewTerminal(t *testing.T, tasks []*task) (terminal, repository) {
	t.Helper()
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.Local)
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	for _, item := range tasks {
		item.Day = day
		if item.State == "" {
			item.State = "pending"
		}
		if item.Sessions == nil {
			item.Sessions = []session{}
		}
	}
	s := &state{Schema: 1, Tasks: tasks, Routines: map[string]routine{}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{
		{ID: "morning", Name: "Morning", Start: "06:00", End: "12:00"},
		{ID: "afternoon", Name: "Afternoon", Start: "12:00", End: "18:00"},
	}}
	return terminal{app: app, data: s, now: now, day: day, height: 40}, repo
}

func TestFilteredCountedActionsUseVisibleRows(t *testing.T) {
	m, repo := reviewTerminal(t, []*task{
		{ID: "a1", Title: "alpha one", SectionID: "afternoon"},
		{ID: "m1", Title: "beta", SectionID: "morning"},
		{ID: "m2", Title: "alpha two", SectionID: "morning"},
		{ID: "m3", Title: "alpha three", SectionID: "morning"},
	})
	m = sectionPress(m, 'l', '/')
	m = typeText(m, "alpha")
	m = sectionPress(m, tea.KeyEnter)
	if got := len(m.visibleRows()); got != 3 {
		t.Fatalf("filter rows = %d", got)
	}
	// visible order: m2, m3 (morning), a1 (afternoon)
	m = sectionPress(m, '2', 'C')
	if m.fieldEdit == nil || strings.Join(m.fieldEdit.ids, ",") != "m2,m3" {
		t.Fatalf("2C targets %+v", m.fieldEdit)
	}
	m = typeText(m, "HubSpot Design")
	m = sectionPress(m, tea.KeyEnter)
	saved, _ := repo.load(t.Context())
	for _, item := range saved.Tasks {
		want := ""
		if item.ID == "m2" || item.ID == "m3" {
			want = "HubSpot Design"
		}
		if item.WorkCode != want {
			t.Errorf("%s code = %q want %q", item.ID, item.WorkCode, want)
		}
	}
	m = sectionPress(m, 'g', '2', 'd', 'd')
	saved, _ = repo.load(t.Context())
	deleted := map[string]bool{}
	for _, item := range saved.Tasks {
		deleted[item.ID] = item.Deleted
	}
	if !deleted["m2"] || !deleted["m3"] || deleted["m1"] || deleted["a1"] {
		t.Fatalf("2dd deleted %v", deleted)
	}
}

func TestChangingDayBackToTodayFollowsMidnight(t *testing.T) {
	m, _ := reviewTerminal(t, []*task{})
	m.changeDay(-1)
	m.changeDay(1)
	if m.pinned {
		t.Fatal("returning to today by Alt+l left the day pinned")
	}
}

func TestResetRequiresConfirmationWhenTimeIsRecorded(t *testing.T) {
	start := time.Date(2026, 11, 20, 9, 0, 0, 0, time.Local)
	end := start.Add(time.Hour)
	m, repo := reviewTerminal(t, []*task{{ID: "x", Title: "Logged", State: "done", Sessions: []session{{Start: start, End: &end}}}})
	m = sectionPress(m, 'l', 'u')
	saved, _ := repo.load(t.Context())
	if saved.Tasks[0].State != "done" || len(saved.Tasks[0].Sessions) != 1 {
		t.Fatal("a single u erased the recorded time")
	}
	m = sectionPress(m, 'u')
	saved, _ = repo.load(t.Context())
	if saved.Tasks[0].State != "pending" || len(saved.Tasks[0].Sessions) != 0 {
		t.Fatalf("confirmed reset did not apply: %+v", saved.Tasks[0])
	}
}

func TestCompletingNumericTitleKeepsItAsTitle(t *testing.T) {
	m := terminal{day: "2026-11-20", data: &state{Tasks: []*task{{Title: "2nd draft"}, {Title: "10 push-ups"}}}}
	m.input = "add 1"
	got := m.addSuggestions()
	if len(got) != 1 {
		t.Fatal(got)
	}
	if _, rest, _ := strings.Cut(got[0].input, " "); strings.HasPrefix(rest, "10 ") {
		t.Fatalf("completion %q would read the title's number as an estimate", got[0].input)
	}
}

func TestRenameCompletesTaskTitleWithoutCopyingOtherFields(t *testing.T) {
	for _, command := range []string{"shortcut", "explicit ID"} {
		t.Run(command, func(t *testing.T) {
			estimate := 15.0
			m, repo := reviewTerminal(t, []*task{
				{ID: "target", Title: "Draft", Minutes: &estimate, Category: "work", WorkCode: "Current"},
				{ID: "history", Title: "Review @ Office", Category: "home", WorkCode: "Previous"},
				{ID: "partial", Title: "Team Review"},
				{ID: "deleted", Title: "Review Later", Deleted: true},
			})
			if command == "shortcut" {
				m = sectionPress(m, 'l', 'r')
				m = typeText(m, "re")
			} else {
				m = sectionPress(m, ':')
				m = typeText(m, "rename target re")
			}
			matches := m.inputSuggestions()
			if len(matches) != 2 || matches[0].label != "Review @ Office" || matches[1].label != "Team Review" {
				t.Fatalf("unexpected suggestions: %+v", matches)
			}
			m = sectionPress(m, tea.KeyTab, tea.KeyEnter)

			s, err := repo.load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			target := s.Tasks[0]
			if target.Title != "Review @ Office" || target.Category != "work" || target.WorkCode != "Current" ||
				target.Minutes == nil || *target.Minutes != estimate {
				t.Fatalf("rename did not update only the title: %+v (notice: %s)", target, m.notice)
			}
		})
	}
}

func TestSaveKeepsSymlinkAndMode(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "tasks.json")
	if err := os.WriteFile(real, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	repo := repository{path: link}
	if err := repo.save(&state{Schema: 1, Tasks: []*task{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.update(t.Context(), func(*state) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(real + ".lock"); err != nil {
		t.Fatalf("update through symlink did not use the target's lock: %v", err)
	}
	if info, _ := os.Lstat(link); info.Mode()&os.ModeSymlink == 0 {
		t.Error("save replaced the symlink with a regular file")
	}
	if info, _ := os.Stat(real); info.Mode().Perm() != 0644 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}
}

func TestDoneRowsOrderIsConsistentWithoutEndTimes(t *testing.T) {
	base := time.Date(2026, 11, 20, 9, 0, 0, 0, time.Local)
	ends := []time.Time{base.Add(3 * time.Hour), base.Add(time.Hour), base.Add(2 * time.Hour)}
	s := &state{}
	for i, e := range ends {
		e := e
		s.Tasks = append(s.Tasks, &task{ID: string(rune('a' + i)), Day: "d", State: "done", Sessions: []session{{Start: base, End: &e}}})
		if i == 0 {
			s.Tasks = append(s.Tasks, &task{ID: "legacy", Day: "d", State: "done"})
		}
	}
	rows := s.rows("d")
	var got []string
	for _, r := range rows {
		got = append(got, r.ID)
	}
	if strings.Join(got, ",") != "legacy,b,c,a" {
		t.Fatalf("order = %v", got)
	}
}

func TestClockWakesOncePerSecond(t *testing.T) {
	for _, nano := range []int{0, 1, 500_000_000, 999_999_999} {
		now := time.Date(2026, 11, 20, 10, 0, 7, nano, time.Local)
		wait := untilNextSecond(now)
		if wait <= 0 || wait > time.Second+10*time.Millisecond || now.Add(wait).Second() != 8 && nano != 0 {
			t.Errorf("nanosecond %d waits %v", nano, wait)
		}
	}
}
