package clientconfig

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// tokenFromEnvFile reads only REET_TOKEN. It never executes shell expressions or
// exports unrelated deployment settings into the CLI process.
func tokenFromEnvFile(path string) (string, error) {
	token, seen, err := envFileValue(path, "REET_TOKEN")
	if err != nil {
		return "", err
	}
	if !seen || token == "" {
		return "", errors.New("env file has no non-empty REET_TOKEN")
	}
	if strings.ContainsAny(token, "$`\\") {
		return "", errors.New("REET_TOKEN must be literal; env file expansion is not supported")
	}
	if err := validateScanToken(token); err != nil {
		return "", err
	}
	return token, nil
}

// envFileValue reads one literal key from an env file without sourcing it.
func envFileValue(path, wantKey string) (string, bool, error) {
	entry, err := os.Lstat(path)
	invalid := err != nil || !entry.Mode().IsRegular() || entry.Size() > 1<<20
	if invalid {
		return "", false, errors.New("env file must be a regular file of at most 1 MiB, not a symlink")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, errors.New("cannot open env file")
	}
	defer f.Close()
	info, err := f.Stat()
	changed := err != nil || !info.Mode().IsRegular() || !os.SameFile(entry, info) ||
		info.Size() > 1<<20
	if changed {
		return "", false, errors.New("env file must be a regular file of at most 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return "", false, errors.New("cannot read env file within size limit")
	}
	result, seen := "", false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != wantKey {
			continue
		}
		if seen {
			return "", false, fmt.Errorf("duplicate %s in env file", wantKey)
		}
		seen = true
		value = strings.TrimSpace(value)
		quoted := len(value) > 0 && (value[0] == '\'' || value[0] == '"')
		if quoted {
			q := value[0]
			end := strings.IndexByte(value[1:], q)
			if end < 0 {
				return "", false, fmt.Errorf("invalid %s quoting in env file", wantKey)
			}
			end++
			rest := strings.TrimSpace(value[end+1:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return "", false, fmt.Errorf("invalid %s suffix in env file", wantKey)
			}
			value = value[1:end]
		} else if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		result = value
	}
	if scanner.Err() != nil {
		return "", false, errors.New("invalid or oversized env file line")
	}
	return result, seen, nil
}

func validateScanToken(value string) error {
	if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("invalid scan token")
	}
	return nil
}

// SavedToken returns the token saved by a previous interactive command.
func SavedToken() (string, error) { return loadSavedToken() }

// TokenPath is the private file that stores an interactively entered personal
// token. It is deliberately separate from client.env, which holds no secrets.
func TokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reet", "token"), nil
}

func loadSavedToken() (string, error) {
	path, err := TokenPath()
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 4098 {
		return "", errors.New("saved token must be a small regular file, not a symlink")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("saved token %s is accessible by other users; run chmod 600 %s or forget it with reet configure --forget-token", path, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimRight(string(data), "\r\n")
	if err := validateScanToken(token); err != nil {
		return "", err
	}
	return token, nil
}

// SaveToken stores an interactively entered token with owner-only permissions.
func SaveToken(token string) (string, error) {
	if token == "" {
		return "", errors.New("empty token")
	}
	if err := validateScanToken(token); err != nil {
		return "", err
	}
	path, err := TokenPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return "", err
	}
	if _, err := io.WriteString(f, token); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return "", err
	}
	return path, nil
}

// ForgetToken removes the saved token. A missing file is not an error.
func ForgetToken() (string, error) {
	path, err := TokenPath()
	if err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

// ResolveToken returns the token to use, looking at the explicit value,
// REET_TOKEN, an env file, the saved token and finally a terminal prompt.
func ResolveToken(explicit, envFile string, in io.Reader, out io.Writer) (string, error) {
	token, _, err := ResolveTokenSource(explicit, envFile, in, out)
	return token, err
}

// ResolveTokenSource is ResolveToken that also says where the token came from.
func ResolveTokenSource(explicit, envFile string, in io.Reader, out io.Writer) (string, Source, error) {
	token, source := explicit, SourceFlag
	if token == "" {
		token, source = os.Getenv("REET_TOKEN"), SourceEnv
	}
	if token == "" && envFile != "" {
		var err error
		token, err = tokenFromEnvFile(envFile)
		if err != nil {
			return "", SourceNone, err
		}
		source = Source("env file " + envFile)
	}
	if token == "" && envFile == "" {
		// REET_ENV_FILE or ./.env, the same files the server and TUI read.
		if file := Discover("", "", "").TokenFile; file != "" {
			var err error
			if token, err = tokenFromEnvFile(file); err != nil {
				return "", SourceNone, err
			}
			source = Source("env file " + file)
		}
	}
	if token == "" {
		var err error
		token, err = loadSavedToken()
		if err != nil {
			return "", SourceNone, err
		}
		source = SourceSaved
	}
	if token == "" && interactiveFn(in) {
		fmt.Fprint(out, "Personal API token (Account on the target server; hidden, Enter for unauthenticated server): ")
		entered, err := readSecret(in)
		fmt.Fprintln(out)
		if err != nil {
			return "", SourceNone, err
		}
		token, source = entered, SourcePrompt
		if token != "" {
			path, err := SaveToken(token)
			if err != nil {
				return "", SourceNone, err
			}
			fmt.Fprintf(out, "Saved personal API token in %s for future scans. Remove it with reet configure --forget-token.\n", path)
		}
	}
	if token == "" {
		source = SourceNone
	}
	return token, source, validateScanToken(token)
}
