package cliui

import (
	"fmt"
	"io"
	"strings"
)

// Command is the catalog shared by home, help and the generated reference.
// Args go to the existing handler verbatim; home never supplies consent.
type Command struct {
	Name, Title, Summary, Group string
	Args                        []string
	Children                    []Command
	Expand                      bool
}

func Commands() []Command {
	return []Command{
		{Name: "doctor", Title: "Doctor", Summary: "Check this installation", Group: "Check & maintain", Args: []string{"doctor"}},
		{Name: "healthcheck", Title: "Healthcheck", Summary: "Check a server's /healthz", Group: "Check & maintain", Args: []string{"healthcheck"}},
		{Name: "upgrade", Title: "Upgrade", Summary: "Check, verify and install a release", Group: "Check & maintain", Args: []string{"upgrade"}},
		{Name: "backup", Title: "Backups", Summary: "List, take and restore sealed copies", Group: "Check & maintain", Args: []string{"backup"}, Children: []Command{
			{Name: "backup list", Title: "List backups", Summary: "Read local instance copies", Args: []string{"backup", "list"}},
			{Name: "backup run", Title: "Take a backup", Summary: "Write a sealed instance copy", Args: []string{"backup", "run"}},
			{Name: "backup restore", Title: "Restore a backup", Summary: "Press e to add the backup ID and options", Args: []string{"backup", "restore"}},
			{Name: "backup open", Title: "Open a backup", Summary: "Press e to add a file and options", Args: []string{"backup", "open"}},
		}},
		{Name: "branches", Title: "Branches", Summary: "List merged local branches and prune them", Group: "Check & maintain", Args: []string{"branches"}, Children: []Command{
			{Name: "branches status", Title: "Branch status", Summary: "Read which local branches are merged", Args: []string{"branches", "status"}},
			{Name: "branches prune", Title: "Prune branches", Summary: "Press e to add --dry-run, --yes or a folder", Args: []string{"branches", "prune"}},
		}},
		{Name: "scan", Title: "Scan", Summary: "Report installed skills to your account", Group: "Skills & setups", Args: []string{"scan"}},
		{Name: "sync", Title: "Sync", Summary: "Preview a saved target; --apply asks first", Group: "Skills & setups", Args: []string{"sync"}},
		{Name: "setup", Title: "Setups", Summary: "Discover, save and restore agent setups", Group: "Skills & setups", Args: []string{"setup"}, Children: []Command{
			{Name: "setup scan", Title: "Discover setups", Summary: "Inspect known local setup locations", Args: []string{"setup", "scan"}},
			{Name: "setup profile", Title: "Setup profiles", Summary: "Open the existing create/push wizard", Args: []string{"setup", "profile"}},
			{Name: "setup list", Title: "List snapshots", Summary: "Read local snapshot headers", Args: []string{"setup", "list"}},
			{Name: "setup backup", Title: "Back up a setup", Summary: "Preview files selected for a snapshot", Args: []string{"setup", "backup", "--preview"}},
			{Name: "setup restore", Title: "Restore a setup", Summary: "Press e to select a snapshot or profile", Args: []string{"setup", "restore"}},
			{Name: "setup rollback", Title: "Roll back a restore", Summary: "Press e to select a journal or profile", Args: []string{"setup", "rollback"}},
		}},
		{Name: "agent", Title: "Agent", Summary: "Enroll a machine and manage its local agent", Group: "Machines & access", Args: []string{"agent"}, Expand: true, Children: []Command{
			{Name: "agent enroll", Title: "Connect machine", Summary: "Enroll with a controller", Group: "Machines & access", Args: []string{"agent", "enroll"}},
			{Name: "agent status", Title: "Agent status", Summary: "This machine's enrollment and daemon", Group: "Machines & access", Args: []string{"agent", "status"}},
		}},
		{Name: "terminal", Title: "Terminal", Summary: "Manage controller terminal access", Group: "Machines & access", Args: []string{"terminal"}, Children: []Command{
			{Name: "terminal status", Title: "Terminal status", Summary: "Inspect controller terminal availability", Args: []string{"terminal", "status"}},
			{Name: "terminal start", Title: "Start terminal access", Summary: "Use the existing connection setup flow", Args: []string{"terminal", "start"}},
			{Name: "terminal stop", Title: "Stop terminal access", Summary: "Preserve running host shells", Args: []string{"terminal", "stop"}},
			{Name: "terminal restart", Title: "Restart terminal access", Summary: "Restart controller terminal access", Args: []string{"terminal", "restart"}},
		}},
		{Name: "configure", Title: "Configure", Summary: "Save connection and release defaults", Group: "More commands", Args: []string{"configure"}},
		{Name: "install", Title: "Install", Summary: "Set up a binary or managed deployment", Group: "More commands", Args: []string{"install"}},
		{Name: "serve", Title: "Serve", Summary: "Run the web app (also the non-interactive default)", Group: "More commands", Args: []string{"serve"}},
		{Name: "version", Title: "Version", Summary: "Binary version and source commit", Group: "More commands", Args: []string{"version"}},
		{Name: "help", Title: "Help", Summary: "Read the command reference", Group: "More commands", Args: []string{"help"}},
	}
}

func HomeCommands() []Command {
	var result []Command
	for _, c := range Commands() {
		if c.Expand {
			result = append(result, c.Children...)
		} else {
			result = append(result, c)
		}
	}
	return result
}

func Help(out io.Writer) {
	fmt.Fprint(out, "reet.sh — your AI agent toolkit. "+Tagline+"\n\nUsage: reet <command> [options]\n\n")
	group := ""
	for _, c := range Commands() {
		if c.Group != group {
			group = c.Group
			fmt.Fprintln(out, group)
		}
		fmt.Fprintf(out, "  %-13s %s\n", c.Name, c.Summary)
	}
	fmt.Fprint(out, "\nRun reet in an interactive terminal for home. Use ↑/↓ or j/k, Enter, / to search,\nEsc to go back and q to quit. Press e to edit a selected command's arguments,\nor ? to read its help. Arguments are parsed as words, never executed by a shell.\n\nRun reet <command> --help for options. init/update alias install/upgrade.\nREET_NO_TUI=1 keeps the no-command serve entry point. --tui is a compatibility\nalias for running a command and returning home, without a separate dashboard.\n")
}

// Reference is generated by cmd/command-reference; do not hand-edit its output.
func Reference(out io.Writer) {
	fmt.Fprint(out, "<!-- Generated by go run ./cmd/command-reference; do not edit. -->\n# Command reference\n\nRun `reet` in a terminal for the grouped home. Direct commands keep their existing\nflags, authentication and output. Use `reet COMMAND --help` for current options.\n\n")
	for _, c := range Commands() {
		fmt.Fprintf(out, "## reet %s\n\n%s.\n\n", c.Name, strings.TrimSuffix(c.Summary, "."))
		for _, child := range c.Children {
			fmt.Fprintf(out, "- `reet %s` — %s.\n", strings.Join(child.Args, " "), child.Summary)
		}
		if len(c.Children) > 0 {
			fmt.Fprintln(out)
		}
	}
	fmt.Fprint(out, "`reet init` and `reet update` alias `install` and `upgrade`.\n\nThe old dashboard is removed. `reet tui` is not a command. The legacy `--tui`\nflag runs the selected CLI command and offers a return home. It does not create\na second implementation of that command.\n")
}
