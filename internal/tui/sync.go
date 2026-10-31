package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/scan"
)

type syncFormValues struct {
	Target string
	Server string
	Token  string
	Host   string
	Dir    string
}

type syncOutcome struct {
	report  scan.SyncReport
	applied bool
}

func defaultSyncValues(opts Options) *syncFormValues {
	dir := opts.SyncDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	host := opts.SyncEnv
	if host == "" {
		host, _ = os.Hostname()
	}
	server := opts.SyncServer
	if server == "" {
		server = os.Getenv("REET_URL")
	}
	return &syncFormValues{
		Target: opts.SyncTarget,
		Server: server,
		Token:  opts.SyncToken,
		Host:   host,
		Dir:    dir,
	}
}

func buildSyncForm(values *syncFormValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Target").
				Description("Target ID from Account → Compare & sync.").
				Value(&values.Target).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("a target ID is required")
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
			huh.NewInput().
				Title("Environment").
				Description("This machine's name; defaults to the hostname.").
				Value(&values.Host),
			huh.NewInput().
				Title("Project directory").
				Description("Only for a project target; defaults to the current directory.").
				Value(&values.Dir),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func (m model) renderSync() string {
	r := m.syncReport
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Sync") + "\n\n")
	if m.err != nil && !r.PlanSet {
		b.WriteString("  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n\n")
		b.WriteString("  " + m.styles.muted.Render("e edits the form · r tries again · esc returns to the menu") + "\n")
		return b.String()
	}
	if r.TargetSet {
		b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("Target  %s (%s, %s)", r.Target.Target.Name, r.Target.Target.Host, r.Target.Mode)) + "\n")
		if r.Target.Target.Project != "" {
			b.WriteString("  " + m.styles.muted.Render("Project "+r.Target.Target.Project) + "\n")
		}
		b.WriteString("  " + m.styles.muted.Render("Reference "+r.Target.SourceName) + "\n")
	}
	b.WriteString("  " + m.styles.muted.Render("Directory "+r.Plan.Directory) + "\n")
	b.WriteString("  " + m.styles.muted.Render("Revision  "+r.Plan.Revision) + "\n\n")
	if len(r.Plan.Actions) == 0 {
		b.WriteString("  " + m.styles.pass.Render("Already in sync.") + "\n")
	} else {
		for _, action := range r.Plan.Actions {
			verb := "REMOVE"
			style := m.styles.fail
			if action.Add && action.Replace {
				verb = "REPLACE"
				style = m.styles.warn
			} else if action.Add {
				verb = "INSTALL"
				style = m.styles.pass
			}
			b.WriteString(fmt.Sprintf("  %s %-40s %s\n", style.Render(verb), action.Skill.Slug, m.styles.muted.Render(action.Skill.Source)))
		}
	}
	if m.syncApplied {
		if m.err == nil {
			b.WriteString("\n  " + m.styles.pass.Render("Sync complete. Updated inventory persisted on the controller.") + "\n")
		}
	} else if len(r.Plan.Actions) > 0 {
		b.WriteString("\n  " + m.styles.muted.Render("Preview only. Press a to apply exactly this plan.") + "\n")
	}
	if m.err != nil {
		b.WriteString("\n  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
		if len(r.Plan.Actions) > 0 {
			b.WriteString("  " + m.styles.muted.Render("Preview again before applying after a failed attempt.") + "\n")
		}
	}
	for _, backup := range r.Backups {
		b.WriteString("  " + m.styles.muted.Render("Backed up "+backup) + "\n")
	}
	b.WriteString("\n")
	if m.syncApplied {
		b.WriteString("  " + m.styles.muted.Render("r previews again · e edits the form · esc returns to the menu") + "\n")
	} else if len(r.Plan.Actions) > 0 {
		b.WriteString("  " + m.styles.muted.Render("a applies · r previews again · e edits the form · esc returns to the menu") + "\n")
	} else {
		b.WriteString("  " + m.styles.muted.Render("r previews again · e edits the form · esc returns to the menu") + "\n")
	}
	return b.String()
}

func (m model) startSync(apply bool) (tea.Model, tea.Cmd) {
	values := m.syncValues
	if values == nil {
		values = defaultSyncValues(m.opts)
		m.syncValues = values
	}
	opts := m.opts
	approved := ""
	if apply {
		approved = m.syncReport.Hash
	}
	m.syncForm = nil
	m.syncApplied = false
	m.err = nil
	m.loading = true
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		report, err := scan.RunSyncReport(scan.SyncReportOptions{
			Server:       strings.TrimSpace(values.Server),
			Token:        values.Token,
			TokenFile:    opts.SyncTokenFile,
			Target:       strings.TrimSpace(values.Target),
			Host:         strings.TrimSpace(values.Host),
			Config:       opts.SyncConfig,
			Dir:          strings.TrimSpace(values.Dir),
			Apply:        apply,
			ApprovedHash: approved,
			Version:      opts.Version,
			In:           strings.NewReader(""),
			Out:          io.Discard,
		})
		return resultMsg{page: ScreenSync, value: syncOutcome{report: report, applied: apply}, err: err}
	})
}
