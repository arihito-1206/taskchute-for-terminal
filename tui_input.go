package main

import (
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

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
			choice := matches[m.suggestion%len(matches)]
			m.input, m.addSource = choice.input, choice.source
			m.inputCursor = len(m.input)
			m.suggestion = 0
			return
		}
	}
	m.suggestion = 0
	switch key.String() {
	case "esc":
		m.fieldEdit = nil
		m.addSource = nil
		m.endEdit = endTimeEdit{}
		m.mode, m.notice = m.commandReturn, ""
		m.input, m.inputCursor, m.commandReturn = "", 0, ""
	case "ctrl+u":
		m.addSource = nil
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
		case "plan", "ls":
			target := m.target()
			m.planning = cmd == "plan"
			if m.planning {
				m.filter = ""
			}
			m.mode = ""
			m.showList("", target)
		case "status":
			m.mode, m.planning, m.list, m.sectionID, m.filter = "", false, false, "", ""
		case "help":
			m.mode, m.helpOffset = modeHelp, 0
		case "routines":
			m.mode = modeRoutines
			m.routineIndex, m.routineOffset = 0, 0
			m.scrollRoutines()
		default:
			executeCmd := cmd
			if source := m.addSuggestionSource(input); source != nil {
				executeCmd = "add-with-fields"
				args = append([]string{source.Category, source.WorkCode}, args...)
			}
			if !m.execute(executeCmd, args...) {
				return
			}
			m.mode = m.commandReturn
			if cmd == "add" && m.planning {
				// add appends the new task to the store.
				added := m.data.Tasks[len(m.data.Tasks)-1]
				if taskIndex(m.visibleRows(), added.ID) < 0 {
					m.filter, m.sectionID = "", ""
				}
				m.selectTask(added.ID)
				m.scroll()
			}
		}
		m.addSource = nil
		m.input, m.inputCursor, m.commandReturn = "", 0, ""
	default:
		if key.Text != "" {
			m.insertCommandText(key.Text)
		}
	}
}

func (m *terminal) openCommand(input string) {
	m.addSource = nil
	m.commandReturn, m.mode, m.input, m.notice = m.mode, modeCommand, input, ""
	m.inputCursor, m.suggestion = len(input), 0
}

func (m terminal) addSuggestionSource(input string) *task {
	source := m.addSource
	if source == nil || (source.Category == "" && source.WorkCode == "") {
		return nil
	}
	words := strings.Fields(input)
	if len(words) == 0 || words[0] != "add" {
		return nil
	}
	title, _, err := parseAdd(words[1:])
	if err != nil || title != strings.Join(strings.Fields(source.Title), " ") {
		return nil
	}
	return source
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
	if m.mode == modeSearch {
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
