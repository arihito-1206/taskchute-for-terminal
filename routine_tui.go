package main

import (
	"fmt"
	"slices"

	"github.com/mattn/go-runewidth"
)

type routineList = sectionList[string]

func (m terminal) routineList() routineList {
	return groupSections(m.data.Order, m.app.sections, func(id string) string {
		return m.data.Routines[id].SectionID
	})
}

func (m *terminal) selectRoutine(id string) {
	list := m.routineList()
	if index := slices.Index(list.rows, id); index >= 0 {
		m.routineIndex = index
	}
	m.scrollRoutineList(list)
}

func (m *terminal) scrollRoutines() {
	m.scrollRoutineList(m.routineList())
}

func (m *terminal) scrollRoutineList(list routineList) {
	m.routineIndex, m.routineOffset = list.reveal(m.routineIndex, m.routineOffset, m.room())
}

func (m *terminal) pageRoutines(list routineList, step int) {
	m.routineIndex, m.routineOffset = list.page(m.routineIndex, m.routineOffset, m.room(), step)
}

func (m terminal) routineTarget(list routineList) string {
	if !list.visible(m.routineIndex, m.routineOffset, m.room()) {
		return ""
	}
	return list.rows[m.routineIndex]
}

func (m *terminal) routineKey(key string) {
	list := m.routineList()
	if id := m.routineDeleteID; id != "" {
		m.routineDeleteID, m.notice = "", ""
		if key == "d" {
			if m.execute("routine-delete", id) {
				m.notice = "Routine deleted."
				m.scrollRoutines()
			}
			return
		}
		if key == "esc" {
			return
		}
	}
	switch key {
	case "esc", "v", "q":
		m.mode, m.notice = "", ""
		return
	case "j", "down":
		m.routineIndex++
	case "k", "up":
		m.routineIndex--
	case "g":
		m.routineIndex = 0
	case "G":
		m.routineIndex = len(list.rows) - 1
	case "ctrl+d", "pgdown":
		m.pageRoutines(list, 1)
		return
	case "ctrl+u", "pgup":
		m.pageRoutines(list, -1)
		return
	case "b":
		if id := m.routineTarget(list); id != "" {
			m.openSectionPicker("routine-section", id, m.day, m.data.Routines[id].SectionID)
		}
		return
	case "c":
		if id := m.routineTarget(list); id != "" {
			m.openCommand("routine-code " + id + " " + m.data.Routines[id].WorkCode)
		}
		return
	case "d":
		if id := m.routineTarget(list); id != "" {
			m.routineDeleteID = id
			m.notice = "Press d again to delete the routine | Esc cancels"
		}
		return
	case "J", "K":
		m.moveRoutine(list, key)
		return
	}
	m.scrollRoutineList(list)
}

func (m *terminal) moveRoutine(list routineList, key string) {
	id := m.routineTarget(list)
	if id == "" {
		return
	}
	step, position, direction := 1, "after", "down"
	if key == "K" {
		step, position, direction = -1, "before", "up"
	}
	next := m.routineIndex + step
	groupIndex := list.groupOf(m.routineIndex)
	if groupIndex < 0 {
		if next >= 0 && next < len(list.rows) && m.execute("routine-move", id, direction) {
			m.selectRoutine(id)
		}
		return
	}
	group := list.groups[groupIndex]
	if next < group.start || next >= group.end {
		groupIndex += step
		if groupIndex < 0 || groupIndex >= len(list.groups) {
			return
		}
		group = list.groups[groupIndex]
		next, position = group.start, "before"
		if step < 0 {
			next, position = group.end-1, "after"
		}
	}
	sectionID := group.section.ID
	if sectionID == "" {
		sectionID = "-"
	}
	args := []string{id, sectionID}
	if group.start < group.end {
		args = append(args, position, list.rows[next])
	}
	if m.execute("routine-place", args...) {
		m.selectRoutine(id)
	}
}

func (m terminal) drawRoutines(line func(string), width int) {
	line("  ROUTINES | j/k select | J/K move | b section | c code | dd delete | Esc back")
	list := m.routineList()
	repeatWidth := min(27, max(7, width/3))
	nameWidth := max(8, width-repeatWidth-14)
	if len(list.rows) == 0 {
		line("  No routines yet. Select a task and press R.")
	} else {
		line(fmt.Sprintf("  %-*s  %-*s %6s", nameWidth, "TASK", repeatWidth, "REPEATS", "EST"))
	}
	list.draw(m.routineOffset, m.room(), width, line, func(i int, id string) {
		r := m.data.Routines[id]
		repeat := r.repeatLabel()
		if len(list.groups) == 0 && r.SectionID != "" {
			repeat += " | " + r.SectionID
		}
		arrow := "  "
		if i == m.routineIndex {
			arrow = "> "
		}
		name := runewidth.FillRight(runewidth.Truncate(r.Title, nameWidth, "…"), nameWidth)
		repeats := runewidth.FillRight(runewidth.Truncate(repeat, repeatWidth, "…"), repeatWidth)
		line(fmt.Sprintf("%s%s  %s %6s", arrow, name, repeats, estimateLabel(r.Minutes)))
	}, nil)
}
