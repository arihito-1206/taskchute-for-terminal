package main

import (
	"errors"
	"time"
)

// load returns an independent snapshot. update commits the entire change atomically
// and discards it on error. A database implementation can use a transaction.
type stateStore interface {
	load() (*state, error)
	update(func(*state) error) (*state, error)
}

type application struct{ store stateStore }

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
	s, err := a.store.update(func(s *state) error {
		if cmd != "routine-move" {
			s.fill(day)
		}
		return s.apply(day, cmd, args, now)
	})
	if err != nil {
		return nil, err
	}
	s.fill(day)
	return s, nil
}
