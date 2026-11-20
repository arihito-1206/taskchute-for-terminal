package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func benchTerminal(n int) terminal {
	now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.Local)
	s := &state{Schema: 1, Routines: map[string]routine{}}
	for i := 0; i < n; i++ {
		day := now.AddDate(0, 0, -(i / 10)).Format(dateLayout)
		s.Tasks = append(s.Tasks, &task{ID: fmt.Sprintf("id%07d", i), Day: day, Title: fmt.Sprintf("Task number %d", i), State: "pending", Sessions: []session{}})
	}
	return terminal{data: s, now: now, day: now.Format(dateLayout), height: 40, list: true, mode: "command", input: "add zzz", inputCursor: 7}
}

func BenchmarkRenderCommand(b *testing.B) {
	m := benchTerminal(30000)
	for b.Loop() {
		m.render(100)
	}
}

func benchRepo(b *testing.B, n int) (repository, *state) {
	b.Helper()
	m := benchTerminal(n)
	for i, t := range m.data.Tasks {
		start := m.now.Add(-time.Duration(i) * time.Minute)
		end := start.Add(10 * time.Minute)
		t.State, t.Sessions, t.Category, t.WorkCode = "done", []session{{Start: start, End: &end}}, "work", "HubSpot Design"
	}
	repo := repository{path: filepath.Join(b.TempDir(), "tasks.json")}
	if err := repo.save(m.data); err != nil {
		b.Fatal(err)
	}
	return repo, m.data
}

// One user action = load + change + save under the lock.
func BenchmarkUpdate(b *testing.B) {
	for _, n := range []int{3000, 30000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			repo, _ := benchRepo(b, n)
			app := application{store: repo}
			now := time.Date(2026, 11, 20, 10, 0, 0, 0, time.Local)
			for b.Loop() {
				if _, err := app.execute(b.Context(), now.Format(dateLayout), "add", []string{"x"}, now); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLoadOnly(b *testing.B) {
	repo, _ := benchRepo(b, 30000)
	info, _ := os.Stat(repo.path)
	b.Logf("file size %d bytes", info.Size())
	for b.Loop() {
		if _, err := repo.load(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
}
