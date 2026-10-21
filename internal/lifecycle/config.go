package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type deployment struct {
	Mode        string            `json:"mode"`
	Binary      string            `json:"binary"`
	Data        string            `json:"data,omitempty"`
	Addr        string            `json:"addr,omitempty"`
	Version     string            `json:"version"`
	Image       string            `json:"image,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

func stateDefault() string {
	p, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(p, "reet")
}
func dataDefault() string {
	p, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(p, ".local", "share", "reet")
}
func binaryDefault() string { p, _ := os.UserHomeDir(); return filepath.Join(p, ".local", "bin") }
func absolute(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	return filepath.Abs(p)
}
func load(state string) (*deployment, error) {
	b, err := os.ReadFile(filepath.Join(state, "deployment.json"))
	if err != nil {
		return nil, err
	}
	var d deployment
	if err = json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("invalid deployment.json: %w", err)
	}
	if d.Mode != "binary" && d.Mode != "native" && d.Mode != "compose" {
		return nil, fmt.Errorf("unknown deployment mode")
	}
	if !filepath.IsAbs(d.Binary) || (d.Mode != "binary" && (!filepath.IsAbs(d.Data) || d.Addr == "")) {
		return nil, fmt.Errorf("deployment paths must be absolute and server address present")
	}
	return &d, nil
}
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicFile(path, append(b, '\n'), 0600)
}
func atomicFile(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".reet-config-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func acquire(state string) (func(), error) {
	if err := os.MkdirAll(state, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(state, "lifecycle.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("another lifecycle operation may be active (%s); if interrupted, confirm it has stopped before removing the lock", path)
	}
	fmt.Fprintln(f, os.Getpid())
	f.Close()
	return func() { os.Remove(path) }, nil
}

// LoadServerConfig supplies settings only where the calling environment has no override.
// It is used by managed native services, keeping credentials out of process arguments.
func LoadServerConfig(path string) error {
	state := filepath.Dir(path)
	if filepath.Base(path) != "deployment.json" {
		return fmt.Errorf("expected a deployment.json path")
	}
	d, err := load(state)
	if err != nil {
		return err
	}
	if d.Mode != "native" {
		return fmt.Errorf("serve config must describe a native deployment")
	}
	settings := map[string]string{"REET_DATA_DIR": d.Data, "REET_ADDR": d.Addr}
	for _, k := range serverEnv {
		if v, ok := d.Environment[k]; ok {
			settings[k] = v
		}
	}
	for k, v := range settings {
		if _, ok := os.LookupEnv(k); !ok {
			if err = os.Setenv(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}

var serverEnv = []string{"REET_AUTH", "REET_TOKEN", "SKILLS_SH_TOKEN", "REET_LOG", "REET_TOPICS", "REET_REFRESH", "REET_CONTROLLER_URL", "REET_CONTROLLER_TOKEN", "REET_ENV"}

func jsonBytes(v any) ([]byte, error) { return json.Marshal(v) }
