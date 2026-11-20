package main

import (
	"errors"
	"fmt"
	"time"
)

// load returns an independent snapshot. update commits the entire change atomically
// and discards it on error. A database implementation can use a transaction.
type stateStore interface {
	load() (*state, error)
	update(func(*state) error) (*state, error)
}

type application struct {
	store    stateStore
	sections []section
}

func (a application) execute(day, cmd string, args []string, now time.Time) (*state, error) {
	if cmd == "status" || cmd == "ls" {
		if len(args) != 0 {
			return nil, errors.New("unexpected arguments")
		}
		s, err := a.store.load()
		if err != nil {
			return nil, err
		}
		s.fill(day)
		return s, nil
	}
	switch cmd {
	case "section", "routine-section", "move-section", "routine-place":
		relative := cmd == "move-section" || cmd == "routine-place"
		if len(args) != 2 && !(relative && len(args) == 4) {
			usage := "ID SECTION|-"
			if relative {
				usage += " [before|after ID]"
			}
			return nil, fmt.Errorf("use %s %s", cmd, usage)
		}
		if args[1] != "-" {
			if _, ok := sectionByID(a.sections, args[1]); !ok {
				return nil, fmt.Errorf("unknown section %q", args[1])
			}
		}
	}
	s, err := a.store.update(func(s *state) error {
		if cmd != "routine-move" && cmd != "routine-section" && cmd != "routine-place" {
			s.fill(day)
		}
		return s.apply(day, cmd, args, now, a.sections)
	})
	if err != nil {
		return nil, err
	}
	s.fill(day)
	return s, nil
}
