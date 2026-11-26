package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestStartShortcutsSwitchAndFollowSelectedTask(t *testing.T) {
	for _, test := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"s", tea.KeyPressMsg{Code: 's'}},
		{"Enter", tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"Alt+s", tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt}},
		{"Option+s character", tea.KeyPressMsg{Code: 'ß', Text: "ß"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now()
			day := now.Format(dateLayout)
			repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
			s := &state{Schema: 1, Tasks: []*task{
				{ID: "old", Day: day, Title: "Interrupted", State: "running",
					Sessions: []session{{Start: now.Add(-time.Minute)}}},
				{ID: "other", Day: day, Title: "Later", State: "pending"},
				{ID: "selected", Day: day, Title: "Next task", State: "pending"},
			}}
			if err := repo.save(s); err != nil {
				t.Fatal(err)
			}
			m := terminal{app: application{store: repo}, data: s, now: now, day: day, list: true, selected: 2}
			next, _ := m.Update(test.key)
			m = next.(terminal)
			running := m.data.running()
			if running == nil || running.ID != "selected" || m.target() != running || m.selected == 2 {
				t.Fatalf("selection did not follow the new running task: %s", m.notice)
			}
			old, err := findTask(m.visibleRows(), "old")
			if err != nil || old.State != "done" || old.Sessions[0].End == nil ||
				!old.Sessions[0].End.Equal(running.Sessions[0].Start) || running.Sessions[0].Start.Before(now) {
				t.Fatalf("switch did not record matching times: old=%+v, next=%+v", old, running)
			}
			queued := firstPending(m.visibleRows())
			if queued == nil || queued.Title != old.Title || queued.ID == old.ID || len(queued.Sessions) != 0 {
				t.Fatalf("unfinished work not queued: %+v", queued)
			}
			count := len(m.data.Tasks)
			next, _ = m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModAlt})
			m = next.(terminal)
			if !strings.Contains(m.notice, "already running") || len(m.data.Tasks) != count || m.target() != running {
				t.Fatalf("Option+s should reject a selected running task: %s", m.notice)
			}
			m = sectionPress(m, 's')
			if m.data.running() != nil || m.target() == nil || m.target().ID != "selected" || len(m.data.Tasks) != count {
				t.Fatal("s on the same selected task should finish it")
			}
		})
	}
}

func TestNamedStartFollowsTaskWithoutUnpinningDay(t *testing.T) {
	now := time.Now()
	day := now.AddDate(0, 0, -1).Format(dateLayout)
	repo := repository{path: filepath.Join(t.TempDir(), "tasks.json")}
	s := &state{Schema: 1, Tasks: []*task{
		{ID: "old", Day: now.Format(dateLayout), State: "running",
			Sessions: []session{{Start: now.Add(-time.Minute)}}},
		{ID: "historical", Day: day, Title: "Backlog task", State: "pending"},
	}}
	if err := repo.save(s); err != nil {
		t.Fatal(err)
	}
	m := terminal{app: application{store: repo}, data: s, now: now, day: day, pinned: true, list: true, filter: "unmatched"}
	m = sectionPress(m, ':')
	m = typeText(m, "start historical")
	m = sectionPress(m, tea.KeyEnter)
	if selected := m.target(); m.day != day || !m.pinned || selected == nil ||
		selected.ID != "historical" || selected.State != "running" {
		t.Fatalf("named start should select the new running task on the pinned day: %s", m.notice)
	}
}
