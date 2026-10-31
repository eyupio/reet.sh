package tui

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/setup"
)

type setupRestoreValues struct {
	Profile    string
	Passphrase string
	Project    string
	Machine    string
	Conflict   string
	Probe      bool
	Inspect    bool
}

type setupRollbackValues struct {
	Profile    string
	Passphrase string
	Project    string
	Confirm    bool
}

type setupActionOutcome struct {
	output  string
	restore bool
	applied bool
}

func validSetupPassphrase(value string) error {
	if len(strings.TrimSpace(value)) < sealed.MinPassword {
		return fmt.Errorf("the setup passphrase needs at least %d characters", sealed.MinPassword)
	}
	return nil
}

func buildSetupRestoreForm(values *setupRestoreValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Title("Profile").Description(values.Profile),
			huh.NewInput().
				Title("Passphrase").
				Description(fmt.Sprintf("The snapshot passphrase (at least %d characters).", sealed.MinPassword)).
				EchoMode(huh.EchoModePassword).
				Value(&values.Passphrase).
				Validate(validSetupPassphrase),
			huh.NewInput().
				Title("Project").
				Description("Optional destination project path.").
				Value(&values.Project),
			huh.NewInput().
				Title("Machine").
				Description("Optional machine name for profile overrides.").
				Value(&values.Machine),
			huh.NewSelect[string]().
				Title("Conflicts").
				Description("Used only when the plan is applied.").
				Options(
					huh.NewOption("Refuse (default)", ""),
					huh.NewOption("Skip", "skip"),
					huh.NewOption("Replace", "replace"),
				).
				Value(&values.Conflict),
			huh.NewConfirm().
				Title("Probe application versions?").
				Value(&values.Probe),
			huh.NewConfirm().
				Title("Inspect local package installations?").
				Value(&values.Inspect),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func buildSetupRollbackForm(values *setupRollbackValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Title("Profile").Description(values.Profile),
			huh.NewInput().
				Title("Passphrase").
				Description(fmt.Sprintf("The snapshot passphrase (at least %d characters).", sealed.MinPassword)).
				EchoMode(huh.EchoModePassword).
				Value(&values.Passphrase).
				Validate(validSetupPassphrase),
			huh.NewInput().
				Title("Project").
				Description("Optional destination project path.").
				Value(&values.Project),
			huh.NewConfirm().
				Title("Roll back the newest completed restore?").
				Description("Reverses that restore's creates and replacements. Newer changes stay.").
				Affirmative("Roll back").
				Negative("Cancel").
				Value(&values.Confirm),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func setupPassphraseFile(passphrase string) (string, func(), error) {
	if err := validSetupPassphrase(passphrase); err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp("", "reet-setup-pass-*")
	if err != nil {
		return "", nil, err
	}
	name := f.Name()
	cleanup := func() { os.Remove(name) }
	if err = f.Chmod(0600); err != nil {
		f.Close()
		cleanup()
		return "", nil, err
	}
	if _, err = f.WriteString(strings.TrimSpace(passphrase)); err != nil {
		f.Close()
		cleanup()
		return "", nil, err
	}
	if err = f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return name, cleanup, nil
}

func runSetupRestore(opts Options, values setupRestoreValues, apply bool) tea.Msg {
	passFile, cleanup, err := setupPassphraseFile(values.Passphrase)
	if err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	defer cleanup()
	args := []string{"restore", "--profile", strings.TrimSpace(values.Profile), "--password-file", passFile}
	if opts.SetupDir != "" {
		args = append(args, "--dir", opts.SetupDir)
	}
	if project := strings.TrimSpace(values.Project); project != "" {
		args = append(args, "--project", project)
	}
	if machine := strings.TrimSpace(values.Machine); machine != "" {
		args = append(args, "--machine", machine)
	}
	if values.Probe {
		args = append(args, "--probe-versions")
	}
	if values.Inspect {
		args = append(args, "--inspect-installation")
	}
	if apply {
		args = append(args, "--apply")
		if values.Conflict != "" {
			args = append(args, "--on-conflict", values.Conflict)
		}
	}
	var out bytes.Buffer
	err = setup.Run(args, &out)
	return resultMsg{page: ScreenSetup, value: setupActionOutcome{output: out.String(), restore: true, applied: apply}, err: err}
}

func runSetupRollback(opts Options, values setupRollbackValues) tea.Msg {
	passFile, cleanup, err := setupPassphraseFile(values.Passphrase)
	if err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	defer cleanup()
	args := []string{"rollback", "--last", "--profile", strings.TrimSpace(values.Profile), "--password-file", passFile}
	if opts.SetupDir != "" {
		args = append(args, "--dir", opts.SetupDir)
	}
	if project := strings.TrimSpace(values.Project); project != "" {
		args = append(args, "--project", project)
	}
	var out bytes.Buffer
	err = setup.Run(args, &out)
	return resultMsg{page: ScreenSetup, value: setupActionOutcome{output: out.String()}, err: err}
}

func (m model) renderSetupAction() string {
	title := "Rollback setup"
	if m.setupActionRestore {
		title = "Restore setup"
	}
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render(title) + "\n\n")
	if m.setupActionOutput != "" {
		b.WriteString(m.setupActionOutput)
		if !strings.HasSuffix(m.setupActionOutput, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString("  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	if m.setupActionApplied && m.err == nil {
		if m.setupActionRestore {
			b.WriteString("  " + m.styles.muted.Render("Restore applied. Review the output above.") + "\n")
		} else {
			b.WriteString("  " + m.styles.muted.Render("Rollback finished. Review the output above.") + "\n")
		}
	}
	b.WriteString("\n")
	switch {
	case m.setupActionRestore && !m.setupActionApplied:
		b.WriteString("  " + m.styles.muted.Render("a applies this plan · r previews again · e edits · esc back") + "\n")
	case m.setupActionRestore:
		b.WriteString("  " + m.styles.muted.Render("r previews again · e edits · esc back") + "\n")
	default:
		b.WriteString("  " + m.styles.muted.Render("r runs again · e edits · esc back") + "\n")
	}
	return b.String()
}

func (m model) startSetupRestore(apply bool) (tea.Model, tea.Cmd) {
	values := *m.setupRestoreValues
	opts := m.opts
	m.setupForm = nil
	m.loading = true
	m.err = nil
	m.setupActionOutput = ""
	m.setupActionSet = true
	m.setupActionRestore = true
	m.setupActionApplied = false
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return runSetupRestore(opts, values, apply) })
}

func (m model) startSetupRollback() (tea.Model, tea.Cmd) {
	values := *m.setupRollbackValues
	opts := m.opts
	m.setupForm = nil
	m.loading = true
	m.err = nil
	m.setupActionOutput = ""
	m.setupActionSet = true
	m.setupActionRestore = false
	m.setupActionApplied = false
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return runSetupRollback(opts, values) })
}

func (m model) openSetupRestoreForm() (tea.Model, tea.Cmd) {
	m.loading = false
	m.err = nil
	m.setupActionOutput = ""
	m.setupActionSet = false
	m.setupActionApplied = false
	m.setupFormKind = "restore"
	m.setupForm = buildSetupRestoreForm(m.setupRestoreValues, m.dark)
	m.resize()
	m.setContent()
	return m, m.setupForm.Init()
}

func (m model) openSetupRollbackForm() (tea.Model, tea.Cmd) {
	m.loading = false
	m.err = nil
	m.setupActionOutput = ""
	m.setupActionSet = false
	m.setupActionApplied = false
	m.setupFormKind = "rollback"
	m.setupForm = buildSetupRollbackForm(m.setupRollbackValues, m.dark)
	m.resize()
	m.setContent()
	return m, m.setupForm.Init()
}
