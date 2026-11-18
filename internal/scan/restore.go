package scan

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// runAgentRestore implements `reet agent restore --list` and
// `reet agent restore <name> [--date YYYY-MM-DD]`. It works on this
// machine's dated backups and needs no sync target.
func runAgentRestore(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("agent restore", flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	list := fs.Bool("list", false, "list backed-up skills")
	date := fs.String("date", "", "backup date, YYYY-MM-DD (default: newest)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	name := ""
	if fs.NArg() > 0 { // allow flags after the name
		name = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return err
		}
	}
	if fs.NArg() != 0 || (*list && name != "") || (!*list && name == "") {
		return errors.New("usage: reet agent restore --list | reet agent restore NAME [--date YYYY-MM-DD]")
	}
	home := ""
	if c, err := loadAgentConfig(*config); err == nil {
		home = c.Home
	}
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = h
	}
	if *list {
		entries, err := listBackups(home)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Fprintln(out, "No backed-up skills.")
		}
		for _, b := range entries {
			fmt.Fprintf(out, "%s  %s  from %s\n", b.Date, b.Name, b.Folder)
		}
		return nil
	}
	restored, err := restoreBackup(home, name, *date)
	for _, p := range restored { // what was put back is said even when a later folder failed
		fmt.Fprintf(out, "Restored %s\n", p)
	}
	return err
}
