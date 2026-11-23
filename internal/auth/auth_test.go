package auth

import (
	"fmt"
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

// Keys are strings an attacker chooses, so the map holds a bounded number
// of them: past the bound the key whose last attempt is oldest is dropped.
func TestLimiterHoldsABoundedNumberOfKeys(t *testing.T) {
	l := NewLimiter(5, time.Hour)
	l.maxKeys = 3
	for _, key := range []string{"a", "b", "c"} {
		l.Take(key)
		time.Sleep(time.Millisecond)
	}
	l.Take("a") // "a" is now the freshest
	time.Sleep(time.Millisecond)
	for i := 0; i < 50; i++ {
		l.Take(fmt.Sprintf("spray-%d", i))
		l.Hit(fmt.Sprintf("hit-%d", i))
	}
	if n := l.Len(); n != 3 {
		t.Fatalf("the limiter holds %d keys, want at most 3", n)
	}
	// A flood of one-off keys does not push out a key holding more attempts:
	// "a", with two, is still counted.
	if l.Wait("a") != 0 || !l.Take("a", "a", "a") || l.Take("a") {
		t.Fatal("the flood dropped the key with the most attempts")
	}
	// An existing key at the bound still counts without dropping another.
	l = NewLimiter(2, time.Hour)
	l.maxKeys = 2
	l.Take("x")
	l.Take("y")
	if !l.Take("x") || l.Take("x") || l.Len() != 2 {
		t.Fatalf("an existing key at the bound: len %d", l.Len())
	}
}

func TestLimiterWaitIsUntilTheOldestAttemptLeavesTheWindow(t *testing.T) {
	l := NewLimiter(2, time.Hour)
	if w := l.Wait("k"); w != 0 {
		t.Fatalf("wait for an unused key = %v", w)
	}
	l.Take("k")
	l.Take("k")
	if w := l.Wait("k", "other"); w <= 59*time.Minute || w > time.Hour {
		t.Fatalf("wait for a full key = %v", w)
	}
}

// A flood of new keys at the bound walks the whole map only now and then,
// not on every attempt, and does not push out a key with many attempts.
func TestLimiterTakeDoesNotWalkEveryKey(t *testing.T) {
	l := NewLimiter(200, time.Hour)
	for i := 0; i < 100; i++ {
		l.Take("victim")
	}
	l.scans = 0
	for i := 0; i < 2*maxLimiterKeys; i++ {
		if !l.Take(fmt.Sprintf("flood-%d", i)) {
			t.Fatalf("a new key was refused at %d", i)
		}
	}
	if l.scans > 20 {
		t.Fatalf("%d attempts walked the whole map %d times", 2*maxLimiterKeys, l.scans)
	}
	if n := l.Len(); n > maxLimiterKeys {
		t.Fatalf("the limiter holds %d keys, over the bound", n)
	}
	for i := 0; i < 100; i++ {
		if !l.Take("victim") {
			t.Fatalf("the victim's key was dropped and refilled: refused after %d more", i)
		}
	}
	if l.Take("victim") {
		t.Fatal("the flood reset the victim's count")
	}
}

// Attempts taken and given back at once leave the count where it started.
func TestLimiterTakeAndForgiveAtOnce(t *testing.T) {
	l := NewLimiter(1000, time.Minute)
	l.Take("k")
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 20; j++ {
				if l.Take("k", "other") {
					l.Wait("k")
					l.Forgive("k")
					l.Forgive("other")
				}
			}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	if n := l.Len(); n != 1 {
		t.Fatalf("keys after taking and giving back = %d, want 1", n)
	}
	for i := 1; i < 1000; i++ {
		if !l.Take("k") {
			t.Fatalf("the count drifted: refused at %d", i)
		}
	}
	if l.Take("k") {
		t.Fatal("the count drifted: over the limit")
	}
}
