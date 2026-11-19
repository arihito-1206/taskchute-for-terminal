package main

import (
	"strings"
	"testing"
	"time"
)

func TestTimelineKeepsNotesWithTheirExecutionInterval(t *testing.T) {
	start := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	pause := start.Add(15 * time.Minute)
	resume := start.Add(time.Hour)
	m := terminal{day: start.Format(dateLayout), now: resume.Add(time.Minute), data: &state{Tasks: []*task{
		{Title: "Read", Sessions: []session{{Start: start, End: &pause}, {Start: resume}}, Notes: []note{
			{At: start.Add(time.Minute), Text: "first note"},
			{At: resume, Text: "second note"},
		}},
		{Title: "Break", Sessions: []session{{Start: pause, End: &resume}}},
		{Title: "Pending"},
	}}}
	lines := strings.Join(m.timelineLines(80), "\n")
	first := strings.Index(lines, "[09:01] first note")
	gap := strings.Index(lines, "Break")
	last := strings.Index(lines, "[10:00] second note")
	if first < 0 || gap <= first || last <= gap || strings.Contains(lines, "Pending") {
		t.Fatalf("unexpected timeline:\n%s", lines)
	}
}
