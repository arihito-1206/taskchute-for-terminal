package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const validBackupData = `{"schema":1,"tasks":[]}`

func TestBackupRejectsUnrelatedFilesIntroducedByMerge(t *testing.T) {
	backup, git, remote := newBackupCheckout(t)
	writeBackupFixture(t, backup.path, validBackupData)
	base := git(remote, "rev-parse", "refs/heads/main")
	git("", "switch", "-c", "side")
	git("", "commit", "--allow-empty", "-m", "Side commit")
	git("", "switch", "main")
	git("", "commit", "--allow-empty", "-m", "Main commit")
	git("", "merge", "--no-commit", "side")
	if err := os.WriteFile(filepath.Join(filepath.Dir(backup.path), "extra.txt"), []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "extra.txt")
	git("", "commit", "-m", "Merge with unrelated file")

	err := backup.syncData(false)
	if err == nil || !strings.Contains(err.Error(), "outgoing commits contain other files") {
		t.Fatalf("expected unrelated outgoing file rejection: %v", err)
	}
	if got := git(remote, "rev-parse", "refs/heads/main"); got != base {
		t.Fatalf("remote advanced after rejection: %s", got)
	}
}

func TestBackupRejectsRenameFromUnrelatedFile(t *testing.T) {
	backup, git, remote := newBackupCheckout(t)
	writeBackupFixture(t, filepath.Join(filepath.Dir(backup.path), "extra.json"), validBackupData)
	git("", "add", "extra.json")
	git("", "commit", "-m", "Existing unrelated file")
	git("", "push", "origin", "main")
	base := git(remote, "rev-parse", "refs/heads/main")
	git("", "mv", "extra.json", "tasks.json")
	git("", "commit", "-m", "Rename unrelated file")

	err := backup.syncData(false)
	if err == nil || !strings.Contains(err.Error(), "outgoing commits contain other files") {
		t.Fatalf("expected rename source to count as an unrelated file: %v", err)
	}
	if got := git(remote, "rev-parse", "refs/heads/main"); got != base {
		t.Fatalf("remote advanced after rejection: %s", got)
	}
}

func TestBackupPushesUntrackedDataWithoutFollowingTags(t *testing.T) {
	backup, git, remote := newBackupCheckout(t)
	git("", "config", "status.showUntrackedFiles", "no")
	git("", "config", "push.followTags", "true")
	git("", "tag", "-a", "local-only", "-m", "Local metadata")
	writeBackupFixture(t, backup.path, validBackupData)
	if err := backup.syncData(false); err != nil {
		t.Fatal(err)
	}
	if got := git(remote, "show", "main:tasks.json"); got != validBackupData {
		t.Fatalf("remote tasks.json was not updated: %s", got)
	}
	if tags := git(remote, "tag", "--list"); tags != "" {
		t.Fatalf("backup pushed local tags: %s", tags)
	}
}

func TestRestoreRejectsUnrelatedFilesAndProtectsData(t *testing.T) {
	backup, git, remote := newBackupCheckout(t)
	sender := filepath.Join(t.TempDir(), "sender")
	git(filepath.Dir(sender), "clone", remote, sender)
	git(sender, "config", "user.name", "TaskChute Tests")
	git(sender, "config", "user.email", "tests@example.invalid")
	writeBackupFixture(t, filepath.Join(sender, "tasks.json"), validBackupData)
	git(sender, "add", "tasks.json")
	git(sender, "commit", "-m", "Backup data only")
	git(sender, "push", "origin", "main")
	if err := backup.syncData(true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(backup.path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("restored file permissions = %o, want 0600", got)
	}
	base := git("", "rev-parse", "HEAD")

	writeBackupFixture(t, filepath.Join(sender, "extra.txt"), "unrelated")
	git(sender, "add", "extra.txt")
	git(sender, "commit", "-m", "Unrelated file")
	git(sender, "push", "origin", "main")
	err = backup.syncData(true)
	if err == nil || !strings.Contains(err.Error(), "incoming commits contain other files") {
		t.Fatalf("expected rejection of unrelated incoming file: %v", err)
	}
	if got := git("", "rev-parse", "HEAD"); got != base {
		t.Fatalf("local checkout advanced after rejection: %s", got)
	}
}

func writeBackupFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func newBackupCheckout(t *testing.T) (repository, func(string, ...string) string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	root := t.TempDir()
	checkout, remote := filepath.Join(root, "checkout"), filepath.Join(root, "remote.git")
	git := func(dir string, args ...string) string {
		t.Helper()
		if dir == "" {
			dir = checkout
		}
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git(root, "init", "--bare", "--initial-branch=main", remote)
	git(root, "clone", remote, checkout)
	git("", "config", "user.name", "TaskChute Tests")
	git("", "config", "user.email", "tests@example.invalid")
	git("", "commit", "--allow-empty", "-m", "Initialize backup")
	git("", "push", "--set-upstream", "origin", "main")
	return repository{path: filepath.Join(checkout, "tasks.json")}, git, remote
}
