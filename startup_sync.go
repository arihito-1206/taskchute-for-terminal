package main

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
)

type startupSyncResult struct{ err error }

func restoreOnStartup(parent context.Context, repo repository) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		err := repo.syncBackup(ctx, true, true)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return startupSyncResult{err: err}
	}
}

func (m *terminal) finishStartupSync(err error) {
	switch {
	case errors.Is(err, errNoBackupCheckout), errors.Is(err, errBackupCurrent):
		if m.notice == "Syncing…" {
			m.notice = ""
		}
		return
	case err != nil:
		m.notice = "Sync skipped: " + err.Error()
		return
	}
	// Read on the UI thread so a completed local edit or a day change cannot
	// be overwritten by a snapshot captured by the background command.
	selected := m.target()
	data, err := m.app.execute(context.Background(), m.day, "status", nil, m.now)
	if err != nil {
		m.notice = "Could not reload synced tasks: " + err.Error()
		return
	}
	m.data = data
	if m.list && selected != nil {
		m.selectTask(selected.ID)
	}
	m.scroll()
	if m.mode == modeRoutines {
		m.scrollRoutines()
	}
	if m.mode == modeNote && m.noteTask() != nil {
		m.layoutNote()
	}
	m.notice = "Synced from remote."
}
