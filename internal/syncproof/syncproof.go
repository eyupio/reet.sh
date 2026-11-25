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
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Path is the controller route that answers a proof request. It needs no
// credential, because its whole purpose is to be asked before any is sent.
const Path = "/api/v2/sync/agent-proof"

const label = "reet-agent-proof/v1"

// boundLabel is the label of a proof signed over the controller's own address as well, so a
// server that only relays the request to the real controller cannot pass for it.
const boundLabel = "reet-agent-proof/v2"

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

// ComputeBound is the proof a controller holding secretHash gives for this machine id and nonce
// as the controller at origin (see Origin). A machine of this release accepts only this form.
func ComputeBound(secretHash, id, nonce, origin string) string {
	m := hmac.New(sha256.New, []byte(secretHash))
	m.Write([]byte(boundLabel + "\n" + id + "\n" + nonce + "\n" + origin))
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyBound checks a controller's bound proof against the machine's own secret and the
// address the machine dialled.
func VerifyBound(secret, id, nonce, origin, proof string) bool {
	want := ComputeBound(HashSecret(secret), id, nonce, origin)
	return origin != "" && len(proof) == len(want) && hmac.Equal([]byte(proof), []byte(want))
}

// Origin is the scheme and host of an address, lower case and without a default port or a
// path, or "" when it is not an http or https address. A controller and a machine write the
// same address differently (a trailing slash, a capital, :443); this is what they both sign.
func Origin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port == "" || (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return u.Scheme + "://" + host
	}
	return u.Scheme + "://" + net.JoinHostPort(host, port)
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
