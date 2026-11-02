package clientconfig

import (
	"os"
	"strings"
)

// Connection is what a command can already work out about the controller,
// the token and this machine's name, so forms only ask for what is missing.
// It never holds the token itself, only where one was found.
type Connection struct {
	Server      string // controller URL
	ServerFrom  string // where Server came from, for display
	Host        string // environment name for this machine
	TokenFrom   string // empty when no token was found
	TokenFile   string // env file holding the token, if that is where it is
	EnvFilePath string // the env file that was read, if any
}

// HasToken reports whether a token was found without prompting.
func (c Connection) HasToken() bool { return c.TokenFrom != "" }

// EnvFileCandidates lists env files in lookup order: REET_ENV_FILE, then .env
// in the current directory.
func envFileCandidates() []string {
	var out []string
	if p := os.Getenv("REET_ENV_FILE"); p != "" {
		out = append(out, p)
	}
	return append(out, ".env")
}

// Discover looks, in order, at explicit values, the process environment,
// client.env, an env file (REET_ENV_FILE or ./.env) and the saved token.
// Unreadable sources are skipped; Discover never prompts and never fails.
func Discover(server, host, tokenFile string) Connection {
	var c Connection
	files := envFileCandidates()
	if tokenFile != "" {
		files = append([]string{tokenFile}, files...)
	}
	fileValue := func(key string) (value, path string) {
		for _, p := range files {
			if v, ok, err := envFileValue(p, key); err == nil && ok && v != "" {
				return v, p
			}
		}
		return "", ""
	}
	setServer := func(v, from string) {
		if c.Server == "" && v != "" && Validate("REET_URL", v) == nil {
			c.Server, c.ServerFrom = v, from
		}
	}
	setServer(server, "option")
	setServer(os.Getenv("REET_URL"), "REET_URL")
	if path, err := Path(); err == nil {
		if values, err := read(path); err == nil {
			setServer(values["REET_URL"], "saved defaults")
		}
	}
	for _, key := range []string{"REET_URL", "REET_CONTROLLER_URL"} {
		v, p := fileValue(key)
		setServer(v, p)
	}

	c.Host = strings.TrimSpace(host)
	if c.Host == "" {
		c.Host = os.Getenv("REET_ENV")
	}
	if c.Host == "" {
		c.Host, _ = fileValue("REET_ENV")
	}
	if c.Host == "" {
		c.Host, _ = os.Hostname()
	}

	switch {
	case os.Getenv("REET_TOKEN") != "":
		c.TokenFrom = "REET_TOKEN"
	default:
		for _, p := range files {
			if _, err := tokenFromEnvFile(p); err == nil {
				c.TokenFrom, c.TokenFile, c.EnvFilePath = p, p, p
				break
			}
		}
		if c.TokenFrom == "" {
			if t, err := loadSavedToken(); err == nil && t != "" {
				c.TokenFrom = "saved token"
			}
		}
	}
	return c
}

// EnvName is the machine name set by REET_ENV, in the process environment or an
// env file. It is empty when neither sets one, so callers fall back to the hostname.
func EnvName() string {
	if v := os.Getenv("REET_ENV"); v != "" {
		return v
	}
	for _, p := range envFileCandidates() {
		if v, ok, err := envFileValue(p, "REET_ENV"); err == nil && ok && v != "" {
			return v
		}
	}
	return ""
}
