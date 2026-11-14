package scan

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A fenced controller refuses every write with 423 and, when its owner said
// where it moved to, names that address. A running agent follows it there on
// its own, under exactly the rule `reet agent set-server` keeps: the new
// address must first prove it holds this machine's record, and only then are
// the machine's credentials sent to it. The address is a hint and no more. An
// old controller that named the wrong place, or anyone answering in its name,
// gets a proof request with nothing secret in it and nothing else.

const agentFollowUsage = "reet agent follow-moves on|off|status [--config PATH]"

// followRetry is how long the agent waits before asking the same address
// again. A fence goes up before the new controller exists, so the first
// attempts are expected to fail; asking every cycle would only fill the log.
const followRetry = time.Minute

// followRestartFn restarts this agent process into the saved enrollment; tests
// replace it.
var followRestartFn = func() error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	return restartAgent(path)
}

// moveFollower remembers the last address tried, so a destination that is not
// ready is asked once a minute and described once.
type moveFollower struct {
	address string
	tried   time.Time
	said    string
}

// say prints a line once, until it changes.
func (f *moveFollower) say(out io.Writer, line string) {
	if f.said == line {
		return
	}
	f.said = line
	fmt.Fprintln(out, line)
}

// movedEcho keeps a waiting agent's log readable. An agent whose controller has
// moved is refused every five seconds until the new one answers, which can be
// an hour of the same sentence; it is worth saying once, and again only when
// it changes or after the agent has got through in between.
type movedEcho struct{ last string }

// worthSaying reports whether a cycle's error should be printed. Anything that
// is not a moved controller's refusal always is.
func (e *movedEcho) worthSaying(err error) bool {
	if err == nil {
		e.last = ""
		return false
	}
	to, moved := movedTo(err)
	if !moved {
		e.last = ""
		return true
	}
	// The address is part of what is said: an owner who lifts the fence and
	// raises it again with the right address has told the machines something.
	now := err.Error() + "\n" + to
	if e.last == now {
		return false
	}
	e.last = now
	return true
}

// movedTo is the address a refusal names, when the refusal is a fenced
// controller saying where it went.
func movedTo(err error) (string, bool) {
	var refusal controllerError
	if !errors.As(err, &refusal) || refusal.Status != http.StatusLocked || refusal.Code != "controller_moved" {
		return "", false
	}
	return strings.TrimSpace(refusal.MovedTo), true
}

// follow acts on a cycle's error when it is a moved controller. It reports
// whether the enrollment now points at the new address; the caller restarts.
func (f *moveFollower) follow(ctx context.Context, cycleErr error, c agentConfig, config string, out io.Writer, version string) bool {
	hint, moved := movedTo(cycleErr)
	if !moved {
		return false
	}
	if hint == "" {
		f.say(out, "This machine's controller has moved and did not say where. When you know the new address, run: reet agent set-server URL")
		return false
	}
	if c.FollowOff {
		f.say(out, fmt.Sprintf("This machine's controller says it has moved to %s. Following a move is turned off here, so nothing was changed. To follow it, run: reet agent set-server %s", hint, hint))
		return false
	}
	server, err := controllerOrigin(hint)
	if err != nil {
		f.say(out, fmt.Sprintf("This machine's controller says it has moved to %s, which this machine will not follow on its own (%v). If the address is right, run: reet agent set-server URL", hint, err))
		return false
	}
	if strings.EqualFold(server, c.Server) {
		return false
	}
	if f.address == server && time.Since(f.tried) < followRetry {
		return false
	}
	f.address, f.tried = server, time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := proveRecord(checkCtx, server, c); err != nil {
		f.say(out, fmt.Sprintf("This machine's controller says it has moved to %s, but nothing was changed: %v. No credential was sent. The agent asks again each minute.", server, err))
		return false
	}
	next := c
	next.Server = server
	if err := confirmCredentials(checkCtx, next, version); err != nil {
		f.say(out, fmt.Sprintf("%s holds this machine's record but did not accept its credentials, so nothing was changed: %v. The agent asks again each minute.", server, err))
		return false
	}
	// A setting changed on this machine since the agent started (allow-apply,
	// a terminal switch) must survive, so the file is read again and only its
	// address is replaced. An enrollment that is no longer this one is left
	// alone: somebody re-enrolled or moved the machine by hand meanwhile.
	saved, err := loadAgentConfig(config)
	if err != nil || saved.ID != c.ID || saved.Secret != c.Secret || !strings.EqualFold(saved.Server, c.Server) {
		f.say(out, "This machine's enrollment changed while its controller was moving, so the agent left it alone. Restart the agent to use it: reet agent restart")
		return false
	}
	saved.Server = server
	if err := writePrivate(config, saved); err != nil {
		f.say(out, fmt.Sprintf("The controller moved to %s and passed both checks, but the enrollment could not be saved: %v", server, err))
		return false
	}
	fmt.Fprintf(out, "This machine's controller moved to %s. It proved it holds this machine's record and accepted its credentials, so this machine now points at it (it was %s).\n", server, c.Server)
	return true
}

// runAgentFollowMoves is `reet agent follow-moves on|off|status`. Like
// allow-apply it can only be changed by a command on the machine itself.
func runAgentFollowMoves(args []string, out io.Writer) error {
	mode := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		mode, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("agent follow-moves", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprintln(out, agentFollowUsage+"\nWhether this machine follows its controller to a new address on its own (on by default).\nEither way the new controller must first prove it holds this machine's record before any credential is sent.\nA running agent picks up the change when it is restarted.")
	}
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || (mode != "on" && mode != "off" && mode != "status") {
		return errors.New("usage: " + agentFollowUsage)
	}
	absolute, err := filepath.Abs(*config)
	if err != nil {
		return err
	}
	c, err := loadAgentConfig(absolute)
	if err != nil {
		return err
	}
	if mode == "status" {
		fmt.Fprintf(out, "Following a moved controller: %s\n", onOff(!c.FollowOff))
		return nil
	}
	c.FollowOff = mode == "off"
	if err := writePrivate(absolute, c); err != nil {
		return err
	}
	if c.FollowOff {
		fmt.Fprintf(out, "%s will not follow its controller to a new address on its own. When the controller moves, run reet agent set-server URL here. Restart a running agent for this to take effect: reet agent restart\n", c.Host)
	} else {
		fmt.Fprintf(out, "%s follows its controller to a new address once that controller proves it holds this machine's record. Restart a running agent for this to take effect: reet agent restart\n", c.Host)
	}
	return nil
}
