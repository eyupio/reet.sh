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

func TestABoundProofIsForOneAddress(t *testing.T) {
	secret := "a-machine-secret"
	hash := HashSecret(secret)
	nonce, _ := NewNonce()
	proof := ComputeBound(hash, "machine-1", nonce, "https://reet.example")
	if !VerifyBound(secret, "machine-1", nonce, "https://reet.example", proof) {
		t.Fatal("a proper bound proof was refused")
	}
	for name, ok := range map[string]bool{
		"another address":      VerifyBound(secret, "machine-1", nonce, "https://evil.example", proof),
		"no address":           VerifyBound(secret, "machine-1", nonce, "", proof),
		"another machine":      VerifyBound(secret, "machine-2", nonce, "https://reet.example", proof),
		"another secret":       VerifyBound("not-the-secret", "machine-1", nonce, "https://reet.example", proof),
		"the unbound proof":    VerifyBound(secret, "machine-1", nonce, "https://reet.example", Compute(hash, "machine-1", nonce)),
		"empty":                VerifyBound(secret, "machine-1", nonce, "https://reet.example", ""),
		"unbound with address": Verify(secret, "machine-1", nonce, proof),
	} {
		if ok {
			t.Errorf("%s was accepted", name)
		}
	}
	// With no address to sign over, even a proof made over no address is not accepted.
	if VerifyBound(secret, "machine-1", nonce, "", ComputeBound(hash, "machine-1", nonce, "")) {
		t.Error("a proof over no address was accepted")
	}
}

func TestOriginIsTheSameForTheSameAddressWrittenDifferently(t *testing.T) {
	same := []string{"https://reet.example", "https://REET.example/", "  https://reet.example:443/some/path?x=1 ", "HTTPS://Reet.Example"}
	for _, in := range same {
		if got := Origin(in); got != "https://reet.example" {
			t.Errorf("Origin(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{
		"http://reet.example:80":       "http://reet.example",
		"https://reet.example:8443":    "https://reet.example:8443",
		"http://127.0.0.1:8080/":       "http://127.0.0.1:8080",
		"https://[2001:db8::1]":        "https://[2001:db8::1]",
		"https://[2001:db8::1]:8443/x": "https://[2001:db8::1]:8443",
	} {
		if got := Origin(in); got != want {
			t.Errorf("Origin(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "reet.example", "ftp://reet.example", "https://", "//reet.example", "https://%zz", "javascript:alert(1)"} {
		if got := Origin(bad); got != "" {
			t.Errorf("Origin(%q) = %q, want none", bad, got)
		}
	}
	if Origin("https://reet.example") == Origin("https://reet.example.evil.com") || Origin("https://a.example") == Origin("http://a.example") {
		t.Error("two different addresses gave one origin")
	}
}
