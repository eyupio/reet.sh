// Package systeminstall builds and runs the operating system's own package
// install command. reet shows the command before running it and never reads a
// password; sudo prompts on the terminal when it needs one.
package systeminstall

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// Request names the packages each platform's manager should install.
type Request struct {
	Linux []string
	Brew  []string
}

// Plan is one concrete install command for this machine.
type Plan struct {
	Label   string
	Command []string
}

type manager struct {
	label  string
	binary string
	args   []string
	root   bool
}

var managers = []manager{
	{label: "apt-get", binary: "apt-get", args: []string{"install", "-y"}, root: true},
	{label: "dnf", binary: "dnf", args: []string{"install", "-y"}, root: true},
	{label: "yum", binary: "yum", args: []string{"install", "-y"}, root: true},
	{label: "zypper", binary: "zypper", args: []string{"install", "-y"}, root: true},
	{label: "pacman", binary: "pacman", args: []string{"-S", "--noconfirm"}, root: true},
	{label: "apk", binary: "apk", args: []string{"add"}, root: true},
}

// For returns the install command for this machine, if a supported package
// manager is available. It never runs anything.
func For(request Request) (Plan, bool) {
	return detect(runtime.GOOS, request, os.Geteuid, exec.LookPath)
}

func detect(goos string, request Request, geteuid func() int, lookPath func(string) (string, error)) (Plan, bool) {
	if goos == "darwin" {
		if len(request.Brew) == 0 {
			return Plan{}, false
		}
		if _, err := lookPath("brew"); err != nil {
			return Plan{}, false
		}
		command := append([]string{"brew", "install"}, request.Brew...)
		return Plan{Label: "Homebrew", Command: command}, true
	}
	if goos != "linux" || len(request.Linux) == 0 {
		return Plan{}, false
	}
	for _, candidate := range managers {
		if _, err := lookPath(candidate.binary); err != nil {
			continue
		}
		command := append([]string{candidate.binary}, candidate.args...)
		command = append(command, request.Linux...)
		if candidate.root && geteuid() != 0 {
			if _, err := lookPath("sudo"); err != nil {
				continue
			}
			command = append([]string{"sudo"}, command...)
		}
		return Plan{Label: candidate.label, Command: command}, true
	}
	return Plan{}, false
}

// Run starts the plan with the real terminal attached so sudo can ask for a
// password. The password is never read, stored or logged by reet.
func Run(ctx context.Context, plan Plan, in io.Reader, out io.Writer) error {
	if len(plan.Command) == 0 {
		return fmt.Errorf("no system install command")
	}
	command := exec.CommandContext(ctx, plan.Command[0], plan.Command[1:]...)
	command.Stdin = in
	command.Stdout = out
	command.Stderr = out
	return command.Run()
}
