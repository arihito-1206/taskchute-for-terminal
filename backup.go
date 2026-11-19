package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// syncData uses a dedicated checkout. The app keeps writing the working copy
// while Git carries committed snapshots between machines.
func (r repository) syncData(restore bool) error {
	path, err := filepath.Abs(r.path)
	if err != nil {
		return err
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %s (%w)", args[0], strings.TrimSpace(string(out)), err)
		}
		return strings.TrimSuffix(string(out), "\n"), nil
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("use --data with tasks.json in a dedicated private Git checkout")
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil || top != dir || name != "tasks.json" {
		return errors.New("--data must point to tasks.json at the root of the backup checkout")
	}
	if info, err := os.Lstat(filepath.Join(dir, name)); err == nil && !info.Mode().IsRegular() {
		return errors.New("tasks.json must be a regular file")
	}
	upstream, err := git("rev-parse", "--symbolic-full-name", "@{upstream}")
	if err != nil || !strings.HasPrefix(upstream, "refs/remotes/origin/") {
		return errors.New("check out a branch tracking origin; initialize the private repository first")
	}
	branch := strings.TrimPrefix(upstream, "refs/remotes/origin/")
	if _, err = git("fetch", "--no-tags", "origin"); err != nil {
		return err
	}
	r.path = filepath.Join(dir, name)
	return r.withLock(func() error {
		changes, err := git("status", "--porcelain", "--", name)
		if err != nil {
			return err
		}
		if restore {
			if changes != "" {
				return errors.New("tasks.json has local changes; back it up before pulling")
			}
			// Validate the exact incoming commit before replacing local data.
			target, err := git("rev-parse", upstream+"^{commit}")
			if err != nil {
				return err
			}
			entry, err := git("ls-tree", target, "--", name)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(entry, "100644 blob ") && !strings.HasPrefix(entry, "100755 blob ") {
				return errors.New("backup must contain tasks.json as a regular file")
			}
			raw, err := git("show", target+":"+name)
			if err != nil {
				return err
			}
			if _, err := decodeState([]byte(raw)); err != nil {
				return fmt.Errorf("invalid backup: %w", err)
			}
			if _, err = git("merge", "--ff-only", target); err != nil {
				return err
			}
			fmt.Println("Backup pulled. Open TaskChute with the same --data path.")
			return nil
		}
		if _, err = git("merge-base", "--is-ancestor", upstream, "HEAD"); err != nil {
			return errors.New("remote has changes; update from it before backing up. If both PCs have edits, keep both copies and reconcile them first")
		}
		if _, err = os.Stat(r.path); err != nil {
			return fmt.Errorf("cannot back up missing data: %w", err)
		}
		if _, err = r.load(); err != nil {
			return fmt.Errorf("cannot back up invalid data: %w", err)
		}
		files, err := git("log", "--format=", "--name-only", upstream+"..HEAD")
		if err != nil {
			return err
		}
		for _, file := range strings.Split(files, "\n") {
			if file != "" && file != name {
				return errors.New("outgoing commits contain other files; use a dedicated backup checkout")
			}
		}
		if changes != "" {
			if _, err = git("add", "--", name); err != nil {
				return err
			}
			if _, err = git("commit", "--only", "-m", "Back up TaskChute data", "--", name); err != nil {
				return err
			}
		}
		if _, err = git("push", "origin", "HEAD:refs/heads/"+branch); err != nil {
			return err
		}
		fmt.Println("Backup pushed to origin/" + branch + ".")
		return nil
	})
}
