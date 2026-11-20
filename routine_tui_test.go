package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRoutineSectionsMoveAndKeepSelection(t *testing.T) {
	now := time.Now()
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{
		Schema: 1,
		Tasks:  []*task{},
		Order:  []string{"z", "c", "a", "b", "d"},
		Routines: map[string]routine{
			"z": {ID: "z", Title: "Unassigned routine"},
			"c": {ID: "c", Title: "Afternoon routine", SectionID: "afternoon"},
			"a": {ID: "a", Title: "First morning routine", SectionID: "morning"},
			"b": {ID: "b", Title: "Second morning routine", SectionID: "morning"},
			"d": {ID: "d", Title: "Stale assignment", SectionID: "removed"},
		},
	}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	app := application{store: repo, sections: []section{
		{ID: "morning", Name: "Morning", Start: "09:00", End: "12:00"},
		{ID: "lunch", Name: "Lunch", Start: "12:00", End: "13:00"},
		{ID: "afternoon", Name: "Afternoon", Start: "13:00", End: "18:00"},
	}}
	m := terminal{app: app, data: s, now: now, day: now.Format(dateLayout)}
	m = sectionPress(m, 'v')
	if list := m.routineList(); !slices.Equal(list.ids, []string{"a", "b", "c", "z", "d"}) ||
		len(list.groups) != 4 || list.groups[1].start != list.groups[1].end {
		t.Fatalf("incorrect routine groups: %+v", list)
	}
	if view := m.render(100); !strings.Contains(view, "Morning  09:00 → 12:00") ||
		!strings.Contains(view, "Lunch  12:00 → 13:00") || !strings.Contains(view, "> First morning routine") {
		t.Fatalf("routine view omitted section headings or the selected row:\n%s", view)
	}
	assertSelection := func(id, section string, order ...string) {
		t.Helper()
		list := m.routineList()
		if m.routineTarget(list) != id || m.data.Routines[id].SectionID != section ||
			!slices.Equal(list.ids, order) {
			t.Fatalf("selected %q in %q with order %v, want %q in %q with %v",
				m.routineTarget(list), m.data.Routines[id].SectionID, list.ids, id, section, order)
		}
	}
	m = sectionPress(m, 'J')
	assertSelection("a", "morning", "b", "a", "c", "z", "d")
	m = sectionPress(m, 'J')
	assertSelection("a", "lunch", "b", "a", "c", "z", "d")
	m = sectionPress(m, 'J')
	assertSelection("a", "afternoon", "b", "a", "c", "z", "d")
	m = sectionPress(m, 'K', 'K')
	assertSelection("a", "morning", "b", "a", "c", "z", "d")
	m = sectionPress(m, 'b', 'g', tea.KeyEnter)
	list := m.routineList()
	if m.mode != "routines" || m.routineTarget(list) != "a" || m.data.Routines["a"].SectionID != "" ||
		list.groups[3].start > m.routineIndex {
		t.Fatalf("picker did not move selection into Unassigned: %+v", list)
	}
}

func TestRoutineViewportPagesThroughEmptyGroups(t *testing.T) {
	m := terminal{
		data: &state{}, height: 14,
		app: application{sections: []section{
			{ID: "first", Name: "First", Start: "06:00", End: "07:00"},
			{ID: "second", Name: "Second", Start: "07:00", End: "08:00"},
		}},
	}
	m = sectionPress(m, 'v')
	for range m.routineList().lineCount() {
		m = sectionPress(m, tea.KeyPgDown)
	}
	if view := m.render(80); !strings.Contains(view, "Unassigned") || strings.Contains(view, "First  06:00 → 07:00") {
		t.Fatalf("could not page to empty Unassigned heading:\n%s", view)
	}
}

func TestRoutineWorkCodeEditingReturnsToRoutineList(t *testing.T) {
	now := time.Now()
	day := now.Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{}, Order: []string{"r1"}, Routines: map[string]routine{
		"r1": {ID: "r1", Title: "Read", WorkCode: "PJ-04", FirstDay: day},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	m := terminal{app: application{store: repo}, data: s, day: day, now: now}
	m = sectionPress(m, 'v', 'c')
	if m.input != "routine-code r1 PJ-04" {
		t.Fatalf("did not prefill the current code: %q", m.input)
	}
	m = sectionPress(m, '2', tea.KeyEnter)
	if m.mode != "routines" || m.data.Routines["r1"].WorkCode != "PJ-042" {
		t.Fatalf("edit did not save and return to routines: %q", m.mode)
	}
	m = sectionPress(m, 'c', tea.KeyEsc)
	if m.mode != "routines" || m.data.Routines["r1"].WorkCode != "PJ-042" {
		t.Fatal("cancel did not return to routines")
	}
}
