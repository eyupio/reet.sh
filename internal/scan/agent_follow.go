package scan

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
// ready is asked once a minute and described once. pending is the address the
// fence named, kept on disk as MovingTo, so the move goes on after a restart
// and after the old controller stops answering.
type moveFollower struct {
	address string
	tried   time.Time
	said    string
	pending string
	// held is what the old controller was last told this machine holds back
	// for, so it is told once, not every cycle.
	held string
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

// follow acts on a cycle's error when it is a moved controller, or when the
// controller this machine was told is moving has stopped answering. It reports
// whether the enrollment now points at the new address; the caller restarts.
func (f *moveFollower) follow(ctx context.Context, cycleErr error, c agentConfig, config string, out io.Writer, version string) bool {
	hint, moved := movedTo(cycleErr)
	switch {
	case moved && hint == "":
		// The fence is the authority on where to go. One that no longer
		// names an address has withdrawn the one this machine kept.
		f.forget(config, c)
		f.say(out, "This machine's controller has moved and did not say where. It follows on its own once the controller's owner gives the new address on the Move page; or run: reet agent set-server URL")
		return false
	case !moved && (cycleErr == nil || f.pending == ""):
		return false
	case !moved && controllerAnswered(cycleErr):
		// The controller took a request without its fence, so the move is
		// off; going to the kept address now would split the machines
		// between two live controllers.
		f.settled(config, c, out)
		return false
	case !moved && !controllerUnreachable(cycleErr):
		// A local failure (a scan, a file) says nothing about where the
		// controller is.
		return false
	case !moved:
		// The old controller is not answering, but it said where it went
		// before it stopped; that is still where to look.
		hint = f.pending
	}
	if c.FollowOff {
		f.tellHeld(ctx, c, "follow-off", hint, version, moved)
		f.say(out, fmt.Sprintf("This machine's controller says it has moved to %s. Following a move is turned off here, so nothing was changed. To follow it, run: reet agent set-server %s", hint, hint))
		return false
	}
	server, err := controllerOrigin(hint)
	if err != nil {
		if moved {
			// The fence now names an address this machine will not use,
			// which replaces the one it kept.
			f.forget(config, c)
		}
		f.tellHeld(ctx, c, "address-refused", hint, version, moved)
		f.say(out, fmt.Sprintf("This machine's controller says it has moved to %s, which this machine will not follow on its own (%v). If the address is right, run: reet agent set-server URL", hint, err))
		return false
	}
	if strings.EqualFold(server, c.Server) {
		return false
	}
	if moved {
		f.remember(config, c, server)
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
	saved.MovingTo = ""
	saved.LeftFrom, saved.LeftAt = c.Server, time.Now().UnixMilli()
	if err := writePrivate(config, saved); err != nil {
		f.say(out, fmt.Sprintf("The controller moved to %s and passed both checks, but the enrollment could not be saved: %v", server, err))
		return false
	}
	f.pending = ""
	(&departureReport{}).report(ctx, config, version)
	fmt.Fprintf(out, "This machine's controller moved to %s. It proved it holds this machine's record and accepted its credentials, so this machine now points at it (it was %s).\n", server, c.Server)
	return true
}

// remember keeps the address a fence named in the enrollment, so the move
// survives a restart of this agent and the old controller being switched off
// before the new one answers. Only the hint is written, into the file as it is
// now, and only while it is still this enrollment.
func (f *moveFollower) remember(config string, c agentConfig, server string) {
	if f.pending == server {
		return
	}
	saved, err := loadAgentConfig(config)
	if err != nil || saved.ID != c.ID || saved.Secret != c.Secret || !strings.EqualFold(saved.Server, c.Server) {
		return
	}
	saved.MovingTo = server
	if writePrivate(config, saved) == nil {
		f.pending = server
	}
}

// controllerUnreachable is a cycle error that means the controller did not
// answer at all: the connection failed, or a proxy in front of it says it is
// down. Only then does a kept address stand in for it.
func controllerUnreachable(err error) bool {
	var refusal controllerError
	if errors.As(err, &refusal) {
		switch refusal.Status {
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return false
	}
	var transport *url.Error
	if errors.As(err, &transport) {
		return true
	}
	var network net.Error
	return errors.As(err, &network)
}

// controllerAnswered is a cycle error the controller itself gave without its
// fence. A fenced controller refuses every agent request with 423 before
// anything else, so any other answer means it is taking requests again.
func controllerAnswered(err error) bool {
	var refusal controllerError
	if !errors.As(err, &refusal) || controllerUnreachable(err) {
		return false
	}
	_, moved := movedTo(err)
	return !moved
}

// forget drops a kept address the fence has withdrawn.
func (f *moveFollower) forget(config string, c agentConfig) {
	if f.pending == "" {
		return
	}
	saved, err := loadAgentConfig(config)
	if err != nil || saved.ID != c.ID || saved.Secret != c.Secret {
		return
	}
	if saved.MovingTo != "" {
		saved.MovingTo = ""
		if writePrivate(config, saved) != nil {
			return
		}
	}
	f.pending = ""
}

// settled forgets a move that is off: the controller this machine points at
// answered without its fence, so the fence was lifted.
func (f *moveFollower) settled(config string, c agentConfig, out io.Writer) {
	// A fence raised again later is a new move, and whatever this machine
	// holds back for is worth saying to it again.
	f.held = ""
	if f.pending == "" {
		return
	}
	saved, err := loadAgentConfig(config)
	if err != nil || saved.ID != c.ID || saved.Secret != c.Secret {
		return
	}
	if saved.MovingTo != "" {
		saved.MovingTo = ""
		if writePrivate(config, saved) != nil {
			return
		}
	}
	f.pending, f.held = "", ""
	fmt.Fprintf(out, "%s answers again, so the move it announced is off; this machine stays with it.\n", c.Server)
}

// tellHeld tells the fenced controller, once for each address it names, that
// this machine will not follow on its own, so its owner sees which machines
// need a hand. It is said only to a controller that is answering with its
// fence. A new address is a new question, so a refusal of it is said again.
func (f *moveFollower) tellHeld(ctx context.Context, c agentConfig, reason, address, version string, answering bool) {
	key := reason + "\n" + address
	if !answering || f.held == key {
		return
	}
	tellCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if tellOldFn(tellCtx, c, map[string]string{"held": reason}, version) == nil {
		f.held = key
	}
}

// fenceLeftPath is where a machine tells the controller it is leaving.
const fenceLeftPath = "/api/v2/sync/moved"

// departureGiveUp is how long a machine keeps trying to tell the controller
// it left; by then that controller has been retired or the owner has moved on.
const departureGiveUp = 7 * 24 * time.Hour

// departureReport tells the controller this machine moved away from
// (LeftFrom) that it now points at its saved address, so the old controller's
// Move page counts it as moved. The machine's credentials go only to the
// controller it was already using. A report the old controller did not take
// for want of an answer is kept in the enrollment and sent again, once a
// minute; any answer from it ends the matter, as does a week passing.
type departureReport struct{ tried time.Time }

func (d *departureReport) report(ctx context.Context, config, version string) {
	if !d.tried.IsZero() && time.Since(d.tried) < followRetry {
		return
	}
	c, err := loadAgentConfig(config)
	if err != nil || c.LeftFrom == "" {
		return
	}
	d.tried = time.Now()
	if time.Since(time.UnixMilli(c.LeftAt)) < departureGiveUp {
		old := c
		old.Server = c.LeftFrom
		tellCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = tellOldFn(tellCtx, old, map[string]string{"to": c.Server}, version)
		cancel()
		// Only a controller that did not answer is asked again: a refusal,
		// or an old controller that does not know the route, is final.
		if err != nil && (!errors.As(err, new(controllerError)) || controllerUnreachable(err)) {
			return
		}
	}
	// Read again so only the report is cleared, never a setting changed meanwhile.
	saved, err := loadAgentConfig(config)
	if err != nil || saved.ID != c.ID || saved.LeftFrom != c.LeftFrom {
		return
	}
	saved.LeftFrom, saved.LeftAt = "", 0
	_ = writePrivate(config, saved)
}

// tellOldFn sends what a machine says of a move to the controller it is
// leaving; tests replace it.
var tellOldFn = tellOld

func tellOld(ctx context.Context, c agentConfig, body map[string]string, version string) error {
	return agentRequestWith(ctx, noRedirectClient, c, fenceLeftPath, body, nil, version)
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
