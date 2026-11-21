package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

type sectionPicker struct {
	command, id, day string
	index            int
	taskIDs          []string
}

type endTimeEdit struct {
	task  *task
	index int
}

func (m *terminal) intervalPickerKey(key string) {
	switch key {
	case "j", "down":
		m.endEdit.index = min(m.endEdit.index+1, len(m.endEdit.task.Sessions)-1)
	case "k", "up":
		m.endEdit.index = max(0, m.endEdit.index-1)
	case "enter":
		m.mode = ""
		m.openCommand("")
	case "esc":
		m.mode, m.endEdit = "", endTimeEdit{}
	}
}

func (m terminal) intervalLabel(t *task, index int) string {
	interval := t.Sessions[index]
	location := m.now.Location()
	end := "Running"
	if interval.End != nil {
		end = timeLabel(*interval.End, t.Day, location)
	}
	return timeLabel(interval.Start, t.Day, location) + " – " + end
}

func (m terminal) intervalPickerView(width int) string {
	edit := m.endEdit
	lines := []string{"EDIT END TIME | " + edit.task.Title}
	visible := max(1, m.helpRows()-2)
	first := max(0, edit.index-visible+1)
	for i := first; i < min(len(edit.task.Sessions), first+visible); i++ {
		marker := "  "
		if i == edit.index {
			marker = "> "
		}
		lines = append(lines, marker+m.intervalLabel(edit.task, i))
	}
	lines = append(lines, "j/k select | Enter edit | Esc cancel")
	for i, line := range lines {
		lines[i] = runewidth.Truncate(terminalText(line, false), width, "…")
	}
	return strings.Join(lines, "\n")
}

func (m *terminal) openSectionPicker(command, id, day, current string) {
	m.mode, m.notice = modeSectionPicker, ""
	m.picker = sectionPicker{command: command, id: id, day: day}
	for i, s := range m.app.sections {
		if s.ID == current {
			m.picker.index = i + 1
			break
		}
	}
}

func (m *terminal) sectionPickerKey(key string) {
	switch key {
	case "j", "down":
		m.picker.index = min(len(m.app.sections), m.picker.index+1)
		return
	case "k", "up":
		m.picker.index = max(0, m.picker.index-1)
		return
	case "g":
		m.picker.index = 0
		return
	case "G":
		m.picker.index = len(m.app.sections)
		return
	case "enter", ".":
		id := "-"
		if m.picker.index > 0 {
			id = m.app.sections[m.picker.index-1].ID
		}
		if key == "." {
			if len(m.picker.taskIDs) == 0 {
				return
			}
			current, ok := sectionAt(m.app.sections, time.Now())
			if !ok {
				m.notice = "No current section."
				return
			}
			id = current.ID
		}
		args := []string{m.picker.id, id}
		if len(m.picker.taskIDs) > 0 {
			args = append([]string{id}, m.picker.taskIDs...)
		}
		if !m.executeDay(m.picker.day, m.picker.command, args...) {
			return
		}
	case "esc":
		m.notice = ""
	default:
		return
	}
	m.mode = ""
	if m.picker.command == "routine-section" {
		m.mode = modeRoutines
		m.selectRoutine(m.picker.id)
	}
	m.picker = sectionPicker{}
}

func (m terminal) sectionPickerView(width int) string {
	heading, footer := "SECTION", "j/k select | Enter save | Esc cancel"
	if count := len(m.picker.taskIDs); count > 0 {
		heading = fmt.Sprintf("MOVE %d TASK(S) TO SECTION", count)
		footer = "j/k select | Enter move | . current | Esc cancel"
	}
	lines := []string{heading}
	visible := max(1, m.helpRows()-2)
	first := max(0, m.picker.index-visible+1)
	for i := first; i < min(len(m.app.sections)+1, first+visible); i++ {
		label := "Unassigned"
		if i > 0 {
			s := m.app.sections[i-1]
			label = s.Name + "  " + s.Start + " – " + s.End
			if len(m.picker.taskIDs) > 0 && sectionEnded(s, m.picker.day, m.now) {
				label += " (ended)"
			}
		}
		marker := "  "
		if i == m.picker.index {
			marker = "> "
		}
		lines = append(lines, marker+label)
	}
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	lines = append(lines, footer)
	for i, line := range lines {
		lines[i] = runewidth.Truncate(terminalText(line, false), width, "…")
	}
	return strings.Join(lines, "\n")
}
