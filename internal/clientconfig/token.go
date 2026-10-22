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
	entry, err := os.Lstat(path)
	if err != nil || !entry.Mode().IsRegular() || entry.Size() > 1<<20 {
		return "", errors.New("token env file must be a regular file of at most 1 MiB, not a symlink")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("cannot open token env file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(entry, info) || info.Size() > 1<<20 {
		return "", errors.New("token env file must be a regular file of at most 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return "", errors.New("cannot read token env file within size limit")
	}
	token, seen := "", false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "REET_TOKEN" {
			continue
		}
		if seen {
			return "", errors.New("duplicate REET_TOKEN in env file")
		}
		seen = true
		value = strings.TrimSpace(value)
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
			q := value[0]
			end := strings.IndexByte(value[1:], q)
			if end < 0 {
				return "", errors.New("invalid REET_TOKEN quoting in env file")
			}
			end++
			rest := strings.TrimSpace(value[end+1:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return "", errors.New("invalid REET_TOKEN suffix in env file")
			}
			value = value[1:end]
		} else if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		if strings.ContainsAny(value, "$`\\") {
			return "", errors.New("REET_TOKEN must be literal; env file expansion is not supported")
		}
		if err := validateScanToken(value); err != nil {
			return "", err
		}
		token = value
	}
	if scanner.Err() != nil {
		return "", errors.New("invalid or oversized env file line")
	}
	if !seen || token == "" {
		return "", errors.New("env file has no non-empty REET_TOKEN")
	}
	return token, nil
}

func validateScanToken(value string) error {
	if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("invalid scan token")
	}
	return nil
}

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
