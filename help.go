package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type commandGuide struct{ syntax, hint string }

var commandGuides = []commandGuide{
	{"add [MIN] TITLE", "add Read  /  add 15 Read"},
	{"start [ID]", "Start next or by ID. Switches from running work if needed."},
	{"interrupt", "Queue remaining work and start an untitled task."},
	{"done", "Finish the running task."},
	{"plan", "Show remaining tasks. Esc returns to the full list."},
	{"ls", "Show today's task list."},
	{"status", "Show the focus view."},
	{"rename @ TITLE", "rename @ Read a chapter"},
	{"estimate @ MIN|-", "estimate @ 15  /  - clears"},
	{"category @ NAME|-", "work / home / hobby / rest / meal / family / - clears"},
	{"code [@|ID] CODE|-", "Use code @ HubSpot Design for spaces / - clears"},
	{"note @ TEXT", "note @ Finished the first draft"},
	{"set-start @ HHMM", "set-start @ 0930"},
	{"set-end @ HHMM|- [INTERVAL]", "Edit an interval end (1-based, default last) / - resumes the last"},
	{"reset @", "Clear recorded times and return to Pending."},
	{"move @ before|after ID", "Reorder a pending task."},
	{"routine @ REPEAT [MIN]", "daily/2 | weekday | holiday | weekly:1:tue | mon/2 | monthly:1 | monthly:2:tue"},
	{"routine-move ID up|down", "Change the routine order."},
	{"routine-delete ID", "Stop generating tasks from the routine and keep saved tasks."},
	{"section @ SECTION|-", "Assign a task to a section / - clears."},
	{"routine-section ID SECTION|-", "Set the default section for a routine / - clears."},
	{"routine-category ID NAME|-", "Set the default category for a routine / - clears."},
	{"routine-code ID CODE|-", "Set the default work code for a routine / - clears."},
	{"delete ID...", "Delete tasks together."},
	{"paste [after ID|section ID]", "Move the cut tasks to this day."},
	{"undo-delete", "Restore the latest deletion."},
	{"routines", "Open the routine list."},
	{"help", "Open keyboard and command help."},
}

func commandUsage(input string) string {
	words := strings.Fields(input)
	if len(words) == 0 {
		return "Type a command, e.g. add 15 Read"
	}
	for _, guide := range commandGuides {
		name, _, _ := strings.Cut(guide.syntax, " ")
		if name == words[0] {
			return guide.syntax + "    " + guide.hint
		}
	}
	return "Type help for commands"
}

var helpLines = func() []string {
	lines := []string{
		"TASKS",
		"  s / Enter   Start at the last end today, or finish now", "  Alt+s       Start now", "              Starting another task queues remaining work", "              On Done, start a copy", "  I           Interrupt with an untitled task", "  a           Add a task", "  n           Open task notes",
		"", "LIST", "  l           Toggle task list", "  t           Open timeline", "  j / k       Select next / previous", "  J / K       Move a pending task across rows or sections", "  g / G       First / last task",
		"  f           Jump to the running task or first unstarted task in the current section",
		"  /           Filter tasks by title or work code", "              Enter applies, Esc cancels or clears", "  dd / 3dd    Cut one / three tasks", "  p           Paste pending tasks after selection", "  U           Undo latest deletion",
		"  L           Toggle current section (Esc to close)", "  b           Choose a section for the task or routine",
		"  v then J/K  Reorder routines or move them across sections",
		"  v then c    Set the default work code for a routine",
		"  v then dd   Delete the selected routine",
		"  R           Create from a completed log or edit a repeat rule",
		"              weekly:1:tue, daily/2, monthly:2:tue",
		"", "EDIT", "  r           Rename", "  e           Edit estimate", "  S / F       Edit start / end time (F picks an interval)", "  [count]c    Set category", "  [count]C    Set work code in task list", "  u           Reset to Pending (press twice if time is recorded)",
		"", "NAVIGATION", "  R / v       Set routine / view routines", "  Alt+h/l     Previous / next day", "  Alt+t       Return to today", "  : or i      Enter a command", "  q           Quit",
		"", "COMMANDS   @ = selected task",
		"  Home/End    Jump to start/end of input",
	}
	for _, guide := range commandGuides {
		lines = append(lines, "  "+guide.syntax)
	}
	return lines
}()

func (m terminal) helpRows() int {
	height := m.height
	if height == 0 {
		height = 24
	}
	return max(1, height-2)
}

func (m terminal) helpView(width int) string {
	rows := m.helpRows()
	first := min(m.helpOffset, max(0, len(helpLines)-rows))
	lines := []string{fmt.Sprintf("HELP   %d-%d / %d", first+1, min(first+rows, len(helpLines)), len(helpLines))}
	lines = append(lines, helpLines[first:min(first+rows, len(helpLines))]...)
	lines = append(lines, "j/k scroll   Esc close")
	for i, line := range lines {
		line = ansi.Truncate(line, width, "…")
		if i == 0 || (i < len(lines)-1 && line != "" && !strings.HasPrefix(line, " ")) {
			line = "\x1b[1;36m" + line + "\x1b[0m"
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
