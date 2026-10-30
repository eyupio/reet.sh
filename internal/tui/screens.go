package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/health"
	"github.com/eyupio/reet.sh/internal/lifecycle"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
)

type agentValue struct {
	status   scan.AgentStatus
	config   string
	enrolled bool
}

func loadDoctor(opts Options) (lifecycle.Report, error) {
	state := opts.DoctorStateDir
	if state == "" {
		state = lifecycle.StateDefault()
	}
	app := lifecycle.New(opts.Version, opts.Commit, strings.NewReader(""), io.Discard)
	return app.DoctorReport(lifecycle.DoctorOptions{
		StateDir:    state,
		Server:      opts.DoctorServer,
		TokenFile:   opts.DoctorTokenFile,
		Interactive: false,
	})
}

func loadHealth(opts Options) (health.Result, error) {
	var endpoint string
	var err error
	switch {
	case opts.HealthLocal:
		endpoint = health.URLFromAddr(envOr("REET_ADDR", ":8080"))
	case opts.HealthURL != "":
		endpoint, err = health.Endpoint(opts.HealthURL)
	case os.Getenv("REET_URL") != "":
		endpoint, err = health.Endpoint(os.Getenv("REET_URL"))
	default:
		endpoint = health.URLFromAddr(envOr("REET_ADDR", ":8080"))
	}
	if err != nil {
		return health.Result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return health.Check(ctx, endpoint)
}

func loadTerminal(opts Options) (terminalcontrol.Status, error) {
	dir := opts.TerminalDataDir
	if dir == "" {
		dir = envOr("REET_DATA_DIR", "./data")
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return terminalcontrol.Status{}, fmt.Errorf("no data directory at %s; terminal status needs the controller's data directory", dir)
		}
		return terminalcontrol.Status{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return terminalcontrol.Command(ctx, dir, "status", os.Getenv("REET_REMOTE_TERMINALS") == "on")
}

func loadAgent(opts Options) (agentValue, error) {
	config := opts.AgentConfig
	if config == "" {
		config = scan.AgentConfigPath()
	}
	if _, err := os.Stat(config); err != nil {
		if os.IsNotExist(err) {
			return agentValue{status: scan.AgentStatus{ConfigPath: config}, config: config}, nil
		}
		return agentValue{config: config}, err
	}
	status, err := scan.ReadAgentStatus(config)
	return agentValue{status: status, config: config, enrolled: true}, err
}

func (m model) statusTag(status string) string {
	label := "[" + strings.ToUpper(status) + "]"
	switch status {
	case "pass":
		return m.styles.pass.Render(label)
	case "warn":
		return m.styles.warn.Render(label)
	case "fail":
		return m.styles.fail.Render(label)
	}
	return m.styles.muted.Render(label)
}

func (m model) renderDoctor() string {
	r := m.doctor
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render(fmt.Sprintf("Doctor — %s (%s)", r.Version, shortCommit(r.Commit))) + "\n\n")
	if len(r.Checks) == 0 {
		b.WriteString("  " + m.styles.muted.Render("No checks were returned.") + "\n")
		return b.String()
	}
	for _, c := range r.Checks {
		b.WriteString("  " + m.statusTag(c.Status) + " " + m.styles.item.Render(c.Name) + "\n")
		b.WriteString("      " + c.Message + "\n")
		if c.Advice != "" {
			b.WriteString("      " + m.styles.muted.Render(c.Advice) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("%d warning(s). Doctor made no configuration changes.", r.Warnings())) + "\n")
	return b.String()
}

func (m model) renderHealth() string {
	r := m.health
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Healthcheck") + "\n\n")
	if r.URL != "" {
		b.WriteString("  " + m.styles.muted.Render("Endpoint") + "  " + r.URL + "\n")
	}
	if r.Status != 0 {
		verdict := m.styles.pass.Render("ok")
		if !r.OK {
			verdict = m.styles.fail.Render("failed")
		}
		b.WriteString("  " + m.styles.muted.Render("Answer") + "    " + verdict + fmt.Sprintf(" (HTTP %d)", r.Status) + "\n")
	}
	if r.Latency > 0 {
		b.WriteString("  " + m.styles.muted.Render("Latency") + "   " + r.Latency.Round(time.Millisecond).String() + "\n")
	}
	if !r.CheckedAt.IsZero() {
		b.WriteString("  " + m.styles.muted.Render("Checked") + "   " + r.CheckedAt.Format(time.RFC3339) + "\n")
	}
	if m.err != nil {
		b.WriteString("\n  " + m.styles.fail.Render("Error: "+m.err.Error()) + "\n")
	}
	return b.String()
}

func (m model) renderTerminal() string {
	s := m.term
	state := "stopped"
	if s.Enabled {
		state = "active"
		if !s.Available {
			state = "configured on"
		}
	}
	available := "disabled"
	if s.Available {
		available = "enabled"
	}
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Terminal status") + "\n\n")
	line := func(label, value string) {
		b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("%-16s", label)) + value + "\n")
	}
	line("Remote", state)
	line("Availability", available)
	line("Controller", yesNoText(s.Running, "running", "stopped"))
	line("Enabled", yesNo(s.Enabled))
	line("Public URL", valueOr(s.PublicURL))
	line("Auth mode", valueOr(s.AuthMode))
	line("Restart needed", yesNo(s.RestartRequired))
	if s.Reason != "" {
		line("Reason", s.Reason)
	}
	if !s.Running {
		b.WriteString("\n  " + m.styles.muted.Render("Start the controller with reet serve, or use the managed service.") + "\n")
	}
	return b.String()
}

func (m model) renderAgent() string {
	s := m.agent
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Agent status") + "\n\n")
	if !m.enrolled {
		b.WriteString("  " + m.styles.warn.Render("Not enrolled on this machine.") + "\n")
		b.WriteString("  " + m.styles.muted.Render("Enrollment file: "+m.config) + "\n\n")
		b.WriteString("  " + m.styles.muted.Render("Run reet agent enroll to connect this machine to a controller.") + "\n")
		return b.String()
	}
	line := func(label, value string) {
		b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("%-16s", label)) + value + "\n")
	}
	line("Enrollment", m.config)
	if !s.Running {
		line("Agent", "stopped")
		line("Connection", "disconnected")
		return b.String()
	}
	if !s.LocalControl {
		line("Agent", "running (local control unavailable; older process)")
		return b.String()
	}
	mode := "background/foreground process"
	if s.Supervised {
		mode = "native service"
	}
	connection := "reconnecting"
	if s.Connected {
		connection = "connected"
	}
	line("Agent", fmt.Sprintf("running (PID %d, %s)", s.PID, mode))
	line("Connection", connection)
	if !s.StartedAt.IsZero() {
		line("Started", s.StartedAt.Format(time.RFC3339))
	}
	line("Version", valueOr(s.Version))
	if !s.LastCycle.IsZero() {
		line("Last cycle", s.LastCycle.Format(time.RFC3339))
	}
	return b.String()
}

func (m model) renderVersion() string {
	var b strings.Builder
	b.WriteString("\n  " + m.styles.title.Render("Version") + "\n\n")
	line := func(label, value string) {
		b.WriteString("  " + m.styles.muted.Render(fmt.Sprintf("%-10s", label)) + value + "\n")
	}
	line("reet", m.opts.Version)
	line("Commit", m.opts.Commit)
	line("Go", runtime.Version())
	line("Platform", runtime.GOOS+"/"+runtime.GOARCH)
	return b.String()
}

func valueOr(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func yesNoText(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}
