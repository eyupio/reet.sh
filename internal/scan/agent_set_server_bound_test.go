package scan

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/syncproof"
)

// theRealController is this release's controller holding the machine's record: asked for a bound
// proof it signs over its own address and refuses to sign for any other; asked with no address,
// as an older machine asks, it gives the unbound form.
func theRealController(t *testing.T) *controller {
	return newController(t, func(c *controller, w http.ResponseWriter, r *http.Request, body []byte) {
		if r.URL.Path != syncproof.Path {
			http.NotFound(w, r)
			return
		}
		var in struct{ ID, Nonce, Origin string }
		json.Unmarshal(body, &in)
		hash := syncproof.HashSecret(testMachineSecret)
		switch {
		case in.Origin == "":
			json.NewEncoder(w).Encode(map[string]string{"proof": syncproof.Compute(hash, in.ID, in.Nonce)})
		case syncproof.Origin(in.Origin) != syncproof.Origin(c.URL):
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"code": "wrong_address", "error": "wrong address"})
		default:
			json.NewEncoder(w).Encode(map[string]string{"proof": syncproof.ComputeBound(hash, in.ID, in.Nonce, syncproof.Origin(c.URL))})
		}
	})
}

func unchanged(t *testing.T, config string, before []byte, restarts *[]string) {
	t.Helper()
	if !bytes.Equal(before, fileBytes(t, config)) || len(*restarts) != 0 {
		t.Fatal("a failed check changed the enrollment or restarted the agent")
	}
}

// A server that only passes the machine's request on to the real controller, and its answer back,
// is refused: the proof it relays was signed for the real controller's address, not its own.
func TestSetServerRefusesAProofRelayedFromTheRealController(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	restarts := withRestart(t, nil)
	real := theRealController(t)
	relay := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, body []byte) {
		if r.URL.Path != syncproof.Path {
			http.NotFound(w, r)
			return
		}
		// It says the real controller's own address, so that the real controller will sign.
		var in map[string]string
		json.Unmarshal(body, &in)
		if _, asked := in["origin"]; asked {
			in["origin"] = real.URL
		}
		forwarded, _ := json.Marshal(in)
		resp, err := http.Post(real.URL+r.URL.Path, "application/json", bytes.NewReader(forwarded))
		if err != nil {
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
	_, err := setServer(t, config, relay.URL)
	if err == nil || !strings.Contains(err.Error(), "Check 1 of 2") || !strings.Contains(err.Error(), "does not hold it") {
		t.Fatalf("a relayed proof was accepted: %v", err)
	}
	noCredentialEverSent(t, relay)
	unchanged(t, config, before, restarts)
}

// The machine accepts only a bound proof, so a controller that gives the unbound form (an older
// release, or one that holds the record but is relaying) does not pass.
func TestSetServerRefusesAnUnboundProof(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	restarts := withRestart(t, nil)
	old := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, body []byte) {
		var in struct{ ID, Nonce string }
		json.Unmarshal(body, &in)
		json.NewEncoder(w).Encode(map[string]string{"proof": syncproof.Compute(syncproof.HashSecret(testMachineSecret), in.ID, in.Nonce)})
	})
	if _, err := setServer(t, config, old.URL); err == nil || !strings.Contains(err.Error(), "Check 1 of 2") {
		t.Fatalf("an unbound proof was accepted: %v", err)
	}
	noCredentialEverSent(t, old)
	unchanged(t, config, before, restarts)
}

// The real controller of this release passes the check at the address it was given, however that
// address is written.
func TestSetServerAcceptsTheRealControllersBoundProof(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	withRestart(t, nil)
	real := honest(t)
	if _, err := setServer(t, config, real.URL); err != nil {
		t.Fatalf("the real controller was refused: %v", err)
	}
}

// What the machine says when the controller cannot bind its proof is a plain next step.
func TestSetServerSaysWhyAControllerCannotBindItsProof(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		code   string
		want   string
	}{
		"no public url":       {http.StatusConflict, "no_public_url", "public URL"},
		"another address":     {http.StatusConflict, "wrong_address", "does not answer on this address"},
		"an older controller": {http.StatusBadRequest, "bad_proof_request", "Upgrade it"},
	} {
		t.Run(name, func(t *testing.T) {
			config := enrolledMachine(t, "https://old.example.com")
			before := fileBytes(t, config)
			restarts := withRestart(t, nil)
			c := newController(t, func(_ *controller, w http.ResponseWriter, r *http.Request, _ []byte) {
				w.WriteHeader(tc.status)
				json.NewEncoder(w).Encode(map[string]string{"code": tc.code, "error": "x"})
			})
			if _, err := setServer(t, config, c.URL); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q in: %v", tc.want, err)
			}
			noCredentialEverSent(t, c)
			unchanged(t, config, before, restarts)
		})
	}
}
