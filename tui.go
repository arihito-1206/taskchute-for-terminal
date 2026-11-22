package main

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

type clockMsg time.Time

type uiMode string

const (
	modeCommand        uiMode = "command"
	modeSearch         uiMode = "search"
	modeRoutines       uiMode = "routines"
	modeSectionPicker  uiMode = "section-picker"
	modeIntervalPicker uiMode = "interval-picker"
	modeHelp           uiMode = "help"
	modeTimeline       uiMode = "timeline"
	modeNote           uiMode = "note"
)

type taskList = sectionList[*task]

type taskFieldEdit struct {
	field, day  string
	ids, titles []string
}

type terminal struct {
	dayEndTarget                                         string
	startupSync                                          tea.Cmd
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
	pinned, splash, list, planning, deleting             bool
	selected, offset, routineIndex, width, height, count int
	input, notice, noteID, noteDay                       string
	// mode is the screen that owns the keyboard; "" is the task list or focus view.
	// commandReturn is the mode to restore when command input closes.
	mode, commandReturn      uiMode
	filter                   string
	routineDeleteID, resetID string
	note                     noteEditor
}

// clock fires just after each whole second, the finest unit the screen shows.
func clock() tea.Cmd {
	return tea.Tick(untilNextSecond(time.Now()), func(t time.Time) tea.Msg { return clockMsg(t) })
}

func untilNextSecond(now time.Time) time.Duration {
	return time.Second - time.Duration(now.Nanosecond()) + 10*time.Millisecond
}

type splashDone struct{}

func (m terminal) Init() tea.Cmd {
	return tea.Batch(m.startupSync, clock(), tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return splashDone{} }))
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
	// Landing on today follows midnight again, exactly like Alt+t.
	m.data, m.day, m.pinned = s, day, pinned && day != m.now.Format(dateLayout)
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
	if m.mode == modeSearch {
		return strings.TrimSpace(m.input)
	}
	return m.filter
}

func (m terminal) taskList() taskList {
	return m.taskListFor(m.taskFilter(), m.planning)
}

func (m terminal) taskListFor(query string, remainingOnly bool) taskList {
	rows := m.data.rows(m.day)
	if remainingOnly {
		rows = slices.DeleteFunc(rows, func(t *task) bool { return t.State == "done" })
		if len(rows) == 0 {
			return taskList{}
		}
	}
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
	reserved := 12
	if m.mode == modeSearch || (m.mode == "" && m.list && m.filter != "") {
		reserved++
	}
	if m.sectionID != "" && m.mode != modeRoutines {
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
	case startupSyncResult:
		m.finishStartupSync(event.err)
	case tea.WindowSizeMsg:
		m.width, m.height = event.Width, event.Height
		m.scroll()
		if m.mode == modeRoutines {
			m.scrollRoutines()
		}
		if m.mode == modeNote {
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
		if m.mode == modeNote {
			return m.updateNote(msg)
		}
		if m.mode == modeCommand || m.mode == modeSearch {
			before := m.input
			m.insertCommandText(strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, event.Content))
			if m.mode == modeSearch && m.input != before {
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
		if m.mode == modeNote {
			return m.updateNote(event)
		}
		if m.mode == modeCommand {
			m.edit(event)
		} else if m.mode == modeSearch {
			m.editSearch(event)
		} else if m.mode == modeRoutines {
			m.routineKey(event.String())
		} else if m.mode == modeSectionPicker {
			m.sectionPickerKey(event.String())
		} else if m.mode == modeIntervalPicker {
			m.intervalPickerKey(event.String())
		} else {
			if m.normalKey(event.String()) {
				return m, tea.Quit
			}
			return m, nil
		}
		m.scroll()
	default:
		if m.mode == modeNote {
			return m.updateNote(msg)
		}
	}
	return m, nil
}
