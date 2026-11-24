package store

import (
	"errors"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ErrMachineLocked is a machine refusing because its owner locked it there
// (reet agent lock). Nothing on the controller can lift it.
var ErrMachineLocked = errors.New(terminalproto.LockedSentence)

// LockedError is ErrMachineLocked that is also the error the refusal was
// before the lock was known (a machine not ready, offline or denying), so
// callers that handle those keep doing so.
type LockedError struct{ Was error }

func (e LockedError) Error() string { return terminalproto.LockedSentence }
func (e LockedError) Is(target error) bool {
	return target == ErrMachineLocked || errors.Is(e.Was, target)
}
