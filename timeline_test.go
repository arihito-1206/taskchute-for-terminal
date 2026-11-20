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

func TestTimelineSharesBordersAndScalesGaps(t *testing.T) {
	start := time.Date(2026, 11, 20, 9, 40, 0, 0, time.UTC)
	firstEnd := start.Add(5 * time.Minute)
	secondStart := firstEnd.Add(35 * time.Second)
	secondEnd := firstEnd.Add(10 * time.Minute)
	resume := secondEnd.Add(10 * time.Minute)
	m := terminal{day: start.Format(dateLayout), now: resume, data: &state{Tasks: []*task{
		{Sessions: []session{{Start: start, End: &firstEnd}, {Start: secondStart, End: &secondEnd}, {Start: resume}}},
	}}}
	lines := m.timelineLines(80)
	var borders []string
	var gap []string
	afterBottom := false
	for _, line := range lines {
		row := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(row, "┌"), strings.HasPrefix(row, "├"), strings.HasPrefix(row, "└"):
			borders = append(borders, string([]rune(row)[0]))
			afterBottom = strings.HasPrefix(row, "└")
		case afterBottom:
			gap = append(gap, row)
		}
	}
	if strings.Join(borders, "") != "┌├└┌└" || len(gap) != 1 || gap[0] != "10:00" {
		t.Fatalf("unexpected timeline:\n%s", strings.Join(lines, "\n"))
	}
}
