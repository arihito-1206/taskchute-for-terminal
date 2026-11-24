package main

import (
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
)

const sectionHeaderHeight = 3

type sectionGroup struct {
	section    section
	start, end int
}

type sectionList[T any] struct {
	rows   []T
	groups []sectionGroup
}

func groupSections[T any](rows []T, sections []section, sectionID func(T) string) sectionList[T] {
	if len(sections) == 0 {
		return sectionList[T]{rows: rows}
	}
	members := make(map[string][]T, len(sections)+1)
	for _, s := range sections {
		members[s.ID] = nil
	}
	for _, row := range rows {
		id := sectionID(row)
		if _, ok := members[id]; !ok {
			id = ""
		}
		members[id] = append(members[id], row)
	}
	list := sectionList[T]{
		rows:   make([]T, 0, len(rows)),
		groups: make([]sectionGroup, 0, len(sections)+1),
	}
	for _, s := range sections {
		list.add(s, members[s.ID])
	}
	list.add(section{Name: "Unassigned"}, members[""])
	return list
}

func (l *sectionList[T]) add(s section, members []T) {
	start := len(l.rows)
	l.rows = append(l.rows, members...)
	l.groups = append(l.groups, sectionGroup{section: s, start: start, end: len(l.rows)})
}

func (l sectionList[T]) groupOf(index int) int {
	for i, g := range l.groups {
		if index < g.end {
			return i
		}
	}
	return -1
}

func (l sectionList[T]) rowLine(index int) int {
	return index + (l.groupOf(index)+1)*sectionHeaderHeight
}

func (l sectionList[T]) lineCount() int {
	return len(l.rows) + len(l.groups)*sectionHeaderHeight
}

func (l sectionList[T]) visible(index, offset, room int) bool {
	if index < 0 || index >= len(l.rows) {
		return false
	}
	line := l.rowLine(index)
	return line >= offset && line < offset+room
}

// reveal keeps the selected row in view and includes its heading when there is room.
func (l sectionList[T]) reveal(index, offset, room int) (int, int) {
	index = max(0, min(index, len(l.rows)-1))
	offset = min(offset, max(0, l.lineCount()-room))
	if len(l.rows) == 0 {
		return index, offset
	}
	line := l.rowLine(index)
	if line < offset {
		offset = line
	}
	if line >= offset+room {
		offset = line - room + 1
	}
	if group := l.groupOf(index); group >= 0 && index == l.groups[group].start {
		offset = min(offset, line-min(sectionHeaderHeight, room-1))
	}
	return index, offset
}

// page can show a viewport containing only section headings.
func (l sectionList[T]) page(index, offset, room, step int) (int, int) {
	jump := step * max(1, room/2)
	offset = max(0, min(offset+jump, l.lineCount()-room))
	lastLine := offset + room - 1
	wanted := max(offset, min(l.rowLine(index)+jump, lastLine))
	i := sort.Search(len(l.rows), func(i int) bool { return l.rowLine(i) >= wanted })
	if i < len(l.rows) && l.rowLine(i) <= lastLine {
		index = i
	} else if i > 0 && l.rowLine(i-1) >= offset {
		index = i - 1
	}
	return index, offset
}

func (l sectionList[T]) draw(offset, room, width int, headerLine func(string), drawRow func(int, T), capacity func(section, []T) string) {
	end := offset + room
	drawRows := func(start, stop, headers int) {
		for i := max(start, offset-headers); i < min(stop, end-headers); i++ {
			drawRow(i, l.rows[i])
		}
	}
	if len(l.groups) == 0 {
		drawRows(0, len(l.rows), 0)
	}
	for i, group := range l.groups {
		heading := group.start + i*sectionHeaderHeight
		if heading < end && heading+sectionHeaderHeight > offset {
			status := ""
			if capacity != nil && group.section.ID != "" {
				status = capacity(group.section, l.rows[group.start:group.end])
			}
			for j, text := range sectionHeaderLines(group.section, status, width) {
				if at := heading + j; at >= offset && at < end {
					headerLine(text)
				}
			}
		}
		drawRows(group.start, group.end, (i+1)*sectionHeaderHeight)
	}
}

func sectionHeaderLines(s section, capacity string, width int) []string {
	width = max(1, width)
	label := "  " + terminalText(s.Name, false)
	if s.ID != "" {
		label += "  " + s.Start + " – " + s.End
	}
	if capacity != "" {
		suffix := "  │  " + capacity
		if room := width - runewidth.StringWidth(suffix); room >= 8 {
			label = runewidth.Truncate(label, room, "…") + suffix
		} else {
			label = "  " + capacity
		}
	}
	label = runewidth.Truncate(terminalText(label, false), width, "…")
	label = runewidth.FillRight(label, width)
	const background = "\x1b[48;2;36;44;56m"
	const reset = "\x1b[0m"
	rule := background + "\x1b[38;5;245m" + strings.Repeat("─", width) + reset
	return []string{rule, background + "\x1b[38;5;252m" + label + reset, rule}
}
