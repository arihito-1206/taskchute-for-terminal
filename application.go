package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Both methods return snapshots owned by the caller. update applies change to
// the current state and commits it atomically with respect to other updates.
// An error from change discards its mutations. Stores may rerun change when
// retrying a transaction, so the callback must have no external side effects.
type stateStore interface {
	load(context.Context) (*state, error)
	update(context.Context, func(*state) error) (*state, error)
}

type application struct {
	store    stateStore
	sections []section
}

func (a application) execute(ctx context.Context, day, cmd string, args []string, now time.Time) (*state, error) {
	if cmd == "status" || cmd == "ls" {
		if len(args) != 0 {
			return nil, errors.New("unexpected arguments")
		}
		s, err := a.store.load(ctx)
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
	s, err := a.store.update(ctx, func(s *state) error {
		if isRoutineCommand(cmd) {
			return s.applyRoutineCommand(cmd, args, a.sections)
		}
		s.fill(day)
		return s.apply(day, cmd, args, now, a.sections)
	})
	if err != nil {
		return nil, err
	}
	s.fill(day)
	return s, nil
}
