package main

import (
	"fmt"
	"slices"
	"sort"

	"github.com/mattn/go-runewidth"
)

type routineList struct {
	ids    []string
	groups []taskGroup
}

func (m terminal) routineList() routineList {
	if len(m.app.sections) == 0 {
		return routineList{ids: m.data.Order}
	}
	members := make(map[string][]string, len(m.app.sections)+1)
	for _, id := range m.data.Order {
		sectionID := m.data.Routines[id].SectionID
		if _, ok := sectionByID(m.app.sections, sectionID); !ok {
			sectionID = ""
		}
		members[sectionID] = append(members[sectionID], id)
	}
	list := routineList{ids: make([]string, 0, len(m.data.Order))}
	for _, s := range m.app.sections {
		list.appendGroup(s, members[s.ID])
	}
	list.appendGroup(section{Name: "Unassigned"}, members[""])
	return list
}

func (l *routineList) appendGroup(s section, ids []string) {
	start := len(l.ids)
	l.ids = append(l.ids, ids...)
	l.groups = append(l.groups, taskGroup{section: s, start: start, end: len(l.ids)})
}

func (l routineList) groupOf(index int) int {
	for i, g := range l.groups {
		if index < g.end {
			return i
		}
	}
	return -1
}

func (l routineList) lineOf(index int) int {
	return index + (l.groupOf(index)+1)*sectionHeaderHeight
}

func (l routineList) lineCount() int {
	return len(l.ids) + len(l.groups)*sectionHeaderHeight
}

func (m *terminal) selectRoutine(id string) {
	list := m.routineList()
	if index := slices.Index(list.ids, id); index >= 0 {
		m.routineIndex = index
	}
	m.scrollRoutineList(list)
}

func (m *terminal) scrollRoutines() {
	m.scrollRoutineList(m.routineList())
}

func (m *terminal) scrollRoutineList(list routineList) {
	room := m.room()
	m.routineIndex = max(0, min(m.routineIndex, len(list.ids)-1))
	m.routineOffset = min(m.routineOffset, max(0, list.lineCount()-room))
	if len(list.ids) == 0 {
		return
	}
	line := list.lineOf(m.routineIndex)
	if line < m.routineOffset {
		m.routineOffset = line
	}
	if line >= m.routineOffset+room {
		m.routineOffset = line - room + 1
	}
	if group := list.groupOf(m.routineIndex); group >= 0 &&
		list.groups[group].start == m.routineIndex {
		m.routineOffset = min(m.routineOffset, line-min(sectionHeaderHeight, room-1))
	}
}

func (m *terminal) pageRoutines(list routineList, step int) {
	room := m.room()
	jump := step * max(1, room/2)
	m.routineOffset = max(0, min(m.routineOffset+jump, list.lineCount()-room))
	lastLine := m.routineOffset + room - 1
	wanted := max(m.routineOffset, min(list.lineOf(m.routineIndex)+jump, lastLine))
	i := sort.Search(len(list.ids), func(i int) bool { return list.lineOf(i) >= wanted })
	if i < len(list.ids) && list.lineOf(i) <= lastLine {
		m.routineIndex = i
	} else if i > 0 && list.lineOf(i-1) >= m.routineOffset {
		m.routineIndex = i - 1
	}
}

func (m terminal) routineTarget(list routineList) string {
	if m.routineIndex < 0 || m.routineIndex >= len(list.ids) {
		return ""
	}
	line := list.lineOf(m.routineIndex)
	if line < m.routineOffset || line >= m.routineOffset+m.room() {
		return ""
	}
	return list.ids[m.routineIndex]
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
		m.routineIndex = len(list.ids) - 1
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
		if next >= 0 && next < len(list.ids) && m.execute("routine-move", id, direction) {
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
		args = append(args, position, list.ids[next])
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
	if len(list.ids) == 0 {
		line("  No routines yet. Select a task and press R.")
	} else {
		line(fmt.Sprintf("  %-*s  %-*s %6s", nameWidth, "TASK", repeatWidth, "REPEATS", "EST"))
	}
	end := m.routineOffset + m.room()
	drawRows := func(start, stop, headers int) {
		for i := max(start, m.routineOffset-headers); i < min(stop, end-headers); i++ {
			r := m.data.Routines[list.ids[i]]
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
		}
	}
	if len(list.groups) == 0 {
		drawRows(0, len(list.ids), 0)
		return
	}
	for i, group := range list.groups {
		heading := group.start + i*sectionHeaderHeight
		if heading+sectionHeaderHeight > m.routineOffset && heading < end {
			for j, header := range sectionHeaderLines(group.section, "", width) {
				if at := heading + j; at >= m.routineOffset && at < end {
					line(header)
				}
			}
		}
		drawRows(group.start, group.end, (i+1)*sectionHeaderHeight)
	}
}
