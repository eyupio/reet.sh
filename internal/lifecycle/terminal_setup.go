package lifecycle

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

func interactiveCommand(in io.Reader, out io.Writer) func(context.Context, string, ...string) error {
	return func(ctx context.Context, name string, args ...string) error {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdin = in
		cmd.Stdout = out
		cmd.Stderr = out
		return cmd.Run()
	}
}
func terminalSetupCommand(d *deployment, state string) []string {
	if d.Mode == "binary" {
		return []string{d.Binary, "agent", "terminal", "setup"}
	}
	args := []string{"terminal", "start", "--data-dir", d.Data, "--skip-host-setup"}
	if d.Mode == "compose" {
		return append([]string{"docker"}, composeArgs(state, "exec", "reet", "/reet", "terminal", "start", "--data-dir", "/data", "--skip-host-setup")...)
	}
	return append([]string{d.Binary}, args...)
}
func quotedCommand(args []string) string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = shellQuote(arg)
	}
	return strings.Join(out, " ")
}

// Deployment success is independent of optional connection setup. Neither --yes
// nor an unattended installation grants consent or starts another prompt.
func (a *App) offerTerminalSetup(d *deployment, state string, running, skip bool) {
	command := terminalSetupCommand(d, state)
	fmt.Fprintln(a.Out, "\nTerminal connection setup")
	fmt.Fprintf(a.Out, "Resume the connection wizard: %s\n", quotedCommand(command))
	if d.Mode != "binary" {
		fmt.Fprintf(a.Out, "Connect this host separately: %s\n", quotedCommand([]string{d.Binary, "agent", "terminal", "setup"}))
	}
	if skip || !clientconfig.Interactive(a.In) {
		return
	}
	if d.Mode != "binary" && !running {
		fmt.Fprintln(a.Out, "Start the controller and complete its owner account setup, then run the wizard above. The controller remains stopped.")
		return
	}
	var answer string
	for {
		value, err := clientconfig.Ask(a.In, a.Out, "Set up terminal connections now? (yes/no)", "yes")
		if err != nil {
			fmt.Fprintln(a.Out, "Connection setup deferred. Your installation is saved.")
			return
		}
		answer = strings.ToLower(strings.TrimSpace(value))
		if answer == "y" || answer == "yes" || answer == "n" || answer == "no" {
			break
		}
		fmt.Fprintln(a.Out, "Choose yes or no.")
	}
	if answer == "n" || answer == "no" {
		fmt.Fprintln(a.Out, "Connection setup deferred. Resume whenever you are ready.")
		return
	}
	if a.interactiveRun == nil {
		a.interactiveRun = interactiveCommand(a.In, a.Out)
	}
	if err := a.interactiveRun(context.Background(), command[0], command[1:]...); err != nil {
		fmt.Fprintln(a.Out, "Terminal setup paused. Your installation is saved; finish any account/authentication or controller restart step shown above, then resume the wizard.")
		return
	}
	if d.Mode != "binary" {
		fmt.Fprintln(a.Out, "\nNow connect this host to the AI workspace. Local shell access still needs your explicit consent.")
		if err := a.interactiveRun(context.Background(), d.Binary, "agent", "terminal", "setup"); err != nil {
			fmt.Fprintln(a.Out, "Host setup paused. The controller installation is saved. Resume with reet agent terminal setup.")
		}
	}
}
