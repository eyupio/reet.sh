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

	"golang.org/x/term"
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
	if err != nil || !entry.Mode().IsRegular() || entry.Size() > 1<<20 {
		return "", false, errors.New("env file must be a regular file of at most 1 MiB, not a symlink")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, errors.New("cannot open env file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(entry, info) || info.Size() > 1<<20 {
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
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
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

func ResolveToken(explicit, envFile string, in io.Reader, out io.Writer) (string, error) {
	token := explicit
	if token == "" {
		token = os.Getenv("REET_TOKEN")
	}
	if token == "" && envFile != "" {
		var err error
		token, err = tokenFromEnvFile(envFile)
		if err != nil {
			return "", err
		}
	}
	if token == "" {
		var err error
		token, err = loadSavedToken()
		if err != nil {
			return "", err
		}
	}
	if token == "" && Interactive(in) {
		fmt.Fprint(out, "Personal API token (Account on the target server; hidden, Enter for unauthenticated server): ")
		secret, err := term.ReadPassword(int(in.(*os.File).Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", errors.New("could not read token from terminal")
		}
		token = string(secret)
		for i := range secret {
			secret[i] = 0
		}
		if token != "" {
			path, err := SaveToken(token)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(out, "Saved personal API token in %s for future scans. Remove it with reet configure --forget-token.\n", path)
		}
	}
	return token, validateScanToken(token)
}
