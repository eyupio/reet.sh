// Package mail is reet's email: an outbox every message goes through, a queue
// that sends it with retries, an SMTP sender, the templates, and the notices
// the rest of reet asks for (password resets, invitations, welcomes and team
// notifications).
//
// Every message is written to the outbox before anything tries to send it,
// and the request that caused it never waits on a mail server. A mail server
// that is down, slow or refusing costs a retry later, never an email and
// never a page that hangs. SMTP is the only protocol: every transactional
// provider (Postmark, SES, Resend, Mailgun and the rest) speaks it, so the
// choice of provider is configuration, not code, and there is no SDK.
package mail

import (
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"
)

// Environment variables. Only EnvOutboxDir is read in normal running, and it
// is for a developer's machine: the mail server itself is set by the instance
// owner on the Admin, Email page. EnvBaseURL falls back to REET_PUBLIC_URL,
// which reet already uses for links elsewhere.
const (
	EnvOutboxDir = "REET_MAIL_OUTBOX_DIR"
	EnvBaseURL   = "REET_PUBLIC_URL"
)

// TLS modes. starttls is the default and upgrades a plain connection before
// anything is said that matters, refusing a server that cannot; implicit is
// TLS from the first byte (port 465); none is plaintext and is accepted only
// for a server on this machine, which is what a local catcher is.
const (
	TLSStartTLS = "starttls"
	TLSImplicit = "implicit"
	TLSNone     = "none"
)

// Secret is a credential that must not reach a log line: printed any way at
// all, it says only whether it is set.
type Secret struct{ v string }

// NewSecret wraps a value.
func NewSecret(v string) Secret { return Secret{v: v} }

// Reveal is the value, for the AUTH command that carries it.
func (s Secret) Reveal() string { return s.v }

// Set says whether there is a value.
func (s Secret) Set() bool { return s.v != "" }

func (s Secret) String() string {
	if s.v == "" {
		return "(unset)"
	}
	return "(redacted)"
}

// GoString keeps %#v from printing the value.
func (s Secret) GoString() string { return s.String() }

// LogValue is what slog prints.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

// MarshalText keeps an encoder from writing it out.
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Config is how reet sends email.
type Config struct {
	Host     string
	Port     int
	User     string
	Password Secret
	TLS      string
	From     mail.Address
	ReplyTo  string
	// BaseURL is what every link in a message starts with: the instance's
	// public address, saved on the Email page or taken from REET_PUBLIC_URL.
	BaseURL string
	// DKIMSelector is the selector the mail server signs with, checked
	// against DNS on the Email page. It is not used to send.
	DKIMSelector string
	// OutboxDir, for local work, is a directory each message is also
	// written to as a .eml file and an .html file to open in a browser. It
	// is not a way to deliver anything.
	OutboxDir string
}

// SMTP says whether a mail server is configured, so messages leave.
func (c Config) SMTP() bool { return c.Host != "" }

// Mode is how messages are handled: "smtp" (sent), "outbox" (written to the
// local directory for development) or "held" (nothing configured: kept in
// the outbox, and admins are told).
func (c Config) Mode() string {
	switch {
	case c.SMTP():
		return "smtp"
	case c.OutboxDir != "":
		return "outbox"
	}
	return "held"
}

// Delivers says whether a message reaches anybody, even only a developer's
// local outbox. Self-service password reset is offered only when it does.
func (c Config) Delivers() bool { return c.Mode() != "held" }

// Summary describes the configuration for a log line or the admin page,
// with no credential in it.
func (c Config) Summary() string {
	switch c.Mode() {
	case "smtp":
		auth := "no sign-in"
		if c.User != "" {
			auth = "signs in as " + c.User
		}
		return fmt.Sprintf("SMTP %s:%d (%s, %s), from %s", c.Host, c.Port, c.TLS, auth, c.From.String())
	case "outbox":
		return "the local outbox directory " + c.OutboxDir + "; nothing leaves this machine"
	}
	return "nothing: messages are kept in the outbox and nobody receives them"
}

// Validate checks a configuration and fills its defaults, wherever it came
// from: the Email page or a test. The messages name the setting the page
// shows, so they read as something to fix.
func Validate(c *Config) error {
	c.Host = strings.TrimSpace(c.Host)
	c.User = strings.TrimSpace(c.User)
	c.ReplyTo = strings.TrimSpace(c.ReplyTo)
	c.DKIMSelector = strings.TrimSpace(c.DKIMSelector)
	c.TLS = strings.ToLower(strings.TrimSpace(c.TLS))
	if c.TLS == "" {
		c.TLS = TLSStartTLS
	}
	switch c.TLS {
	case TLSStartTLS, TLSImplicit, TLSNone:
	default:
		return fmt.Errorf("email: security is %q; use starttls, implicit or none", c.TLS)
	}
	if c.Port == 0 {
		c.Port = 587
		if c.TLS == TLSImplicit {
			c.Port = 465
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("email: port %d is not a port number", c.Port)
	}
	from := strings.TrimSpace(c.From.Address)
	c.From.Address = from
	if c.From.Name == "" {
		c.From.Name = "reet.sh"
	}
	c.From.Name = strings.TrimSpace(c.From.Name)
	if c.SMTP() {
		if from == "" {
			return fmt.Errorf("email: a mail server is set but no from address; say which address messages come from")
		}
		if c.TLS == TLSNone && !local(c.Host) {
			return fmt.Errorf("email: security \"none\" sends the password and every message in the clear; it is allowed only for a server on this machine, not %s", c.Host)
		}
		if c.User != "" && !c.Password.Set() {
			return fmt.Errorf("email: a username is set with no password")
		}
	}
	if from != "" {
		// A bare address only. "Name <addr>" parses too, but it would go
		// into MAIL FROM whole and malform it; the name has its own field.
		a, err := mail.ParseAddress(from)
		if err != nil || a.Name != "" || a.Address != from {
			return fmt.Errorf("email: the from address %q should be a bare address such as hello@example.com, with the name in its own field", from)
		}
	}
	if strings.ContainsAny(c.From.Name, "\r\n") {
		return fmt.Errorf("email: the from name has a line break in it")
	}
	if c.ReplyTo != "" {
		if _, err := mail.ParseAddress(c.ReplyTo); err != nil {
			return fmt.Errorf("email: the reply-to address %q is not an address", c.ReplyTo)
		}
	}
	if c.DKIMSelector != "" && (len(c.DKIMSelector) > 63 || strings.ContainsAny(c.DKIMSelector, " \t\r\n._")) {
		return fmt.Errorf("email: the DKIM selector %q should be one label, such as default or mail", c.DKIMSelector)
	}
	if c.From.Address == "" {
		// Held or written locally, a message still has a sender to show.
		c.From.Address = "no-reply@localhost"
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	u, err := url.Parse(c.BaseURL)
	if c.BaseURL == "" {
		if c.SMTP() {
			return fmt.Errorf("email: every link in a message is absolute; enter the instance's public address")
		}
	} else if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("email: every link in a message is absolute, and %q is not an address to start one with", c.BaseURL)
	}
	return nil
}

func local(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
