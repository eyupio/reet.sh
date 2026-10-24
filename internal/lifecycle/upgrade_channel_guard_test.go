package lifecycle

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

func TestUpgradeInheritedStablePreferenceCannotReplaceRunningDev(t *testing.T) {
	for _, saved := range []bool{false, true} {
		t.Run(fmt.Sprint("saved=", saved), func(t *testing.T) {
			a, state, binary := testApp(t)
			a.Version = "dev"
			if saved {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				if _, err := clientconfig.Save(map[string]string{"REET_VERSION": "v1.0.0"}); err != nil {
					t.Fatal(err)
				}
				if err := clientconfig.Apply(); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("REET_VERSION", "v1.0.0")
			}
			d := deployment{Mode: "binary", Binary: binary, Version: "v1.0.0"}
			raw, _ := json.Marshal(d)
			file(t, filepath.Join(state, "deployment.json"), string(raw))
			a.client = releaseClient(t, "v1.0.0", archive(t, []string{"reet"}, tar.TypeReg), nil)
			a.run = func(context.Context, string, ...string) (string, error) {
				t.Fatal("executed host action before channel confirmation")
				return "", nil
			}
			err := a.Run("upgrade", []string{"--state-dir", state, "--yes"})
			if err == nil || !strings.Contains(err.Error(), "--version dev") {
				t.Fatal("inherited stable replaced dev", err)
			}
			if contents(t, binary) != "old binary" {
				t.Fatal("binary changed")
			}
			if _, err = os.Stat(binary + ".previous"); !os.IsNotExist(err) {
				t.Fatal("backup changed", err)
			}
			got, err := load(state)
			if err != nil || got.Version != "v1.0.0" {
				t.Fatal(got, err)
			}
		})
	}
}
func TestUpgradeExplicitStableSwitchRemainsAvailable(t *testing.T) {
	a, state, binary := testApp(t)
	a.Version = "dev"
	t.Setenv("REET_VERSION", "v1.0.0")
	a.client = releaseClient(t, "v1.0.0", archive(t, []string{"reet"}, tar.TypeReg), nil)
	a.run = func(_ context.Context, name string, args ...string) (string, error) {
		if strings.Contains(name, ".reet-download-") {
			return "reet v1.0.0 (stable)\n", nil
		}
		return "", fmt.Errorf("unexpected command")
	}
	if err := a.Run("upgrade", []string{"--state-dir", state, "--version", "v1.0.0", "--yes", "--skip-terminal-setup"}); err != nil {
		t.Fatal(err)
	}
	if contents(t, binary) != "new binary" {
		t.Fatal("explicit downgrade blocked")
	}
}
func TestUpgradeInheritedStableDryRunWarnsWithoutApplying(t *testing.T) {
	a, state, binary := testApp(t)
	a.Version = "dev"
	t.Setenv("REET_VERSION", "v1.0.0")
	a.client = releaseClient(t, "v1.0.0", archive(t, []string{"reet"}, tar.TypeReg), nil)
	a.run = func(_ context.Context, name string, args ...string) (string, error) {
		if strings.Contains(name, ".reet-download-") {
			return "reet v1.0.0 (stable)\n", nil
		}
		return "", fmt.Errorf("unexpected command")
	}
	if err := a.Run("upgrade", []string{"--state-dir", state, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if contents(t, binary) != "old binary" || !strings.Contains(a.Out.(*bytes.Buffer).String(), "Channel change preview") {
		t.Fatal("unsafe or unexplained preview")
	}
}
