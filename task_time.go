package main

import (
	"errors"
	"strconv"
	"time"
)

func (s *state) setStart(t *task, args []string, now time.Time) error {
	if len(args) != 1 {
		return errInvalidCommand
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
		if s.running() != nil {
			return errors.New("finish the running task first")
		}
		t.State = "running"
		t.Sessions = append(t.Sessions, session{Start: start})
	}
	return nil
}

func (s *state) setEnd(t *task, args []string, now time.Time) error {
	if len(args) < 1 || len(args) > 2 {
		return errInvalidCommand
	}
	index := len(t.Sessions) - 1
	if len(args) == 2 {
		number, err := strconv.Atoi(args[1])
		if err != nil || number < 1 || number > len(t.Sessions) {
			return errors.New("execution interval not found")
		}
		index = number - 1
	}
	if index < 0 {
		return errors.New("task has no recorded execution intervals")
	}
	selected := &t.Sessions[index]
	if args[0] == "-" {
		if index != len(t.Sessions)-1 || t.State != "done" || selected.End == nil {
			return errors.New("only the last interval of a completed task can be resumed")
		}
		if t.Day > now.Format(dateLayout) {
			return errors.New("cannot resume a task scheduled for a future day")
		}
		if s.running() != nil {
			return errors.New("finish the running task first")
		}
		selected.End = nil
		t.State = "running"
		return nil
	}
	date := now
	if selected.End != nil {
		date = *selected.End
	}
	end, err := parseTime(date.In(now.Location()).Format(dateLayout), args[0], now)
	if err != nil {
		return err
	}
	if end.Before(selected.Start) || end.After(now) {
		return errors.New("end must be between this interval's start and now")
	}
	if index+1 < len(t.Sessions) && end.After(t.Sessions[index+1].Start) {
		return errors.New("end cannot be after the next interval's start")
	}
	if selected.End == nil && t.State == "running" && index == len(t.Sessions)-1 {
		t.State = "done"
	}
	selected.End = &end
	return nil
}
