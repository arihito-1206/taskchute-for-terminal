package main

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
)

func duration(d time.Duration) string {
	seconds := int64(d / time.Second)
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func roundedMinutes(minutes float64) int {
	return int(math.Round(minutes))
}

func estimateColumn(minutes *float64) string {
	if minutes == nil {
		return "-"
	}
	total := roundedMinutes(*minutes)
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func estimateLabel(m *float64) string {
	if m == nil {
		return "-"
	}
	return fmt.Sprintf("%dm", roundedMinutes(*m))
}

// Treat stored text as terminal content, never as terminal instructions.
func terminalText(value string, multiline bool) string {
	return strings.Map(func(r rune) rune {
		if multiline && r == '\n' {
			return r
		}
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func title(t *task) string {
	return categoryTitle(t.Title, t.Category)
}

var categoryIcons = map[string]string{
	"work": "💼", "仕事": "💼",
	"home": "🏠", "家事": "🏠",
	"hobby": "🎨", "趣味": "🎨",
	"study": "📚", "学習": "📚",
	"rest": "☕", "休憩": "☕",
	"meal": "🍖", "食事": "🍖",
	"family": "👪", "家族": "👪", "育児": "👪",
}

func categoryTitle(name, category string) string {
	if name == "" {
		name = "(untitled)"
	}
	icon := categoryIcons[strings.ToLower(category)]
	if category != "" && icon == "" {
		icon = "🏷"
	}
	if icon != "" {
		return icon + " " + terminalText(name, false)
	}
	return terminalText(name, false)
}

func dateWithWeekday(day string) string {
	date, err := time.Parse(dateLayout, day)
	if err != nil {
		return day
	}
	return date.Format(dateLayout + " (Mon)")
}

func timeLabel(t time.Time, day string, location *time.Location) string {
	t = t.In(location)
	if t.Format(dateLayout) != day {
		return t.Format("01/02 15:04")
	}
	return t.Format("15:04")
}

func row(t *task, day string, now time.Time, width int, name, codeLabel string) string {
	label := map[string]string{"pending": "Pending", "running": "Running", "done": "Done"}[t.State]
	start, end := "-", "-"
	if len(t.Sessions) > 0 {
		start = timeLabel(t.Sessions[0].Start, day, now.Location())
		last := t.Sessions[len(t.Sessions)-1]
		if t.State == "done" && last.End != nil {
			end = timeLabel(*last.End, day, now.Location())
		}
		if t.legacyPaused() {
			label = "Paused"
		}
	}
	if codeLabel == "" {
		name = runewidth.Truncate(name, width, "...")
	} else {
		codeWidth := runewidth.StringWidth(codeLabel)
		if codeWidth >= width {
			name = runewidth.Truncate(codeLabel, width, "...")
		} else {
			available := width - codeWidth - 1
			name = strings.TrimSpace(runewidth.Truncate(name, available, "...") + " " + codeLabel)
		}
	}
	name = runewidth.FillRight(name, width)
	return fmt.Sprintf("%-7s  %s  %6s  %-11s  %-11s  %s", label, name, estimateColumn(t.Minutes), start, end, duration(elapsed(t, now)))
}

type taskOutput struct {
	*task
	Elapsed   float64  `json:"elapsed_seconds"`
	Remaining *float64 `json:"remaining_seconds"`
}

type output struct {
	Day       string       `json:"day"`
	Tasks     []taskOutput `json:"tasks"`
	Running   *taskOutput  `json:"running"`
	Next      *taskOutput  `json:"next"`
	Remaining float64      `json:"remaining_seconds"`
	Unknown   int          `json:"unestimated_count"`
	Finish    time.Time    `json:"finish_at"`
	Available bool         `json:"forecast_available"`
	Routines  []routine    `json:"routines"`
}

func formatTask(t *task, now time.Time) taskOutput {
	item := taskOutput{task: t, Elapsed: elapsed(t, now).Seconds()}
	if t.State == "done" || t.Minutes != nil {
		left := 0.0
		if t.State != "done" {
			left = max(0, *t.Minutes*60-item.Elapsed)
		}
		item.Remaining = &left
	}
	return item
}

func (s *state) output(day string, now time.Time) output {
	return s.outputForRows(day, now, s.rows(day), s.running())
}

func (s *state) outputForRows(day string, now time.Time, rows []*task, running *task) output {
	o := output{Day: day, Tasks: []taskOutput{}, Routines: []routine{}, Available: day == now.Format(dateLayout)}
	for _, id := range s.Order {
		o.Routines = append(o.Routines, s.Routines[id])
	}
	for _, t := range rows {
		item := formatTask(t, now)
		o.Tasks = append(o.Tasks, item)
		if t.State == "pending" && o.Next == nil {
			o.Next = &item
		}
		if item.Remaining == nil {
			o.Unknown++
		} else {
			o.Remaining += *item.Remaining
		}
	}
	if t := running; t != nil {
		item := formatTask(t, now)
		o.Running = &item
		o.Available = o.Available && t.Day == day
	}
	o.Finish = now.Add(time.Duration(o.Remaining * float64(time.Second)))
	return o
}

func summary(s *state, day string, now time.Time) string {
	return renderSummary(s.output(day, now), now)
}

func renderSummary(o output, now time.Time) string {
	focus := "Ready"
	if o.Running != nil {
		focus = title(o.Running.task) + "    " + duration(elapsed(o.Running.task, now))
	}
	next, finish := "-", "-"
	if o.Next != nil {
		next = title(o.Next.task) + " / " + estimateLabel(o.Next.Minutes)
	}
	if o.Available {
		finish = timeLabel(o.Finish, o.Day, now.Location())
		if o.Unknown > 0 {
			finish += fmt.Sprintf(" (+%d unestimated)", o.Unknown)
		}
	}
	return fmt.Sprintf("%s\nNext %s | Day ends %s", focus, next, finish)
}
