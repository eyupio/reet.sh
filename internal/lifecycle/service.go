package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type runner func(context.Context, string, ...string) (string, error)

func execute(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	// Tool output can include credentials from Compose/configuration. Never echo it on errors.
	if err != nil {
		return string(b), fmt.Errorf("%s failed; check its status/logs locally", name)
	}
	return string(b), nil
}
func unitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(home, ".config", "systemd", "user", "reet.service"), nil
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", "sh.reet.server.plist"), nil
	}
	return "", fmt.Errorf("native services support Linux systemd and macOS launchd")
}
func systemdQuote(v string) string {
	v = strings.ReplaceAll(v, "%", "%%")
	v = strings.ReplaceAll(v, "$", "$$")
	return strconv.Quote(v)
}
func xmlEscape(v string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;")
	return r.Replace(v)
}
func nativeUnit(d *deployment, state string) string {
	config := filepath.Join(state, "deployment.json")
	if runtime.GOOS == "darwin" {
		return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>sh.reet.server</string><key>ProgramArguments</key><array><string>` + xmlEscape(d.Binary) + `</string><string>serve</string><string>--config</string><string>` + xmlEscape(config) + `</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>StandardOutPath</key><string>` + xmlEscape(filepath.Join(state, "server.log")) + `</string><key>StandardErrorPath</key><string>` + xmlEscape(filepath.Join(state, "server.log")) + `</string></dict></plist>`
	}
	return "[Unit]\nDescription=reet personal skills server\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=" + systemdQuote(d.Binary) + " serve --config " + systemdQuote(config) + "\nRestart=on-failure\nRestartSec=5\nUMask=0077\n\n[Install]\nWantedBy=default.target\n"
}
func (a *App) native(ctx context.Context, action string) (string, error) {
	if runtime.GOOS == "linux" {
		return a.run(ctx, "systemctl", "--user", action, "reet.service")
	}
	target := "gui/" + strconv.Itoa(os.Getuid()) + "/sh.reet.server"
	path, err := unitPath()
	if err != nil {
		return "", err
	}
	switch action {
	case "start":
		return a.run(ctx, "launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), path)
	case "stop":
		return a.run(ctx, "launchctl", "bootout", target)
	case "status":
		return a.run(ctx, "launchctl", "print", target)
	}
	return "", fmt.Errorf("unsupported service action")
}
func (a *App) active(ctx context.Context, d *deployment, state string) (bool, error) {
	if d.Mode == "compose" {
		s, err := a.compose(ctx, state, "ps", "--status", "running", "-q", "reet")
		return strings.TrimSpace(s) != "", err
	}
	if runtime.GOOS == "linux" {
		s, err := a.run(ctx, "systemctl", "--user", "show", "reet.service", "--property=ActiveState", "--value")
		if err != nil {
			return false, err
		}
		switch strings.TrimSpace(s) {
		case "active":
			return true, nil
		case "inactive", "failed":
			return false, nil
		default:
			return false, fmt.Errorf("service is transitioning or not installed; inspect systemctl --user status reet")
		}
	}
	// A booted launchd job must be stopped even when it is between restarts.
	output, err := a.native(ctx, "status")
	if err != nil && !strings.Contains(output, "Could not find service") {
		return false, err
	}
	return err == nil, nil
}
func composeArgs(state string, args ...string) []string {
	hash := sha256.Sum256([]byte(state))
	base := []string{"compose", "--project-name", fmt.Sprintf("reet-%x", hash[:6]), "--project-directory", state, "-f", filepath.Join(state, "compose.json")}
	return append(base, args...)
}
func (a *App) compose(ctx context.Context, state string, args ...string) (string, error) {
	return a.run(ctx, "docker", composeArgs(state, args...)...)
}
func composeConfig(d *deployment) ([]byte, error) {
	host, port, err := net.SplitHostPort(d.Addr)
	if err != nil {
		return nil, fmt.Errorf("address must be HOST:PORT, such as 127.0.0.1:8080")
	}
	if host == "" {
		host = "0.0.0.0"
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	env := map[string]string{"REET_DATA_DIR": "/data", "REET_ADDR": ":8080"}
	for k, v := range d.Environment {
		env[k] = strings.ReplaceAll(v, "$", "$$")
	}
	service := map[string]any{"image": d.Image, "restart": "unless-stopped", "user": fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "ports": []any{map[string]any{"target": 8080, "published": port, "host_ip": host}}, "environment": env, "volumes": []any{map[string]any{"type": "bind", "source": strings.ReplaceAll(d.Data, "$", "$$"), "target": "/data"}}, "healthcheck": map[string]any{"test": []string{"CMD", "/reet", "healthcheck", "--local"}, "interval": "5s", "timeout": "3s", "retries": 12}}
	return json.MarshalIndent(map[string]any{"services": map[string]any{"reet": service}}, "", "  ")
}
func (a *App) writeCompose(d *deployment, state string) error {
	b, err := composeConfig(d)
	if err != nil {
		return err
	}
	return atomicFile(filepath.Join(state, "compose.json"), b, 0600)
}
func (a *App) pull(ctx context.Context, tag string) (string, error) {
	image := imageRepo + ":" + tag
	if _, err := a.run(ctx, "docker", "pull", image); err != nil {
		return "", err
	}
	b, err := a.run(ctx, "docker", "image", "inspect", "--format", "{{json .RepoDigests}}", image)
	if err != nil {
		return "", err
	}
	var digests []string
	if json.Unmarshal([]byte(b), &digests) != nil {
		return "", fmt.Errorf("cannot read pulled image digest")
	}
	for _, digest := range digests {
		if strings.HasPrefix(digest, imageRepo+"@sha256:") && len(strings.TrimPrefix(digest, imageRepo+"@sha256:")) == 64 {
			return digest, nil
		}
	}
	return "", fmt.Errorf("pulled image has no immutable reet digest")
}
func (a *App) stop(ctx context.Context, d *deployment, state string) error {
	if d.Mode == "native" && runtime.GOOS == "darwin" {
		active, err := a.active(ctx, d, state)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}
	}
	if d.Mode == "compose" {
		_, err := a.compose(ctx, state, "stop", "reet")
		return err
	}
	_, err := a.native(ctx, "stop")
	return err
}
func (a *App) start(ctx context.Context, d *deployment, state string) error {
	if d.Mode == "compose" {
		_, err := a.compose(ctx, state, "up", "-d", "--no-deps", "reet")
		return err
	}
	_, err := a.native(ctx, "start")
	return err
}
func localHealth(addr string) string {
	host, port, _ := net.SplitHostPort(addr)
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz"
}
func (a *App) waitHealthy(ctx context.Context, d *deployment) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		b, err := get(ctx, a.client, localHealth(d.Addr), 1<<20)
		var health struct {
			OK      bool   `json:"ok"`
			Version string `json:"version"`
		}
		if err == nil && json.Unmarshal(b, &health) == nil && health.OK && strings.TrimPrefix(health.Version, "v") == strings.TrimPrefix(d.Version, "v") {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("server did not become healthy at the expected version; inspect service logs")
		case <-time.After(time.Second):
		}
	}
}
