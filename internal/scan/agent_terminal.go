package scan

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var rootUser = func() bool { return os.Geteuid() == 0 }

func runAgentTerminal(args []string, out io.Writer) error {
	return runAgentTerminalInput(args, os.Stdin, out, "")
}
func runAgentTerminalInput(args []string, in io.Reader, out io.Writer, version string) error {
	if len(args) == 0 {
		return errors.New("usage: reet agent terminal setup|enable|disable|status|sessions|end [--config PATH]")
	}
	action := args[0]
	fs := flag.NewFlagSet("agent terminal "+action, flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	session := fs.String("session", "", "local terminal session name")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	absolute, err := filepath.Abs(*config)
	if err != nil {
		return err
	}
	*config = absolute
	if action == "setup" {
		if !clientconfig.Interactive(in) {
			return errors.New("terminal setup needs an interactive terminal; run reet agent terminal setup from your shell. For automation, use explicit terminal enable and service install commands")
		}
		return SetupTerminalHost(TerminalSetupOptions{Config: *config, Version: version, Ask: func(label, current string) (string, error) { return clientconfig.Ask(in, out, label, current) }}, in, out)
	}
	c, err := loadAgentConfig(*config)
	if err != nil {
		return err
	}
	layout, err := terminalhost.NewEnrollmentLayout(*config, c.ID)
	if err != nil {
		return err
	}
	switch action {
	case "enable":
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			return errors.New("terminal access is supported on Linux and macOS hosts")
		}
		tmux, err := terminalhost.FindTmux(context.Background())
		if err != nil {
			return err
		}
		if err := terminalhost.Ensure(layout); err != nil {
			return err
		}
		c.TerminalEnabled = true
		if err := writePrivate(*config, c); err != nil {
			return err
		}
		fmt.Fprintf(out, "Terminal access enabled locally for %s.\n", c.Host)
		if rootUser() {
			fmt.Fprintln(out, "Remote shells run as root, the user owning this enrollment.")
		}
		fmt.Fprintf(out, "tmux: %s (%s)\nPrivate socket: %s\n", tmux.Path, tmux.Version, layout.Socket)
		fmt.Fprintf(out, "Install or refresh the independent services with: reet agent service install --config %q\n", *config)
		return nil
	case "disable":
		c.TerminalEnabled = false
		if err := writePrivate(*config, c); err != nil {
			return err
		}
		fmt.Fprintf(out, "Remote terminal access disabled for %s. Local tmux sessions continue; list or end them with reet agent terminal sessions or end.\n", c.Host)
		return nil
	case "status":
		readiness := terminalhost.Readiness(context.Background(), layout, c.TerminalEnabled)
		fmt.Fprintf(out, "Machine: %s\nTerminal access: %t\nReadiness: %s\n", c.Host, c.TerminalEnabled, readiness)
		if tmux, err := terminalhost.FindTmux(context.Background()); err == nil {
			fmt.Fprintf(out, "tmux: %s (%s)\n", tmux.Path, tmux.Version)
		} else {
			fmt.Fprintf(out, "tmux: %v\n", err)
		}
		fmt.Fprintf(out, "Private socket: %s\n", layout.Socket)
		if plan, err := servicePlan(*config, layout, true); err == nil {
			status := terminalhost.ReadStatus(context.Background(), plan)
			fmt.Fprintf(out, "Services installed: %t\nServices active: %t\n", status.Installed, status.Active)
		}
		return nil
	case "sessions":
		tmux, err := terminalhost.FindTmux(context.Background())
		if err != nil {
			return err
		}
		sessions, err := terminalhost.Sessions(context.Background(), tmux, layout)
		if errors.Is(err, terminalhost.ErrNotRunning) {
			fmt.Fprintln(out, "Terminal service is not running; no local sessions.")
			return nil
		}
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Fprintln(out, "No local terminal sessions.")
			return nil
		}
		for _, item := range sessions {
			attached := ""
			if item.Attached {
				attached = " attached"
			}
			fmt.Fprintf(out, "%s  %s  %d window(s)%s\n", item.Name, time.UnixMilli(item.Created).Format(time.RFC3339), item.Windows, attached)
		}
		return nil
	case "end":
		if *session == "" {
			return errors.New("agent terminal end requires --session NAME")
		}
		tmux, err := terminalhost.FindTmux(context.Background())
		if err != nil {
			return err
		}
		if err := terminalhost.EndSession(context.Background(), tmux, layout, *session); err != nil {
			return err
		}
		fmt.Fprintf(out, "Ended local terminal session %s.\n", *session)
		return nil
	default:
		return errors.New("unknown terminal action; use enable, disable, status, sessions or end")
	}
}

func runAgentService(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: reet agent service install|status|uninstall [--config PATH]")
	}
	action := args[0]
	fs := flag.NewFlagSet("agent service "+action, flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	prefix := fs.String("service-prefix", "reet", "native service name prefix")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	absolute, err := filepath.Abs(*config)
	if err != nil {
		return err
	}
	c, err := loadAgentConfig(absolute)
	if err != nil {
		return err
	}
	layout, err := terminalhost.NewEnrollmentLayout(absolute, c.ID)
	if err != nil {
		return err
	}
	plan, err := terminalhost.PlanServices(context.Background(), terminalhost.ServiceOptions{Prefix: *prefix, ConfigPath: absolute, Layout: layout, AllowMissingTmux: action != "install"})
	if err != nil {
		return err
	}
	switch action {
	case "install":
		if err := installAgentServices(plan, out); err != nil {
			return err
		}
		fmt.Fprintf(out, "Installed %s and %s.\n", plan.AgentName, plan.TerminalName)
		fmt.Fprintln(out, terminalhost.LingerGuidance(context.Background()))
		fmt.Fprintln(out, "A running terminal service is preserved during refresh. New tmux settings take effect after you explicitly restart it locally.")
		warnTerminalPrivileges(plan, out)
		return nil
	case "status":
		status := terminalhost.ReadStatus(context.Background(), plan)
		fmt.Fprintf(out, "Agent unit: %s\nTerminal unit: %s\nInstalled: %t\nActive: %t\n", plan.AgentPath, plan.TerminalPath, status.Installed, status.Active)
		warnTerminalPrivileges(plan, out)
		if !status.ManagerAvailable {
			fmt.Fprintln(out, "Service manager unavailable or user services are not enabled on this host.")
		}
		return nil
	case "uninstall":
		fmt.Fprintln(out, "Stopping the terminal service ends running local tmux sessions.")
		if err := terminalhost.Uninstall(context.Background(), plan); err != nil {
			return err
		}
		fmt.Fprintln(out, "Removed the agent and terminal services. Local terminal state and consent were left in place.")
		return nil
	default:
		return errors.New("unknown service action; use install, status or uninstall")
	}
}

func servicePlan(configPath string, layout terminalhost.Layout, allowMissingTmux bool) (terminalhost.ServicePlan, error) {
	return terminalhost.PlanServices(context.Background(), terminalhost.ServiceOptions{
		Prefix: "reet", Binary: "", ConfigPath: configPath, Layout: layout, AllowMissingTmux: allowMissingTmux,
	})
}

func terminalReadiness(configPath, agentID string, enabled bool) terminalproto.Readiness {
	layout, err := terminalhost.NewEnrollmentLayout(configPath, agentID)
	if err != nil {
		return terminalproto.SupervisorUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return terminalhost.Readiness(ctx, layout, enabled)
}

func warnTerminalPrivileges(plan terminalhost.ServicePlan, out io.Writer) {
	restricted, err := terminalhost.TerminalNoNewPrivileges(context.Background(), plan)
	if err == nil && restricted {
		fmt.Fprintln(out, "Existing terminal server has NoNewPrivileges set; sudo cannot elevate in its shells.")
		fmt.Fprintln(out, "Refreshing units preserves that server and cannot clear the flag. Finish its terminal work, then plan a local terminal-service restart from a normal SSH/login shell. Restarting the terminal service ends its sessions and jobs.")
	}
}
