package terminalhost

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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

// With gh at an absolute path, the args name gh for github.com only and
// leave the person's own credential helpers alone: a machine whose gh is
// signed out still fetches with the helper their ~/.gitconfig names.
func TestGhCredentialArgsNameOnlyGitHub(t *testing.T) {
	dir := t.TempDir()
	gh := filepath.Join(dir, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	want := []string{"-c", "credential.https://github.com.helper=!'" + gh + "' auth git-credential"}
	if got := ghCredentialArgs(); !slices.Equal(got, want) {
		t.Fatal(got)
	}
	if slices.Contains(ghCredentialArgs(), "credential.helper=") {
		t.Fatal("the inherited credential helpers were reset", ghCredentialArgs())
	}
	t.Setenv("PATH", t.TempDir())
	if got := ghCredentialArgs(); got != nil {
		t.Fatal("args without gh", got)
	}
}

// The helper string is run by a shell, so a gh in a folder whose name has a
// space is still one word to it.
func TestGhCredentialArgsQuoteThePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my tools")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(dir, "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\necho spaced\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	args := ghCredentialArgs()
	want := "credential.https://github.com.helper=!'" + gh + "' auth git-credential"
	if !slices.Equal(args, []string{"-c", want}) {
		t.Fatal(args)
	}
	// The shell really runs it: the quoted word is the program, not two.
	helper, _ := strings.CutPrefix(want, "credential.https://github.com.helper=!")
	out, err := exec.Command("/bin/sh", "-c", helper).Output()
	if err != nil || strings.TrimSpace(string(out)) != "spaced" {
		t.Fatalf("the shell could not run %q: %v: %q", helper, err, out)
	}
}

func TestNetworkGitEnvDisablesPrompt(t *testing.T) {
	want := []string{"GIT_TERMINAL_PROMPT=0"}
	if got := networkGitEnv(); !slices.Equal(got, want) {
		t.Fatal(got)
	}
}
