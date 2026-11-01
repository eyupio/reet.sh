// Package clientconfig stores non-secret CLI defaults without executing env files.
package clientconfig

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattn/go-isatty"
)

const maxBytes = 16 << 10

var versionPattern = regexp.MustCompile(`^(latest|dev|v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)$`)

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reet", "client.env"), nil
}
func Interactive(in io.Reader) bool {
	if os.Getenv("REET_NON_INTERACTIVE") != "" {
		return false
	}
	f, ok := in.(*os.File)
	return ok && isatty.IsTerminal(f.Fd())
}
func Validate(key, value string) error {
	if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("invalid %s", key)
	}
	switch key {
	case "REET_VERSION":
		if !versionPattern.MatchString(value) {
			return errors.New("REET_VERSION must be dev, latest or a version such as v1.2.3")
		}
	case "REET_URL":
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("REET_URL must be an HTTP(S) server URL without credentials, query or fragment")
		}
	default:
		return errors.New("unsupported client setting")
	}
	return nil
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func read(path string) (map[string]string, error) {
	values := map[string]string{}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, errors.New("client.env must be a small regular file, not a symlink")
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.OpenFile(filepath.Base(path), os.O_RDONLY|readFlags, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("client.env changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, errors.New("client.env is too large")
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok || len(raw) < 2 || raw[0] != '\'' || raw[len(raw)-1] != '\'' {
			return nil, errors.New("invalid client.env; use reet configure to manage defaults")
		}
		value := strings.ReplaceAll(raw[1:len(raw)-1], "'\\''", "'")
		if quote(value) != raw {
			return nil, errors.New("invalid quoting in client.env")
		}
		if _, exists := values[key]; exists {
			return nil, errors.New("duplicate client.env setting")
		}
		if err := Validate(key, value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

// Apply fills missing process environment values; it never changes the parent shell.
func Apply() error {
	path, err := Path()
	if err != nil {
		return err
	}
	values, err := read(path)
	if err != nil {
		return err
	}
	for key, value := range values {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}
func Save(changes map[string]string) (string, error) {
	for key, value := range changes {
		if err := Validate(key, value); err != nil {
			return "", err
		}
	}
	path, err := Path()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", errors.New("client defaults are locked; retry after the other command finishes")
	}
	lock.Close()
	defer os.Remove(path + ".lock")
	values, err := read(path)
	if err != nil {
		return "", err
	}
	for key, value := range changes {
		values[key] = value
	}
	var b strings.Builder
	b.WriteString("# reet CLI defaults; read as data, never executed. No tokens belong here.\n")
	for _, key := range []string{"REET_VERSION", "REET_URL"} {
		if value := values[key]; value != "" {
			fmt.Fprintf(&b, "%s=%s\n", key, quote(value))
		}
	}
	f, err := os.CreateTemp(dir, ".client-env-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.WriteString(b.String()); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

// Prompt asks for one value, with bounded input. Callers decide whether prompting
// is appropriate; prompts go to stderr in commands that have structured stdout.
func Prompt(in io.Reader, out io.Writer, key, current string) (string, error) {
	if current != "" {
		if err := Validate(key, current); err != nil {
			return "", err
		}
	}
	label := "Target reet server URL"
	if key == "REET_VERSION" {
		label = "Release channel or version"
	}
	value, err := Ask(in, out, label, current)
	if err != nil {
		return "", err
	}
	if err = Validate(key, value); err != nil {
		return "", err
	}
	return value, nil
}

// Ask reads one answer for an arbitrary setup question without consuming the
// next answer. Callers validate their own domain-specific values.
func Ask(in io.Reader, out io.Writer, label, current string) (string, error) {
	if current != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, current)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	// Read one byte at a time to avoid consuming a later confirmation from stdin.
	var b strings.Builder
	one := make([]byte, 1)
	for b.Len() <= 4096 {
		n, err := in.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				break
			}
			b.WriteByte(one[0])
		}
		if err != nil {
			if err == io.EOF {
				return "", errors.New("input ended; no defaults saved")
			}
			return "", err
		}
	}
	value := strings.TrimSpace(b.String())
	if value == "" {
		value = current
	}
	if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("invalid or oversized answer")
	}
	return value, nil
}
func ResolveURL(value string, in io.Reader, out io.Writer) (string, error) {
	if value != "" {
		return value, nil
	}
	if !Interactive(in) {
		return "", errors.New("target server is missing; set REET_URL, pass --server/--url, or run reet configure")
	}
	value, err := Prompt(in, out, "REET_URL", "")
	if err != nil {
		return "", err
	}
	path, err := Save(map[string]string{"REET_URL": value})
	if err != nil {
		return "", err
	}
	fmt.Fprintf(out, "Saved target server in %s\n", path)
	return value, nil
}
