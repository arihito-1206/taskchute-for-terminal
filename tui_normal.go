package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (m *terminal) moveTask(list taskList, target *task, key string) {
	if target == nil || target.State != "pending" {
		m.notice = "Select a pending task."
		return
	}
	index := taskIndex(list.rows, target.ID)
	if index < 0 {
		return
	}
	step, position, entry := 1, "after", "before"
	if key == "K" {
		step, position, entry = -1, "before", "after"
	}
	groupIndex := list.groupOf(index)
	start, end := 0, len(list.rows)
	if groupIndex >= 0 {
		group := list.groups[groupIndex]
		start, end = group.start, group.end
	}
	for i := index + step; i >= start && i < end; i += step {
		if list.rows[i].State == "pending" {
			m.execute("move", target.ID, position, list.rows[i].ID)
			return
		}
	}
	if !m.list || groupIndex < 0 {
		return
	}
	now := time.Now()
	next := groupIndex + step
	for next >= 0 && next < len(list.groups) {
		candidate := list.groups[next].section
		if candidate.ID == "" || !sectionEnded(candidate, target.Day, now) {
			break
		}
		next += step
	}
	if next < 0 || next >= len(list.groups) {
		m.notice = "No section available in that direction (earlier sections have already ended)."
		return
	}
	destination := list.groups[next]
	sectionID := destination.section.ID
	if sectionID == "" {
		sectionID = "-"
	}
	args := []string{target.ID, sectionID}
	members := list.rows[destination.start:destination.end]
	if step < 0 {
		for i := len(members) - 1; i >= 0; i-- {
			if members[i].State == "pending" {
				args = append(args, entry, members[i].ID)
				break
			}
		}
	} else if first := firstPending(members); first != nil {
		args = append(args, entry, first.ID)
	}
	m.execute("move-section", args...)
}

func (m *terminal) normalKey(key string) bool {
	if m.mode == modeTimeline {
		m.timelineKey(key)
		return false
	}
	if m.mode == modeHelp {
		switch key {
		case "down", "j":
			m.helpOffset = min(m.helpOffset+1, max(0, len(helpLines)-m.helpRows()))
		case "up", "k":
			m.helpOffset = max(0, m.helpOffset-1)
		default:
			m.mode = ""
		}
		return false
	}
	confirmReset := m.resetID
	m.resetID = ""
	if confirmReset != "" && key != "u" {
		m.notice = ""
	}
	followSelection := true
	defer func() {
		if followSelection {
			m.scroll()
		}
	}()
	list := m.taskList()
	rows := list.rows
	target := m.targetIn(list)
	if m.list {
		if m.deleting {
			count := max(1, m.count)
			m.count, m.deleting, m.notice = 0, false, ""
			if key == "d" && target != nil {
				end := m.selected + min(count, len(rows)-m.selected)
				ids := []string{}
				for _, t := range rows[m.selected:end] {
					ids = append(ids, t.ID)
				}
				if m.execute("cut", ids...) {
					m.notice = fmt.Sprintf("Cut %d task(s). p paste pending · U restore.", len(ids))
				}
				return false
			}
			if key == "esc" {
				return false
			}
		} else if digit, err := strconv.Atoi(key); err == nil && len(key) == 1 {
			m.count = min(len(rows)+1, m.count*10+digit)
			m.notice = fmt.Sprintf("%d | dd delete · c category · C code · b section", m.count)
			return false
		} else if key == "d" && target != nil {
			m.deleting = true
			m.notice = "Press d again to delete | Esc cancels"
			return false
		} else if key == "C" || (key == "c" && m.count > 0) {
			count := max(1, m.count)
			m.count = 0
			if target == nil {
				m.notice = "No task selected."
				return false
			}
			field := "category"
			if key == "C" {
				field = "code"
			}
			edit := &taskFieldEdit{field: field, day: m.day}
			for _, t := range rows[m.selected:min(m.selected+count, len(rows))] {
				edit.ids = append(edit.ids, t.ID)
				edit.titles = append(edit.titles, t.Title)
			}
			m.openCommand("")
			m.fieldEdit = edit
			return false
		} else if m.count != 0 && key != "b" {
			m.count, m.notice = 0, ""
		}
	}
	switch key {
	case "q":
		return true
	case "/":
		m.mode, m.list, m.input, m.notice = modeSearch, true, m.filter, ""
		m.inputCursor = len(m.input)
	case "esc":
		m.notice = ""
		if m.filter != "" {
			m.filter = ""
			m.showList(m.sectionID, target)
			break
		}
		if m.planning {
			if m.sectionID == "" {
				m.planning = false
			}
			m.showList("", target)
		} else if m.sectionID != "" {
			m.list, m.sectionID = false, ""
		}
	case "l":
		if !m.list || m.sectionID != "" {
			m.showList("", target)
		} else {
			m.list, m.planning, m.filter = false, false, ""
		}
	case "L":
		if m.sectionID != "" {
			m.list, m.planning, m.sectionID, m.notice, m.filter = false, false, "", "", ""
			break
		}
		s, ok := sectionAt(m.app.sections, m.now)
		if !ok {
			m.notice = "No section at the current time."
			if len(m.app.sections) == 0 {
				m.notice = "Set sections in config.json to use this view."
			}
			break
		}
		m.showList(s.ID, target)
		m.notice = ""
	case "alt+h", "˙":
		m.changeDay(-1)
	case "alt+l", "¬":
		m.changeDay(1)
	case "alt+t", "†":
		m.showDay(m.now.Format(dateLayout), false)
	case "j", "down":
		m.list = true
		m.selected++
	case "k", "up":
		m.list = true
		m.selected--
	case "ctrl+d", "pgdown":
		m.list = true
		m.pageList(list, 1)
		followSelection = false
	case "ctrl+u", "pgup":
		m.list = true
		m.pageList(list, -1)
		followSelection = false
	case "g":
		m.list = true
		m.selected = 0
	case "G":
		m.list = true
		m.selected = len(rows) - 1
	case "f":
		if !m.list || m.day != m.now.Format(dateLayout) {
			break
		}
		index := slices.IndexFunc(rows, func(t *task) bool { return t.State == "running" })
		if index < 0 {
			if section, ok := sectionAt(m.app.sections, m.now); ok {
				index = slices.IndexFunc(rows, func(t *task) bool {
					return t.State == "pending" && !t.legacyPaused() && t.SectionID == section.ID
				})
			}
		}
		if index >= 0 {
			m.selected = index
		}
	case "J", "K":
		if m.filter != "" {
			list = m.taskListFor(m.data.rows(m.day), "", m.planning)
		}
		m.moveTask(list, target, key)
	case "p":
		if !m.list {
			m.notice = "Open the task list to paste."
			break
		}
		args := []string{}
		if target != nil {
			args = []string{"after", target.ID}
		} else if m.sectionID != "" {
			args = []string{"section", m.sectionID}
		}
		m.execute("paste", args...)
	case "I":
		m.execute("interrupt")
	case "s", "enter", "alt+s", "ß":
		if target == nil {
			m.notice = "No task selected."
			break
		}
		if target.State == "running" && (key == "s" || key == "enter") {
			m.execute("done")
			break
		}
		cmd := "start"
		if key == "s" || key == "enter" {
			cmd = "start-contiguous"
		}
		if m.execute(cmd, target.ID) && (target.State == "done" || target.legacyPaused()) {
			for i, row := range m.visibleRows() {
				if row.State == "running" {
					m.selected = i
					break
				}
			}
		}
	case "U":
		if m.list {
			m.execute("undo-delete")
		}
	case "u":
		if m.list && target != nil {
			// Clearing a recorded log cannot be undone, so ask once.
			if len(target.Sessions) > 0 && confirmReset != target.ID {
				m.resetID = target.ID
				m.notice = "Press u again to clear the recorded time | any other key cancels"
				break
			}
			m.execute("reset", target.ID)
		}
	case "b":
		count := max(1, m.count)
		m.count = 0
		if target == nil {
			m.notice = "No task selected."
			break
		}
		selected := []*task{target}
		if m.list {
			if count > len(rows)-m.selected || list.groupOf(m.selected) != list.groupOf(m.selected+count-1) {
				m.notice = "Not enough tasks in this section."
				break
			}
			selected = rows[m.selected : m.selected+count]
		}
		ids := make([]string, 0, count)
		for _, task := range selected {
			if task.State != "pending" {
				break
			}
			ids = append(ids, task.ID)
		}
		if len(ids) != count {
			m.notice = "Select consecutive pending tasks."
			break
		}
		m.openSectionPicker("batch-section", target.ID, target.Day, target.SectionID)
		m.picker.taskIDs = ids
	case "v":
		m.mode = modeRoutines
		m.routineIndex, m.routineOffset = 0, 0
		m.scrollRoutines()
		m.notice = ""
	case "t":
		m.mode, m.timelineOffset = modeTimeline, 0
	case "?":
		m.mode, m.helpOffset = modeHelp, 0
	case "n":
		if target != nil && (m.list || target.State == "running") {
			m.mode, m.noteID, m.noteDay, m.input, m.notice = modeNote, target.ID, target.Day, "", ""
			m.openNote()
		} else {
			m.notice = "Start a task or select one in the list."
		}
	case "F":
		if target == nil || len(target.Sessions) == 0 {
			m.notice = "Select a task with recorded execution intervals."
			break
		}
		m.endEdit = endTimeEdit{task: target, index: len(target.Sessions) - 1}
		m.notice = ""
		if len(target.Sessions) > 1 {
			m.mode = modeIntervalPicker
		} else {
			m.openCommand("")
		}
	case ":", "i", "a", "r", "e", "S", "R", "c":
		prefix := map[string]string{"a": "add ", "r": "rename @ ", "e": "estimate @ ", "S": "set-start @ ", "R": "routine @ ", "c": "category @ "}[key]
		if strings.Contains(prefix, "@") && target == nil {
			m.notice = "No task selected."
			break
		}
		if key == "R" {
			if _, exists := m.data.Routines[target.RoutineID]; !exists && !target.hasCompletedLog() {
				m.notice = "Select a completed task with recorded time to create a routine."
				break
			}
		}
		m.openCommand(prefix)
	}
	return false
}
