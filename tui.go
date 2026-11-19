package main

import (
	"fmt"
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

type terminal struct {
	timelineOffset                                       int
	helpOffset                                           int
	suggestion                                           int
	app                                                  application
	data                                                 *state
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
	s, err := m.app.execute(day, cmd, args, time.Now())
	if err != nil {
		m.notice = err.Error()
		return false
	}
	if day != m.day {
		s.fill(m.day)
	}
	m.data = s
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
}

func (m terminal) target() *task {
	rows := m.data.rows(m.day)
	if !m.list {
		if t := m.data.running(); t != nil {
			return t
		}
		for _, t := range rows {
			if t.State == "pending" {
				return t
			}
		}
		return nil
	}
	if m.selected < 0 || m.selected >= len(rows) {
		return nil
	}
	return rows[m.selected]
}

func (m *terminal) scroll() {
	m.selected = max(0, min(m.selected, len(m.data.rows(m.day))-1))
	room := m.room()
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+room {
		m.offset = m.selected - room + 1
	}
}

func (m terminal) room() int {
	if m.height == 0 {
		return 12
	}
	return max(1, m.height-11)
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
		} else {
			if m.normalKey(event.String()) {
				return m, tea.Quit
			}
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
		for i, arg := range args {
			if cmd != "add" && arg == "@" {
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
			if cmd == "ls" || cmd == "status" {
				m.list = cmd == "ls"
			}
			m.mode = ""
			if cmd == "help" {
				m.mode, m.helpOffset = "help", 0
			}
			if cmd == "routines" {
				m.mode = "routines"
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

func (m *terminal) routineKey(key string) {
	count := len(m.data.Order)
	switch key {
	case "esc", "v", "q":
		m.mode = ""
		m.notice = ""
	case "j", "down":
		m.routineIndex = min(max(0, count-1), m.routineIndex+1)
	case "k", "up":
		m.routineIndex = max(0, m.routineIndex-1)
	case "g":
		m.routineIndex = 0
	case "G":
		m.routineIndex = max(0, count-1)
	case "J", "K":
		if m.routineIndex >= count {
			return
		}
		next, dir := m.routineIndex+1, "down"
		if key == "K" {
			next, dir = m.routineIndex-1, "up"
		}
		if next >= 0 && next < count && m.execute("routine-move", m.data.Order[m.routineIndex], dir) {
			m.routineIndex = next
		}
	}
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
	rows := m.data.rows(m.day)
	target := m.target()
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
		if !m.list && target != nil {
			for i, t := range rows {
				if t.ID == target.ID {
					m.selected = i
					break
				}
			}
		}
		m.list = !m.list
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
		m.selected += max(1, m.room()/2)
	case "ctrl+u", "pgup":
		m.list = true
		m.selected -= max(1, m.room()/2)
	case "g":
		m.list = true
		m.selected = 0
	case "G":
		m.list = true
		m.selected = len(rows) - 1
	case "J", "K":
		if target == nil || target.State != "pending" {
			m.notice = "Select a pending task."
			break
		}
		step, position := 1, "after"
		if key == "K" {
			step, position = -1, "before"
		}
		for j := m.selected + step; j >= 0 && j < len(rows); j += step {
			if rows[j].State == "pending" {
				day := m.day
				if m.executeDay(day, "move", target.ID, position, rows[j].ID) {
					m.selected = j
				}
				break
			}
		}
	case "s", "enter":
		if target == nil {
			m.notice = "No task selected."
			break
		}
		if target.State == "running" {
			m.execute("done")
		} else {
			m.execute("start", target.ID)
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
	case "v":
		m.mode = "routines"
		m.routineIndex = 0
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
			m.notice = "💼 work / 仕事   🏠 home / 家事   📚 study / 学習   ☕ rest / 休憩 | - clears"
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
	if m.splash && width >= 42 && m.height >= 15 {
		return startupMark
	}
	var b strings.Builder
	line := func(s string) { fmt.Fprintln(&b, runewidth.Truncate(terminalText(s, false), max(1, width), "…")) }
	line("  " + m.day + " | Alt+h/l: day")
	for i, s := range strings.Split(summary(m.data, m.day, m.now), "\n") {
		if i == 1 && strings.Contains(s, " | Day ends ") {
			next, finish, _ := strings.Cut(s, " | Day ends ")
			next = runewidth.Truncate(strings.TrimPrefix(next, "Next "), max(0, width-runewidth.StringWidth(finish)-20), "...")
			b.WriteString("  \x1b[2mNext\x1b[0m \x1b[35m" + next + "\x1b[0m | \x1b[1mDay ends " + finish + "\x1b[0m\n")
		} else {
			b.WriteString("  \x1b[36;1m" + runewidth.Truncate(s, max(1, width-2), "…") + "\x1b[0m\n")
		}
	}
	if t := m.data.running(); t != nil && len(t.Sessions) > 0 {
		line("  Started " + timeLabel(t.Sessions[0].Start, m.day, m.now.Location()) + " | Est. " + estimateLabel(t.Minutes))
		if bar := estimateProgress(t, m.now, width); bar != "" {
			fmt.Fprintln(&b, "  "+bar)
		}
	}
	line(strings.Repeat("-", min(width, 90)))
	rows := m.data.rows(m.day)
	switch m.mode {
	case "routines":
		line("  ROUTINES | j/k select | J/K move | Esc back")
		if len(m.data.Order) == 0 {
			line("  No routines yet. Select a task and press R.")
		}
		first := max(0, m.routineIndex-m.room()+1)
		for i, id := range m.data.Order {
			if i < first || i >= first+m.room() {
				continue
			}
			r := m.data.Routines[id]
			days := strings.Join(r.Days, ",")
			if len(r.Days) == 7 {
				days = "daily"
			}
			arrow := "  "
			if i == m.routineIndex {
				arrow = "> "
			}
			line(fmt.Sprintf("%s%-27s %6s  %s", arrow, days, estimateLabel(r.Minutes), r.Title))
		}
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
		line("  s start/finish | n note | l list | t timeline | a add | ? help | q quit")
		if m.list {
			titleWidth := max(8, min(60, width-56))
			line(fmt.Sprintf("  %-7s  %-*s     EST  %-11s  %-11s  ELAPSED", "STATE", titleWidth, "TASK", "START", "END"))
			for i := m.offset; i < min(len(rows), m.offset+m.room()); i++ {
				arrow := "  "
				if i == m.selected {
					arrow = "> "
				}
				text := arrow + row(rows[i], m.day, m.now, titleWidth)
				if i == m.selected {
					fmt.Fprintln(&b, "\x1b[30;46m"+ansi.Truncate(text, width, "…")+"\x1b[0m")
				} else {
					line(text)
				}
			}
			line("  j/k select | J/K move | [count]dd delete | U undo")
		}
	}
	if m.notice != "" {
		line("  " + m.notice)
	}
	return strings.TrimRight(b.String(), "\n")
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
