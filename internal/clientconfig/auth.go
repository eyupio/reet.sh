package clientconfig

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// This file is the one place a reet CLI command reacts to a controller that
// rejects its token. A command that talks to the controller should:
//
//  1. turn the HTTP status into an error with NewAuthError (401 and 403 only),
//  2. build a Credential with NewCredential, and
//  3. run its request inside Authorize.
//
// Authorize then does what every command should: name the source of the
// rejected token, fall back to a different saved token, ask for a new one
// when a person is at the terminal, save it when that helps, and retry. It
// never prompts without a terminal, for a 403, or for any other error, and
// only a different saved token is tried without one.

// Source says where the token in use came from, so a rejection can say which
// file or variable to fix.
type Source string

const (
	SourceFlag   Source = "the --token option"
	SourceEnv    Source = "REET_TOKEN"
	SourceSaved  Source = "the saved token"
	SourcePrompt Source = "the token you entered"
	SourceNone   Source = "no token"
)

// maxTokenPrompts bounds typing mistakes without looping forever.
const maxTokenPrompts = 3

// Hooks so tests can drive the terminal.
var (
	interactiveFn = Interactive
	readSecret    = readSecretFromTerminal
)

func readSecretFromTerminal(in io.Reader) (string, error) {
	f, ok := in.(*os.File)
	if !ok {
		return "", errors.New("could not read token from terminal")
	}
	secret, err := term.ReadPassword(int(f.Fd()))
	if err != nil {
		return "", errors.New("could not read token from terminal")
	}
	token := string(secret)
	for i := range secret {
		secret[i] = 0
	}
	return token, nil
}

// AuthError is a controller refusal of the credentials a command sent.
type AuthError struct {
	Status  int    // 401 or 403
	Message string // what the controller said, if anything
}

func (e *AuthError) Error() string {
	var text string
	switch e.Status {
	case 401:
		text = "server replied 401 Unauthorized"
	default:
		text = fmt.Sprintf("server replied %d Forbidden", e.Status)
	}
	if e.Message != "" {
		text += ": " + e.Message
	}
	if e.Status == 401 {
		text += "; use a personal API token from Account on the target server via REET_TOKEN, --env-file .env or the interactive prompt, and remove a stale saved token with reet configure --forget-token"
	}
	return text
}

// NewAuthError returns an *AuthError for a 401 or 403 response and nil for any
// other status, so callers can write: if err := NewAuthError(...); err != nil.
func NewAuthError(status int, message string) error {
	if status != 401 && status != 403 {
		return nil
	}
	return &AuthError{Status: status, Message: message}
}

// IsRejected reports whether err is a 401: the controller does not accept the
// token. A 403 is not included; the token is valid but lacks the needed role,
// and another token from the same account would not change that.
func IsRejected(err error) bool {
	var ae *AuthError
	return errors.As(err, &ae) && ae.Status == 401
}

// Credential is the token a command is using and where it came from.
type Credential struct {
	Token  string
	Source Source
	Server string    // named in the prompt
	In     io.Reader // the terminal to ask on
	Out    io.Writer // where prompts and notes go; keep it off structured stdout

	prompts    int
	triedSaved bool
}

// NewCredential resolves a token as ResolveToken does and remembers its source.
func NewCredential(explicit, envFile, server string, in io.Reader, out io.Writer) (*Credential, error) {
	token, source, err := ResolveTokenSource(explicit, envFile, in, out)
	if err != nil {
		return nil, err
	}
	if token == "" {
		source = SourceNone
	}
	return &Credential{Token: token, Source: source, Server: server, In: in, Out: out}, nil
}

// Authorize runs op with the credential's token. When op fails with a
// rejected token it asks Retry for a replacement and runs op again.
func Authorize(c *Credential, op func(token string) error) error {
	for {
		err := op(c.Token)
		retry, retryErr := c.Retry(err)
		if retryErr != nil {
			return retryErr
		}
		if !retry {
			if IsRejected(err) {
				return fmt.Errorf("%w (token from %s)", err, c.Source)
			}
			return err
		}
	}
}

// Retry decides what to do after op returned err. It reports true when
// c.Token has been replaced and op should run again.
func (c *Credential) Retry(err error) (bool, error) {
	if !IsRejected(err) {
		return false, nil
	}
	out := c.Out
	if out == nil {
		out = io.Discard
	}

	// A different saved token is the cheapest fix and needs no typing.
	if !c.triedSaved {
		c.triedSaved = true
		if saved, savedErr := loadSavedToken(); savedErr == nil && saved != "" && saved != c.Token {
			fmt.Fprintf(out, "The server rejected the token from %s; trying the saved token.\n", c.Source)
			c.Token, c.Source = saved, SourceSaved
			return true, nil
		}
	}
	// Past this point only a person can help.
	if !interactiveFn(c.In) || c.prompts >= maxTokenPrompts {
		return false, nil
	}

	fmt.Fprintf(out, "The server rejected the token from %s.", c.Source)
	if ae := new(AuthError); errors.As(err, &ae) && ae.Message != "" {
		fmt.Fprintf(out, " It said: %s.", ae.Message)
	}
	where := "Account on the target server"
	if c.Server != "" {
		where = "Account on " + c.Server
	}
	fmt.Fprintf(out, "\nEnter a personal API token from %s (hidden, Enter to cancel): ", where)
	token, readErr := readSecret(c.In)
	fmt.Fprintln(out)
	if readErr != nil {
		return false, readErr
	}
	c.prompts++
	if token == "" {
		return false, nil
	}
	if err := validateScanToken(token); err != nil {
		return false, err
	}

	// REET_TOKEN, --token and an env file outrank the saved token, so saving
	// would not stop the next run from using the stale one. Say what to change.
	switch c.Source {
	case SourceSaved, SourcePrompt, SourceNone:
		path, saveErr := SaveToken(token)
		if saveErr != nil {
			return false, saveErr
		}
		fmt.Fprintf(out, "Saved the token in %s. Remove it with reet configure --forget-token.\n", path)
	default:
		fmt.Fprintf(out, "Using it for this run only: %s takes precedence over a saved token. Replace the token there, or remove it, to stop seeing this.\n", c.Source)
	}
	c.Token, c.Source = token, SourcePrompt
	return true, nil
}
