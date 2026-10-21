package auth

import (
	"testing"
	"time"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("not-a-hash", "x") {
		t.Fatal("malformed hash accepted")
	}
}

func TestTokenIsRandomAndHashed(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if a == b || len(a) < 40 {
		t.Fatalf("tokens: %q %q", a, b)
	}
	if HashToken(a) == a || HashToken(a) != HashToken(a) {
		t.Fatal("token hash is not stable")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	if !l.Allow("ip", "email") {
		t.Fatal("first attempt blocked")
	}
	l.Hit("ip", "email")
	l.Hit("ip", "email")
	if l.Allow("ip", "email") {
		t.Fatal("third attempt allowed")
	}
	if !l.Allow("other", "email2") {
		t.Fatal("unrelated key blocked")
	}
}
