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

type routine struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Category  string   `json:"category,omitempty"`
	SectionID string   `json:"section_id,omitempty"`
	FirstDay  string   `json:"first_day"`
	Days      []string `json:"days"`
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

func identifier() string {
	var bytes [16]byte
	rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}

func (s *state) fill(day string) {
	d, _ := time.Parse(dateLayout, day)
	weekday := strings.ToLower(d.Weekday().String()[:3])
	for _, id := range s.Order {
		r := s.Routines[id]
		if day < r.FirstDay || !slices.Contains(r.Days, weekday) {
			continue
		}
		found := slices.ContainsFunc(s.Tasks, func(t *task) bool { return t.Day == day && t.RoutineID == id })
		if !found {
			s.Tasks = append(s.Tasks, &task{ID: occurrenceID(id, day), Day: day, RoutineID: id, Title: r.Title, Category: r.Category, SectionID: r.SectionID, Minutes: r.Minutes, State: "pending", Sessions: []session{}})
		}
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
		if a.State == "done" && b.State == "done" && len(a.Sessions) > 0 && len(b.Sessions) > 0 {
			x, y := a.Sessions[len(a.Sessions)-1].End, b.Sessions[len(b.Sessions)-1].End
			if x != nil && y != nil {
				return x.Compare(*y)
			}
		}
		return order[a.State] - order[b.State]
	})
	return rows
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

func (s *state) apply(day, cmd string, args []string, now time.Time, sections []section) error {
	rows := s.rows(day)
	running := s.running()
	find := func(ref string) (*task, error) {
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
	invalid := errors.New("invalid command or arguments; use help")
	switch cmd {
	case "add":
		if len(args) == 0 {
			return invalid
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
			return invalid
		}
		s.Tasks = append(s.Tasks, &task{ID: identifier(), Day: day, Title: title, State: "pending", Minutes: estimate, Sessions: []session{}})
	case "start":
		if day > now.Format(dateLayout) {
			return errors.New("cannot start a task scheduled for a future day")
		}
		if len(args) > 1 {
			return invalid
		}
		if running != nil {
			return errors.New("finish or pause the running task first")
		}
		var t *task
		if len(args) == 1 {
			var err error
			t, err = find(args[0])
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
		if t != nil && t.State == "done" {
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
	case "done", "pause":
		if len(args) != 0 {
			return invalid
		}
		if running == nil || len(running.Sessions) == 0 {
			return errors.New("no task is running")
		}
		running.Sessions[len(running.Sessions)-1].End = &now
		running.State = "pending"
		if cmd == "done" {
			running.State = "done"
		}
	case "delete":
		if len(args) == 0 {
			return invalid
		}
		batch := deletion{Day: day}
		var targets []*task
		for _, id := range args {
			t, err := find(id)
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
			return invalid
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
	case "routine-move":
		if len(args) != 2 || (args[1] != "up" && args[1] != "down") {
			return invalid
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
	case "routine-section", "routine-place":
		if len(args) != 2 && (cmd != "routine-place" || len(args) != 4) {
			return invalid
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
				return invalid
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
	default:
		if len(args) == 0 {
			return invalid
		}
		t, err := find(args[0])
		if err != nil {
			return err
		}
		args = args[1:]
		switch cmd {
		case "edit-note", "delete-note":
			want := 4
			if cmd == "delete-note" {
				want = 3
			}
			if len(args) != want {
				return invalid
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
		case "rename", "category", "note":
			value := strings.TrimSpace(strings.Join(args, " "))
			if value == "" {
				return invalid
			}
			switch cmd {
			case "rename":
				t.Title = value
			case "category":
				if value == "-" {
					value = ""
				}
				t.Category = value
			case "note":
				t.Notes = append(t.Notes, note{Text: value, At: now})
			}
		case "code":
			if len(args) != 1 || args[0] == "" || strings.ContainsFunc(args[0], func(r rune) bool {
				return unicode.IsSpace(r) || unicode.IsControl(r)
			}) {
				return errors.New("use code ID CODE or code ID -")
			}
			t.WorkCode = args[0]
			if t.WorkCode == "-" {
				t.WorkCode = ""
			}
		case "estimate":
			if len(args) != 1 {
				return invalid
			}
			t.Minutes, err = minutes(args[0])
			return err
		case "section":
			if len(args) != 1 {
				return invalid
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
				return invalid
			}
			t.State = "pending"
			t.Sessions = []session{}
		case "set-start":
			if len(args) != 1 {
				return invalid
			}
			start, err := parseTime(t.Day, args[0], now)
			if err != nil {
				return err
			}
			if start.After(now) {
				return errors.New("start cannot be in the future")
			}
			if len(t.Sessions) > 0 {
				old := &t.Sessions[0]
				if old.End != nil && start.After(*old.End) {
					return errors.New("start cannot be after end")
				}
				old.Start = start
			} else {
				if running != nil {
					return errors.New("finish or pause the running task first")
				}
				t.State = "running"
				t.Sessions = append(t.Sessions, session{Start: start})
			}
		case "set-end":
			if len(args) != 1 {
				return invalid
			}
			if t.State != "done" || len(t.Sessions) == 0 {
				return errors.New("select a completed task")
			}
			last := &t.Sessions[len(t.Sessions)-1]
			if last.End == nil {
				return errors.New("task has no end time")
			}
			if args[0] == "-" {
				if t.Day > now.Format(dateLayout) {
					return errors.New("cannot resume a task scheduled for a future day")
				}
				if running != nil {
					return errors.New("finish or pause the running task first")
				}
				last.End = nil
				t.State = "running"
				return nil
			}
			end, err := parseTime(last.End.In(now.Location()).Format(dateLayout), args[0], now)
			if err != nil {
				return err
			}
			if end.Before(last.Start) || end.After(now) {
				return errors.New("end must be between start and now")
			}
			last.End = &end
		case "move", "move-section":
			destination := t.SectionID
			if cmd == "move-section" {
				if len(args) != 1 && len(args) != 3 {
					return invalid
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
				return invalid
			}
			other, err := find(args[1])
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
				return invalid
			}
			weekdays := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
			days := strings.Split(strings.ToLower(args[0]), ",")
			if strings.EqualFold(args[0], "daily") {
				days = weekdays
			}
			for _, d := range days {
				if !slices.Contains(weekdays, d) {
					return errors.New("use daily or mon,wed,fri")
				}
			}
			r, exists := s.Routines[t.RoutineID]
			if !exists {
				r = routine{ID: identifier(), Title: t.Title, Category: t.Category, SectionID: t.SectionID, FirstDay: day, Minutes: t.Minutes}
			}
			r.Days = days
			if len(args) == 2 {
				r.Minutes, err = minutes(args[1])
				if err != nil {
					return err
				}
			}
			if !exists {
				s.Order = append(s.Order, r.ID)
			}
			s.Routines[r.ID] = r
			t.RoutineID = r.ID
		default:
			return fmt.Errorf("unknown command %q", cmd)
		}
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
