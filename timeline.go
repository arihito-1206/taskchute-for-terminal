package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

type timelineEntry struct {
	session
	task  *task
	notes []note
}

func (m terminal) timelineEntries() []timelineEntry {
	var entries []timelineEntry
	location := m.now.Location()
	for _, task := range m.data.Tasks {
		if task.Deleted || len(task.Sessions) == 0 {
			continue
		}
		segments := make([]timelineEntry, len(task.Sessions))
		for i, interval := range task.Sessions {
			segments[i] = timelineEntry{session: interval, task: task}
		}
		slices.SortStableFunc(segments, func(a, b timelineEntry) int { return a.Start.Compare(b.Start) })
		for _, note := range task.Notes {
			index := 0
			for i := 1; i < len(segments); i++ {
				if note.At.Before(segments[i].Start) {
					break
				}
				index = i
			}
			segments[index].notes = append(segments[index].notes, note)
		}
		for _, entry := range segments {
			if entry.Start.In(location).Format(dateLayout) == m.day {
				entries = append(entries, entry)
			}
		}
	}
	slices.SortStableFunc(entries, func(a, b timelineEntry) int { return a.Start.Compare(b.Start) })
	return entries
}

func (entry timelineEntry) elapsed(now time.Time) time.Duration {
	if entry.End != nil {
		now = *entry.End
	}
	return max(0, now.Sub(entry.Start))
}

func (m terminal) timelineLines(width int) []string {
	width = max(1, width)
	entries := m.timelineEntries()
	if len(entries) == 0 {
		return []string{ansi.Truncate("No recorded sessions.", width, "")}
	}
	const unit = 5 * time.Minute
	const gutter = 8
	if width < 24 {
		return []string{ansi.Truncate("Widen terminal", width, "")}
	}
	width -= gutter
	lines := []string{ansi.Truncate("5 min per row · extra rows for comments", width+gutter, ""), ""}
	inner := width - 4
	border := strings.Repeat("─", width-2)
	boxRow := func(text string) string {
		text = ansi.Truncate(text, inner, "…")
		return "│ " + text + strings.Repeat(" ", inner-ansi.StringWidth(text)) + " │"
	}
	location := m.now.Location()
	var lastHour time.Time
	for _, entry := range entries {
		end := "now"
		if entry.End != nil {
			end = timeLabel(*entry.End, m.day, location)
		}
		spent := entry.elapsed(m.now)
		header := fmt.Sprintf("%s–%s  %s", timeLabel(entry.Start, m.day, location), end, duration(spent))
		heading := " " + ansi.Truncate(header, inner, "…") + " "
		start := entry.Start.In(location)
		hour := start.Add(-time.Duration(start.Minute())*time.Minute - time.Duration(start.Second())*time.Second - time.Duration(start.Nanosecond()))
		if lastHour.IsZero() {
			lastHour = hour.Add(-time.Hour)
		}
		for tick := lastHour.Add(time.Hour); !tick.After(hour); tick = tick.Add(time.Hour) {
			lines = append(lines, tick.In(location).Format("15:04")+" "+strings.Repeat("─", width+gutter-6))
			lastHour = tick
		}
		lines = append(lines, strings.Repeat(" ", gutter)+"┌"+heading+strings.Repeat("─", width-2-ansi.StringWidth(heading))+"┐")

		content := []string{title(entry.task)}
		for _, note := range entry.notes {
			stamp := "[--:--]"
			if !note.At.IsZero() {
				stamp = "[" + timeLabel(note.At, m.day, location) + "]"
			}
			text := stamp + " " + terminalText(note.Text, true)
			content = append(content, strings.Split(ansi.Hardwrap(text, inner, true), "\n")...)
		}
		// A time row represents up to five minutes. Additional comment rows
		// stay in the same box and do not advance the hourly ruler.
		shown := min(spent, 24*time.Hour)
		rows := int((shown + unit - 1) / unit)
		for i := range max(rows, len(content)) {
			prefix := strings.Repeat(" ", gutter)
			rowEnd := start.Add(min(shown, time.Duration(i+1)*unit))
			tick := lastHour.Add(time.Hour)
			if i < rows && !tick.After(rowEnd) {
				prefix = tick.In(location).Format("15:04") + " ──"
				lastHour = tick
			}
			var row string
			if i < len(content) {
				row = content[i]
			}
			lines = append(lines, prefix+boxRow(row))
		}
		lines = append(lines, strings.Repeat(" ", gutter)+"└"+border+"┘", "")
	}
	return lines[:len(lines)-1]
}

func (m terminal) timelineView(width int) string {
	lines := m.timelineLines(width)
	rows := max(1, m.helpRows()-1)
	first := min(m.timelineOffset, max(0, len(lines)-rows))
	header := ansi.Truncate("TIMELINE  "+m.day, width, "")
	footer := ansi.Truncate("j/k scroll  Alt+h/l day  Esc close", width, "")
	body := strings.Join(lines[first:min(len(lines), first+rows)], "\n")
	return header + "\n\n" + body + "\n" + footer
}

func (m *terminal) timelineKey(key string) {
	switch key {
	case "esc", "t", "q":
		m.mode = ""
	case "j", "down":
		width := m.width
		if width == 0 {
			width = 80
		}
		m.timelineOffset = min(m.timelineOffset+1, max(0, len(m.timelineLines(width))-max(1, m.helpRows()-1)))
	case "k", "up":
		m.timelineOffset = max(0, m.timelineOffset-1)
	case "alt+h", "˙", "alt+l", "¬":
		step := 1
		if key == "alt+h" || key == "˙" {
			step = -1
		}
		m.changeDay(step)
		m.timelineOffset = 0
	}
}
