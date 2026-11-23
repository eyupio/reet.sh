// Package auth hashes passwords, mints session tokens and rate-limits sign-in.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword returns an argon2id hash in the standard encoded form.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches an encoded argon2id hash.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	valid := len(parts) == 5 && parts[0] == "argon2id" && parts[1] == "v=19"
	if !valid {
		return false
	}
	var memory uint32
	var iterations uint32
	var threads uint8
	for _, field := range strings.Split(parts[2], ",") {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return false
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return false
		}
		switch key {
		case "m":
			memory = uint32(n)
		case "t":
			iterations = uint32(n)
		case "p":
			threads = uint8(n)
		}
	}
	validParams := memory != 0 && iterations != 0 && threads != 0
	if !validParams {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NewToken returns a URL-safe random token.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken is what the database stores for a session or API token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Limiter is a fixed-window attempt limiter keyed by one or more strings.
type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
	// maxKeys bounds the map: keys are strings an attacker chooses, such as
	// emails that do not exist, and an hour-long window would otherwise keep
	// every one of them.
	maxKeys int
}

// maxLimiterKeys is how many keys one limiter holds at once.
const maxLimiterKeys = 100_000

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, hits: map[string][]time.Time{}, maxKeys: maxLimiterKeys}
}

// recordLocked records one attempt for key. A new key past the bound drops
// the key with the fewest attempts, the oldest among equals, rather than
// refusing everyone once the map is full: a flood of one-off keys then
// cannot push out the key of an account that is being guessed at.
func (l *Limiter) recordLocked(key string, now time.Time) {
	if _, ok := l.hits[key]; !ok && len(l.hits) >= l.maxKeys {
		var stale string
		var staleAt time.Time
		staleN := 0
		for k, times := range l.hits {
			last := times[len(times)-1]
			if staleN == 0 || len(times) < staleN || len(times) == staleN && last.Before(staleAt) {
				stale, staleAt, staleN = k, last, len(times)
			}
		}
		delete(l.hits, stale)
	}
	l.hits[key] = append(l.hits[key], now)
}

func (l *Limiter) pruneLocked(now time.Time) {
	cutoff := now.Add(-l.window)
	for key, times := range l.hits {
		kept := times[:0]
		for _, at := range times {
			if at.After(cutoff) {
				kept = append(kept, at)
			}
		}
		if len(kept) == 0 {
			delete(l.hits, key)
		} else {
			l.hits[key] = kept
		}
	}
}

// Allow reports whether every key is still under the limit.
func (l *Limiter) Allow(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.pruneLocked(now)
	for _, key := range keys {
		if len(l.hits[key]) >= l.max {
			return false
		}
	}
	return true
}

// Len reports how many keys have attempts inside the window.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(time.Now())
	return len(l.hits)
}

// Hit records one attempt for every key.
func (l *Limiter) Hit(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.pruneLocked(now)
	for _, key := range keys {
		l.recordLocked(key, now)
	}
}

// Take records one attempt for every key if every key is still under the
// limit, and reports whether it did. The check and the record happen under
// one lock, so attempts made at the same moment cannot all slip through.
func (l *Limiter) Take(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.pruneLocked(now)
	for _, key := range keys {
		if len(l.hits[key]) >= l.max {
			return false
		}
	}
	for _, key := range keys {
		l.recordLocked(key, now)
	}
	return true
}

// Wait reports how long until every full key has room for another attempt:
// until the oldest attempt of the fullest key leaves the window.
func (l *Limiter) Wait(keys ...string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.pruneLocked(now)
	var wait time.Duration
	for _, key := range keys {
		if times := l.hits[key]; len(times) >= l.max {
			wait = max(wait, times[len(times)-l.max].Add(l.window).Sub(now))
		}
	}
	return wait
}

// Forgive takes back the newest attempt recorded for key, for an attempt
// that turned out not to count.
func (l *Limiter) Forgive(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	times := l.hits[key]
	if len(times) <= 1 {
		delete(l.hits, key)
		return
	}
	l.hits[key] = times[:len(times)-1]
}
