package scan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type TerminalSetupOptions struct {
	Config, Server, Token, Version string
	Ask                            func(string, string) (string, error)
}

// OfferTerminalHostSetup preserves a completed enrollment when optional setup is
// declined, unavailable or interrupted. Automation never gains shell consent.
func OfferTerminalHostSetup(options TerminalSetupOptions, in io.Reader, out io.Writer) {
	resume := "reet agent terminal setup"
	if options.Config != "" {
		resume += " --config '" + strings.ReplaceAll(options.Config, "'", "'\\''") + "'"
	}
	if !clientconfig.Interactive(in) {
		fmt.Fprintf(out, "Terminal access is a separate local opt-in. Run %s in your terminal to connect this machine.\n", resume)
		return
	}
	fmt.Fprintln(out, "\nTerminal connection setup · your machine, your consent.")
	if options.Ask == nil {
		options.Ask = func(label, current string) (string, error) { return clientconfig.Ask(in, out, label, current) }
	}
	if err := SetupTerminalHost(options, in, out); err != nil {
		fmt.Fprintf(out, "Enrollment is saved. Terminal setup paused: %v\nResume with: %s\n", err, resume)
	}
}

// SetupTerminalHost offers explicit, persistent host opt-in after the controller
// is enabled. It never grants root access or enrolls a machine without consent.
func SetupTerminalHost(options TerminalSetupOptions, in io.Reader, out io.Writer) error {
	if options.Ask == nil {
		return errors.New("host setup requires an interactive prompt")
	}
	ask := options.Ask
	confirm := func(label, def string) (bool, error) {
		for {
			answer, err := ask(label+" (yes/no)", def)
			if err != nil {
				return false, err
			}
			switch strings.ToLower(strings.TrimSpace(answer)) {
			case "y", "yes":
				return true, nil
			case "n", "no":
				return false, nil
			default:
				fmt.Fprintln(out, "Choose yes or no.")
			}
		}
	}
	config := options.Config
	if config == "" {
		config = agentConfigPath()
	}
	var err error
	config, err = filepath.Abs(config)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(config); os.IsNotExist(err) {
		yes, err := confirm("Also set up this machine for web terminal access?", "no")
		if err != nil {
			return err
		}
		if !yes {
			fmt.Fprintln(out, "This machine was not enrolled or granted shell access. Resume with reet agent terminal setup.")
			return nil
		}
		config, err = ask("Machine enrollment file", config)
		if err != nil {
			return err
		}
		config, err = filepath.Abs(config)
		if err != nil {
			return err
		}
		if _, err = os.Lstat(config); os.IsNotExist(err) {
			server := options.Server
			if server == "" {
				server, err = clientconfig.ResolveURL("", in, out)
				if err != nil {
					return err
				}
			}
			token, err := clientconfig.ResolveToken(options.Token, "", in, out)
			if err != nil {
				return err
			}
			if token == "" {
				return errors.New("a personal API token is required to enroll this host")
			}
			if err = runAgentInput([]string{"enroll", "--config", config, "--server", server, "--token", token, "--skip-terminal-setup"}, in, out, options.Version); err != nil {
				return err
			}
		}
	}
	c, err := loadAgentConfig(config)
	if err != nil {
		return err
	}
	if options.Server != "" && strings.TrimRight(c.Server, "/") != strings.TrimRight(options.Server, "/") {
		return errors.New("this host enrollment belongs to another controller; select the correct --agent-config instead of replacing it")
	}
	if !c.TerminalEnabled {
		user := os.Getenv("USER")
		if rootUser() {
			user = "root"
		}
		if user == "" {
			user = "the current OS user"
		}
		yes, err := confirm(fmt.Sprintf("Allow your enrolling account to open shells on %s as %s?", c.Host, user), "no")
		if err != nil {
			return err
		}
		if !yes {
			fmt.Fprintln(out, "Local shell consent remains disabled. The existing enrollment is kept.")
			return nil
		}
		if _, err = terminalhost.FindTmux(context.Background()); err != nil {
			yes, err = confirm("tmux 3.3+ is required. Install tmux using the local package manager now?", "yes")
			if err != nil {
				return err
			}
			if !yes {
				return errors.New("host setup paused; tmux is required and shell consent was not changed")
			}
			if err = installSetupTmux(in, out, ask); err != nil {
				return err
			}
		}
		if err = runAgentTerminal([]string{"enable", "--config", config}, out); err != nil {
			return err
		}
	}
	layout, err := terminalhost.NewEnrollmentLayout(config, c.ID)
	if err != nil {
		return err
	}
	plan, err := servicePlan(config, layout, true)
	if err != nil {
		return err
	}
	service := terminalhost.ReadStatus(context.Background(), plan)
	native, err := localTerminalAgentNative(config, options.Version)
	if err != nil {
		return err
	}
	// Development releases share one version label across builds. Offer a
	// service refresh even when that label matches so an upgraded binary is used.
	if !service.Active || !native || options.Version == "dev" {
		yes, err := confirm("Install/repair or refresh native agent and terminal services now? Existing tmux jobs will be preserved.", "yes")
		if err != nil {
			return err
		}
		if !yes {
			fmt.Fprintln(out, "Local consent is saved. Native service setup was deferred; no readiness is claimed.")
			return nil
		}
		if _, err = terminalhost.FindTmux(context.Background()); err != nil {
			yes, err = confirm("Install required tmux now?", "yes")
			if err != nil {
				return err
			}
			if !yes {
				return errors.New("tmux is required for local terminal services")
			}
			if err = installSetupTmux(in, out, ask); err != nil {
				return err
			}
		}
		if err = runAgentService([]string{"install", "--config", config}, out); err != nil {
			return err
		}
	}
	current, err := loadAgentConfig(config)
	if err != nil {
		return err
	}
	if current.ID != c.ID || current.Secret != c.Secret {
		return errors.New("enrollment changed during setup; no readiness claimed")
	}
	ready := terminalReadiness(config, c.ID, current.TerminalEnabled)
	if ready != terminalproto.Ready {
		return fmt.Errorf("host setup has not reached terminal readiness: %s", ready)
	}
	native, err = localTerminalAgentNative(config, options.Version)
	if err != nil {
		return err
	}
	if !native {
		return errors.New("native agent handoff was not verified")
	}
	manager, err := terminalhost.NewManager(context.Background(), terminalhost.ManagerOptions{EnrollmentID: current.ID, Layout: layout, Home: current.Home, Consent: func() error {
		latest, err := loadAgentConfig(config)
		if err != nil {
			return err
		}
		if !latest.TerminalEnabled {
			return errors.New("local consent is disabled")
		}
		return nil
	}})
	if err != nil {
		return fmt.Errorf("local terminal manager is not ready: %w", err)
	}
	manager.Close()
	fmt.Fprintln(out, "Checking the private machine connection…")
	deadline := time.Now().Add(15 * time.Second)
	for !localTerminalAgentConnected(config) {
		if time.Now().After(deadline) {
			return errors.New("local setup is saved, but the private connection has not become ready; no remote readiness is claimed")
		}
		time.Sleep(250 * time.Millisecond)
	}
	fmt.Fprintf(out, "Local terminal setup verified for %s. Consent is saved and native services are running. Check this machine in Terminals for controller readiness.\n", c.Host)
	return nil
}

func installSetupTmux(in io.Reader, out io.Writer, ask func(string, string) (string, error)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var command string
	var args []string
	brewDir := ""
	if runtime.GOOS == "darwin" {
		command, _ = exec.LookPath("brew")
		if command == "" {
			for _, candidate := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
				if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
					command = candidate
					break
				}
			}
		}
		if command == "" {
			return errors.New("Homebrew is not installed; tmux installation cannot proceed automatically")
		}
		brewDir = filepath.Dir(command)
		args = []string{"install", "tmux"}
		if rootUser() {
			for {
				account, err := ask("Homebrew OS user (Homebrew cannot run as root)", os.Getenv("SUDO_USER"))
				if err != nil {
					return err
				}
				owner, err := user.Lookup(account)
				if err != nil || owner.Uid == "0" {
					fmt.Fprintln(out, "Choose an existing non-root Homebrew account.")
					continue
				}
				args = append([]string{"-H", "--user", account, "--", command}, args...)
				command = "sudo"
				break
			}
		}
	} else {
		for _, candidate := range []struct {
			name string
			args []string
		}{{"apt-get", []string{"install", "-y", "tmux"}}, {"dnf", []string{"install", "-y", "tmux"}}, {"yum", []string{"install", "-y", "tmux"}}, {"pacman", []string{"-S", "--needed", "--noconfirm", "tmux"}}} {
			if path, err := exec.LookPath(candidate.name); err == nil {
				command = path
				args = candidate.args
				break
			}
		}
		if command == "" {
			return errors.New("no supported package manager found for automatic tmux installation")
		}
		if !rootUser() {
			args = append([]string{"--", command}, args...)
			command = "sudo"
		}
	}
	cmd := exec.CommandContext(ctx, command, args...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "REET_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux installation did not complete: %w", err)
	}
	if brewDir != "" {
		if err := os.Setenv("PATH", brewDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
			return err
		}
	}
	_, err := terminalhost.FindTmux(context.Background())
	return err
}
