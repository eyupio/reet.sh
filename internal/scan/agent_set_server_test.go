package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/syncproof"
)

const (
	testMachineID     = "machine-0123456789abcdef"
	testMachineSecret = "this-machines-very-private-secret"
	testMachineToken  = "personal-token-for-enrolment"
)

type seen struct {
	path    string
	headers http.Header
	body    string
}

// controller is a fake controller that records every request it gets.
type controller struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []seen
}

func (c *controller) requests() []seen {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]seen(nil), c.reqs...)
}

func newController(t *testing.T, handler func(c *controller, w http.ResponseWriter, r *http.Request, body []byte)) *controller {
	t.Helper()
	c := &controller{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.reqs = append(c.reqs, seen{path: r.URL.Path, headers: r.Header.Clone(), body: string(b)})
		c.mu.Unlock()
		handler(c, w, r, b)
	}))
	t.Cleanup(c.Close)
	return c
}

// honest holds this machine's record: it proves it, and names the machine for
// its credentials.
func honest(t *testing.T) *controller {
	return newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, body []byte) {
		switch r.URL.Path {
		case syncproof.Path:
			var in struct{ ID, Nonce string }
			json.Unmarshal(body, &in)
			json.NewEncoder(w).Encode(map[string]string{"proof": syncproof.Compute(syncproof.HashSecret(testMachineSecret), in.ID, in.Nonce)})
		case "/api/v2/sync/identity":
			if r.Header.Get("X-Reet-Agent") != testMachineID || r.Header.Get("X-Reet-Agent-Key") != testMachineSecret {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unknown machine"})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"id": testMachineID, "host": "studio"})
		default:
			http.NotFound(w, r)
		}
	})
}

func enrolledMachine(t *testing.T, server string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	hostname, _ := os.Hostname()
	path := filepath.Join(home, "agent.json")
	c := agentConfig{Server: server, Token: testMachineToken, ID: testMachineID, Secret: testMachineSecret, Host: "studio", Hostname: hostname, Home: home, AllowApply: true, TerminalEnabled: true, BrowseOff: true}
	if err := writePrivate(path, c); err != nil {
		t.Fatal(err)
	}
	return path
}

func withRestart(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	old := controlAgentFn
	controlAgentFn = func(action, config, prefix string, out io.Writer) error {
		calls = append(calls, action+" "+config)
		return err
	}
	t.Cleanup(func() { controlAgentFn = old })
	return &calls
}

func fileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// noCredentialEverSent is the rule the command exists to keep.
func noCredentialEverSent(t *testing.T, c *controller) {
	t.Helper()
	for _, r := range c.requests() {
		for _, secret := range []string{testMachineSecret, testMachineToken} {
			if strings.Contains(r.body, secret) {
				t.Fatalf("a credential was sent in the body of %s", r.path)
			}
			for name, values := range r.headers {
				for _, v := range values {
					if strings.Contains(v, secret) {
						t.Fatalf("a credential was sent in header %s of %s", name, r.path)
					}
				}
			}
		}
	}
}

func setServer(t *testing.T, config, address string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := RunAgentInput([]string{"set-server", address, "--config", config}, strings.NewReader(""), &out, "test")
	return out.String(), err
}

// An address that does not hold the machine's record is refused, and the
// machine's credentials are never sent to it.
func TestSetServerRefusesAControllerWithNoRecordAndSendsItNoCredential(t *testing.T) {
	old := honest(t)
	config := enrolledMachine(t, old.URL)
	before := fileBytes(t, config)
	restarts := withRestart(t, nil)
	stranger := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "no record", "code": "unknown_machine"})
	})
	_, err := setServer(t, config, stranger.URL)
	if err == nil || !strings.Contains(err.Error(), "Check 1 of 2") || !strings.Contains(err.Error(), "no record of this machine") {
		t.Fatalf("got %v", err)
	}
	noCredentialEverSent(t, stranger)
	if n := len(stranger.requests()); n != 1 || stranger.requests()[0].path != syncproof.Path {
		t.Fatalf("the stranger was sent %d requests: %+v", n, stranger.requests())
	}
	if !bytes.Equal(before, fileBytes(t, config)) || len(*restarts) != 0 {
		t.Fatal("a failed check changed the enrollment or restarted the agent")
	}
}

// A controller that answers with a proof it could not have made is refused too.
func TestSetServerRefusesAForgedProof(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	restarts := withRestart(t, nil)
	impostor := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, body []byte) {
		var in struct{ ID, Nonce string }
		json.Unmarshal(body, &in)
		// It knows the id and the nonce, and guesses at the hash.
		json.NewEncoder(w).Encode(map[string]string{"proof": syncproof.Compute("a-guess", in.ID, in.Nonce)})
	})
	_, err := setServer(t, config, impostor.URL)
	if err == nil || !strings.Contains(err.Error(), "Check 1 of 2") || !strings.Contains(err.Error(), "does not hold it") {
		t.Fatalf("got %v", err)
	}
	noCredentialEverSent(t, impostor)
	if !bytes.Equal(before, fileBytes(t, config)) || len(*restarts) != 0 {
		t.Fatal("a failed check changed the enrollment or restarted the agent")
	}
}

// A proof is for one nonce. A controller that replays an earlier one is refused.
func TestSetServerRefusesAReplayedProof(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	withRestart(t, nil)
	stale := syncproof.Compute(syncproof.HashSecret(testMachineSecret), testMachineID, strings.Repeat("a", 64))
	replayer := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, _ []byte) {
		json.NewEncoder(w).Encode(map[string]string{"proof": stale})
	})
	if _, err := setServer(t, config, replayer.URL); err == nil || !strings.Contains(err.Error(), "Check 1 of 2") {
		t.Fatalf("a replayed proof was accepted: %v", err)
	}
	noCredentialEverSent(t, replayer)
	if !bytes.Equal(before, fileBytes(t, config)) {
		t.Fatal("the enrollment changed")
	}
}

// A redirect is not followed: the answer must come from the address given, and
// the address it points to is sent nothing.
func TestSetServerDoesNotFollowARedirectWithAnything(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	withRestart(t, nil)
	elsewhere := honest(t)
	redirecting := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, _ []byte) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	})
	if _, err := setServer(t, config, redirecting.URL); err == nil || !strings.Contains(err.Error(), "Check 1 of 2") {
		t.Fatalf("got %v", err)
	}
	if n := len(elsewhere.requests()); n != 0 {
		t.Fatalf("a redirect target was sent %d requests", n)
	}
	noCredentialEverSent(t, redirecting)
	if !bytes.Equal(before, fileBytes(t, config)) {
		t.Fatal("the enrollment changed")
	}
}

// Success keeps every credential and changes only the address, then restarts.
func TestSetServerKeepsTheCredentialsChangesOnlyTheAddressAndRestarts(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	restarts := withRestart(t, nil)
	next := honest(t)
	out, err := setServer(t, config, next.URL+"/")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	var was, now map[string]any
	json.Unmarshal(before, &was)
	json.Unmarshal(fileBytes(t, config), &now)
	if now["server"] != next.URL || was["server"] != "https://old.example.com" {
		t.Fatalf("server: %v then %v", was["server"], now["server"])
	}
	delete(was, "server")
	delete(now, "server")
	if !jsonEqual(was, now) {
		t.Fatalf("something other than the address changed:\nwas %v\nnow %v", was, now)
	}
	if len(*restarts) != 1 || !strings.HasPrefix((*restarts)[0], "restart ") {
		t.Fatalf("restarts: %v", *restarts)
	}
	// The proof comes first and carries nothing secret; only then are the
	// credentials sent.
	reqs := next.requests()
	if len(reqs) != 2 || reqs[0].path != syncproof.Path || reqs[1].path != "/api/v2/sync/identity" {
		t.Fatalf("requests: %+v", reqs)
	}
	if reqs[0].headers.Get("X-Reet-Agent-Key") != "" || reqs[0].headers.Get("X-Reet-Token") != "" || strings.Contains(reqs[0].body, testMachineSecret) {
		t.Fatal("the proof request carried a credential")
	}
	if !strings.Contains(out, "Check 1 of 2 passed") || !strings.Contains(out, "Check 2 of 2 passed") {
		t.Fatalf("output: %s", out)
	}
}

func jsonEqual(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// The controller proves it holds the record but refuses the credentials: the
// second check fails and nothing changes.
func TestSetServerNamesTheSecondCheckWhenTheCredentialsAreRefused(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	restarts := withRestart(t, nil)
	c := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, body []byte) {
		if r.URL.Path == syncproof.Path {
			var in struct{ ID, Nonce string }
			json.Unmarshal(body, &in)
			json.NewEncoder(w).Encode(map[string]string{"proof": syncproof.Compute(syncproof.HashSecret(testMachineSecret), in.ID, in.Nonce)})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "that account is disabled"})
	})
	_, err := setServer(t, config, c.URL)
	if err == nil || !strings.Contains(err.Error(), "Check 2 of 2") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("got %v", err)
	}
	if !bytes.Equal(before, fileBytes(t, config)) || len(*restarts) != 0 {
		t.Fatal("a failed check changed the enrollment or restarted the agent")
	}
}

func TestSetServerSaysSoWhenTheRestartFails(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	withRestart(t, errors.New("no service manager"))
	next := honest(t)
	_, err := setServer(t, config, next.URL)
	if err == nil || !strings.Contains(err.Error(), "the address was saved") || !strings.Contains(err.Error(), "reet agent restart") {
		t.Fatalf("got %v", err)
	}
	if c, lerr := loadAgentConfig(config); lerr != nil || c.Server != next.URL {
		t.Fatalf("the address was not saved: %+v %v", c, lerr)
	}
}

func TestSetServerToTheSameAddressChangesNothingAndDoesNotRestart(t *testing.T) {
	next := honest(t)
	config := enrolledMachine(t, next.URL)
	before := fileBytes(t, config)
	restarts := withRestart(t, nil)
	out, err := setServer(t, config, next.URL)
	if err != nil || !strings.Contains(out, "nothing changed") || len(next.requests()) != 0 || len(*restarts) != 0 {
		t.Fatalf("%v %q %d %v", err, out, len(next.requests()), *restarts)
	}
	if !bytes.Equal(before, fileBytes(t, config)) {
		t.Fatal("the enrollment changed")
	}
}

func TestSetServerRefusesABadAddressWithoutAskingAnyone(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	for _, bad := range []string{"ftp://x.example", "http://remote.example.com", "https://u:p@x.example", "https://x.example/path", "not a url", ""} {
		if _, err := setServer(t, config, bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	if !bytes.Equal(before, fileBytes(t, config)) {
		t.Fatal("the enrollment changed")
	}
}

func TestSetServerNeedsAnEnrolledMachine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := setServer(t, filepath.Join(home, "absent.json"), "https://x.example"); err == nil {
		t.Fatal("an absent enrollment was accepted")
	}
}
