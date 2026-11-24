package terminalproto

import "strings"

// MachineLockFeature negotiates the machine lock: the Locked capability and
// the locked result code. The lock itself is a local switch on the machine
// (reet agent lock); a controller can read it, never set or lift it.
const MachineLockFeature = "machine-lock-v1"

// LockedSentence is what a person is told when the machine refuses because
// it is locked.
const LockedSentence = "Locked on the machine. Run `reet agent unlock` there."

// Locked refuses a request because the machine is locked. It is sent only to
// a controller that negotiated machine-lock-v1; an older one is answered
// denied instead.
const Locked ResultCode = "locked"

// SupportsMachineLock reports whether features includes machine-lock-v1.
func SupportsMachineLock(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == MachineLockFeature {
			return true
		}
	}
	return false
}
