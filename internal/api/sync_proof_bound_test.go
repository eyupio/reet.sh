package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/store/storetest"
	"github.com/eyupio/reet.sh/internal/syncproof"
)

// proofFrom asks a controller, set up with publicURL (or none), for a proof of one machine's
// record, saying the address the machine dialled.
func proofFrom(t *testing.T, publicURL, dialled string) (code int, answer map[string]string, id, secret, nonce string) {
	t.Helper()
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	uid, _ := st.DefaultOwner()
	agent, secret, err := st.EnrollSyncAgent(uid, "studio", "test")
	if err != nil {
		t.Fatal(err)
	}
	opts := []Option{WithAuthMode(AuthAccounts)}
	if publicURL != "" {
		opts = append(opts, WithPublicURL(publicURL))
	}
	srv := httptest.NewServer(New(st, deadFetcher(t), "", opts...))
	t.Cleanup(srv.Close)
	nonce, _ = syncproof.NewNonce()
	in := map[string]string{"id": agent.ID, "nonce": nonce}
	if dialled != "" {
		in["origin"] = dialled
	}
	body, _ := json.Marshal(in)
	code, text := do(t, srv, "POST", syncproof.Path, string(body))
	answer = map[string]string{}
	_ = json.Unmarshal([]byte(text), &answer)
	return code, answer, agent.ID, secret, nonce
}

// A machine that names the address it dialled gets a proof signed over the controller's own
// configured address, and only if they are the same address.
func TestTheProofRouteSignsOverTheControllersOwnAddress(t *testing.T) {
	code, a, id, secret, nonce := proofFrom(t, "https://reet.example", "https://REET.example:443/")
	if code != 200 || !syncproof.VerifyBound(secret, id, nonce, "https://reet.example", a["proof"]) {
		t.Fatalf("a bound proof was not given: %d %v", code, a)
	}
	if syncproof.VerifyBound(secret, id, nonce, "https://evil.example", a["proof"]) {
		t.Fatal("the proof verifies at another address")
	}
}

func TestTheProofRouteWillNotSignForAnotherAddress(t *testing.T) {
	// A relay names the real controller's address to the real controller only if it knows it; one
	// that passes the machine's request on unchanged names its own address, which is refused.
	code, a, _, _, _ := proofFrom(t, "https://reet.example", "https://evil.example")
	if code != 409 || a["code"] != "wrong_address" || a["proof"] != "" {
		t.Fatalf("a proof for another address: %d %v", code, a)
	}
}

func TestTheProofRouteWithNoPublicURLCannotBindAndSaysSo(t *testing.T) {
	code, a, _, _, _ := proofFrom(t, "", "https://reet.example")
	if code != 409 || a["code"] != "no_public_url" || a["proof"] != "" {
		t.Fatalf("no public URL: %d %v", code, a)
	}
}

// A machine of an older release names no address and still gets the unbound proof it checks.
func TestTheProofRouteStillAnswersAnOlderMachineWithTheUnboundProof(t *testing.T) {
	for _, publicURL := range []string{"https://reet.example", ""} {
		code, a, id, secret, nonce := proofFrom(t, publicURL, "")
		if code != 200 || !syncproof.Verify(secret, id, nonce, a["proof"]) {
			t.Fatalf("public %q: %d %v", publicURL, code, a)
		}
	}
}
