package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type repository struct{ path string }

func decodeState(raw []byte) (*state, error) {
	s := &state{Routines: map[string]routine{}}
	if err := json.Unmarshal(raw, s); err != nil {
		return nil, err
	}
	if s.Schema != 1 || s.Tasks == nil {
		return nil, errors.New("unsupported data format")
	}
	for i, t := range s.Tasks {
		if t == nil || t.ID == "" {
			return nil, fmt.Errorf("invalid task at index %d", i)
		}
		if t.State == "running" && (len(t.Sessions) == 0 || t.Sessions[len(t.Sessions)-1].End != nil) {
			return nil, fmt.Errorf("running task at index %d has no open session", i)
		}
	}
	// Python stored routine order as the order of keys in the JSON object.
	if len(s.Order) == 0 && len(s.Routines) > 0 {
		var source struct{ Routines json.RawMessage }
		if err := json.Unmarshal(raw, &source); err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(source.Routines))
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			var value routine
			if err = dec.Decode(&value); err != nil {
				return nil, err
			}
			s.Order = append(s.Order, key.(string))
		}
	}
	if s.Routines == nil {
		s.Routines = map[string]routine{}
	}
	return s, nil
}

func (r repository) load(ctx context.Context) (*state, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return &state{Schema: 1, Tasks: []*task{}, Routines: map[string]routine{}}, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeState(raw)
}

func (r repository) save(s *state) error {
	// Replace the file a symlinked data path points to, and keep its mode.
	path, mode := r.path, os.FileMode(0600)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".taskchute-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	err = enc.Encode(s)
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	// Make the rename durable; a failure here does not undo the save.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		dir.Sync()
		dir.Close()
	}
	return nil
}

func (r repository) withLock(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(r.path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	return fn()
}

// update holds the file lock across the complete read-modify-write operation.
func (r repository) update(ctx context.Context, change func(*state) error) (*state, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var result *state
	err := r.withLock(func() error {
		s, err := r.load(ctx)
		if err != nil {
			return err
		}
		if err := change(s); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.save(s); err != nil {
			return err
		}
		result = s
		return nil
	})
	return result, err
}

func (r repository) importFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s, err := decodeState(raw)
	if err != nil {
		return err
	}
	return r.withLock(func() error {
		if _, err := os.Stat(r.path); !errors.Is(err, os.ErrNotExist) {
			return errors.New("destination already exists or is inaccessible; choose a new --data path")
		}
		return r.save(s)
	})
}
