package main

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const dateLayout = "2006-01-02"

type session struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end"`
}

type note struct {
	Text string    `json:"text"`
	At   time.Time `json:"at,omitzero"`
}

func (n *note) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &n.Text)
	}
	type plain note
	return json.Unmarshal(data, (*plain)(n))
}

type task struct {
	ID        string    `json:"id"`
	Day       string    `json:"day"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Minutes   *float64  `json:"minutes"`
	Sessions  []session `json:"sessions"`
	Notes     []note    `json:"notes,omitempty"`
	WorkCode  string    `json:"work_code,omitempty"`
	Category  string    `json:"category,omitempty"`
	SectionID string    `json:"section_id,omitempty"`
	RoutineID string    `json:"routine_id,omitempty"`
	Deleted   bool      `json:"deleted,omitempty"`
}

func (t *task) legacyPaused() bool {
	return t.State == "pending" && len(t.Sessions) > 0
}

func (t *task) hasCompletedLog() bool {
	return t.State == "done" && slices.ContainsFunc(t.Sessions, func(interval session) bool {
		return !interval.Start.IsZero() && interval.End != nil && !interval.End.Before(interval.Start)
	})
}

type routine struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	WorkCode  string   `json:"work_code,omitempty"`
	Category  string   `json:"category,omitempty"`
	SectionID string   `json:"section_id,omitempty"`
	FirstDay  string   `json:"first_day"`
	Days      []string `json:"days"`
	Repeat    string   `json:"repeat,omitempty"`
	Minutes   *float64 `json:"minutes"`
}

type deletion struct {
	Day string   `json:"day"`
	IDs []string `json:"ids"`
}

type state struct {
	Schema    int                `json:"schema"`
	Tasks     []*task            `json:"tasks"`
	Routines  map[string]routine `json:"routines,omitempty"`
	Order     []string           `json:"routine_order,omitempty"`
	Deletions []deletion         `json:"deletions,omitempty"`
}

func parseWorkCode(cmd string, args []string) (string, error) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf("use %s ID CODE or %s ID -", cmd, cmd)
	}
	if strings.ContainsFunc(args[0], unicode.IsControl) {
		return "", errors.New("work code cannot contain control characters")
	}
	code := strings.TrimSpace(args[0])
	if code == "-" {
		return "", nil
	}
	return code, nil
}

func identifier() string {
	var bytes [16]byte
	rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}

func (s *state) fill(day string) {
	d, err := time.Parse(dateLayout, day)
	if err != nil || len(s.Order) == 0 {
		return
	}
	// Deleted occurrences still count so fill does not recreate them.
	existing := make(map[string]bool, len(s.Order))
	for _, t := range s.Tasks {
		if t.Day == day && t.RoutineID != "" {
			existing[t.RoutineID] = true
		}
	}
	for _, id := range s.Order {
		r, ok := s.Routines[id]
		if !ok || existing[id] || !r.occursOn(d) {
			continue
		}
		s.Tasks = append(s.Tasks, &task{
			ID: occurrenceID(id, day), Day: day, RoutineID: id,
			Title: r.Title, WorkCode: r.WorkCode, Category: r.Category,
			SectionID: r.SectionID, Minutes: r.Minutes,
			State: "pending", Sessions: []session{},
		})
		existing[id] = true
	}
}

func (s *state) rows(day string) []*task {
	rows := []*task{}
	for _, t := range s.Tasks {
		if t.Day == day && !t.Deleted {
			rows = append(rows, t)
		}
	}
	order := map[string]int{"done": 0, "running": 1, "pending": 2}
	slices.SortStableFunc(rows, func(a, b *task) int {
		if c := order[a.State] - order[b.State]; c != 0 {
			return c
		}
		if a.State == "done" {
			// Logs without an end time sort first so the order stays transitive.
			return lastEnd(a).Compare(lastEnd(b))
		}
		return 0
	})
	return rows
}

func lastEnd(t *task) time.Time {
	if len(t.Sessions) > 0 {
		if end := t.Sessions[len(t.Sessions)-1].End; end != nil {
			return *end
		}
	}
	return time.Time{}
}

func (s *state) running() *task {
	for _, t := range s.Tasks {
		if !t.Deleted && t.State == "running" {
			return t
		}
	}
	return nil
}

func elapsed(t *task, now time.Time) time.Duration {
	var total time.Duration
	for _, ss := range t.Sessions {
		end := now
		if ss.End != nil {
			end = *ss.End
		}
		total += max(0, end.Sub(ss.Start))
	}
	return total
}

func minutes(value string) (*float64, error) {
	if value == "-" {
		return nil, nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1440 {
		return nil, errors.New("estimate must be 0–1440 minutes or -")
	}
	return &n, nil
}

func parseTime(day, value string, now time.Time) (time.Time, error) {
	if len(value) != 4 {
		return time.Time{}, errors.New("enter four digits (HHMM)")
	}
	t, err := time.ParseInLocation(dateLayout+" 1504", day+" "+value, now.Location())
	if err != nil {
		return t, errors.New("enter a valid HHMM time")
	}
	return t, nil
}

var errInvalidCommand = errors.New("invalid command or arguments; use help")

func findTask(rows []*task, ref string) (*task, error) {
	var found *task
	for _, t := range rows {
		if ref != "" && strings.HasPrefix(t.ID, ref) {
			if found != nil {
				return nil, errors.New("task ID is ambiguous")
			}
			found = t
		}
	}
	if found == nil {
		return nil, errors.New("task not found on this day")
	}
	return found, nil
}

func (s *state) apply(day, cmd string, args []string, now time.Time, sections []section) error {
	switch cmd {
	case "batch-section":
		if len(args) < 2 {
			return errInvalidCommand
		}
		destination := args[0]
		if destination == "-" {
			destination = ""
		} else {
			section, ok := sectionByID(sections, destination)
			if !ok {
				return fmt.Errorf("unknown section %q", destination)
			}
			if sectionEnded(section, day, now) {
				return fmt.Errorf("section %q has already ended", section.Name)
			}
		}
		rows := s.rows(day)
		moving := make([]*task, 0, len(args)-1)
		selected := make(map[*task]bool, len(args)-1)
		for _, id := range args[1:] {
			t, err := findTask(rows, id)
			if err != nil {
				return err
			}
			if t.State != "pending" || selected[t] {
				return errors.New("choose distinct pending tasks")
			}
			selected[t] = true
			moving = append(moving, t)
		}
		for _, t := range moving {
			t.SectionID = destination
		}
		s.Tasks = append(slices.DeleteFunc(s.Tasks, func(t *task) bool { return selected[t] }), moving...)
	case "batch-code", "batch-category":
		if len(args) < 2 {
			return errInvalidCommand
		}
		field := strings.TrimPrefix(cmd, "batch-")
		rows := s.rows(day)
		for _, id := range args[1:] {
			t, err := findTask(rows, id)
			if err != nil {
				return err
			}
			if err := s.applyTaskCommand(t, rows, field, args[:1], now, sections); err != nil {
				return err
			}
		}
	case "add":
		if len(args) == 0 {
			return errInvalidCommand
		}
		var estimate *float64
		if len(args) > 1 {
			_, number := strconv.ParseFloat(args[0], 64)
			if number == nil || args[0] == "-" {
				var err error
				estimate, err = minutes(args[0])
				if err != nil {
					return err
				}
				args = args[1:]
			}
		}
		title := strings.TrimSpace(strings.Join(args, " "))
		if title == "" {
			return errInvalidCommand
		}
		s.Tasks = append(s.Tasks, &task{ID: identifier(), Day: day, Title: title, State: "pending", Minutes: estimate, Sessions: []session{}})
	case "start":
		if day > now.Format(dateLayout) {
			return errors.New("cannot start a task scheduled for a future day")
		}
		if len(args) > 1 {
			return errInvalidCommand
		}
		if s.running() != nil {
			return errors.New("finish the running task first")
		}
		rows := s.rows(day)
		var t *task
		if len(args) == 1 {
			var err error
			t, err = findTask(rows, args[0])
			if err != nil {
				return err
			}
		} else {
			for _, item := range rows {
				if item.State == "pending" {
					t = item
					break
				}
			}
		}
		if t != nil && (t.State == "done" || t.legacyPaused()) {
			// Keep execution history on the original task.
			t.State = "done"
			copy := *t
			copy.ID = identifier()
			copy.State = "pending"
			copy.Sessions = nil
			copy.Notes = nil
			t = &copy
			s.Tasks = append(s.Tasks, t)
		}
		if t == nil || t.State != "pending" {
			return errors.New("select a pending task")
		}
		t.State = "running"
		t.Sessions = append(t.Sessions, session{Start: now})
	case "done":
		if len(args) != 0 {
			return errInvalidCommand
		}
		running := s.running()
		if running == nil || len(running.Sessions) == 0 {
			return errors.New("no task is running")
		}
		running.Sessions[len(running.Sessions)-1].End = &now
		running.State = "done"
	case "delete":
		if len(args) == 0 {
			return errInvalidCommand
		}
		batch := deletion{Day: day}
		rows := s.rows(day)
		var targets []*task
		for _, id := range args {
			t, err := findTask(rows, id)
			if err != nil {
				return err
			}
			if t.State == "running" {
				return errors.New("selection includes a running task; nothing deleted")
			}
			if !slices.Contains(batch.IDs, t.ID) {
				batch.IDs = append(batch.IDs, t.ID)
				targets = append(targets, t)
			}
		}
		for _, t := range targets {
			t.Deleted = true
		}
		s.Deletions = append(s.Deletions, batch)
	case "undo-delete":
		if len(args) != 0 {
			return errInvalidCommand
		}
		for i := len(s.Deletions) - 1; i >= 0; i-- {
			batch := s.Deletions[i]
			if batch.Day != day {
				continue
			}
			for _, t := range s.Tasks {
				if slices.Contains(batch.IDs, t.ID) {
					t.Deleted = false
				}
			}
			s.Deletions = slices.Delete(s.Deletions, i, i+1)
			return nil
		}
		return errors.New("no deletion to undo for this day")
	default:
		if len(args) == 0 {
			return errInvalidCommand
		}
		rows := s.rows(day)
		t, err := findTask(rows, args[0])
		if err != nil {
			return err
		}
		args = args[1:]
		return s.applyTaskCommand(t, rows, cmd, args, now, sections)
	}
	return nil
}

func parseCategory(args []string) (string, error) {
	value := strings.TrimSpace(strings.Join(args, " "))
	switch value {
	case "":
		return "", errInvalidCommand
	case "-":
		return "", nil
	default:
		return value, nil
	}
}

func isRoutineCommand(cmd string) bool {
	switch cmd {
	case "routine-move", "routine-delete", "routine-code", "routine-category", "routine-section", "routine-place":
		return true
	default:
		return false
	}
}

func (s *state) applyRoutineCommand(cmd string, args []string, sections []section) error {
	switch cmd {
	case "routine-move":
		if len(args) != 2 || (args[1] != "up" && args[1] != "down") {
			return errInvalidCommand
		}
		i := slices.Index(s.Order, args[0])
		if i < 0 {
			return errors.New("routine not found")
		}
		j := i + 1
		if args[1] == "up" {
			j = i - 1
		}
		if j >= 0 && j < len(s.Order) {
			s.Order[i], s.Order[j] = s.Order[j], s.Order[i]
		}
	case "routine-delete":
		if len(args) != 1 {
			return errInvalidCommand
		}
		id := args[0]
		if _, ok := s.Routines[id]; !ok {
			return errors.New("routine not found")
		}
		delete(s.Routines, id)
		if i := slices.Index(s.Order, id); i >= 0 {
			s.Order = slices.Delete(s.Order, i, i+1)
		}
	case "routine-category":
		if len(args) < 2 {
			return errInvalidCommand
		}
		r, ok := s.Routines[args[0]]
		if !ok {
			return errors.New("routine not found")
		}
		category, err := parseCategory(args[1:])
		if err != nil {
			return err
		}
		r.Category = category
		s.Routines[args[0]] = r
	case "routine-code":
		if len(args) != 2 {
			return errInvalidCommand
		}
		r, ok := s.Routines[args[0]]
		if !ok {
			return errors.New("routine not found")
		}
		code, err := parseWorkCode(cmd, args[1:])
		if err != nil {
			return err
		}
		r.WorkCode = code
		s.Routines[args[0]] = r
	case "routine-section", "routine-place":
		if len(args) != 2 && (cmd != "routine-place" || len(args) != 4) {
			return errInvalidCommand
		}
		r, ok := s.Routines[args[0]]
		if !ok {
			return errors.New("routine not found")
		}
		destination := args[1]
		if destination == "-" {
			destination = ""
		}
		if len(args) == 4 {
			if args[2] != "before" && args[2] != "after" {
				return errInvalidCommand
			}
			anchor, ok := s.Routines[args[3]]
			i, j := slices.Index(s.Order, args[0]), slices.Index(s.Order, args[3])
			if !ok || i < 0 || j < 0 || i == j {
				return errors.New("choose two different routines in the list")
			}
			anchorSection, _ := sectionByID(sections, anchor.SectionID)
			if anchorSection.ID != destination {
				return errors.New("destination routine is no longer in that section")
			}
			s.Order = slices.Delete(s.Order, i, i+1)
			if i < j {
				j--
			}
			if args[2] == "after" {
				j++
			}
			s.Order = slices.Insert(s.Order, j, args[0])
			sourceSection, _ := sectionByID(sections, r.SectionID)
			if sourceSection.ID == destination {
				return nil
			}
		}
		r.SectionID = destination
		s.Routines[args[0]] = r
	}
	return nil
}

func (s *state) applyTaskCommand(t *task, rows []*task, cmd string, args []string, now time.Time, sections []section) error {
	switch cmd {
	case "edit-note", "delete-note":
		want := 4
		if cmd == "delete-note" {
			want = 3
		}
		if len(args) != want {
			return errInvalidCommand
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 || n > len(t.Notes) {
			return errors.New("comment not found")
		}
		entry := &t.Notes[n-1]
		if entry.At.Format(time.RFC3339Nano) != args[1] || entry.Text != args[2] {
			return errors.New("comment changed on disk; reopen the note and try again")
		}
		if cmd == "delete-note" {
			t.Notes = slices.Delete(t.Notes, n-1, n)
			return nil
		}
		if strings.TrimSpace(args[3]) == "" {
			return errors.New("comment cannot be empty")
		}
		entry.Text = args[3]
	case "category":
		category, err := parseCategory(args)
		if err != nil {
			return err
		}
		t.Category = category
	case "rename", "note":
		value := strings.TrimSpace(strings.Join(args, " "))
		if value == "" {
			return errInvalidCommand
		}
		switch cmd {
		case "rename":
			t.Title = value
		case "note":
			t.Notes = append(t.Notes, note{Text: value, At: now})
		}
	case "code":
		code, err := parseWorkCode(cmd, args)
		if err != nil {
			return err
		}
		t.WorkCode = code
	case "estimate":
		if len(args) != 1 {
			return errInvalidCommand
		}
		estimate, err := minutes(args[0])
		if err != nil {
			return err
		}
		t.Minutes = estimate
	case "section":
		if len(args) != 1 {
			return errInvalidCommand
		}
		destination := args[0]
		if destination == "-" {
			destination = ""
		}
		if err := checkSectionPlacement(t, destination, sections, now); err != nil {
			return err
		}
		t.SectionID = destination
	case "reset":
		if len(args) != 0 {
			return errInvalidCommand
		}
		t.State = "pending"
		t.Sessions = []session{}
	case "set-start":
		return s.setStart(t, args, now)
	case "set-end":
		return s.setEnd(t, args, now)
	case "move", "move-section":
		destination := t.SectionID
		if cmd == "move-section" {
			if len(args) != 1 && len(args) != 3 {
				return errInvalidCommand
			}
			if t.State != "pending" {
				return errors.New("choose a pending task")
			}
			destination = args[0]
			if destination == "-" {
				destination = ""
			}
			if err := checkSectionPlacement(t, destination, sections, now); err != nil {
				return err
			}
			args = args[1:]
			if len(args) == 0 {
				t.SectionID = destination
				return nil
			}
		}
		if len(args) != 2 || (args[0] != "before" && args[0] != "after") {
			return errInvalidCommand
		}
		other, err := findTask(rows, args[1])
		if err != nil {
			return err
		}
		if t == other || t.State != "pending" || other.State != "pending" {
			return errors.New("choose two different pending tasks")
		}
		if cmd == "move-section" && taskSection(other, sections, now) != destination {
			return errors.New("destination task is no longer in that section")
		}
		t.SectionID = destination
		i := slices.Index(s.Tasks, t)
		s.Tasks = slices.Delete(s.Tasks, i, i+1)
		j := slices.Index(s.Tasks, other)
		if args[0] == "after" {
			j++
		}
		s.Tasks = slices.Insert(s.Tasks, j, t)
	case "routine":
		if len(args) < 1 || len(args) > 2 {
			return errInvalidCommand
		}
		r, exists := s.Routines[t.RoutineID]
		if !exists {
			if !t.hasCompletedLog() {
				return errors.New("create a routine from a completed task with recorded time")
			}
			r = routine{ID: identifier(), Title: t.Title, WorkCode: t.WorkCode, Category: t.Category, SectionID: t.SectionID, FirstDay: t.Day, Minutes: t.Minutes}
		}
		if err := r.setRepeat(args[0]); err != nil {
			return err
		}
		if r.Repeat != "" {
			if _, err := time.Parse(dateLayout, r.FirstDay); err != nil {
				r.FirstDay = t.Day
			}
		}
		if len(args) == 2 {
			estimate, err := minutes(args[1])
			if err != nil {
				return err
			}
			r.Minutes = estimate
		}
		if !exists {
			s.Order = append(s.Order, r.ID)
		}
		s.Routines[r.ID] = r
		t.RoutineID = r.ID
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}

// Stable UUIDv5-compatible identifier shared with the Python prototype.
func occurrenceID(id, day string) string {
	namespace, err := hex.DecodeString(id)
	if err != nil || len(namespace) != 16 {
		namespace = []byte(id)
	}
	h := sha1.New()
	h.Write(namespace)
	io.WriteString(h, day)
	sum := h.Sum(nil)[:16]
	sum[6] = (sum[6] & 15) | 0x50
	sum[8] = (sum[8] & 63) | 0x80
	return hex.EncodeToString(sum)
}
