package terminalbroker

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A machine refuses a chat whose effort its agent did not confirm under the
// general ACPUnavailable code, with a sentence of its own. Only that sentence
// marks the refusal as one for the effort; any other reason under the code,
// a model the agent did not use among them, does not.
func TestAnEffortRefusalIsToldApartFromTheOthers(t *testing.T) {
	var refused *ACPUnavailableError
	if err := acpRefusal(terminalproto.ACPUnavailable, terminalproto.ACPEffortUnconfirmedDetail); !errors.As(err, &refused) || !refused.EffortNotConfirmed() {
		t.Fatalf("effort refusal: %#v", err)
	}
	for _, detail := range []string{
		"The agent did not use the chosen model, so the chat was not started.",
		"The agent did not use the chosen source, so the chat was not started.",
		"That agent could not start the chat in that folder.",
		"The agent did not confirm the chosen effort for this model.",
	} {
		if err := acpRefusal(terminalproto.ACPUnavailable, detail); !errors.As(err, &refused) || refused.EffortNotConfirmed() {
			t.Fatalf("%q was taken for an effort refusal", detail)
		}
	}
	var none *ACPUnavailableError
	if none.EffortNotConfirmed() {
		t.Fatal("no refusal is no effort refusal")
	}
}
