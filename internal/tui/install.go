package tui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/lifecycle"
)

type installFormValues struct {
	Mode    string
	Dir     string
	DataDir string
	Addr    string
	Version string
	Start   bool
	DryRun  bool
	Confirm bool
}

type installOutcome struct {
	output string
	dryRun bool
}

func defaultInstallValues(opts Options) *installFormValues {
	mode := opts.InstallMode
	if mode == "" {
		mode = "binary"
	}
	dir := opts.InstallDir
	if dir == "" {
		dir = lifecycle.BinaryDefault()
	}
	dataDir := opts.InstallDataDir
	if dataDir == "" {
		dataDir = lifecycle.DataDefault()
	}
	addr := opts.InstallAddr
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	version := opts.InstallVersion
	if version == "" {
		version = os.Getenv("REET_VERSION")
	}
	if version == "" {
		version = "latest"
	}
	return &installFormValues{
		Mode:    mode,
		Dir:     dir,
		DataDir: dataDir,
		Addr:    addr,
		Version: version,
		Start:   !opts.InstallNoStart,
		DryRun:  opts.InstallDryRun,
	}
}

func buildInstallForm(values *installFormValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Mode").
				Description("Binary installs this executable; native writes a user service; Compose pulls an image.").
				Options(
					huh.NewOption("Binary (this executable)", "binary"),
					huh.NewOption("Native user service", "native"),
					huh.NewOption("Docker Compose", "compose"),
				).
				Value(&values.Mode),
			huh.NewInput().
				Title("Install directory").
				Description("Binary mode writes reet here.").
				Value(&values.Dir),
			huh.NewInput().
				Title("Data directory").
				Description("Server database and backups; kept across upgrades.").
				Value(&values.DataDir),
			huh.NewInput().
				Title("Address").
				Description("HOST:PORT; loopback by default.").
				Value(&values.Addr),
			huh.NewInput().
				Title("Compose release").
				Description("latest, dev or a tag; only Compose uses this.").
				Value(&values.Version),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Start the server now?").
				Affirmative("Start").
				Negative("Leave stopped").
				Value(&values.Start),
			huh.NewConfirm().
				Title("Dry run only?").
				Description("Show the plan without changing the deployment.").
				Value(&values.DryRun),
			huh.NewConfirm().
				Title("Install this deployment now?").
				Description("The installer writes the deployment, installs the binary or service, and keeps an existing database.").
				Affirmative("Install").
				Negative("Cancel").
				Value(&values.Confirm),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func (m model) renderInstall() string {
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Install") + "\n\n")
	b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("Running %s (%s)", m.opts.Version, shortCommit(m.opts.Commit))) + "\n\n")
	if m.installOutput != "" {
		b.WriteString(m.installOutput)
		if !strings.HasSuffix(m.installOutput, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString("  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	if m.installDone && m.err == nil {
		if m.installDryRun {
			b.WriteString("  " + m.styles.muted.Render("Dry run: nothing changed.") + "\n")
		} else {
			b.WriteString("  " + m.styles.muted.Render("Installation finished. Quit and start reet again to use the new deployment.") + "\n")
		}
	}
	b.WriteString("\n  " + m.styles.muted.Render("r runs the form again · esc returns to the menu") + "\n")
	return b.String()
}

func (m model) startInstall() (tea.Model, tea.Cmd) {
	values := *m.installValues
	opts := m.opts
	m.installForm = nil
	m.loading = true
	m.err = nil
	m.installOutput = ""
	m.installDone = false
	m.installDryRun = false
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		var out bytes.Buffer
		app := lifecycle.New(opts.Version, opts.Commit, strings.NewReader(""), &out)
		app.Progress = io.Discard
		args := []string{
			"--mode", strings.TrimSpace(values.Mode),
			"--dir", strings.TrimSpace(values.Dir),
			"--data-dir", strings.TrimSpace(values.DataDir),
			"--addr", strings.TrimSpace(values.Addr),
			"--yes", "--skip-terminal-setup",
		}
		if values.Mode == "compose" {
			version := strings.TrimSpace(values.Version)
			if version == "" {
				version = "latest"
			}
			args = append(args, "--version", version)
		}
		if !values.Start {
			args = append(args, "--no-start")
		}
		if values.DryRun {
			args = append(args, "--dry-run")
		}
		if opts.InstallStateDir != "" {
			args = append(args, "--state-dir", opts.InstallStateDir)
		}
		err := app.Run("install", args)
		return resultMsg{page: ScreenInstall, value: installOutcome{output: out.String(), dryRun: values.DryRun}, err: err}
	})
}

func (m model) finishInstallForm() (tea.Model, tea.Cmd) {
	values := m.installValues
	m.installForm = nil
	if values != nil && values.Confirm {
		return m.startInstall()
	}
	m.setContent()
	return m, nil
}
