package tui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/lifecycle"
)

type upgradeFormValues struct {
	Release string
	Confirm bool
}

type upgradeOutcome struct {
	output string
}

func defaultUpgradeRelease(opts Options) string {
	if opts.UpgradeVersion != "" {
		return opts.UpgradeVersion
	}
	if value := os.Getenv("REET_VERSION"); value != "" {
		return value
	}
	if opts.Version == "dev" {
		return "dev"
	}
	return "latest"
}

func buildUpgradeForm(values *upgradeFormValues, current string, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Title("Current").Description(current),
			huh.NewInput().
				Title("Release").
				Description("latest, dev, or a tag such as v1.2.3.").
				Value(&values.Release).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("a release is required")
					}
					return nil
				}),
			huh.NewConfirm().
				Title("Upgrade this deployment now?").
				Description("Checks, downloads and verifies the release, stops the server, backs up SQLite, replaces the binary, restarts and verifies it, and rolls back on failure.").
				Affirmative("Upgrade").
				Negative("Cancel").
				Value(&values.Confirm),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func (m model) renderUpgrade() string {
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Upgrade") + "\n\n")
	b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("Running %s (%s)", m.opts.Version, shortCommit(m.opts.Commit))) + "\n\n")
	if m.upgradeOutput != "" {
		b.WriteString(m.upgradeOutput)
		if !strings.HasSuffix(m.upgradeOutput, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString("  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	if m.upgradeDone && m.err == nil {
		b.WriteString("  " + m.styles.muted.Render("The running process is the previous build. Quit and start reet again to use the new one.") + "\n")
	}
	b.WriteString("\n  " + m.styles.muted.Render("r runs the form again · esc returns to the menu") + "\n")
	return b.String()
}

func (m model) startUpgrade() (tea.Model, tea.Cmd) {
	values := *m.upgradeValues
	opts := m.opts
	m.upgradeForm = nil
	m.loading = true
	m.err = nil
	m.upgradeOutput = ""
	m.upgradeDone = false
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		var out bytes.Buffer
		app := lifecycle.New(opts.Version, opts.Commit, strings.NewReader(""), &out)
		app.Progress = io.Discard
		args := []string{"--version", strings.TrimSpace(values.Release), "--yes", "--skip-terminal-setup"}
		if opts.UpgradeStateDir != "" {
			args = append(args, "--state-dir", opts.UpgradeStateDir)
		}
		err := app.Run("upgrade", args)
		return resultMsg{page: ScreenUpgrade, value: upgradeOutcome{output: out.String()}, err: err}
	})
}

func (m model) finishUpgradeForm() (tea.Model, tea.Cmd) {
	values := m.upgradeValues
	m.upgradeForm = nil
	if values != nil && values.Confirm {
		return m.startUpgrade()
	}
	m.setContent()
	return m, nil
}
