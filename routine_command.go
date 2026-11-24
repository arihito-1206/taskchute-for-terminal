package main

import (
	"errors"
	"slices"
)

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
