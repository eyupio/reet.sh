package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/syncproof"
)

// controlAgentFn is how the agent is restarted; tests replace it.
var controlAgentFn = controlAgent

// runAgentSetServer is `reet agent set-server URL`: it points this machine's
// enrollment at a controller that holds the same machine record, for when a
// controller has moved and this machine should follow.
//
// The machine's credentials are never sent to an address that has not first
// proved it holds this machine's record (see internal/syncproof). Two checks
// run, in order, and the enrollment file is written only when both pass:
//
//  1. proof: the controller answers a fresh nonce with an HMAC only a holder of
//     this machine's record can make. Nothing secret is sent: the request has
//     the machine id and the nonce.
//  2. credentials: with the proof in hand, the machine's own credentials are
//     sent once to ask the controller who they belong to, and the answer must
//     be this machine.
//
// On any failure nothing is changed and the failing check is named.
func runAgentSetServer(args []string, out io.Writer, version string) error {
	// The address comes first or last; flags may follow or precede it.
	var address string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		address, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("agent set-server", flag.ContinueOnError)
	fs.SetOutput(out)
	config := fs.String("config", agentConfigPath(), "private machine enrollment file")
	servicePrefix := fs.String("service-prefix", "reet", "native service name prefix")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if address == "" && fs.NArg() == 1 {
		address = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return errors.New("usage: reet agent set-server URL [--config PATH]")
	}
	if address == "" {
		return errors.New("usage: reet agent set-server URL [--config PATH]")
	}
	absolute, err := filepath.Abs(*config)
	if err != nil {
		return err
	}
	server, err := controllerOrigin(address)
	if err != nil {
		return err
	}
	c, err := loadAgentConfig(absolute)
	if err != nil {
		return err
	}
	if c.ID == "" || c.Secret == "" {
		return errors.New("this installation is not enrolled with a controller; enroll it first")
	}
	if strings.EqualFold(c.Server, server) {
		fmt.Fprintf(out, "This machine already points at %s; nothing changed.\n", server)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := proveRecord(ctx, server, c); err != nil {
		return fmt.Errorf("nothing was changed. Check 1 of 2 failed (the controller must prove it holds this machine's record): %w", err)
	}
	fmt.Fprintf(out, "Check 1 of 2 passed: %s holds this machine's record.\n", server)
	moved := c
	moved.Server = server
	moved.MovingTo = ""
	if err := confirmCredentials(ctx, moved, version); err != nil {
		return fmt.Errorf("nothing was changed. Check 2 of 2 failed (the controller must accept this machine's credentials): %w", err)
	}
	fmt.Fprintln(out, "Check 2 of 2 passed: it accepts this machine's credentials.")
	// Only the address changes; the identity, secret and settings are kept.
	if err := writePrivate(absolute, moved); err != nil {
		return fmt.Errorf("nothing was changed: could not save the enrollment: %w", err)
	}
	fmt.Fprintf(out, "This machine now points at %s (it was %s).\n", server, c.Server)
	// The old controller's Move page counts this machine as moved.
	tellLeft(ctx, c, server, version)
	if err := controlAgentFn("restart", absolute, *servicePrefix, out); err != nil {
		return fmt.Errorf("the address was saved, but the agent could not be restarted (%w); run reet agent restart", err)
	}
	return nil
}

// proveRecord asks the controller at server to prove it holds this machine's
// record. It sends the machine id and a fresh nonce, no credential, and never
// follows a redirect, so the answer comes from the address that was given.
func proveRecord(ctx context.Context, server string, c agentConfig) error {
	nonce, err := syncproof.NewNonce()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"id": c.ID, "nonce": nonce})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+syncproof.Path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", server, err)
	}
	defer resp.Body.Close()
	var answer struct {
		Proof string `json:"proof"`
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 8<<10)).Decode(&answer)
	switch {
	case resp.StatusCode == http.StatusLocked:
		return errors.New("that controller has itself moved and is not accepting changes; point this machine at the controller it moved to")
	case resp.StatusCode == http.StatusNotFound && answer.Code == "unknown_machine":
		return errors.New("that controller has no record of this machine. It is not the controller this machine was enrolled with, nor a copy of it")
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
		return errors.New("that controller does not support the proof request; it is not a reet controller of a release that has it. Upgrade it first")
	case resp.StatusCode >= 300:
		return fmt.Errorf("that controller answered %d", resp.StatusCode)
	}
	if !syncproof.Verify(c.Secret, c.ID, nonce, answer.Proof) {
		return errors.New("that controller answered, but its proof does not match this machine's record, so it does not hold it")
	}
	return nil
}

// confirmCredentials sends the machine's credentials to a controller that has
// already proved it holds the record, and checks it names this machine.
func confirmCredentials(ctx context.Context, c agentConfig, version string) error {
	var got struct {
		ID   string
		Host string
	}
	if err := agentRequestWith(ctx, noRedirectClient, c, "/api/v2/sync/identity", map[string]string{}, &got, version); err != nil {
		var ce controllerError
		if errors.As(err, &ce) {
			return fmt.Errorf("it answered %d: %s", ce.Status, ce.Message)
		}
		return err
	}
	if got.ID != c.ID {
		return errors.New("it named a different machine than this one")
	}
	return nil
}
