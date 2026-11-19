package main

import (
	tea "charm.land/bubbletea/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransaction(t *testing.T) {
	repo := repository{filepath.Join(t.TempDir(), "tasks.json")}
	app := application{repo}
	now := time.Now()
	day := now.Format(dateLayout)
	errs := make(chan error, 8)
	for range cap(errs) {
		go func() { _, err := app.execute(day, "add", []string{"test"}, now); errs <- err }()
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
	s, err := app.execute(day, "status", nil, now)
	if err != nil || len(s.Tasks) != cap(errs) {
		t.Fatal("lost update", err)
	}
	if _, err := app.execute(day, "delete", []string{s.Tasks[0].ID, "missing"}, now); err == nil {
		t.Fatal("expected invalid id")
	}
	after, err := os.ReadFile(repo.path)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed command changed data", err)
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
	m.app = application{repo}
	m.mode = "command"
	m.edit(tea.KeyPressMsg{Code: tea.KeyEnter})
	saved, err := repo.load()
	if err != nil || len(saved.Tasks) != 1 {
		t.Fatal("registration failed", err)
	}
	task := saved.Tasks[0]
	if task.Title != "read" || task.Minutes == nil || *task.Minutes != 15 {
		t.Fatal(task)
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
		WorkCode: "PJ-042", Category: "work", RoutineID: "routine-1",
		Sessions: []session{{Start: started, End: &ended}}, Notes: []note{{Text: "finished a chapter"}},
	}
	s := &state{Tasks: []*task{original}}
	if err := s.apply(day, "start", []string{original.ID}, now); err != nil {
		t.Fatal(err)
	}
	if len(s.Tasks) != 2 {
		t.Fatalf("got %d tasks", len(s.Tasks))
	}
	restarted := s.running()
	if restarted == nil || restarted.ID == original.ID || restarted.Title != original.Title ||
		restarted.Day != day || restarted.WorkCode != original.WorkCode || restarted.Category != original.Category ||
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
