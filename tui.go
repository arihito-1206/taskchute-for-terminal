package main

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

type clockMsg time.Time

const sectionHeaderHeight = 3

type sectionPicker struct {
	command, id, day string
	index            int
}

type taskGroup struct {
	section    section
	start, end int
}

type taskList struct {
	rows   []*task
	groups []taskGroup
}

func (l taskList) groupOf(index int) int {
	for i, g := range l.groups {
		if index < g.end {
			return i
		}
	}
	return -1
}

func (l taskList) taskLine(index int) int {
	return index + (l.groupOf(index)+1)*sectionHeaderHeight
}

func (l taskList) lineCount() int {
	return len(l.rows) + len(l.groups)*sectionHeaderHeight
}

type terminal struct {
	timelineOffset                                       int
	helpOffset                                           int
	suggestion                                           int
	routineOffset                                        int
	app                                                  application
	data                                                 *state
	sectionID                                            string
	picker                                               sectionPicker
	day                                                  string
	now                                                  time.Time
	pinned, splash, list, deleting                       bool
	selected, offset, routineIndex, width, height, count int
	mode, input, notice, noteID, noteDay                 string
	note                                                 noteEditor
}

func clock() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return clockMsg(t) })
}

type splashDone struct{}

func (m terminal) Init() tea.Cmd {
	return tea.Batch(clock(), tea.Tick(450*time.Millisecond, func(time.Time) tea.Msg { return splashDone{} }))
}

func (m *terminal) execute(cmd string, args ...string) bool {
	return m.executeDay(m.day, cmd, args...)
}

func (m *terminal) executeDay(day, cmd string, args ...string) bool {
	var selected *task
	if m.list {
		selected = m.target()
	}
	s, err := m.app.execute(day, cmd, args, time.Now())
	if err != nil {
		m.notice = err.Error()
		return false
	}
	if day != m.day {
		s.fill(m.day)
	}
	m.data = s
	if selected != nil {
		m.selectTask(selected.ID)
	}
	m.notice = "Saved"
	return true
}

func (m *terminal) changeDay(delta int) {
	d, _ := time.Parse(dateLayout, m.day)
	next := d.AddDate(0, 0, delta).Format(dateLayout)
	s, err := m.app.execute(next, "status", nil, m.now)
	if err != nil {
		m.notice = err.Error()
		return
	}
	m.data, m.day = s, next
	m.pinned = true
	m.selected, m.offset, m.count = 0, 0, 0
	m.deleting = false
	m.list = true
	m.scroll()
}

func (m terminal) target() *task {
	return m.targetIn(m.taskList())
}

func (m terminal) targetIn(list taskList) *task {
	if !m.list {
		if t := m.data.running(); t != nil {
			return t
		}
		return firstPending(list.rows)
	}
	if m.selected < 0 || m.selected >= len(list.rows) {
		return nil
	}
	selectedLine := list.taskLine(m.selected)
	if selectedLine < m.offset || selectedLine >= m.offset+m.room() {
		return nil
	}
	return list.rows[m.selected]
}

func firstPending(rows []*task) *task {
	for _, t := range rows {
		if t.State == "pending" {
			return t
		}
	}
	return nil
}

func (m terminal) visibleRows() []*task {
	return m.taskList().rows
}

func (m terminal) taskList() taskList {
	rows := m.data.rows(m.day)
	sections := m.app.sections
	if len(sections) == 0 {
		return taskList{rows: rows}
	}
	if m.sectionID != "" {
		visible := rows[:0]
		for _, t := range rows {
			if taskSection(t, sections, m.now) == m.sectionID {
				visible = append(visible, t)
			}
		}
		return taskList{rows: visible}
	}
	bySection := make(map[string][]*task, len(sections)+1)
	for _, t := range rows {
		id := taskSection(t, sections, m.now)
		bySection[id] = append(bySection[id], t)
	}
	list := taskList{rows: rows[:0], groups: make([]taskGroup, 0, len(sections)+1)}
	for _, s := range sections {
		list.appendGroup(s, bySection[s.ID])
	}
	list.appendGroup(section{Name: "Unassigned"}, bySection[""])
	return list
}

func (l *taskList) appendGroup(s section, members []*task) {
	start := len(l.rows)
	l.rows = append(l.rows, members...)
	l.groups = append(l.groups, taskGroup{section: s, start: start, end: len(l.rows)})
}

func (m *terminal) showList(sectionID string, target *task) {
	m.list, m.sectionID, m.selected, m.offset = true, sectionID, 0, 0
	if target != nil {
		m.selectTask(target.ID)
	}
	m.scroll()
}

func (m *terminal) selectTask(id string) {
	if i := taskIndex(m.visibleRows(), id); i >= 0 {
		m.selected = i
	}
}

func taskIndex(rows []*task, id string) int {
	return slices.IndexFunc(rows, func(t *task) bool { return t.ID == id })
}

func (m *terminal) scroll() {
	list := m.taskList()
	m.selected = max(0, min(m.selected, len(list.rows)-1))
	room := m.room()
	m.offset = min(m.offset, max(0, list.lineCount()-room))
	if len(list.rows) == 0 {
		return
	}
	selectionLine := list.taskLine(m.selected)
	if selectionLine < m.offset {
		m.offset = selectionLine
	}
	if selectionLine >= m.offset+room {
		m.offset = selectionLine - room + 1
	}
	if group := list.groupOf(m.selected); group >= 0 && m.selected == list.groups[group].start {
		m.offset = min(m.offset, selectionLine-min(sectionHeaderHeight, room-1))
	}
}

func (m *terminal) pageList(list taskList, step int) {
	room := m.room()
	jump := step * max(1, room/2)
	m.offset = max(0, min(m.offset+jump, list.lineCount()-room))
	lastLine := m.offset + room - 1
	wanted := max(m.offset, min(list.taskLine(m.selected)+jump, lastLine))
	i := sort.Search(len(list.rows), func(i int) bool { return list.taskLine(i) >= wanted })
	if i < len(list.rows) && list.taskLine(i) <= lastLine {
		m.selected = i
	} else if i > 0 && list.taskLine(i-1) >= m.offset {
		m.selected = i - 1
	}
}

func (m terminal) room() int {
	if m.height == 0 {
		return 12
	}
	reserved := 11
	if m.sectionID != "" && m.mode != "routines" {
		reserved += sectionHeaderHeight
	}
	return max(1, m.height-reserved)
}

func (m terminal) noteTask() *task {
	for _, t := range m.data.Tasks {
		if t.ID == m.noteID && !t.Deleted {
			return t
		}
	}
	return nil
}

func (m terminal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = event.Width, event.Height
		m.scroll()
		if m.mode == "routines" {
			m.scrollRoutines()
		}
		if m.mode == "note" {
			m.layoutNote()
		}
	case splashDone:
		m.splash = false
	case clockMsg:
		now := time.Time(event)
		m.now = now
		if !m.pinned && m.day != now.Format(dateLayout) {
			next := now.Format(dateLayout)
			if s, err := m.app.execute(next, "status", nil, now); err == nil {
				m.day, m.data = next, s
				m.selected, m.offset = 0, 0
				m.scroll()
			} else {
				m.notice = err.Error()
			}
		}
		return m, clock()
	case tea.PasteMsg:
		if m.mode == "note" {
			return m.updateNote(msg)
		}
		if m.mode == "command" {
			m.input += strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, event.Content)
		}
	case tea.KeyPressMsg:
		if event.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if event.String() == "ctrl+z" {
			return m, tea.Suspend
		}
		m.splash = false
		if m.mode == "note" {
			return m.updateNote(event)
		}
		if m.mode == "command" {
			m.edit(event)
		} else if m.mode == "routines" {
			m.routineKey(event.String())
		} else if m.mode == "section-picker" {
			m.sectionPickerKey(event.String())
		} else {
			if m.normalKey(event.String()) {
				return m, tea.Quit
			}
			return m, nil
		}
		m.scroll()
	default:
		if m.mode == "note" {
			return m.updateNote(msg)
		}
	}
	return m, nil
}

func (m *terminal) edit(key tea.KeyPressMsg) {
	prefix, matches := m.addSuggestions()
	if len(matches) > 0 {
		switch key.String() {
		case "down", "up":
			step := 1
			if key.String() == "up" {
				step = -1
			}
			m.suggestion = (m.suggestion + step + len(matches)) % len(matches)
			return
		case "tab":
			m.input = prefix + matches[m.suggestion%len(matches)]
			m.suggestion = 0
			return
		}
	}
	m.suggestion = 0
	switch key.String() {
	case "esc":
		m.mode, m.input, m.notice = "", "", ""
	case "ctrl+u":
		m.input = ""
	case "backspace":
		_, size := utf8.DecodeLastRuneInString(m.input)
		m.input = m.input[:len(m.input)-size]
	case "enter":
		input := strings.TrimSpace(m.input)
		if input == "" {
			return
		}
		parts := strings.Fields(input)
		cmd, args := parts[0], parts[1:]
		if cmd == "start" && len(args) == 0 && len(m.app.sections) > 0 {
			next := firstPending(m.visibleRows())
			if next != nil {
				args = []string{next.ID}
			} else if m.sectionID != "" {
				m.notice = "No pending task in this section."
				return
			}
		}
		if cmd == "code" && len(args) == 1 {
			args = []string{"@", args[0]}
		}
		for i, arg := range args {
			if cmd != "add" && (cmd != "code" || i == 0) && arg == "@" {
				target := m.target()
				if target == nil {
					m.notice = "No task selected."
					return
				}
				args[i] = target.ID
			}
		}
		switch cmd {
		case "ls", "status", "help", "routines":
			if cmd == "ls" {
				m.showList("", m.target())
			}
			if cmd == "status" {
				m.list, m.sectionID = false, ""
			}
			m.mode = ""
			if cmd == "help" {
				m.mode, m.helpOffset = "help", 0
			}
			if cmd == "routines" {
				m.mode = "routines"
				m.routineIndex, m.routineOffset = 0, 0
				m.scrollRoutines()
			}
		default:
			if !m.execute(cmd, args...) {
				return
			}
			m.mode = ""
		}
		m.input = ""
	default:
		if key.Text != "" {
			m.input += key.Text
		}
	}
}

func (m *terminal) openSectionPicker(command, id, day, current string) {
	m.mode, m.notice = "section-picker", ""
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
	case "enter":
		id := "-"
		if m.picker.index > 0 {
			id = m.app.sections[m.picker.index-1].ID
		}
		if !m.executeDay(m.picker.day, m.picker.command, m.picker.id, id) {
			return
		}
	case "esc":
		m.notice = ""
	default:
		return
	}
	m.mode = ""
	if m.picker.command == "routine-section" {
		m.mode = "routines"
		m.selectRoutine(m.picker.id)
	}
	m.picker = sectionPicker{}
}

func (m terminal) sectionPickerView(width int) string {
	lines := []string{"SECTION"}
	visible := max(1, m.helpRows()-2)
	first := max(0, m.picker.index-visible+1)
	for i := first; i < min(len(m.app.sections)+1, first+visible); i++ {
		label := "Unassigned"
		if i > 0 {
			s := m.app.sections[i-1]
			label = s.Name + "  " + s.Start + "–" + s.End
			if m.picker.command == "section" && sectionEnded(s, m.picker.day, m.now) {
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
	lines = append(lines, "j/k select | Enter save | Esc cancel")
	for i, line := range lines {
		lines[i] = runewidth.Truncate(terminalText(line, false), width, "…")
	}
	return strings.Join(lines, "\n")
}

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
	next := groupIndex + step
	if !m.list || groupIndex < 0 || next < 0 || next >= len(list.groups) {
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
	if m.mode == "timeline" {
		m.timelineKey(key)
		return false
	}
	if m.mode == "help" {
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
				if m.execute("delete", ids...) {
					m.notice = fmt.Sprintf("Deleted %d task(s). U to undo.", len(ids))
				}
				return false
			}
			if key == "esc" {
				return false
			}
		} else if digit, err := strconv.Atoi(key); err == nil && len(key) == 1 {
			m.count = min(max(1, len(rows)), m.count*10+digit)
			m.notice = fmt.Sprintf("%d | dd deletes", m.count)
			return false
		} else if key == "d" && target != nil {
			m.deleting = true
			m.notice = "Press d again to delete | Esc cancels"
			return false
		} else if m.count != 0 {
			m.count, m.notice = 0, ""
		}
	}
	switch key {
	case "q":
		return true
	case "esc":
		m.notice = ""
	case "l":
		if !m.list || m.sectionID != "" {
			m.showList("", target)
		} else {
			m.list = false
		}
	case "L":
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
	case "J", "K":
		m.moveTask(list, target, key)
	case "s", "enter":
		if target == nil {
			m.notice = "No task selected."
			break
		}
		if target.State == "running" {
			m.execute("done")
		} else if m.execute("start", target.ID) && target.State == "done" {
			for i, row := range m.visibleRows() {
				if row.State == "running" {
					m.selected = i
					break
				}
			}
		}
	case "p":
		m.execute("pause")
	case "U":
		if m.list {
			m.execute("undo-delete")
		}
	case "u":
		if m.list && target != nil {
			m.execute("reset", target.ID)
		}
	case "b":
		if target == nil {
			m.notice = "No task selected."
			break
		}
		m.openSectionPicker("section", target.ID, target.Day, target.SectionID)
	case "v":
		m.mode = "routines"
		m.routineIndex, m.routineOffset = 0, 0
		m.scrollRoutines()
		m.notice = ""
	case "t":
		m.mode, m.timelineOffset = "timeline", 0
	case "?":
		m.mode, m.helpOffset = "help", 0
	case "n":
		if target != nil && (m.list || target.State == "running") {
			m.mode, m.noteID, m.noteDay, m.input, m.notice = "note", target.ID, target.Day, "", ""
			m.openNote()
		} else {
			m.notice = "Start a task or select one in the list."
		}
	case ":", "i", "a", "r", "e", "S", "F", "R", "c":
		prefix := map[string]string{"a": "add ", "r": "rename @ ", "e": "estimate @ ", "S": "set-start @ ", "F": "set-end @ ", "R": "routine @ ", "c": "category @ "}[key]
		if strings.Contains(prefix, "@") && target == nil {
			m.notice = "No task selected."
			break
		}
		m.mode, m.input, m.notice = "command", prefix, ""
		if key == "c" {
			m.notice = "💼 work / 仕事   🏠 home / 家事   🎨 hobby / 趣味   ☕ rest / 休憩 | - clears"
		}

	}
	return false
}

const startupMark = `                   ######
                  ######

   ######################################
  ######################################
                ######
               ######   ##############
              ######   #####
             ######   #####
            ######   #############
           ######
          ####################
         ####################`

func (m terminal) View() tea.View {
	width := m.width
	if width == 0 {
		width = 80
	}
	var cursor *tea.Cursor
	if m.mode == "note" {
		raw, noteCursor := m.renderNote(width)
		view := tea.NewView(raw)
		view.Cursor, view.AltScreen = noteCursor, true
		return view
	}
	raw := m.render(width)
	if m.mode == "command" {
		for y, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(ansi.Strip(line), "  : ") {
				cursor = tea.NewCursor(min(width-1, ansi.StringWidth(line)), y)
				break
			}
		}
	}
	view := tea.NewView(raw)
	view.Cursor = cursor
	view.AltScreen = true
	return view
}

func (m terminal) render(width int) string {
	if m.mode == "timeline" {
		return m.timelineView(width)
	}
	if m.mode == "help" {
		return m.helpView(width)
	}
	if m.mode == "section-picker" {
		return m.sectionPickerView(width)
	}
	if m.splash && width >= 42 && m.height >= 15 {
		return startupMark
	}
	var b strings.Builder
	line := func(s string) { fmt.Fprintln(&b, runewidth.Truncate(terminalText(s, false), max(1, width), "…")) }
	line("  " + m.day + " | Alt+h/l: day")
	list := m.taskList()
	rows := list.rows
	summaryLines := strings.Split(summaryWithNext(m.data, m.day, m.now, firstPending(rows)), "\n")
	if m.sectionID != "" && m.mode == "" {
		summaryLines[1], _, _ = strings.Cut(summaryLines[1], " | Day ends ")
	}
	running := m.data.running()
	for i, s := range summaryLines {
		if i == 0 && running != nil {
			fmt.Fprintln(&b, runningBand("  ▶ "+s, min(width, 90)))
			continue
		}
		if i == 1 && strings.Contains(s, " | Day ends ") {
			next, finish, _ := strings.Cut(s, " | Day ends ")
			next = runewidth.Truncate(strings.TrimPrefix(next, "Next "), max(0, width-runewidth.StringWidth(finish)-20), "...")
			b.WriteString("  \x1b[2mNext\x1b[0m \x1b[35m" + next + "\x1b[0m | \x1b[1mDay ends " + finish + "\x1b[0m\n")
		} else {
			b.WriteString("  \x1b[36;1m" + runewidth.Truncate(s, max(1, width-2), "…") + "\x1b[0m\n")
		}
	}
	if t := running; t != nil && len(t.Sessions) > 0 {
		details := "  Started " + timeLabel(t.Sessions[0].Start, m.day, m.now.Location()) + " | Est. " + estimateLabel(t.Minutes)
		if t.WorkCode != "" {
			details += " | Code " + terminalText(t.WorkCode, false)
		}
		line(details)
		if bar := estimateProgress(t, m.now, width); bar != "" {
			fmt.Fprintln(&b, "  "+bar)
		}
	}
	line(strings.Repeat("-", min(width, 90)))
	switch m.mode {
	case "routines":
		m.drawRoutines(line, width)
	case "command":
		line("  " + commandUsage(m.input))
		m.input = terminalText(m.input, false)
		fmt.Fprintln(&b, "\x1b[36;1m  : \x1b[0m"+ansi.TruncateLeft(m.input, max(0, ansi.StringWidth(m.input)-max(0, width-5)), ""))
		if _, matches := m.addSuggestions(); len(matches) > 0 {
			line("  Tab completes | Up/Down select")
			for i, name := range matches {
				marker := "    "
				if i == m.suggestion%len(matches) {
					marker = "  > "
				}
				line(marker + name)
			}
		}
	default:
		line("  s start/finish | n note | l list | L section | t timeline | a add | ? help | q quit")
		if m.list {
			if s, ok := sectionByID(m.app.sections, m.sectionID); ok {
				for _, text := range sectionHeaderLines(s, sectionCapacity(s, rows), width) {
					line(text)
				}
			}
			titleWidth := max(8, min(60, width-56))
			line(fmt.Sprintf("  %-7s  %-*s     EST  %-11s  %-11s  ELAPSED", "STATE", titleWidth, "TASK", "START", "END"))
			if m.sectionID != "" && len(rows) == 0 {
				line("  No tasks in this section.")
			}
			end := m.offset + m.room()
			drawRows := func(start, stop, headers int) {
				for i := max(start, m.offset-headers); i < min(stop, end-headers); i++ {
					text := row(rows[i], m.day, m.now, titleWidth)
					prefix := "  "
					if i == m.selected {
						prefix = "> "
					}
					switch {
					case m.sectionID != "" && rows[i].State == "running":
						fmt.Fprintln(&b, runningBand(prefix+text, min(width, 90)))
					case m.sectionID != "" && i == m.selected:
						fmt.Fprintln(&b, "\x1b[38;5;252;48;5;237m"+ansi.Truncate(prefix+text, width, "…")+"\x1b[0m")
					case rows[i].State == "running":
						fmt.Fprintln(&b, "\x1b[1;36m"+ansi.Truncate(prefix+text, width, "…")+"\x1b[0m")
					default:
						line(prefix + text)
					}
				}
			}
			if len(list.groups) == 0 {
				drawRows(0, len(rows), 0)
			}
			for i, g := range list.groups {
				heading := g.start + i*sectionHeaderHeight
				if heading < end && heading+sectionHeaderHeight > m.offset {
					capacity := ""
					if g.section.ID != "" {
						capacity = sectionCapacity(g.section, rows[g.start:g.end])
					}
					for j, text := range sectionHeaderLines(g.section, capacity, width) {
						if heading+j >= m.offset && heading+j < end {
							line(text)
						}
					}
				}
				drawRows(g.start, g.end, (i+1)*sectionHeaderHeight)
			}
			line("  j/k select | J/K move | b section | [count]dd delete | U undo")
		}
	}
	if m.notice != "" {
		line("  " + m.notice)
	}
	return strings.TrimRight(b.String(), "\n")
}

func sectionHeaderLines(s section, capacity string, width int) []string {
	width = max(1, width)
	label := "  " + terminalText(s.Name, false)
	if s.ID != "" {
		label += "  " + s.Start + "–" + s.End
	}
	if capacity != "" {
		suffix := "  │  " + capacity
		if room := width - runewidth.StringWidth(suffix); room >= 8 {
			label = runewidth.Truncate(label, room, "…") + suffix
		} else {
			label = "  " + capacity
		}
	}
	rule := strings.Repeat("─", width)
	return []string{rule, label, rule}
}

func runningBand(text string, width int) string {
	text = ansi.Truncate(text, width, "…")
	return "\x1b[1;97;48;5;24m" + text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text))) + "\x1b[0m"
}

func (m terminal) notePanelWidth() int {
	width := m.width
	if width == 0 {
		width = 80
	}
	return min(68, width-4)
}

// renderNote draws a centered panel over the existing screen. The top timer
// stays visible in normal terminal sizes.
func (m terminal) renderNote(width int) (string, *tea.Cursor) {
	task := m.noteTask()
	height := m.height
	if height == 0 {
		height = 24
	}
	if task == nil {
		return m.render(width), nil
	}
	if width < 24 || height < 11 {
		return "  Enlarge terminal for notes | Esc closes", nil
	}
	panelWidth := m.notePanelWidth()
	contentWidth := panelWidth - 4
	box := func(text, style string) string {
		text = ansi.Truncate(text, contentWidth, "…")
		padding := strings.Repeat(" ", max(0, contentWidth-ansi.StringWidth(text)))
		return "\x1b[36m│\x1b[0m " + style + text + "\x1b[0m" + padding + " \x1b[36m│\x1b[0m"
	}
	border := func(left, right string) string {
		return "\x1b[36m" + left + strings.Repeat("─", panelWidth-2) + right + "\x1b[0m"
	}
	panel := []string{border("╭", "╮"), box("Note  /  "+title(task), "\x1b[1;36m"), border("├", "┤")}
	for _, row := range strings.Split(m.note.history.View(), "\n") {
		panel = append(panel, box(row, ""))
	}
	panel = append(panel, border("├", "┤"))
	inputRow := len(panel)
	for _, row := range strings.Split(m.note.draft.View(), "\n") {
		panel = append(panel, box(row, ""))
	}
	hint := "Enter save | Ctrl+n newline | Ctrl+w h history | Esc"
	if m.note.historyFocus {
		hint = "j/k select | e edit | dd delete | Ctrl+w l editor"
	}
	if m.note.editing >= 0 && !m.note.historyFocus {
		hint = "Editing comment | Enter save | Ctrl+n newline | Esc"
	}
	if m.notice != "" && m.notice != "Saved" {
		hint = terminalText(m.notice, false)
	}
	panel = append(panel, box(hint, "\x1b[2m"), border("╰", "╯"))
	m.mode, m.notice = "", ""
	background := strings.Split(m.render(width), "\n")
	top := max(3, (height-len(panel))/2)
	left := (width - panelWidth) / 2
	for len(background) < top+len(panel) {
		background = append(background, "")
	}
	for i, text := range panel {
		original := background[top+i]
		before := ansi.Cut(original, 0, left)
		before += strings.Repeat(" ", max(0, left-ansi.StringWidth(before)))
		after := ansi.Cut(original, left+panelWidth, width)
		after = strings.Repeat(" ", max(0, min(1, width-left-panelWidth-ansi.StringWidth(after)))) + after
		background[top+i] = before + "\x1b[0m" + text + after
	}
	cursor := m.note.draft.Cursor()
	if cursor != nil {
		cursor.Position.X += left + 2
		cursor.Position.Y += top + inputRow
		cursor.Shape = tea.CursorBar
	}
	return strings.Join(background, "\n"), cursor
}

// addSuggestions preserves the optional estimate while completing only the title.
func (m terminal) addSuggestions() (string, []string) {
	body, ok := strings.CutPrefix(m.input, "add ")
	if !ok {
		return "", nil
	}
	prefix := "add "
	body = strings.TrimLeft(body, " ")
	if first, rest, spaced := strings.Cut(body, " "); spaced {
		if _, err := minutes(first); err == nil {
			prefix += first + " "
			body = strings.TrimLeft(rest, " ")
		}
	}
	if body == "" {
		return "", nil
	}
	query := strings.ToLower(body)
	var matches []string
	seen := map[string]bool{}
	for i := len(m.data.Tasks) - 1; i >= 0 && len(matches) < 5; i-- {
		t := m.data.Tasks[i]
		if t.Deleted || t.Day > m.day {
			continue
		}
		name := strings.ToLower(t.Title)
		if !seen[name] && strings.HasPrefix(name, query) {
			seen[name] = true
			matches = append(matches, t.Title)
		}
	}
	return prefix, matches
}
