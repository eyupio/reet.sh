package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// readLocalEnv reads literal settings without sourcing a shell or printing values.
func readLocalEnv(explicit string) (map[string]string, error) {
	path := explicit
	if path == "" {
		path = os.Getenv("REET_ENV_FILE")
	}
	required := path != ""
	if path == "" {
		path = ".env"
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) && !required {
		return map[string]string{}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("env file must be a regular file of at most 1 MiB, not a symlink")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read env file")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("env file changed while opening")
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(io.LimitReader(f, (1<<20)+1))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !strings.HasPrefix(key, "REET_") {
			continue
		}
		for _, c := range key {
			if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
				return nil, errors.New("invalid env setting name")
			}
		}
		if _, seen := values[key]; seen {
			return nil, fmt.Errorf("duplicate %s in env file", key)
		}
		value = strings.TrimSpace(value)
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
			q := value[0]
			end := strings.IndexByte(value[1:], q)
			if end < 0 {
				return nil, fmt.Errorf("invalid %s quoting", key)
			}
			end++
			rest := strings.TrimSpace(value[end+1:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return nil, fmt.Errorf("invalid %s suffix", key)
			}
			value = value[1:end]
		} else if at := strings.Index(value, " #"); at >= 0 {
			value = strings.TrimSpace(value[:at])
		}
		values[key] = value
	}
	if scanner.Err() != nil {
		return nil, errors.New("could not parse env file")
	}
	return values, nil
}

func applyServerEnvFile(path string) error {
	values, err := readLocalEnv(path)
	if err != nil {
		return err
	}
	for key, value := range values {
		if _, set := os.LookupEnv(key); !set {
			if err = os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	if os.Getenv("REET_PUBLIC_URL") == "" && os.Getenv("REET_URL") != "" {
		return os.Setenv("REET_PUBLIC_URL", os.Getenv("REET_URL"))
	}
	return nil
}
