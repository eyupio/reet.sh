package clientconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// authFixture isolates the config directory and replaces the terminal hooks.
type authFixture struct {
	out     bytes.Buffer
	answers []string
	prompts int
}

func newAuthFixture(t *testing.T, interactive bool, answers ...string) *authFixture {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("REET_TOKEN", "")
	t.Setenv("REET_ENV_FILE", "")
	t.Chdir(t.TempDir())
	f := &authFixture{answers: answers}
	prevInteractive, prevRead := interactiveFn, readSecret
	interactiveFn = func(io.Reader) bool { return interactive }
	readSecret = func(io.Reader) (string, error) {
		f.prompts++
		if len(f.answers) == 0 {
			return "", nil
		}
		next := f.answers[0]
		f.answers = f.answers[1:]
		return next, nil
	}
	t.Cleanup(func() { interactiveFn, readSecret = prevInteractive, prevRead })
	return f
}

func TestNewAuthErrorOnlyForAuthStatuses(t *testing.T) {
	if NewAuthError(200, "") != nil || NewAuthError(404, "") != nil || NewAuthError(500, "") != nil {
		t.Fatal("only 401 and 403 are authentication failures")
	}
	if !IsRejected(NewAuthError(401, "")) {
		t.Fatal("401 should be a rejected token")
	}
	forbidden := NewAuthError(403, "needs write role")
	if IsRejected(forbidden) {
		t.Fatal("403 means the token is valid but lacks the role; a new prompt would not help")
	}
	var ae *AuthError
	if !errors.As(forbidden, &ae) || ae.Status != 403 || !strings.Contains(forbidden.Error(), "needs write role") {
		t.Fatalf("server message must reach the user: %v", forbidden)
	}
}

func TestResolveTokenSourceNamesWhereItCameFrom(t *testing.T) {
	newAuthFixture(t, false)
	if _, src, _ := ResolveTokenSource("flag", "", nil, nil); src != SourceFlag {
		t.Fatalf("flag source = %q", src)
	}
	t.Setenv("REET_TOKEN", "env")
	if _, src, _ := ResolveTokenSource("", "", nil, nil); src != SourceEnv {
		t.Fatalf("env source = %q", src)
	}
	t.Setenv("REET_TOKEN", "")
	if _, err := SaveToken("saved"); err != nil {
		t.Fatal(err)
	}
	if tok, src, _ := ResolveTokenSource("", "", nil, nil); tok != "saved" || src != SourceSaved {
		t.Fatalf("saved source = %q %q", tok, src)
	}
}

func TestAuthorizeRunsOnceWhenAccepted(t *testing.T) {
	newAuthFixture(t, true)
	c := &Credential{Token: "good", Source: SourceEnv, Server: "https://x", In: nil, Out: &bytes.Buffer{}}
	calls := 0
	if err := Authorize(c, func(string) error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestAuthorizeNonInteractiveReportsSourceAndNeverPrompts(t *testing.T) {
	f := newAuthFixture(t, false)
	c := &Credential{Token: "stale", Source: Source("env file .env"), Server: "https://x", Out: &f.out}
	err := Authorize(c, func(string) error { return NewAuthError(401, "bootstrap token disabled; create a personal API token") })
	if err == nil || !IsRejected(err) {
		t.Fatalf("original rejection must survive: %v", err)
	}
	for _, want := range []string{"env file .env", "bootstrap token disabled"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should mention %q", err, want)
		}
	}
	if f.prompts != 0 {
		t.Fatal("must not prompt without a terminal")
	}
}

func TestAuthorizePromptsAndRetriesOnce(t *testing.T) {
	f := newAuthFixture(t, true, "fresh")
	c := &Credential{Token: "stale", Source: SourceEnv, Server: "https://dev.reet.sh", Out: &f.out}
	var seen []string
	err := Authorize(c, func(tok string) error {
		seen = append(seen, tok)
		if tok != "fresh" {
			return NewAuthError(401, "")
		}
		return nil
	})
	if err != nil || strings.Join(seen, ",") != "stale,fresh" {
		t.Fatalf("seen=%v err=%v", seen, err)
	}
	if !strings.Contains(f.out.String(), "https://dev.reet.sh") {
		t.Fatalf("prompt should name the server: %q", f.out.String())
	}
}

func TestAuthorizeSavesOnlyWhenSavedTokenWouldBeUsedNextTime(t *testing.T) {
	// REET_TOKEN outranks the saved token, so saving would not help; say why instead.
	f := newAuthFixture(t, true, "fresh")
	c := &Credential{Token: "stale", Source: SourceEnv, Server: "https://x", Out: &f.out}
	_ = Authorize(c, func(tok string) error {
		if tok == "fresh" {
			return nil
		}
		return NewAuthError(401, "")
	})
	if saved, _ := SavedToken(); saved != "" {
		t.Fatal("a token overridden by REET_TOKEN must not be saved silently")
	}
	if !strings.Contains(f.out.String(), "REET_TOKEN") {
		t.Fatalf("user should be told what to change: %q", f.out.String())
	}

	// A stale saved token is replaced.
	f2 := newAuthFixture(t, true, "fresh")
	if _, err := SaveToken("stale"); err != nil {
		t.Fatal(err)
	}
	c2 := &Credential{Token: "stale", Source: SourceSaved, Server: "https://x", Out: &f2.out}
	_ = Authorize(c2, func(tok string) error {
		if tok == "fresh" {
			return nil
		}
		return NewAuthError(401, "")
	})
	if saved, _ := SavedToken(); saved != "fresh" {
		t.Fatalf("saved token = %q, want fresh", saved)
	}
}

func TestAuthorizeTriesDifferentSavedTokenBeforePrompting(t *testing.T) {
	f := newAuthFixture(t, true)
	if _, err := SaveToken("saved-good"); err != nil {
		t.Fatal(err)
	}
	c := &Credential{Token: "env-stale", Source: SourceEnv, Server: "https://x", Out: &f.out}
	var seen []string
	err := Authorize(c, func(tok string) error {
		seen = append(seen, tok)
		if tok == "saved-good" {
			return nil
		}
		return NewAuthError(401, "")
	})
	if err != nil || strings.Join(seen, ",") != "env-stale,saved-good" || f.prompts != 0 {
		t.Fatalf("seen=%v prompts=%d err=%v", seen, f.prompts, err)
	}
}

func TestAuthorizeTriesDifferentSavedTokenWithoutATerminal(t *testing.T) {
	f := newAuthFixture(t, false)
	if _, err := SaveToken("saved-good"); err != nil {
		t.Fatal(err)
	}
	c := &Credential{Token: "env-stale", Source: SourceEnv, Server: "https://x", Out: &f.out}
	var seen []string
	err := Authorize(c, func(tok string) error {
		seen = append(seen, tok)
		if tok == "saved-good" {
			return nil
		}
		return NewAuthError(401, "")
	})
	if err != nil || strings.Join(seen, ",") != "env-stale,saved-good" || f.prompts != 0 {
		t.Fatalf("seen=%v prompts=%d err=%v", seen, f.prompts, err)
	}
}

func TestAuthorizeEnterCancels(t *testing.T) {
	f := newAuthFixture(t, true, "")
	c := &Credential{Token: "stale", Source: SourceEnv, Server: "https://x", Out: &f.out}
	calls := 0
	err := Authorize(c, func(string) error { calls++; return NewAuthError(401, "") })
	if !IsRejected(err) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestAuthorizeGivesUpAfterThreeEntries(t *testing.T) {
	f := newAuthFixture(t, true, "a", "b", "c", "d")
	c := &Credential{Token: "stale", Source: SourceEnv, Server: "https://x", Out: &f.out}
	calls := 0
	err := Authorize(c, func(string) error { calls++; return NewAuthError(401, "") })
	if !IsRejected(err) || f.prompts != 3 || calls != 4 {
		t.Fatalf("prompts=%d calls=%d err=%v", f.prompts, calls, err)
	}
}

func TestAuthorizeDoesNotPromptForForbiddenOrOtherErrors(t *testing.T) {
	f := newAuthFixture(t, true, "fresh")
	for _, failure := range []error{NewAuthError(403, "read-only"), fmt.Errorf("network down")} {
		c := &Credential{Token: "t", Source: SourceEnv, Server: "https://x", Out: &f.out}
		calls := 0
		err := Authorize(c, func(string) error { calls++; return failure })
		if err == nil || calls != 1 || f.prompts != 0 {
			t.Fatalf("%v: calls=%d prompts=%d", failure, calls, f.prompts)
		}
	}
}
