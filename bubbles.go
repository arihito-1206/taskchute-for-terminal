package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type noteEditor struct {
	draft                                   textarea.Model
	history                                 viewport.Model
	selected, editing                       int
	original                                note
	historyFocus, panePrefix, deletePending bool
}

func (m *terminal) openNote() {
	draft := textarea.New()
	draft.Prompt = ""
	draft.ShowLineNumbers = false
	draft.CharLimit = 0
	draft.MaxWidth = 0
	draft.SetVirtualCursor(false)
	draft.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+n", "alt+enter"))
	draft.Focus()
	m.note = noteEditor{draft: draft, history: viewport.New(), selected: -1, editing: -1}
	if t := m.noteTask(); t != nil {
		m.note.selected = len(t.Notes) - 1
	}
	m.layoutNote()
	m.note.history.GotoBottom()
}

func (m *terminal) layoutNote() {
	height := m.height
	if height == 0 {
		height = 24
	}
	width := max(1, m.notePanelWidth()-4)
	// The panel starts below the timer. Split its available space between
	// the independent history viewport and the editor.
	rows := max(2, height-9)
	draftHeight := min(4, max(1, rows/2))
	historyHeight := min(8, max(1, rows-draftHeight))
	m.note.draft.SetWidth(width)
	m.note.draft.SetHeight(draftHeight)
	atBottom := m.note.history.AtBottom()
	m.note.history.SetWidth(width)
	m.note.history.SetHeight(historyHeight)
	content := "No notes yet"
	if t := m.noteTask(); t != nil && len(t.Notes) > 0 {
		lines := make([]string, 0, len(t.Notes))
		for i, entry := range t.Notes {
			stamp := ""
			if !entry.At.IsZero() {
				stamp = "[" + entry.At.In(m.now.Location()).Format("15:04") + "] "
			}
			prefix := "  "
			if m.note.historyFocus && i == m.note.selected {
				prefix = "> "
			}
			lines = append(lines, ansi.Hardwrap(prefix+stamp+terminalText(entry.Text, true), width, true))
		}
		content = strings.Join(lines, "\n")
	}
	m.note.history.SetContent(content)
	if atBottom {
		m.note.history.GotoBottom()
	}
}

func (m *terminal) focusNoteHistory(history bool) tea.Cmd {
	m.note.historyFocus = history
	if history {
		m.note.draft.Blur()
		m.revealNote()
		return nil
	}
	m.layoutNote()
	return m.note.draft.Focus()
}

func (m *terminal) revealNote() {
	m.layoutNote()
	t := m.noteTask()
	if t == nil || m.note.selected < 0 || m.note.selected >= len(t.Notes) {
		return
	}
	start := 0
	width := max(1, m.notePanelWidth()-4)
	for _, entry := range t.Notes[:m.note.selected] {
		text := "  " + entry.Text
		if !entry.At.IsZero() {
			text = "  [00:00] " + entry.Text
		}
		start += 1 + strings.Count(ansi.Hardwrap(text, width, true), "\n")
	}
	m.note.history.SetYOffset(start)
}

func (m terminal) updateNote(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		name := k.String()
		if m.note.deletePending {
			m.note.deletePending = false
			m.notice = ""
			if name == "esc" {
				return m, nil
			}
			if name == "d" {
				t := m.noteTask()
				index := m.note.selected
				if t != nil && index >= 0 && index < len(t.Notes) {
					entry := t.Notes[index]
					if m.executeDay(m.noteDay, "delete-note", m.noteID, strconv.Itoa(index+1), entry.At.Format(time.RFC3339Nano), entry.Text) {
						m.note.selected = min(index, len(t.Notes)-2)
						m.revealNote()
						m.notice = "Comment deleted."
					}
				}
				return m, nil
			}
		}
		if name == "esc" {
			m.note.draft.Blur()
			m.mode, m.notice = "", ""
			return m, nil
		}
		if m.note.panePrefix {
			m.note.panePrefix = false
			m.notice = ""
			var cmd tea.Cmd
			switch name {
			case "h", "k":
				cmd = m.focusNoteHistory(true)
			case "l", "j":
				cmd = m.focusNoteHistory(false)
			case "w", "ctrl+w":
				cmd = m.focusNoteHistory(!m.note.historyFocus)
			}
			return m, cmd
		}
		if name == "ctrl+w" {
			m.note.panePrefix = true
			m.notice = "Pane: h/k history | l/j editor | w switch"
			return m, nil
		}
		if m.note.historyFocus {
			t := m.noteTask()
			switch name {
			case "j", "k":
				if t != nil && len(t.Notes) > 0 {
					step := 1
					if name == "k" {
						step = -1
					}
					m.note.selected = max(0, min(len(t.Notes)-1, m.note.selected+step))
					m.revealNote()
				}
			case "up", "down", "pgup", "pgdown":
				var cmd tea.Cmd
				m.note.history, cmd = m.note.history.Update(msg)
				return m, cmd
			case "d":
				if m.note.editing >= 0 {
					m.notice = "Finish editing before deleting another comment."
				} else if t != nil && m.note.selected >= 0 && m.note.selected < len(t.Notes) {
					m.note.deletePending = true
					m.notice = "Press d again to delete this comment | Esc cancels"
				}
			case "e", "enter":
				if t != nil && m.note.selected >= 0 && m.note.selected < len(t.Notes) {
					if strings.TrimSpace(m.note.draft.Value()) != "" {
						m.notice = "Save the current draft before editing another comment."
						return m, nil
					}
					m.note.editing = m.note.selected
					m.note.original = t.Notes[m.note.selected]
					m.note.draft.SetValue(terminalText(m.note.original.Text, true))
					cmd := m.focusNoteHistory(false)
					return m, cmd
				}
			}
			return m, nil
		}
		if name == "enter" {
			value := strings.TrimSpace(m.note.draft.Value())
			if value == "" {
				return m, nil
			}
			saved := false
			if m.note.editing >= 0 {
				original := m.note.original
				saved = m.executeDay(m.noteDay, "edit-note", m.noteID, strconv.Itoa(m.note.editing+1), original.At.Format(time.RFC3339Nano), original.Text, value)
			} else {
				saved = m.executeDay(m.noteDay, "note", m.noteID, value)
			}
			if saved {
				m.note.draft.Reset()
				m.note.editing = -1
				if t := m.noteTask(); t != nil {
					m.note.selected = len(t.Notes) - 1
				}
				m.layoutNote()
				m.note.history.GotoBottom()
			}
			return m, nil
		}
		switch name {
		case "alt+k", "pgup", "˚":
			m.note.history.ScrollUp(1)
			return m, nil
		case "alt+j", "pgdown", "∆":
			m.note.history.ScrollDown(1)
			return m, nil
		}
	}
	if m.note.historyFocus {
		return m, nil
	}
	var cmd tea.Cmd
	m.note.draft, cmd = m.note.draft.Update(msg)
	return m, cmd
}

func estimateProgress(t *task, now time.Time, width int) string {
	if t.Minutes == nil || width < 24 {
		return ""
	}
	estimate := time.Duration(*t.Minutes * float64(time.Minute))
	used := elapsed(t, now)
	label := ""
	ratio := 1.0
	if estimate > 0 {
		ratio = min(1, float64(used)/float64(estimate))
	}
	if used > estimate {
		over := used - estimate
		if over < time.Minute {
			label = fmt.Sprintf("+%ds", int(over.Seconds()))
		} else {
			label = fmt.Sprintf("+%dm", int(over.Minutes()))
		}
	} else {
		label = fmt.Sprintf("%.0f%%", ratio*100)
	}
	bar := progress.New(
		progress.WithWidth(min(22, width-ansi.StringWidth(label)-4)),
		progress.WithoutPercentage(),
		progress.WithFillCharacters('━', '─'),
		progress.WithColors(lipgloss.Color("#5ABFAE")),
	)
	if used > estimate {
		bar = progress.New(progress.WithWidth(min(22, width-ansi.StringWidth(label)-4)),
			progress.WithoutPercentage(), progress.WithFillCharacters('━', '─'),
			progress.WithColors(lipgloss.Color("#E7A768")))
	}
	return bar.ViewAs(ratio) + " " + label
}
