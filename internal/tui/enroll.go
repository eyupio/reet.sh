package tui

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/eyupio/reet.sh/internal/scan"
)

type enrollFormValues struct {
	Server     string
	Token      string
	Host       string
	Projects   string
	AllowApply bool
	Confirm    bool
}

type enrollOutcome struct {
	output string
}

func defaultEnrollValues(opts Options) *enrollFormValues {
	server := opts.EnrollServer
	if server == "" {
		server = os.Getenv("REET_URL")
	}
	host := opts.EnrollHost
	if host == "" {
		host = os.Getenv("REET_ENV")
	}
	if host == "" {
		host, _ = os.Hostname()
	}
	return &enrollFormValues{
		Server:     server,
		Token:      opts.EnrollToken,
		Host:       host,
		Projects:   opts.EnrollProjects,
		AllowApply: opts.EnrollAllowApply,
	}
}

func buildEnrollForm(values *enrollFormValues, dark bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Controller").
				Description("The reet site origin, such as https://reet.example.").
				Value(&values.Server).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return fmt.Errorf("a controller URL is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Token").
				Description("Personal API token; empty uses REET_TOKEN or the saved token.").
				EchoMode(huh.EchoModePassword).
				Value(&values.Token),
			huh.NewInput().
				Title("Environment").
				Description("A stable name for this machine; defaults to the hostname.").
				Value(&values.Host),
			huh.NewInput().
				Title("Projects").
				Description("Absolute project folders, comma or newline separated; optional.").
				Value(&values.Projects),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Allow approved sync jobs to change files here?").
				Description("Off by default; this machine can still report inventories and previews.").
				Value(&values.AllowApply),
			huh.NewConfirm().
				Title("Enroll this machine now?").
				Description("Writes the private enrollment file. Terminal and browser access stay separate opt-ins.").
				Affirmative("Enroll").
				Negative("Cancel").
				Value(&values.Confirm),
		),
	).WithTheme(formTheme(dark)).WithShowHelp(true)
}

func enrollProjects(value string) []string {
	var projects []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' }) {
		if part = strings.TrimSpace(part); part != "" {
			projects = append(projects, part)
		}
	}
	return projects
}

func (m model) renderEnroll() string {
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Connect machine") + "\n\n")
	if m.enrollOutput != "" {
		b.WriteString(m.enrollOutput)
		if !strings.HasSuffix(m.enrollOutput, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if m.err != nil {
		b.WriteString("  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	if m.enrollDone && m.err == nil {
		b.WriteString("  " + m.styles.muted.Render("Enrollment saved. Start the agent with reet agent start, then check Agent status.") + "\n")
	}
	b.WriteString("\n  " + m.styles.muted.Render("r runs the form again · esc returns to the menu") + "\n")
	return b.String()
}

func (m model) startEnroll() (tea.Model, tea.Cmd) {
	values := *m.enrollValues
	opts := m.opts
	m.enrollForm = nil
	m.loading = true
	m.err = nil
	m.enrollOutput = ""
	m.enrollDone = false
	m.setContent()
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		var out bytes.Buffer
		args := []string{"enroll", "--server", strings.TrimSpace(values.Server), "--env", strings.TrimSpace(values.Host), "--skip-terminal-setup"}
		if strings.TrimSpace(values.Token) != "" {
			args = append(args, "--token", strings.TrimSpace(values.Token))
		}
		if values.AllowApply {
			args = append(args, "--allow-apply")
		}
		for _, project := range enrollProjects(values.Projects) {
			args = append(args, "--project", project)
		}
		if opts.EnrollConfig != "" {
			args = append(args, "--config", opts.EnrollConfig)
		}
		if opts.EnrollTokenFile != "" {
			args = append(args, "--env-file", opts.EnrollTokenFile)
		}
		err := scan.RunAgentInput(args, strings.NewReader(""), &out, opts.Version)
		return resultMsg{page: ScreenEnroll, value: enrollOutcome{output: out.String()}, err: err}
	})
}

func (m model) finishEnrollForm() (tea.Model, tea.Cmd) {
	values := m.enrollValues
	m.enrollForm = nil
	if values != nil && values.Confirm {
		return m.startEnroll()
	}
	m.setContent()
	return m, nil
}
