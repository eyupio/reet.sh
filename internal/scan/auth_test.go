package scan

import (
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

// A controller 401 from any agent request must be recognised as a rejected
// token, so the shared clientconfig flow can replace it; other refusals must not.
func TestControllerErrorIsRecognisedByTheSharedAuthFlow(t *testing.T) {
	wrapped := fmt.Errorf("enroll: %w", controllerError{Status: 401, Message: "sign in required"})
	if !clientconfig.IsRejected(wrapped) {
		t.Fatal("a controller 401 should be a rejected token")
	}
	for _, status := range []int{403, 404, 409, 500, 503} {
		if clientconfig.IsRejected(controllerError{Status: status}) {
			t.Fatalf("status %d must not trigger a token prompt", status)
		}
	}
	// Existing classification is unchanged.
	if !permanentAgentError(controllerError{Status: 403}) || permanentAgentError(controllerError{Status: 503}) {
		t.Fatal("permanentAgentError changed")
	}
}
