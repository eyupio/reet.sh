//go:build linux || darwin

package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
)

func terminalInput(t *testing.T, answers string) *os.File {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { master.Close(); slave.Close() })
	if answers != "" {
		if _, err = master.Write([]byte(answers)); err != nil {
			t.Fatal(err)
		}
	}
	return slave
}

func TestInstallAndUpgradeOfferWizardOnRealTTYWithoutLosingConfirmation(t *testing.T) {
	for _, action := range []string{"install", "upgrade"} {
		t.Run(action, func(t *testing.T) {
			a, state, binary := testApp(t)
			a.In = terminalInput(t, "yes\nyes\n")
			var out bytes.Buffer
			a.Out = &out
			var calls [][]string
			a.interactiveRun = func(_ context.Context, name string, args ...string) error {
				calls = append(calls, append([]string{name}, args...))
				return errors.New("optional setup interrupted")
			}
			args := []string{"--state-dir", state}
			target := binary
			if action == "install" {
				target = filepath.Join(t.TempDir(), "new bin", "reet")
				args = append(args, "--dir", filepath.Dir(target))
			}
			if err := a.Run(action, args); err != nil {
				t.Fatal("deployment failed due to optional wizard", err)
			}
			if len(calls) != 1 || strings.Join(calls[0][1:], " ") != "agent terminal setup" || calls[0][0] != target {
				t.Fatal("wizard was not invoked", calls, out.String())
			}
			want := "old binary"
			if action == "upgrade" {
				want = "new binary"
				if contents(t, binary+".previous") != "old binary" {
					t.Fatal("previous binary lost")
				}
			}
			if contents(t, target) != want || !strings.Contains(out.String(), "installation is saved") {
				t.Fatal("wizard interruption rolled back the deployment", out.String())
			}
		})
	}
}

func TestUnattendedAndSkippedInstallNeverInvokeTerminalWizard(t *testing.T) {
	for _, flags := range [][]string{{"--yes"}, {"--skip-terminal-setup"}, {"--dry-run"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			a, state, _ := testApp(t)
			a.In = terminalInput(t, "yes\n")
			var out bytes.Buffer
			a.Out = &out
			a.interactiveRun = func(context.Context, string, ...string) error {
				t.Fatal("unattended deployment launched a consent wizard")
				return nil
			}
			args := append([]string{"--state-dir", state, "--dir", filepath.Join(t.TempDir(), "bin")}, flags...)
			if err := a.Run("install", args); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "Set up terminal connections now?") {
				t.Fatal("unexpected terminal prompt", out.String())
			}
		})
	}
}

func TestManagedWizardKeepsControllerAndHostScopesSeparate(t *testing.T) {
	for _, mode := range []string{"native", "compose"} {
		t.Run(mode, func(t *testing.T) {
			var out bytes.Buffer
			a := New("test", "test", terminalInput(t, "yes\n"), &out)
			var calls [][]string
			a.interactiveRun = func(_ context.Context, name string, args ...string) error {
				calls = append(calls, append([]string{name}, args...))
				return nil
			}
			d := &deployment{Mode: mode, Binary: "/example/reet", Data: "/example/data"}
			a.offerTerminalSetup(d, "/example/state", true, false)
			if len(calls) != 2 || !strings.Contains(strings.Join(calls[0], " "), "--skip-host-setup") || strings.Join(calls[1], " ") != "/example/reet agent terminal setup" {
				t.Fatal(calls)
			}
			if mode == "compose" && (calls[0][0] != "docker" || !strings.Contains(strings.Join(calls[0], " "), "exec reet /reet terminal start --data-dir /data")) {
				t.Fatal("controller wizard did not run inside its container", calls)
			}
		})
	}
	var out bytes.Buffer
	a := New("test", "test", terminalInput(t, ""), &out)
	a.interactiveRun = func(context.Context, string, ...string) error { t.Fatal("started a stopped controller"); return nil }
	a.offerTerminalSetup(&deployment{Mode: "native", Binary: "/example/reet", Data: "/example/data"}, "/example/state", false, false)
	if !strings.Contains(out.String(), "controller remains stopped") {
		t.Fatal(out.String())
	}
}
