package main

import (
	"context"
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
	return r.syncBackup(context.Background(), restore, false)
}

var errNoBackupCheckout = errors.New("use --data with tasks.json in a dedicated private Git checkout")
var errBackupCurrent = errors.New("backup already up to date")

func (r repository) syncBackup(ctx context.Context, restore, automatic bool) error {
	path, err := filepath.Abs(r.path)
	if err != nil {
		return err
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if automatic && errors.Is(err, os.ErrNotExist) {
		return errNoBackupCheckout
	}
	if err != nil {
		return err
	}
	name := filepath.Base(path)
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		if automatic {
			cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		}
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %s (%w)", args[0], strings.TrimSpace(string(out)), err)
		}
		return strings.TrimSuffix(string(out), "\n"), nil
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return errNoBackupCheckout
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil || top != dir || name != "tasks.json" {
		if automatic {
			return errNoBackupCheckout
		}
		return errors.New("--data must point to tasks.json at the root of the backup checkout")
	}
	upstream, err := git("rev-parse", "--symbolic-full-name", "@{upstream}")
	if err != nil || !strings.HasPrefix(upstream, "refs/remotes/origin/") {
		if automatic {
			return errNoBackupCheckout
		}
		return errors.New("check out a branch tracking origin; initialize the private repository first")
	}
	branch := strings.TrimPrefix(upstream, "refs/remotes/origin/")
	if _, err = git("fetch", "--no-tags", "origin"); err != nil {
		return err
	}
	r.path = filepath.Join(dir, name)
	return r.withLock(func() error {
		info, err := os.Lstat(r.path)
		if err == nil && !info.Mode().IsRegular() {
			return errors.New("tasks.json must be a regular file")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		changes, err := git("status", "--porcelain", "--untracked-files=all", "--", name)
		if err != nil {
			return err
		}
		if restore {
			// Validate the exact incoming commit before replacing local data.
			target, err := git("rev-parse", upstream+"^{commit}")
			if err != nil {
				return err
			}
			if automatic {
				if _, err := git("merge-base", "--is-ancestor", target, "HEAD"); err == nil {
					return errBackupCurrent
				}
			}
			if changes != "" {
				return errors.New("tasks.json has local changes; reconcile them before restoring")
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
			if err := checkBackupPaths(git, "HEAD.."+target, name, "incoming"); err != nil {
				return err
			}
			if _, err = git("merge", "--ff-only", target); err != nil {
				return err
			}
			restored, err := os.Stat(r.path)
			if err != nil {
				return fmt.Errorf("inspect restored data: %w", err)
			}
			// Preserve Git's executable bit to keep older checkouts clean.
			if err := os.Chmod(r.path, 0600|restored.Mode().Perm()&0100); err != nil {
				return fmt.Errorf("protect restored data: %w", err)
			}
			if !automatic {
				fmt.Println("Backup pulled. Open TaskChute with the same --data path.")
			}
			return nil
		}
		if _, err = git("merge-base", "--is-ancestor", upstream, "HEAD"); err != nil {
			return errors.New("remote has changes; update from it before backing up. If both PCs have edits, keep both copies and reconcile them first")
		}
		if _, err = os.Stat(r.path); err != nil {
			return fmt.Errorf("cannot back up missing data: %w", err)
		}
		if _, err = r.load(context.Background()); err != nil {
			return fmt.Errorf("cannot back up invalid data: %w", err)
		}
		if err := checkBackupPaths(git, upstream+"..HEAD", name, "outgoing"); err != nil {
			return err
		}
		if changes != "" {
			if _, err = git("add", "--", name); err != nil {
				return err
			}
			if _, err = git("commit", "--only", "-m", "Back up TaskChute data", "--", name); err != nil {
				return err
			}
		}
		if _, err = git("push", "--no-follow-tags", "--recurse-submodules=no", "origin", "HEAD:refs/heads/"+branch); err != nil {
			return err
		}
		fmt.Println("Backup pushed to origin/" + branch + ".")
		return nil
	})
}

func checkBackupPaths(git func(...string) (string, error), revisionRange, dataPath, direction string) error {
	// Merge commits can introduce paths absent from both parents.
	// Disable rename detection so both the deleted and added paths are checked.
	files, err := git("log", "--format=", "--name-only", "--diff-merges=separate", "--no-renames", "-z", revisionRange)
	if err != nil {
		return err
	}
	for file := range strings.SplitSeq(files, "\x00") {
		if file != "" && file != dataPath {
			return fmt.Errorf("%s commits contain other files; use a dedicated backup checkout", direction)
		}
	}
	return nil
}
