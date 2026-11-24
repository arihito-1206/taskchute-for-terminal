package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

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
	if m.mode == modeNote {
		raw, noteCursor := m.renderNote(width)
		view := tea.NewView(raw)
		view.Cursor, view.AltScreen = noteCursor, true
		return view
	}
	raw := m.render(width)
	if m.mode == modeCommand || m.mode == modeSearch {
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

// dayProgress counts the whole displayed day, including tasks outside the current filter.
func (m terminal) dayProgress(width int, rows []*task) string {
	started, done, total := 0, 0, len(rows)
	for _, task := range rows {
		if task.State == "done" {
			done++
		}
		if task.State == "done" || task.State == "running" || task.legacyPaused() {
			started++
		}
	}

	count := fmt.Sprintf("%d / %d started", started, total)
	complete := total > 0 && done == total
	if complete {
		count += " 🎉"
	}
	label := "Today"
	if m.day != m.now.Format(dateLayout) {
		label = "Day"
	}
	prefix := "  " + label + "  "
	cells := min(10, width-ansi.StringWidth(prefix+count)-2)
	if cells < 4 || total == 0 {
		return ansi.Truncate(prefix+count, width, "…")
	}

	filled := started * cells / total
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("#55D9DC"))
	violet := lipgloss.NewStyle().Foreground(lipgloss.Color("#B38CFF"))
	gold := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD166"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#51586A"))
	var bar strings.Builder
	for i := range cells {
		if i >= filled {
			bar.WriteString(muted.Render("▱"))
			continue
		}
		style := cyan
		switch {
		case i*5 >= cells*4:
			style = gold
		case i*2 >= cells:
			style = violet
		}
		bar.WriteString(style.Render("▰"))
	}
	countStyle := lipgloss.NewStyle().Bold(true)
	if complete {
		countStyle = gold.Bold(true)
	}
	return prefix + bar.String() + "  " + countStyle.Render(count)
}

func dayEndDisplay(finish, target string, forecast output, location *time.Location) (string, string) {
	const defaultColor = "\x1b[1;35m"
	finish = "Day ends " + finish
	if target == "" || !forecast.Available {
		return finish, defaultColor
	}
	day, err := time.ParseInLocation(dateLayout, forecast.Day, location)
	if err != nil {
		return finish, defaultColor
	}
	minutes := sectionMinute(target)
	deadline := time.Date(day.Year(), day.Month(), day.Day(), minutes/60, minutes%60, 0, 0, location)
	finish += " / " + target
	over := forecast.Finish.Truncate(time.Minute).Sub(deadline)
	if over > 0 {
		return fmt.Sprintf("%s (+%dm)", finish, over/time.Minute), "\x1b[1;31m"
	}
	return finish, "\x1b[1;34m"
}

func (m terminal) render(width int) string {
	if m.mode == modeTimeline {
		return m.timelineView(width)
	}
	if m.mode == modeHelp {
		return m.helpView(width)
	}
	if m.mode == modeSectionPicker {
		return m.sectionPickerView(width)
	}
	if m.mode == modeIntervalPicker {
		return m.intervalPickerView(width)
	}
	if m.splash && width >= 42 && m.height >= 15 {
		return startupMark
	}
	var b strings.Builder
	line := func(s string) {
		fmt.Fprintln(&b, runewidth.Truncate(terminalText(s, false), max(1, width), "…"))
	}
	headerLine := func(s string) { fmt.Fprintln(&b, s) }
	viewLabel := ""
	if m.planning {
		viewLabel = "PLAN | "
	}
	line("  " + viewLabel + dateWithWeekday(m.day) + " | Alt+h/l: day | Alt+t: today")
	dayRows := m.data.rows(m.day)
	list := m.taskListFor(dayRows, m.taskFilter(), m.planning)
	rows := list.rows
	running := m.data.running()
	forecast := m.data.outputForRows(m.day, m.now, dayRows, running)
	forecast.Next = nil
	if next := firstPending(rows); next != nil {
		item := formatTask(next, m.now)
		forecast.Next = &item
	}
	summaryLines := strings.Split(renderSummary(forecast, m.now), "\n")
	if m.sectionID != "" && m.mode == "" {
		summaryLines[1], _, _ = strings.Cut(summaryLines[1], " | Day ends ")
	}
	for i, s := range summaryLines {
		if i == 0 && running != nil {
			fmt.Fprintln(&b, runningBand("  ▶ "+s, min(width, 90)))
			continue
		}
		if i == 1 && strings.Contains(s, " | Day ends ") {
			next, finish, _ := strings.Cut(s, " | Day ends ")
			finish, color := dayEndDisplay(finish, m.dayEndTarget, forecast, m.now.Location())
			remaining := width - runewidth.StringWidth("  Next  | "+finish)
			if remaining < 1 {
				b.WriteString("  " + color + runewidth.Truncate(finish, max(0, width-2), "…") + "\x1b[0m\n")
				continue
			}
			next = runewidth.Truncate(strings.TrimPrefix(next, "Next "), remaining, "...")
			b.WriteString("  \x1b[2mNext " + next + "\x1b[0m | " + color + finish + "\x1b[0m\n")
		} else {
			b.WriteString("  \x1b[36;1m" + runewidth.Truncate(s, max(1, width-2), "…") + "\x1b[0m\n")
		}
	}
	fmt.Fprintln(&b, m.dayProgress(width, dayRows))
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
	case modeRoutines:
		m.drawRoutines(line, headerLine, width)
	case modeCommand:
		categoryInput := false
		if edit := m.endEdit; edit.task != nil {
			line("  End time (HHMM) | " + m.intervalLabel(edit.task, edit.index))
		} else if edit := m.fieldEdit; edit != nil {
			categoryInput = edit.field == "category"
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
			categoryInput = len(words) > 0 && (words[0] == "category" || words[0] == "routine-category")
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
		if source := m.addSuggestionSource(m.input); source != nil {
			var fields []string
			if source.Category != "" {
				fields = append(fields, "Category "+source.Category)
			}
			if source.WorkCode != "" {
				fields = append(fields, "Code "+source.WorkCode)
			}
			line("  " + strings.Join(fields, " | "))
		}
		if categoryInput {
			line("  💼 work / 仕事 · 🏠 home / 家事 · 🎨 hobby / 趣味 · 📚 study / 学習")
			line("  ☕ rest / 休憩 · 🍖 meal / 食事 · 👪 family / 家族 | - clears")
		}
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
			if query != "" || m.planning {
				full := m.taskListFor(dayRows, "", false)
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
			visibleCapacity := func(s section, members []*task) string {
				if sectionEnded(s, m.day, m.now) {
					return ""
				}
				return capacity(s, members)
			}
			if s, ok := sectionByID(m.app.sections, m.sectionID); ok {
				for _, text := range sectionHeaderLines(s, visibleCapacity(s, rows), width) {
					headerLine(text)
				}
			}
			titleWidth := max(8, min(60, width-56))
			line(fmt.Sprintf("  %-7s  %-*s     EST  %-11s  %-11s  ELAPSED", "STATE", titleWidth, "TASK", "START", "END"))
			if m.mode == modeSearch {
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
				} else if m.planning {
					line("  No remaining tasks. Press a to add.")
				}
			}
			list.draw(m.offset, m.room(), width, headerLine, func(i int, task *task) {
				codeLabel := ""
				if task.WorkCode != "" {
					codeLabel = "🔖"
				}
				text := row(task, m.day, m.now, titleWidth, title(task), codeLabel)
				prefix := "  "
				if i == m.selected {
					prefix = "> "
				}
				doneForeground := ""
				switch {
				case m.sectionID != "" && task.State == "running":
					fmt.Fprintln(&b, runningBand(prefix+text, min(width, 90)))
				case task.State == "done":
					doneForeground = "\x1b[38;5;245"
					background := ""
					if m.sectionID != "" && i == m.selected {
						doneForeground = "\x1b[38;5;249"
						background = ";48;5;237"
					}
					fmt.Fprintln(&b, doneForeground+background+"m"+ansi.Truncate(prefix+text, width, "…")+"\x1b[0m")
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
						detail := "           └ " + strings.Join(details, " │ ")
						if doneForeground != "" {
							detail = runewidth.Truncate(terminalText(detail, false), max(1, width), "…")
							fmt.Fprintln(&b, doneForeground+"m"+detail+"\x1b[0m")
						} else {
							line(detail)
						}
					}
				}
			}, visibleCapacity)
			line("  j/k select | f focus | J/K move | b section | / filter | [count]dd delete | U undo")
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
