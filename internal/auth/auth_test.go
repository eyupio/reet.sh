package auth

import (
	"testing"
	"time"
)

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()
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

func TestLimiterTakeChecksAndRecordsTogether(t *testing.T) {
	l := NewLimiter(2, time.Minute)
	if !l.Take("a", "b") || !l.Take("a") {
		t.Fatal("attempts under the limit were refused")
	}
	// "a" is full, so nothing is recorded for "b" either.
	if l.Take("a", "b") || l.Take("b", "a") {
		t.Fatal("an attempt over the limit was taken")
	}
	if !l.Take("b") || l.Take("b") {
		t.Fatal("a refused attempt was still recorded for the other key")
	}
	l.Forgive("a")
	if !l.Take("a") || l.Take("a") {
		t.Fatal("a forgiven attempt was not given back exactly once")
	}
	l.Forgive("never-seen")
	if l.Len() != 2 {
		t.Fatalf("keys = %d", l.Len())
	}

	// Many attempts at once still take no more than the limit.
	l = NewLimiter(5, time.Minute)
	taken := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() { taken <- l.Take("k") }()
	}
	n := 0
	for i := 0; i < 100; i++ {
		if <-taken {
			n++
		}
	}
	if n != 5 {
		t.Fatalf("%d attempts were taken at once, want 5", n)
	}
}
