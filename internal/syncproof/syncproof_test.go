package syncproof

import (
	"strings"
	"testing"
)

func TestAProofIsForOneMachineAndOneNonce(t *testing.T) {
	secret := "a-machine-secret"
	hash := HashSecret(secret)
	nonce, err := NewNonce()
	if err != nil || !ValidNonce(nonce) {
		t.Fatalf("nonce %q: %v", nonce, err)
	}
	proof := Compute(hash, "machine-1", nonce)
	if !Verify(secret, "machine-1", nonce, proof) {
		t.Fatal("a proper proof was refused")
	}
	other, _ := NewNonce()
	for name, ok := range map[string]bool{
		"another nonce":   Verify(secret, "machine-1", other, proof),
		"another machine": Verify(secret, "machine-2", nonce, proof),
		"another secret":  Verify("not-the-secret", "machine-1", nonce, proof),
		"empty":           Verify(secret, "machine-1", nonce, ""),
		"truncated":       Verify(secret, "machine-1", nonce, proof[:len(proof)-2]),
		"the hash itself": Verify(secret, "machine-1", nonce, hash),
	} {
		if ok {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestTheProofRevealsNeitherTheHashNorTheSecret(t *testing.T) {
	secret := "a-machine-secret"
	proof := Compute(HashSecret(secret), "machine-1", strings.Repeat("a", 64))
	if strings.Contains(proof, HashSecret(secret)) || strings.Contains(proof, secret) {
		t.Fatal("the proof contains what it is made from")
	}
}

func TestNoncesAreBoundedAndFresh(t *testing.T) {
	for _, bad := range []string{"", "short", strings.Repeat("a", 129), strings.Repeat("g", 64), strings.Repeat("A", 64)} {
		if ValidNonce(bad) {
			t.Errorf("%q was accepted", bad)
		}
	}
	a, _ := NewNonce()
	b, _ := NewNonce()
	if a == b {
		t.Fatal("two nonces were the same")
	}
}
