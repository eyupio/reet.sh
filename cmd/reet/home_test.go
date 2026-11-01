package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCLIProbe(t *testing.T) {
	if os.Getenv("REET_CLI_PROBE") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"reet"}, os.Args[i+1:]...)
			main()
			os.Exit(0)
		}
	}
	os.Exit(99)
}

func TestRemovedDashboardAndPlainCompatibilityEntryPoints(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
		out  string
	}{
		{[]string{"tui"}, 2, ""},
		{[]string{"version", "--tui"}, 1, ""},
		{[]string{"--non-interactive", "version"}, 0, "reet dev (unknown)"},
		{[]string{"--no-animation", "version"}, 0, "reet dev (unknown)"},
		{[]string{"help"}, 0, "Check & maintain"},
	} {
		cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCLIProbe$", "--"}, tc.args...)...)
		cmd.Env = append(cleanHomeEnvironment(os.Environ()), "REET_CLI_PROBE=1")
		output, err := cmd.Output()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatal(err)
			}
			code = exit.ExitCode()
		}
		if code != tc.code || !strings.Contains(string(output), tc.out) || strings.Contains(string(output), "\x1b") {
			t.Fatalf("%v: code=%d out=%q err=%v", tc.args, code, output, err)
		}
	}
}

func TestUpgradeContinuationKeepsSelectedCommandAndFlagsAndClearsMarkers(t *testing.T) {
	for _, command := range []string{"upgrade", "update"} {
		t.Run(command, func(t *testing.T) {
			args := []string{command, "--version", "dev", "--state-dir", "/tmp/reet state", "--no-animation", "--skip-terminal-setup"}
			calls := 0
			err := resumeUpgrade("/opt/reet", args, func(path string, argv, env []string) error {
				calls++
				if path != "/opt/reet" || !slices.Equal(argv, append([]string{path}, args...)) {
					t.Fatalf("argv=%v", argv)
				}
				for _, entry := range env {
					key, value, _ := strings.Cut(entry, "=")
					if key == homeReturnEnv || key == homeResumeEnv {
						t.Setenv(key, value)
					}
				}
				return nil
			})
			if err != nil || calls != 1 {
				t.Fatal(err, calls)
			}
			r, err := takeHomeContinuation(args)
			if err != nil || r == nil || !slices.Equal(r.Args, args) {
				t.Fatal(r, err)
			}
			for _, key := range []string{homeReturnEnv, homeResumeEnv} {
				if _, ok := os.LookupEnv(key); ok {
					t.Fatal("stale marker", key)
				}
			}
			runs := 0
			pauses := 0
			code, err := homeLoop(homeActions{
				choose: func() ([]string, error) { return nil, nil },
				run:    func([]string) (int, error) { runs++; return 0, nil },
				pause:  func(int) (bool, error) { pauses++; return false, nil },
			}, nil, r)
			if code != 0 || err != nil || runs != 0 || pauses != 1 {
				t.Fatal("completed upgrade repeated", runs, pauses, err)
			}
		})
	}
}

func TestAnUpgradeReplacementFromHomeReexecsTheSelectedFlagsExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reet")
	if err := os.WriteFile(path, []byte("old"), 0700); err != nil {
		t.Fatal(err)
	}
	args := []string{"upgrade", "--version", "dev", "--state-dir", "/tmp/reet state", "--no-animation"}
	runs, reexecs := 0, 0
	code, err := runHomeSelection(path, args, func(got []string) int {
		runs++
		if !slices.Equal(got, args) {
			t.Fatal(got)
		}
		if err := os.WriteFile(path+".next", []byte("new"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".next", path); err != nil {
			t.Fatal(err)
		}
		return 0
	}, func(got string, argv, env []string) error {
		reexecs++
		if got != path || !slices.Equal(argv[1:], args) || !slices.Contains(env, homeReturnEnv+"=1") {
			t.Fatal(got, argv)
		}
		return nil
	})
	if code != 0 || err != nil || runs != 1 || reexecs != 1 {
		t.Fatal(code, err, runs, reexecs)
	}
	_, err = runHomeSelection(path, args, func([]string) int { return 0 }, func(string, []string, []string) error { t.Fatal("unchanged check re-execed"); return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestInvalidUpgradeContinuationCannotSkipAnotherCommand(t *testing.T) {
	t.Setenv(homeReturnEnv, "1")
	t.Setenv(homeResumeEnv, `{"Args":["upgrade","--yes"],"Code":0}`)
	if _, err := takeHomeContinuation([]string{"doctor"}); err == nil {
		t.Fatal("stale continuation accepted")
	}
	if os.Getenv(homeReturnEnv) != "" || os.Getenv(homeResumeEnv) != "" {
		t.Fatal("markers not cleared on invalid continuation")
	}
}

func TestHomePreservesFailureExitStatusAndDoesNotRetry(t *testing.T) {
	runs := 0
	code, err := homeLoop(homeActions{
		choose: func() ([]string, error) { return []string{"doctor", "--json"}, nil },
		run: func(args []string) (int, error) {
			runs++
			if !slices.Equal(args, []string{"doctor", "--json"}) {
				t.Fatal(args)
			}
			return 7, nil
		},
		pause: func(code int) (bool, error) {
			if code != 7 {
				t.Fatal(code)
			}
			return true, nil
		},
	}, nil, nil)
	if err != nil || code != 7 || runs != 1 {
		t.Fatal(code, err, runs)
	}
}

func TestFailedReexecReturnsTheFailureWithoutRetryingUpgrade(t *testing.T) {
	want := errors.New("exec refused")
	calls := 0
	err := resumeUpgrade("/opt/reet", []string{"upgrade", "--version", "dev"}, func(string, []string, []string) error { calls++; return want })
	if !errors.Is(err, want) || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestStructuredArgumentsDisableInteractivePresentation(t *testing.T) {
	for _, args := range [][]string{{"--json"}, {"--json=true"}, {"--output", "json"}, {"--output=json"}, {"--non-interactive"}} {
		if !structuredArgs(args) {
			t.Fatal(args)
		}
	}
	if structuredArgs([]string{"--json=false"}) {
		t.Fatal("false JSON flag treated as structured")
	}
}

func TestHomeChildExitStatusAndContextCancellation(t *testing.T) {
	if code := runHomeCommand(context.Background(), "/bin/sh", []string{"-c", "exit 7"}); code != 7 {
		t.Fatal(code)
	}
	if code := runHomeCommand(context.Background(), "/bin/sh", []string{"-c", "kill -TERM $$"}); code != 143 {
		t.Fatal(code)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := runHomeCommand(ctx, "/bin/sh", []string{"-c", "trap 'exit 19' INT; while :; do :; done"}); code == 0 {
		t.Fatal("cancelled child reported success")
	}
}
