package scan

import (
	"errors"
	"runtime"
	"time"

	"github.com/eyupio/reet.sh/internal/filelock"
)

// errLeftAlone is what an edit returns to leave the enrolment as it is.
var errLeftAlone = errors.New("the enrolment was left as it is")

// configWriteError is an edit whose new enrolment could not be saved.
type configWriteError struct{ err error }

func (e configWriteError) Error() string { return e.err.Error() }
func (e configWriteError) Unwrap() error { return e.err }

// configEditWait bounds how long an edit waits for another to finish.
var configEditWait = 10 * time.Second

// editAgentConfig reads the enrolment, applies edit and writes it back while
// holding a lock beside it, so two writers (a command here, the running agent
// following a move) never write back a copy read before the other's change.
// Where the platform has no private file lock it edits without one.
func editAgentConfig(config string, edit func(*agentConfig) error) error {
	deadline := time.Now().Add(configEditWait)
	for {
		lock, err := filelock.Acquire(config + ".edit.lock")
		if err == nil {
			defer lock.Close()
			break
		}
		if errors.Is(err, filelock.ErrUnsafe) && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			break
		}
		if !errors.Is(err, filelock.ErrLocked) {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("another reet command is changing this machine's enrolment; try again")
		}
		time.Sleep(10 * time.Millisecond)
	}
	c, err := loadAgentConfig(config)
	if err != nil {
		return err
	}
	if err := edit(&c); err != nil {
		return err
	}
	if err := writePrivate(config, c); err != nil {
		return configWriteError{err}
	}
	return nil
}
