package terminalhost

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestGitHubRemoteAcceptsOnlyGitHubDotCom(t *testing.T) {
	for in, want := range map[string]string{
		"https://github.com/eyupio/reet.sh.git": "eyupio/reet.sh",
		"https://github.com/eyupio/reet.sh":     "eyupio/reet.sh",
		"git@github.com:eyupio/reet.sh.git":     "eyupio/reet.sh",
		"ssh://git@github.com/eyupio/reet.sh":   "eyupio/reet.sh",
	} {
		o, r, ok := GitHubRemote(in)
		if !ok || o+"/"+r != want {
			t.Fatal(in, o, r, ok)
		}
	}
	for _, in := range []string{
		"",
		"https://github.example.com/a/b",
		"https://gitlab.com/a/b",
		"https://github.com/a",
		"https://github.com/a/b/c",
		"https://evil@github.com.x/a/b",
		"https://github.com/-a/b",
		"https://github.com/a/..",
	} {
		if _, _, ok := GitHubRemote(in); ok {
			t.Fatal("accepted", in)
		}
	}
}

// With gh at an absolute path, the args reset inherited helpers and name gh
// for github.com only.
func TestGhCredentialArgsNameOnlyGitHub(t *testing.T) {
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	want := []string{"-c", "credential.helper=", "-c", "credential.https://github.com.helper=!" + gh + " auth git-credential"}
	if got := ghCredentialArgs(); !slices.Equal(got, want) {
		t.Fatal(got)
	}
	t.Setenv("PATH", t.TempDir())
	if got := ghCredentialArgs(); got != nil {
		t.Fatal("args without gh", got)
	}
}

func TestNetworkGitEnvDisablesPrompt(t *testing.T) {
	want := []string{"GIT_TERMINAL_PROMPT=0"}
	if got := networkGitEnv(); !slices.Equal(got, want) {
		t.Fatal(got)
	}
}
