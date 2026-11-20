package main

import (
	"fmt"
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
