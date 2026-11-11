// Package syncproof is how a controller shows a machine that it holds the
// machine's enrollment record, before the machine sends it any credential.
//
// A controller stores only a hash of a machine's secret, never the secret. So
// the machine asks for a proof that can be made from that hash alone: the
// controller answers a fresh nonce with an HMAC keyed by the stored hash, and
// the machine, which can hash its own secret, checks the answer. A controller
// that does not hold the record cannot make it, a recorded answer cannot be
// replayed because the nonce is fresh each time, and the answer reveals neither
// the hash nor the secret.
package syncproof

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

// Path is the controller route that answers a proof request. It needs no
// credential, because its whole purpose is to be asked before any is sent.
const Path = "/api/v2/sync/agent-proof"

const label = "reet-agent-proof/v1"

// HashSecret is the form in which a controller stores a machine's secret.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Compute is the proof a controller holding secretHash gives for this machine
// id and nonce.
func Compute(secretHash, id, nonce string) string {
	m := hmac.New(sha256.New, []byte(secretHash))
	m.Write([]byte(label + "\n" + id + "\n" + nonce))
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a controller's proof against the machine's own secret.
func Verify(secret, id, nonce, proof string) bool {
	want := Compute(HashSecret(secret), id, nonce)
	return len(proof) == len(want) && hmac.Equal([]byte(proof), []byte(want))
}

var nonceShape = regexp.MustCompile(`^[0-9a-f]{32,128}$`)

// ValidNonce bounds what a controller will sign.
func ValidNonce(nonce string) bool { return nonceShape.MatchString(nonce) }

// NewNonce is 32 random bytes as hex.
func NewNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
