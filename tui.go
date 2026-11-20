package main

import (
	"context"
	"fmt"
	"slices"
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

type sectionPicker struct {
	command, id, day string
	index            int
}

type taskList = sectionList[*task]

type taskFieldEdit struct {
	field, day  string
	ids, titles []string
}

type endTimeEdit struct {
	task  *task
	index int
}

type terminal struct {
	endEdit                                              endTimeEdit
	fieldEdit                                            *taskFieldEdit
	timelineOffset                                       int
	helpOffset                                           int
	suggestion                                           int
	inputCursor                                          int
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
	commandReturn                                        string
	filter                                               string
	routineDeleteID                                      string
	note                                                 noteEditor
}

func clock() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return clockMsg(t) })
}

type splashDone struct{}

func (m terminal) Init() tea.Cmd {
	return tea.Batch(clock(), tea.Tick(time.Second, func(time.Time) tea.Msg { return splashDone{} }))
}

func (m *terminal) execute(cmd string, args ...string) bool {
	return m.executeDay(m.day, cmd, args...)
}

func (m *terminal) executeDay(day, cmd string, args ...string) bool {
	var selected *task
	if m.list {
		selected = m.target()
	}
	s, err := m.app.execute(context.Background(), day, cmd, args, time.Now())
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
	m.showDay(d.AddDate(0, 0, delta).Format(dateLayout), true)
}

func (m *terminal) showDay(day string, pinned bool) {
	s, err := m.app.execute(context.Background(), day, "status", nil, m.now)
	if err != nil {
		m.notice = err.Error()
		return
	}
	m.data, m.day, m.pinned = s, day, pinned
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
	if !list.visible(m.selected, m.offset, m.room()) {
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

func (m terminal) taskFilter() string {
	if m.mode == "search" {
		return strings.TrimSpace(m.input)
	}
	return m.filter
}

func (m terminal) taskList() taskList {
	return m.taskListFor(m.taskFilter())
}

func (m terminal) taskListFor(query string) taskList {
	rows := m.data.rows(m.day)
	if query != "" {
		query = strings.ToLower(query)
		rows = slices.DeleteFunc(rows, func(t *task) bool {
			return !strings.Contains(strings.ToLower(t.Title), query) &&
				!strings.Contains(strings.ToLower(t.WorkCode), query)
		})
	}
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
	list := groupSections(rows, sections, func(t *task) string {
		return taskSection(t, sections, m.now)
	})
	if query != "" {
		list.groups = slices.DeleteFunc(list.groups, func(g sectionGroup) bool { return g.start == g.end })
	}
	return list
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
	m.selected, m.offset = m.taskList().reveal(m.selected, m.offset, m.room())
}

func (m *terminal) pageList(list taskList, step int) {
	m.selected, m.offset = list.page(m.selected, m.offset, m.room(), step)
}

func (m terminal) room() int {
	if m.height == 0 {
		return 12
	}
	reserved := 11
	if m.mode == "search" || (m.mode == "" && m.list && m.filter != "") {
		reserved++
	}
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
			if s, err := m.app.execute(context.Background(), next, "status", nil, now); err == nil {
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
		if m.mode == "command" || m.mode == "search" {
			before := m.input
			m.insertCommandText(strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, event.Content))
			if m.mode == "search" && m.input != before {
				m.selected, m.offset = 0, 0
				m.scroll()
			}
			m.suggestion = 0
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
		} else if m.mode == "search" {
			m.editSearch(event)
		} else if m.mode == "routines" {
			m.routineKey(event.String())
		} else if m.mode == "section-picker" {
			m.sectionPickerKey(event.String())
		} else if m.mode == "interval-picker" {
			m.intervalPickerKey(event.String())
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
	matches := m.inputSuggestions()
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
			m.input = matches[m.suggestion%len(matches)].input
			m.inputCursor = len(m.input)
			m.suggestion = 0
			return
		}
	}
	m.suggestion = 0
	switch key.String() {
	case "esc":
		m.fieldEdit = nil
		m.endEdit = endTimeEdit{}
		m.mode, m.notice = m.commandReturn, ""
		m.input, m.inputCursor, m.commandReturn = "", 0, ""
	case "ctrl+u":
		m.input, m.inputCursor = "", 0
	case "left", "ctrl+b":
		_, size := utf8.DecodeLastRuneInString(m.input[:m.inputCursor])
		m.inputCursor -= size
	case "right", "ctrl+f":
		_, size := utf8.DecodeRuneInString(m.input[m.inputCursor:])
		m.inputCursor += size
	case "home", "ctrl+a":
		m.inputCursor = 0
	case "end", "ctrl+e":
		m.inputCursor = len(m.input)
	case "backspace":
		_, size := utf8.DecodeLastRuneInString(m.input[:m.inputCursor])
		start := m.inputCursor - size
		m.input = m.input[:start] + m.input[m.inputCursor:]
		m.inputCursor = start
	case "delete":
		_, size := utf8.DecodeRuneInString(m.input[m.inputCursor:])
		m.input = m.input[:m.inputCursor] + m.input[m.inputCursor+size:]
	case "enter":
		input := strings.TrimSpace(m.input)
		if input == "" {
			return
		}
		if edit := m.fieldEdit; edit != nil {
			args := append([]string{input}, edit.ids...)
			if m.executeDay(edit.day, "batch-"+edit.field, args...) {
				m.fieldEdit = nil
				m.input, m.inputCursor, m.mode = "", 0, ""
			}
			return
		}
		if edit := m.endEdit; edit.task != nil {
			if m.executeDay(edit.task.Day, "set-end", edit.task.ID, input, strconv.Itoa(edit.index+1)) {
				m.endEdit = endTimeEdit{}
				m.input, m.inputCursor, m.commandReturn, m.mode = "", 0, "", ""
			}
			return
		}
		parts := strings.Fields(input)
		cmd, args := parts[0], parts[1:]
		if cmd == "start" && len(args) == 0 && (len(m.app.sections) > 0 || m.filter != "") {
			next := firstPending(m.visibleRows())
			if next != nil {
				args = []string{next.ID}
			} else if m.filter != "" || m.sectionID != "" {
				m.notice = "No pending task in this section."
				if m.filter != "" {
					m.notice = "No pending task matches the filter."
				}
				return
			}
		}
		switch {
		case cmd == "code" && len(args) == 1:
			args = []string{"@", args[0]}
		case (cmd == "code" || cmd == "routine-code") && len(args) >= 2:
			body := strings.TrimSpace(strings.TrimPrefix(input, cmd))
			args = []string{args[0], strings.TrimSpace(strings.TrimPrefix(body, args[0]))}
		}
		for i, arg := range args {
			if cmd != "add" && cmd != "routine-code" && (cmd != "code" || i == 0) && arg == "@" {
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
				m.list, m.sectionID, m.filter = false, "", ""
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
			m.mode = m.commandReturn
		}
		m.input, m.inputCursor, m.commandReturn = "", 0, ""
	default:
		if key.Text != "" {
			m.insertCommandText(key.Text)
		}
	}
}

func (m *terminal) openCommand(input string) {
	m.commandReturn, m.mode, m.input, m.notice = m.mode, "command", input, ""
	m.inputCursor, m.suggestion = len(input), 0
}

func (m *terminal) insertCommandText(value string) {
	m.input = m.input[:m.inputCursor] + value + m.input[m.inputCursor:]
	m.inputCursor += len(value)
}

func (m *terminal) editSearch(key tea.KeyPressMsg) {
	switch key.String() {
	case "enter":
		m.filter = m.taskFilter()
		m.mode, m.input, m.inputCursor = "", "", 0
	case "esc":
		m.mode, m.input, m.inputCursor = "", "", 0
		m.selected, m.offset = 0, 0
	default:
		before := m.input
		m.edit(key)
		if before != m.input {
			m.selected, m.offset = 0, 0
		}
	}
}

func (m terminal) inputPrompt() string {
	if m.mode == "search" {
		return "  / "
	}
	return "  : "
}

func (m terminal) commandLine(width int) (string, int) {
	prompt := m.inputPrompt()
	room := max(0, width-len(prompt))
	before := terminalText(m.input[:m.inputCursor], false)
	after := terminalText(m.input[m.inputCursor:], false)
	before = ansi.TruncateLeft(before, max(0, ansi.StringWidth(before)-max(0, room-1)), "")
	used := ansi.StringWidth(before)
	after = ansi.Truncate(after, max(0, room-used), "")
	return "\x1b[36;1m" + prompt + "\x1b[0m" + before + after, min(width-1, len(prompt)+used)
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
			label = s.Name + "  " + s.Start + " – " + s.End
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
			m.notice = fmt.Sprintf("%d | dd delete · c category · C code", m.count)
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
		} else if m.count != 0 {
			m.count, m.notice = 0, ""
		}
	}
	switch key {
	case "q":
		return true
	case "/":
		m.mode, m.list, m.input, m.notice = "search", true, m.filter, ""
		m.inputCursor = len(m.input)
	case "esc":
		m.notice = ""
		if m.filter != "" {
			m.filter = ""
			m.showList(m.sectionID, target)
			break
		}
		if m.sectionID != "" {
			m.list, m.sectionID = false, ""
		}
	case "l":
		if !m.list || m.sectionID != "" {
			m.showList("", target)
		} else {
			m.list, m.filter = false, ""
		}
	case "L":
		if m.sectionID != "" {
			m.list, m.sectionID, m.notice, m.filter = false, "", "", ""
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
	case "J", "K":
		if m.filter != "" {
			list = m.taskListFor("")
		}
		m.moveTask(list, target, key)
	case "s", "enter":
		if target == nil {
			m.notice = "No task selected."
			break
		}
		if target.State == "running" {
			m.execute("done")
		} else if m.execute("start", target.ID) && (target.State == "done" || target.legacyPaused()) {
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
	case "F":
		if target == nil || len(target.Sessions) == 0 {
			m.notice = "Select a task with recorded execution intervals."
			break
		}
		m.endEdit = endTimeEdit{task: target, index: len(target.Sessions) - 1}
		m.notice = ""
		if len(target.Sessions) > 1 {
			m.mode = "interval-picker"
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
	if m.mode == "command" || m.mode == "search" {
		_, x := m.commandLine(width)
		for y, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(ansi.Strip(line), m.inputPrompt()) {
				cursor = tea.NewCursor(x, y)
				cursor.Shape = tea.CursorBar
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
	if m.mode == "interval-picker" {
		return m.intervalPickerView(width)
	}
	if m.splash && width >= 42 && m.height >= 15 {
		return startupMark
	}
	var b strings.Builder
	line := func(s string) { fmt.Fprintln(&b, runewidth.Truncate(terminalText(s, false), max(1, width), "…")) }
	line("  " + dateWithWeekday(m.day) + " | Alt+h/l: day | Alt+t: today")
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
		if t.WorkCode != "" && !(m.list && m.mode == "") {
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
		if edit := m.endEdit; edit.task != nil {
			line("  End time (HHMM) | " + m.intervalLabel(edit.task, edit.index))
		} else if edit := m.fieldEdit; edit != nil {
			line(fmt.Sprintf("  Set %s on %d task(s) | - clears | Esc cancels", edit.field, len(edit.ids)))
			shown := min(len(edit.titles), max(1, m.room()-2))
			for _, name := range edit.titles[:shown] {
				line("  > " + name)
			}
			if shown < len(edit.titles) {
				line(fmt.Sprintf("  … and %d more", len(edit.titles)-shown))
			}
		} else {
			line("  " + commandUsage(m.input))
			words := strings.Fields(m.input)
			if len(words) >= 2 && words[1] == "@" && words[0] != "add" && words[0] != "routine-code" {
				if target := m.targetIn(list); target != nil {
					line("  @ " + title(target))
					if len(target.Sessions) > 0 {
						line(fmt.Sprintf("    %s | Actual %s | Est. %s", target.Day, duration(elapsed(target, m.now)), estimateLabel(target.Minutes)))
					}
				} else {
					line("  @ No task selected.")
				}
			}
		}
		command, _ := m.commandLine(width)
		fmt.Fprintln(&b, command)
		if matches := m.inputSuggestions(); len(matches) > 0 {
			line("  Tab completes | Up/Down select")
			for i, name := range matches {
				marker := "    "
				if i == m.suggestion%len(matches) {
					marker = "  > "
				}
				line(marker + name.label)
			}
		}
	default:
		line("  s start/finish | n note | l list | L section | t timeline | a add | ? help | q quit")
		if m.list {
			query := m.taskFilter()
			capacity := sectionCapacity
			if query != "" {
				full := m.taskListFor("")
				capacity = func(s section, _ []*task) string {
					if m.sectionID != "" {
						return sectionCapacity(s, full.rows)
					}
					for _, group := range full.groups {
						if group.section.ID == s.ID {
							return sectionCapacity(s, full.rows[group.start:group.end])
						}
					}
					return ""
				}
			}
			if s, ok := sectionByID(m.app.sections, m.sectionID); ok {
				for _, text := range sectionHeaderLines(s, capacity(s, rows), width) {
					line(text)
				}
			}
			titleWidth := max(8, min(60, width-56))
			line(fmt.Sprintf("  %-7s  %-*s     EST  %-11s  %-11s  ELAPSED", "STATE", titleWidth, "TASK", "START", "END"))
			if m.mode == "search" {
				input, _ := m.commandLine(width)
				if hint := fmt.Sprintf("  %d matches | Enter apply · Esc cancel", len(rows)); ansi.StringWidth(input)+ansi.StringWidth(hint) <= width {
					input += hint
				}
				fmt.Fprintln(&b, input)
			} else if query != "" {
				line(fmt.Sprintf("  %d matches | / edit · Esc clear | Filter: %s", len(rows), query))
			}
			if len(rows) == 0 {
				if query != "" {
					line("  No matching tasks.")
				} else if m.sectionID != "" {
					line("  No tasks in this section.")
				}
			}
			list.draw(m.offset, m.room(), width, line, func(i int, task *task) {
				codeLabel := ""
				if task.WorkCode != "" {
					codeLabel = "🔖"
				}
				text := row(task, m.day, m.now, titleWidth, title(task), codeLabel)
				prefix := "  "
				if i == m.selected {
					prefix = "> "
				}
				switch {
				case m.sectionID != "" && task.State == "running":
					fmt.Fprintln(&b, runningBand(prefix+text, min(width, 90)))
				case task.State == "done":
					style := "\x1b[38;5;245m"
					if m.sectionID != "" && i == m.selected {
						style = "\x1b[38;5;249;48;5;237m"
					}
					fmt.Fprintln(&b, style+ansi.Truncate(prefix+text, width, "…")+"\x1b[0m")
				case m.sectionID != "" && i == m.selected:
					fmt.Fprintln(&b, "\x1b[38;5;252;48;5;237m"+ansi.Truncate(prefix+text, width, "…")+"\x1b[0m")
				case task.State == "running":
					fmt.Fprintln(&b, "\x1b[1;36m"+ansi.Truncate(prefix+text, width, "…")+"\x1b[0m")
				default:
					line(prefix + text)
				}
				if i == m.selected {
					var details []string
					if task.RoutineID != "" {
						if routine, ok := m.data.Routines[task.RoutineID]; ok {
							details = append(details, "Routine · "+routine.repeatLabel())
						} else {
							details = append(details, "Former routine")
						}
					}
					if task.WorkCode != "" {
						details = append(details, "Code "+task.WorkCode)
					}
					if len(details) > 0 {
						line("           └ " + strings.Join(details, " │ "))
					}
				}
			}, capacity)
			line("  j/k select | J/K move | b section | / filter | [count]dd delete | U undo")
		}
	}
	if m.notice != "" {
		line("  " + m.notice)
	}
	return strings.TrimRight(b.String(), "\n")
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

type inputSuggestion struct {
	label string
	input string
}

func (m terminal) inputSuggestions() []inputSuggestion {
	if m.mode == "search" || m.endEdit.task != nil {
		return nil
	}
	if m.fieldEdit != nil {
		if m.fieldEdit.field == "code" {
			return m.workCodeSuggestions("", m.input)
		}
		return nil
	}
	command, body, found := strings.Cut(m.input, " ")
	if found && (command == "code" || command == "routine-code") {
		body = strings.TrimLeft(body, " ")
		target, query, separated := strings.Cut(body, " ")
		if command == "code" && !separated && target != "@" {
			return m.workCodeSuggestions("code @ ", target)
		}
		if target != "" && (separated || target == "@") {
			return m.workCodeSuggestions(command+" "+target+" ", query)
		}
		return nil
	}
	return m.addSuggestions()
}

func (m terminal) workCodeSuggestions(prefix, input string) []inputSuggestion {
	query := strings.ToLower(strings.TrimSpace(input))
	seen := make(map[string]bool)
	var matches []inputSuggestion
	for i := len(m.data.Tasks) - 1; i >= 0 && len(matches) < 5; i-- {
		t := m.data.Tasks[i]
		if t.Deleted || t.Day > m.day || t.WorkCode == "" || strings.ContainsFunc(t.WorkCode, unicode.IsControl) {
			continue
		}
		code := strings.ToLower(t.WorkCode)
		if !seen[code] && strings.Contains(code, query) {
			seen[code] = true
			matches = append(matches, inputSuggestion{label: t.WorkCode, input: prefix + t.WorkCode})
		}
	}
	return matches
}

// addSuggestions inherits the estimate unless the user supplied one.
func (m terminal) addSuggestions() []inputSuggestion {
	body, ok := strings.CutPrefix(m.input, "add ")
	if !ok {
		return nil
	}
	prefix := "add "
	explicitEstimate := false
	body = strings.TrimLeft(body, " ")
	if first, rest, spaced := strings.Cut(body, " "); spaced {
		if _, err := minutes(first); err == nil {
			prefix += first + " "
			explicitEstimate = true
			body = strings.TrimLeft(rest, " ")
		}
	}
	if body == "" {
		return nil
	}
	query := strings.ToLower(body)
	var matches []inputSuggestion
	seen := map[string]bool{}
	for i := len(m.data.Tasks) - 1; i >= 0 && len(matches) < 5; i-- {
		t := m.data.Tasks[i]
		if t.Deleted || t.Day > m.day {
			continue
		}
		name := strings.ToLower(t.Title)
		if !seen[name] && strings.HasPrefix(name, query) {
			seen[name] = true
			candidate := inputSuggestion{label: t.Title, input: prefix + t.Title}
			if t.Minutes != nil {
				candidate.label += " · " + estimateLabel(t.Minutes)
				if !explicitEstimate {
					candidate.input = fmt.Sprintf("add %g %s", *t.Minutes, t.Title)
				}
			}
			matches = append(matches, candidate)
		}
	}
	return matches
}
