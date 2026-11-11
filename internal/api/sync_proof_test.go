package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/syncproof"
)

func proofRequest(t *testing.T, tg target, body string) reply {
	t.Helper()
	return tg.fenceCall(t, "POST", syncproof.Path, "", body)
}

// The controller holding a machine's record can prove it with no credential at
// all, and nobody else's proof verifies against this machine's secret.
func TestTheProofRouteAnswersAMachineWithoutACredentialAndOnlyForItsRecord(t *testing.T) {
	tg := newTarget(t, false)
	owner, _, _ := tg.st.OwnerUser()
	agent, secret, err := tg.st.EnrollSyncAgent(owner.ID, "studio", "test")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := tg.st.EnrollSyncAgent(owner.ID, "desk", "test")
	if err != nil {
		t.Fatal(err)
	}
	nonce, _ := syncproof.NewNonce()
	r := proofRequest(t, tg, `{"id":"`+agent.ID+`","nonce":"`+nonce+`"}`)
	proof, _ := r.v["proof"].(string)
	if r.code != 200 || !syncproof.Verify(secret, agent.ID, nonce, proof) {
		t.Fatalf("the holder of the record could not prove it: %d %s", r.code, r.text)
	}
	// The answer for the other machine does not verify as this one's.
	r = proofRequest(t, tg, `{"id":"`+other.ID+`","nonce":"`+nonce+`"}`)
	if p, _ := r.v["proof"].(string); syncproof.Verify(secret, agent.ID, nonce, p) || syncproof.Verify(secret, other.ID, nonce, p) {
		t.Fatal("another machine's proof verified against this machine's secret")
	}
	if strings.Contains(r.text, secret) || strings.Contains(r.text, syncproof.HashSecret(secret)) {
		t.Fatal("the answer reveals the secret or its hash")
	}
}

func TestTheProofRouteRefusesWhatItCannotSign(t *testing.T) {
	tg := newTarget(t, false)
	nonce, _ := syncproof.NewNonce()
	if r := proofRequest(t, tg, `{"id":"no-such-machine","nonce":"`+nonce+`"}`); r.code != 404 || r.v["code"] != "unknown_machine" {
		t.Fatalf("unknown machine: %d %s", r.code, r.text)
	}
	owner, _, _ := tg.st.OwnerUser()
	agent, _, _ := tg.st.EnrollSyncAgent(owner.ID, "studio", "test")
	for _, bad := range []string{`{}`, `{"id":"` + agent.ID + `"}`, `{"id":"` + agent.ID + `","nonce":"short"}`, `{"id":"` + agent.ID + `","nonce":"` + nonce + `","extra":1}`, `not json`} {
		if r := proofRequest(t, tg, bad); r.code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", bad, r.code, r.text)
		}
	}
}

// A controller that has itself moved says so, rather than proving anything.
func TestAFencedControllerDoesNotAnswerAProofRequest(t *testing.T) {
	tg := newTarget(t, false)
	owner, _, _ := tg.st.OwnerUser()
	agent, _, _ := tg.st.EnrollSyncAgent(owner.ID, "studio", "test")
	if _, err := tg.st.SetFence("https://next.example.com"); err != nil {
		t.Fatal(err)
	}
	nonce, _ := syncproof.NewNonce()
	if r := proofRequest(t, tg, `{"id":"`+agent.ID+`","nonce":"`+nonce+`"}`); r.code != http.StatusLocked {
		t.Fatalf("a fenced controller answered: %d %s", r.code, r.text)
	}
}
