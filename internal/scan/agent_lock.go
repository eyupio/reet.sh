package scan

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const agentLockUsage = "reet agent lock|unlock [--config PATH]; reet agent lock status [--config PATH]"

// parseAgentLock reads lock, unlock and lock status. Like allow-apply it
// returns the arguments with a placeholder first, so the rest parse as flags.
func parseAgentLock(args []string, out io.Writer) (string, []string, error) {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			fmt.Fprintln(out, agentLockUsage+"\nLock this machine against its controller: while locked it refuses every launch, attach, chat, upload, skill install and reet update the controller starts.\nRunning sessions keep running, ending a session still works, and inventory scans and health reports continue. Only a command on this machine locks or unlocks it.\nA running agent picks up the change within 5 seconds.")
			return "", nil, flag.ErrHelp
		}
	}
	if args[0] == "unlock" {
		return "off", args, nil
	}
	if len(args) > 1 && args[1] == "status" {
		return "status", args[1:], nil
	}
	return "on", args, nil
}

// runAgentLock flips the machine's lock. It is reachable only through a
// command on the machine itself: no controller message or job writes it.
func runAgentLock(c agentConfig, config, mode string, out io.Writer) error {
	switch mode {
	case "on", "off":
		c.Locked = mode == "on"
		if err := writePrivate(config, c); err != nil {
			return err
		}
		if c.Locked {
			fmt.Fprintf(out, "Locked %s. It now refuses every launch, attach, chat, upload, skill install and update its controller starts. Running sessions keep running; scans and health reports continue. Run reet agent unlock to lift it.\n", c.Host)
		} else {
			fmt.Fprintf(out, "Unlocked %s. Its controller can start sessions, chats and updates here again.\n", c.Host)
		}
		return nil
	case "status":
		fmt.Fprintf(out, "Locked on this machine: %s\n", onOff(c.Locked))
		return nil
	}
	return errors.New("usage: " + agentLockUsage)
}
