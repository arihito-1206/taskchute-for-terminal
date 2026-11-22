package main

import (
	"errors"
	"slices"
	"time"
)

func (s *state) paste(day string, args []string, sections []section, now time.Time) error {
	index := slices.IndexFunc(s.Deletions, func(batch deletion) bool { return batch.Cut })
	if index < 0 {
		return errors.New("no tasks to paste")
	}
	batch := s.Deletions[index]
	var anchor *task
	sectionID := ""
	switch {
	case len(args) == 0:
	case len(args) == 2 && args[0] == "after":
		t, err := findTask(s.rows(day), args[1])
		if err != nil {
			return err
		}
		anchor = t
		sectionID = taskSection(t, sections, now)
	case len(args) == 2 && args[0] == "section":
		if _, ok := sectionByID(sections, args[1]); !ok {
			return errors.New("unknown section")
		}
		sectionID = args[1]
	default:
		return errInvalidCommand
	}
	if sec, ok := sectionByID(sections, sectionID); ok && sectionEnded(sec, day, now) {
		return errors.New("destination section has already ended")
	}
	byID := make(map[string]*task, len(batch.IDs))
	for _, t := range s.Tasks {
		if t.Deleted && t.Day == batch.Day {
			byID[t.ID] = t
		}
	}
	moving := make([]*task, 0, len(batch.IDs))
	for _, id := range batch.IDs {
		t := byID[id]
		if t == nil || t.State != "pending" || len(t.Sessions) > 0 {
			return errors.New("only unstarted tasks can be pasted; use U on the original day to restore")
		}
		moving = append(moving, t)
	}
	s.Tasks = slices.DeleteFunc(s.Tasks, func(t *task) bool { return t.Deleted && t.Day == batch.Day && slices.Contains(batch.IDs, t.ID) })
	at := len(s.Tasks)
	if anchor != nil {
		if anchor.State == "pending" {
			at = slices.Index(s.Tasks, anchor) + 1
		} else {
			at = 0
		}
	}
	for _, t := range moving {
		t.Day, t.SectionID, t.Deleted = day, sectionID, false
	}
	s.Tasks = slices.Insert(s.Tasks, at, moving...)
	s.Deletions = slices.Delete(s.Deletions, index, index+1)
	return nil
}
