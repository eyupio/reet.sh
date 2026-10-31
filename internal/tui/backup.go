package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/store"
)

type backupOutcome struct {
	items   []backup.Item
	message string
	listSet bool
}

type newBackupValues struct {
	Confirm bool
}

type restoreFormValues struct {
	Password string
	Confirm  bool
}

func buildNewBackupForm(values *newBackupValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Take an instance backup now?").
				Description("A sealed copy is written beside the database and shipped to any configured destination.").
				Affirmative("Take").
				Negative("Cancel").
				Value(&values.Confirm),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func buildRestoreForm(id string, values *restoreFormValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Password").
				Description("Only if this copy is protected with one.").
				EchoMode(huh.EchoModePassword).
				Value(&values.Password),
			huh.NewConfirm().
				Title("Restore "+id+"?").
				Description("This replaces the live database. Stop reet first. The current database is kept beside it.").
				Affirmative("Restore").
				Negative("Cancel").
				Value(&values.Confirm),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func backupDataDir(opts Options) string {
	if opts.BackupDataDir != "" {
		return opts.BackupDataDir
	}
	return envOr("REET_DATA_DIR", "./data")
}

func openBackupService(opts Options) (*store.Store, *backup.Service, error) {
	dataDir := backupDataDir(opts)
	st, err := store.Open(dataDir)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := backup.FromEnv(func(key string) string {
		if key == "REET_DATA_DIR" {
			return dataDir
		}
		return os.Getenv(key)
	})
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	return st, backup.New(st, cfg), nil
}

func loadBackups(opts Options) (backupOutcome, error) {
	st, svc, err := openBackupService(opts)
	if err != nil {
		return backupOutcome{}, err
	}
	defer st.Close()
	items, err := svc.LocalItems(0)
	if err != nil {
		return backupOutcome{}, err
	}
	return backupOutcome{items: items, listSet: true}, nil
}

func takeBackup(opts Options) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	st, svc, err := openBackupService(opts)
	if err != nil {
		return resultMsg{page: ScreenBackup, err: err}
	}
	item, err := svc.RunInstance(ctx, "tui")
	st.Close()
	if err != nil {
		return resultMsg{page: ScreenBackup, err: err}
	}
	outcome := backupOutcome{message: fmt.Sprintf("Took %s (%s).", item.ID, humanBytes(item.Bytes))}
	if list, listErr := loadBackups(opts); listErr == nil {
		outcome.items, outcome.listSet = list.items, true
	} else {
		outcome.message += " Could not reload the list: " + listErr.Error()
	}
	return resultMsg{page: ScreenBackup, value: outcome}
}

func restoreBackup(opts Options, id, password string) tea.Msg {
	st, svc, err := openBackupService(opts)
	if err != nil {
		return resultMsg{page: ScreenBackup, err: err}
	}
	result, err := backup.RestoreInstance(context.Background(), svc, backupDataDir(opts), id, password, st.Close)
	if err != nil {
		st.Close()
		return resultMsg{page: ScreenBackup, err: err}
	}
	outcome := backupOutcome{message: result.Message}
	if list, listErr := loadBackups(opts); listErr == nil {
		outcome.items, outcome.listSet = list.items, true
	}
	return resultMsg{page: ScreenBackup, value: outcome}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func (m model) renderBackup() string {
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Backups") + "\n\n")
	b.WriteString("  " + m.styles.muted.Render("Data "+backupDataDir(m.opts)) + "\n\n")
	if m.backupMessage != "" {
		for _, line := range strings.Split(strings.TrimSpace(m.backupMessage), "\n") {
			b.WriteString("  " + m.styles.pass.Render(line) + "\n")
		}
		b.WriteString("\n")
	}
	if len(m.backupItems) == 0 {
		b.WriteString("  " + m.styles.muted.Render("No backups are stored here yet. Press n to take one.") + "\n")
	} else {
		b.WriteString(fmt.Sprintf("  %-44s %-20s %10s\n", "BACKUP", "TAKEN", "SIZE"))
		for i, item := range m.backupItems {
			marker := "  "
			if i == m.backupCursor {
				marker = m.styles.selected.Render("› ")
			}
			taken := time.UnixMilli(item.TakenAt).Local().Format("2006-01-02 15:04")
			b.WriteString(fmt.Sprintf("%s%-44s %-20s %10s\n", marker, item.ID, taken, humanBytes(item.Bytes)))
		}
	}
	if m.err != nil {
		b.WriteString("\n  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	b.WriteString("\n  " + m.styles.muted.Render("n new backup · x restore selected · r refresh · esc back") + "\n")
	return b.String()
}

func (m model) startBackup() (tea.Model, tea.Cmd) {
	opts := m.opts
	m.backupForm = nil
	m.loading = true
	m.err = nil
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return takeBackup(opts) })
}

func (m model) startRestore(id, password string) (tea.Model, tea.Cmd) {
	opts := m.opts
	m.backupForm = nil
	m.loading = true
	m.err = nil
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return restoreBackup(opts, id, password) })
}

func (m model) finishBackupForm() (tea.Model, tea.Cmd) {
	kind := m.backupFormKind
	m.backupForm = nil
	switch kind {
	case "new":
		if m.backupNewValues != nil && m.backupNewValues.Confirm {
			return m.startBackup()
		}
	case "restore":
		if m.backupRestoreValues != nil && m.backupRestoreValues.Confirm {
			return m.startRestore(m.backupRestoreID, m.backupRestoreValues.Password)
		}
	}
	m.setContent()
	return m, nil
}
