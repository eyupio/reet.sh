package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/eyupio/reet.sh/internal/cliui"
)

const homeReturnEnv = "REET_RETURN_HOME"
const homeResumeEnv = "REET_UPGRADE_CONTINUATION"

type homeResult struct {
	Args []string
	Code int
}

func cleanHomeEnvironment(env []string) []string {
	return slices.DeleteFunc(append([]string{}, env...), func(s string) bool {
		return strings.HasPrefix(s, homeReturnEnv+"=") || strings.HasPrefix(s, homeResumeEnv+"=")
	})
}

// The operation already finished, including cleanup, in the previous process.
// Resume its result after exec, never repeat the upgrade. Bind the marker to
// the original argv so stale continuation state cannot affect another command.
func takeHomeContinuation(args []string) (*homeResult, error) {
	returnHome, raw := os.Getenv(homeReturnEnv), os.Getenv(homeResumeEnv)
	_ = os.Unsetenv(homeReturnEnv)
	_ = os.Unsetenv(homeResumeEnv)
	if returnHome == "" && raw == "" {
		return nil, nil
	}
	var r homeResult
	if returnHome != "1" || len(raw) > 64<<10 || json.Unmarshal([]byte(raw), &r) != nil ||
		r.Code != 0 || !slices.Equal(args, r.Args) || len(args) == 0 || (args[0] != "upgrade" && args[0] != "update") {
		return nil, fmt.Errorf("invalid upgrade continuation; restart reet (the upgrade will not be repeated)")
	}
	return &r, nil
}

func resumeUpgrade(path string, args []string, reexec func(string, []string, []string) error) error {
	raw, err := json.Marshal(homeResult{Args: args})
	if err != nil {
		return err
	}
	env := append(cleanHomeEnvironment(os.Environ()), homeReturnEnv+"=1", homeResumeEnv+"="+string(raw))
	return reexec(path, append([]string{path}, args...), env)
}

type homeActions struct {
	choose func() ([]string, error)
	run    func([]string) (int, error)
	pause  func(int) (bool, error)
}

func homeLoop(a homeActions, initial []string, resumed *homeResult) (int, error) {
	if resumed != nil {
		quit, err := a.pause(resumed.Code)
		if quit || err != nil {
			return resumed.Code, err
		}
	}
	for {
		args := initial
		initial = nil
		if len(args) == 0 {
			var err error
			args, err = a.choose()
			if err != nil || len(args) == 0 {
				return 0, err
			}
		}
		code, err := a.run(args)
		if err != nil {
			return 1, err
		}
		quit, err := a.pause(code)
		if quit || err != nil {
			return code, err
		}
	}
}

func runHome(initial []string, resumed *homeResult) (int, error) {
	path, err := os.Executable()
	if err != nil {
		return 1, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return 1, err
	}
	ctx := context.Background()
	return homeLoop(homeActions{
		choose: func() ([]string, error) { return cliui.Choose(ctx, os.Stdin, os.Stdout, version) },
		pause:  func(code int) (bool, error) { return cliui.Pause(ctx, os.Stdin, os.Stdout, code) },
		run: func(args []string) (int, error) {
			return runHomeSelection(path, args, func(args []string) int { return runHomeCommand(ctx, path, args) }, reexecBinary)
		},
	}, initial, resumed)
}

func runHomeSelection(path string, args []string, run func([]string) int, reexec func(string, []string, []string) error) (int, error) {
	before, _ := os.Stat(path)
	code := run(args)
	after, err := os.Stat(path)
	if code == 0 && len(args) > 0 && (args[0] == "upgrade" || args[0] == "update") && err == nil && before != nil && !os.SameFile(before, after) {
		if err := resumeUpgrade(path, args, reexec); err != nil {
			return code, fmt.Errorf("upgrade finished, but could not restart home: %w", err)
		}
	}
	return code, nil
}

// The launcher is fully stopped before the child starts. Inheriting the real
// terminal preserves password input, confirmations and both output streams.
// Do not use CommandContext: killing a child would interrupt its recovery.
func runHomeCommand(ctx context.Context, path string, args []string) int {
	if ctx.Err() != nil {
		return 130
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = cleanHomeEnvironment(os.Environ())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	done, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		cancelled := ctx.Done()
		for {
			select {
			case sig := <-signals:
				_ = cmd.Process.Signal(sig)
			case <-cancelled:
				_ = cmd.Process.Signal(os.Interrupt)
				cancelled = nil
			case <-done:
				return
			}
		}
	}()
	err := cmd.Wait()
	close(done)
	<-joined
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if code := exit.ExitCode(); code >= 0 {
			return code
		}
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return 130
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
