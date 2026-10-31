package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/setup"
)

type setupOutcome struct {
	profiles  setup.SetupProfileList
	snapshots setup.SetupSnapshotList
	message   string
	listSet   bool
}

type setupNewValues struct {
	Name       string
	Snapshot   string
	Passphrase string
}

type setupPushValues struct {
	Name   string
	Server string
	Token  string
}

type setupPullValues struct {
	Name   string
	Server string
	Token  string
}

func setupServer(opts Options) string {
	if opts.SetupServer != "" {
		return opts.SetupServer
	}
	return os.Getenv("REET_URL")
}

func defaultSetupPushValues(opts Options, name string) *setupPushValues {
	return &setupPushValues{Name: name, Server: setupServer(opts), Token: opts.SetupToken}
}

func defaultSetupPullValues(opts Options) *setupPullValues {
	return &setupPullValues{Server: setupServer(opts), Token: opts.SetupToken}
}

func buildNewSetupForm(values *setupNewValues, newest string, dark bool) *huh.Form {
	if values.Snapshot == "" {
		values.Snapshot = newest
	}
	description := "Snapshot ID or path; empty uses the newest."
	if newest == "" {
		description = "No local snapshots; run reet setup backup first."
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Profile name").
				Description("1-64 letters, digits, '-' or '_'.").
				Value(&values.Name).
				Validate(func(value string) error {
					if !setup.ValidProfileName(strings.TrimSpace(value)) {
						return errors.New("use 1-64 letters, digits, '-' or '_'")
					}
					return nil
				}),
			huh.NewInput().
				Title("Snapshot").
				Description(description).
				Value(&values.Snapshot).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("a snapshot is required; run reet setup backup first")
					}
					return nil
				}),
			huh.NewInput().
				Title("Passphrase").
				Description("Optional; opens the snapshot to record preview and software evidence.").
				EchoMode(huh.EchoModePassword).
				Value(&values.Passphrase),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func buildPushSetupForm(values *setupPushValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().Title("Profile").Description(values.Name),
			huh.NewInput().
				Title("Controller").
				Description("HTTPS site URL; defaults to REET_URL.").
				Value(&values.Server),
			huh.NewInput().
				Title("Token").
				Description("Personal API token; empty uses REET_TOKEN or the saved token.").
				EchoMode(huh.EchoModePassword).
				Value(&values.Token),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func buildPullSetupForm(values *setupPullValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Profile name").
				Description("The profile to fetch from the controller.").
				Value(&values.Name).
				Validate(func(value string) error {
					if !setup.ValidProfileName(strings.TrimSpace(value)) {
						return errors.New("use 1-64 letters, digits, '-' or '_'")
					}
					return nil
				}),
			huh.NewInput().
				Title("Controller").
				Description("HTTPS site URL; defaults to REET_URL.").
				Value(&values.Server),
			huh.NewInput().
				Title("Token").
				Description("Personal API token; empty uses REET_TOKEN or the saved token.").
				EchoMode(huh.EchoModePassword).
				Value(&values.Token),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func loadSetup(opts Options) (setupOutcome, error) {
	profiles, err := setup.ListSetupProfiles(opts.SetupDir)
	if err != nil {
		return setupOutcome{}, err
	}
	snapshots, err := setup.ListSetupSnapshots(opts.SetupDir)
	if err != nil {
		return setupOutcome{}, err
	}
	return setupOutcome{profiles: profiles, snapshots: snapshots, listSet: true}, nil
}

func setupToken(opts Options, explicit string) (string, error) {
	return clientconfig.ResolveToken(explicit, opts.SetupTokenFile, strings.NewReader(""), io.Discard)
}

func saveSetupProfile(opts Options, values setupNewValues) tea.Msg {
	snapshot := strings.TrimSpace(values.Snapshot)
	if snapshot == "" {
		return resultMsg{page: ScreenSetup, err: errors.New("no local setup snapshot; run reet setup backup first")}
	}
	profile, revision, err := setup.SaveSetupProfileWithOptions(strings.TrimSpace(values.Name), values.Passphrase, snapshot, opts.SetupDir, time.Now().UTC(), setup.SaveSetupProfileOptions{})
	if err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	outcome, listErr := loadSetup(opts)
	outcome.message = fmt.Sprintf("Saved profile %s revision %d for snapshot %s.", profile.Name, revision.Revision, revision.SnapshotID)
	if listErr != nil {
		outcome.message += " Could not reload the list: " + listErr.Error()
	}
	return resultMsg{page: ScreenSetup, value: outcome}
}

func pushSetupProfile(opts Options, values setupPushValues) tea.Msg {
	token, err := setupToken(opts, values.Token)
	if err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	profile, _, err := setup.LoadProfile(strings.TrimSpace(values.Name), opts.SetupDir, 0)
	if err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	if err := (setup.HTTPRemote{}).Push(strings.TrimSpace(values.Server), token, profile); err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	outcome, listErr := loadSetup(opts)
	outcome.message = fmt.Sprintf("Pushed profile %s.", profile.Name)
	if listErr != nil {
		outcome.message += " Could not reload the list: " + listErr.Error()
	}
	return resultMsg{page: ScreenSetup, value: outcome}
}

func pullSetupProfile(opts Options, values setupPullValues) tea.Msg {
	token, err := setupToken(opts, values.Token)
	if err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	profile, err := (setup.HTTPRemote{}).Pull(strings.TrimSpace(values.Server), token, strings.TrimSpace(values.Name))
	if err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	if err := setup.WriteSetupProfile(profile, opts.SetupDir); err != nil {
		return resultMsg{page: ScreenSetup, err: err}
	}
	revision := 0
	if len(profile.Revisions) > 0 {
		revision = profile.Revisions[len(profile.Revisions)-1].Revision
	}
	outcome, listErr := loadSetup(opts)
	outcome.message = fmt.Sprintf("Pulled profile %s revision %d.", profile.Name, revision)
	if listErr != nil {
		outcome.message += " Could not reload the list: " + listErr.Error()
	}
	return resultMsg{page: ScreenSetup, value: outcome}
}

func (m model) renderSetup() string {
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Setups") + "\n\n")
	b.WriteString("  " + m.styles.muted.Render("Profiles "+m.setupProfiles.Dir) + "\n")
	b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("Snapshots %d", len(m.setupSnapshots.Snapshots))) + "\n\n")
	if m.setupMessage != "" {
		for _, line := range strings.Split(strings.TrimSpace(m.setupMessage), "\n") {
			b.WriteString("  " + m.styles.pass.Render(line) + "\n")
		}
		b.WriteString("\n")
	}
	if len(m.setupProfiles.Profiles) == 0 {
		b.WriteString("  " + m.styles.muted.Render("No setup profiles yet. Press n to create one.") + "\n")
	} else {
		b.WriteString(fmt.Sprintf("  %-24s %6s  %-34s %s\n", "PROFILE", "REV", "SNAPSHOT", "UPDATED"))
		for i, profile := range m.setupProfiles.Profiles {
			marker := "  "
			if i == m.setupCursor {
				marker = m.styles.selected.Render("› ")
			}
			updated := ""
			if !profile.UpdatedAt.IsZero() {
				updated = profile.UpdatedAt.Local().Format("2006-01-02 15:04")
			}
			b.WriteString(fmt.Sprintf("%s%-24s %6d  %-34s %s\n", marker, profile.Name, profile.Revision, profile.SnapshotID, updated))
		}
	}
	if m.err != nil {
		b.WriteString("\n  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	b.WriteString("\n  " + m.styles.muted.Render("n new · p push selected · u pull · ↑/↓ select · r refresh · esc back") + "\n")
	return b.String()
}

func (m model) startSetupNew() (tea.Model, tea.Cmd) {
	values := *m.setupNewValues
	opts := m.opts
	m.setupForm = nil
	m.loading = true
	m.err = nil
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return saveSetupProfile(opts, values) })
}

func (m model) startSetupPush() (tea.Model, tea.Cmd) {
	values := *m.setupPushValues
	opts := m.opts
	m.setupForm = nil
	m.loading = true
	m.err = nil
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return pushSetupProfile(opts, values) })
}

func (m model) startSetupPull() (tea.Model, tea.Cmd) {
	values := *m.setupPullValues
	opts := m.opts
	m.setupForm = nil
	m.loading = true
	m.err = nil
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg { return pullSetupProfile(opts, values) })
}

func (m model) finishSetupForm() (tea.Model, tea.Cmd) {
	kind := m.setupFormKind
	m.setupForm = nil
	switch kind {
	case "new":
		return m.startSetupNew()
	case "push":
		return m.startSetupPush()
	case "pull":
		return m.startSetupPull()
	}
	m.setContent()
	return m, nil
}
