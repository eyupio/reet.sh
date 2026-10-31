package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/scan"
)

type scanFormValues struct {
	Host      string
	Server    string
	Token     string
	Dirs      string
	Prune     bool
	Reconcile bool
	DryRun    bool
}

func defaultScanValues(opts Options) *scanFormValues {
	host := opts.ScanEnv
	if host == "" {
		host, _ = os.Hostname()
	}
	server := opts.ScanServer
	if server == "" {
		server = os.Getenv("REET_URL")
	}
	return &scanFormValues{
		Host:      host,
		Server:    server,
		Token:     opts.ScanToken,
		Dirs:      opts.ScanDirs,
		Prune:     opts.ScanPrune,
		Reconcile: opts.ScanReconcile,
		DryRun:    opts.ScanDryRun,
	}
}

func buildScanForm(values *scanFormValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Environment").
				Description("A stable name for this machine; defaults to the hostname.").
				Value(&values.Host),
			huh.NewInput().
				Title("Controller").
				Description("HTTPS site URL. Empty is allowed for a dry run.").
				Value(&values.Server),
			huh.NewInput().
				Title("Token").
				Description("Personal API token; empty uses REET_TOKEN or the saved token.").
				EchoMode(huh.EchoModePassword).
				Value(&values.Token),
			huh.NewInput().
				Title("Extra folders").
				Description("Comma-separated skill folders to scan, optional.").
				Value(&values.Dirs),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Dry run?").
				Description("List what would be sent without contacting the controller.").
				Value(&values.DryRun),
			huh.NewConfirm().
				Title("Prune?").
				Description("Remove stale inventory reports for these environments.").
				Value(&values.Prune),
			huh.NewConfirm().
				Title("Reconcile?").
				Description("Merge an older hostname into this environment first.").
				Value(&values.Reconcile),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func scanDirs(value string) []string {
	var dirs []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' }) {
		if part = strings.TrimSpace(part); part != "" {
			dirs = append(dirs, part)
		}
	}
	return dirs
}

func (m model) renderScan() string {
	r := m.scanReport
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Scan") + "\n\n")
	if m.err != nil && !m.scanSet {
		b.WriteString("  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n\n")
		b.WriteString("  " + m.styles.muted.Render("e edits the form · r tries again · esc returns to the menu") + "\n")
		return b.String()
	}
	if len(r.Found) == 0 {
		b.WriteString("  " + m.styles.muted.Render("No installed skills were found in the scanned folders.") + "\n\n")
	} else {
		scan.RenderFindings(&b, r.Found)
		b.WriteString("\n")
	}
	if r.ReconciledFrom != "" {
		b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("Reconciled %s into %s.", r.ReconciledFrom, r.Host)) + "\n")
	}
	switch {
	case r.DryRun:
		b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("%d skills found in %d environments (dry run, nothing sent).", len(r.Found), len(r.Envs))) + "\n")
	case r.Sent:
		b.WriteString(fmt.Sprintf("  %d new, %d ignored, %d pruned on %s\n", r.Created, r.Ignored, r.Pruned, r.Host))
		if r.User != "" {
			b.WriteString("  reported as " + r.User + "\n")
		}
		b.WriteString(fmt.Sprintf("  reported %d skills; existing shelf choices preserved\n", r.Imported))
	}
	if m.err != nil {
		b.WriteString("\n  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	for _, e := range r.Errors {
		b.WriteString("  " + m.styles.warn.Render("skipped: "+e) + "\n")
	}
	b.WriteString("\n  " + m.styles.muted.Render("r runs it again · e edits the form · esc returns to the menu") + "\n")
	return b.String()
}
