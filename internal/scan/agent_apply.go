package scan

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const agentApplyUsage = "reet agent allow-apply on|off|status [--config PATH]"

// parseAgentApply takes the on|off|status word from the front of the
// allow-apply arguments so the rest can be parsed as ordinary flags.
func parseAgentApply(args []string, out io.Writer) (string, []string, error) {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			fmt.Fprintln(out, agentApplyUsage+"\nAllow or refuse plans approved in the web UI to change this enrolled machine's skills (off by default).\nScans and previews work either way. A running agent picks up the change on its next job.")
			return "", nil, flag.ErrHelp
		}
	}
	if len(args) == 0 || args[0] != "on" && args[0] != "off" && args[0] != "status" {
		return "", nil, errors.New("usage: " + agentApplyUsage)
	}
	return args[0], args, nil
}

// runAgentAllowApply flips the machine's local opt-in. It is deliberately only
// reachable through a command on the machine itself, never from the controller:
// that is what stops an approved plan gaining write access on its own.
func runAgentAllowApply(c agentConfig, config, mode string, out io.Writer) error {
	switch mode {
	case "on", "off":
		c.AllowApply = mode == "on"
		if err := writePrivate(config, c); err != nil {
			return err
		}
		if c.AllowApply {
			fmt.Fprintf(out, "Remote apply enabled locally for %s. Plans you approve in the web UI can now change this machine's skills; removals are backed up beside the skills folder.\n", c.Host)
		} else {
			fmt.Fprintf(out, "Remote apply disabled for %s. Scans and previews still work; approved plans wait until you turn it on again.\n", c.Host)
		}
		return nil
	default:
		fmt.Fprintf(out, "Remote apply enabled locally: %s\n", onOff(c.AllowApply))
		return nil
	}
}
