package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// SMTP sends through a mail server. It is written on net/textproto rather
// than net/smtp for one reason: "Check connection" and "Send test email" show
// the server's own replies, line by line, and net/smtp keeps them to itself
// once STARTTLS has wrapped the connection. The protocol it speaks is the
// few commands every submission server takes -- EHLO, STARTTLS, AUTH PLAIN
// or LOGIN, MAIL, RCPT, DATA, QUIT -- and each message is its own connection:
// the volume is a handful an hour, and a connection held open is one more
// thing to go stale between them.
type SMTP struct {
	Config Config
	// Timeout bounds one whole conversation, dial to QUIT. Zero is 30
	// seconds.
	Timeout time.Duration
	// TLSConfig overrides the TLS settings, for a test server's
	// certificate. Nil verifies the server's name against the system roots.
	TLSConfig *tls.Config
}

// Name implements Sender.
func (s *SMTP) Name() string { return "smtp" }

// Send implements Sender.
func (s *SMTP) Send(ctx context.Context, m Message) error {
	_, err := s.converse(ctx, &m)
	return err
}

// SendTraced sends and returns what was said, for the test email.
func (s *SMTP) SendTraced(ctx context.Context, m Message) (Transcript, error) {
	return s.converse(ctx, &m)
}

// Check connects, secures the connection and signs in, then says goodbye
// without sending anything: the cheap test before saving settings.
func (s *SMTP) Check(ctx context.Context) (Transcript, error) {
	return s.converse(ctx, nil)
}

// Transcript is a conversation with a server, one line per command and
// reply. The client's AUTH lines are never in it: the password is base64
// on the wire, which is an encoding, not a secret.
type Transcript []string

// String is the transcript as a person reads it.
func (t Transcript) String() string { return strings.Join(t, "\n") }

type conversation struct {
	conn net.Conn
	tp   *textproto.Conn
	log  Transcript
	ext  map[string]string
}

func (c *conversation) note(s string) { c.log = append(c.log, s) }

// cmd sends a line and reads the reply, which must have code expect. The
// line is written to the transcript as shown, so a secret is shown as a
// placeholder by passing show.
func (c *conversation) cmd(expect int, show, line string) (string, error) {
	if show == "" {
		show = line
	}
	c.note("C: " + show)
	id, err := c.tp.Cmd("%s", line)
	if err != nil {
		return "", err
	}
	c.tp.StartResponse(id)
	defer c.tp.EndResponse(id)
	return c.read(expect)
}

func (c *conversation) read(expect int) (string, error) {
	code, msg, err := c.tp.ReadResponse(expect)
	if code != 0 {
		for i, l := range strings.Split(msg, "\n") {
			sep := " "
			if i > 0 {
				sep = "-"
			}
			c.note("S: " + strconv.Itoa(code) + sep + l)
		}
	}
	return msg, err
}

func (c *conversation) hello(name string) error {
	msg, err := c.cmd(250, "", "EHLO "+name)
	if err != nil {
		return fmt.Errorf("EHLO: %w", err)
	}
	c.ext = map[string]string{}
	for i, l := range strings.Split(msg, "\n") {
		if i == 0 {
			continue
		}
		k, v, _ := strings.Cut(l, " ")
		c.ext[strings.ToUpper(k)] = v
	}
	return nil
}

func (s *SMTP) converse(ctx context.Context, m *Message) (Transcript, error) {
	var raw []byte
	if m != nil {
		var err error
		if raw, err = Encode(*m); err != nil {
			return nil, err
		}
	}
	timeout := s.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cfg := s.Config
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	tc := s.TLSConfig
	if tc == nil {
		tc = &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	}
	c := &conversation{}
	c.note("connecting to " + addr + " (" + cfg.TLS + ")")

	d := &net.Dialer{}
	var err error
	if cfg.TLS == TLSImplicit {
		c.conn, err = (&tls.Dialer{NetDialer: d, Config: tc}).DialContext(ctx, "tcp", addr)
	} else {
		c.conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		c.note("could not connect: " + err.Error())
		return c.log, fmt.Errorf("smtp: connecting to %s: %w", addr, err)
	}
	// Closing the connection is what stops a server that has gone quiet
	// mid-conversation; the deadline covers the rest.
	stop := context.AfterFunc(ctx, func() { c.conn.Close() })
	defer stop()
	defer func() { c.conn.Close() }()
	deadline, _ := ctx.Deadline()
	_ = c.conn.SetDeadline(deadline)
	c.tp = textproto.NewConn(c.conn)

	fail := func(what string, err error) (Transcript, error) {
		return c.log, fmt.Errorf("smtp: %s: %w", what, err)
	}
	if _, err := c.read(220); err != nil {
		return fail("the server's greeting", err)
	}
	name := helloName(cfg)
	if err := c.hello(name); err != nil {
		return fail("saying hello", err)
	}
	secure := cfg.TLS == TLSImplicit
	if cfg.TLS == TLSStartTLS {
		if _, ok := c.ext["STARTTLS"]; !ok {
			// Carrying on in the clear would put the password and every
			// reset link on the wire. A server that cannot do TLS is a
			// configuration to fix, and retrying will not fix it.
			c.note("the server does not offer STARTTLS, so nothing more was sent")
			return c.log, fmt.Errorf("smtp: %s does not offer STARTTLS; use implicit TLS on port 465, or a server that does", addr)
		}
		if _, err := c.cmd(220, "", "STARTTLS"); err != nil {
			return fail("STARTTLS", err)
		}
		tlsConn := tls.Client(c.conn, tc)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			c.note("TLS handshake failed: " + err.Error())
			return fail("the TLS handshake", err)
		}
		st := tlsConn.ConnectionState()
		c.note("TLS " + tls.VersionName(st.Version) + ", " + tls.CipherSuiteName(st.CipherSuite))
		c.conn, c.tp, secure = tlsConn, textproto.NewConn(tlsConn), true
		if err := c.hello(name); err != nil {
			return fail("saying hello again after STARTTLS", err)
		}
	}
	if cfg.User != "" {
		if !secure && !local(cfg.Host) {
			return c.log, errors.New("smtp: refusing to send the password over a connection that is not encrypted")
		}
		if err := c.auth(cfg); err != nil {
			return fail("signing in as "+cfg.User, err)
		}
	}
	if m == nil {
		_, _ = c.cmd(221, "", "QUIT")
		return c.log, nil
	}
	if _, err := c.cmd(250, "", "MAIL FROM:<"+cfg.From.Address+">"); err != nil {
		return fail("MAIL FROM", err)
	}
	if _, err := c.cmd(250, "", "RCPT TO:<"+m.To+">"); err != nil {
		err = classify(err)
		return fail("RCPT TO "+m.To, err)
	}
	if _, err := c.cmd(354, "", "DATA"); err != nil {
		return fail("DATA", err)
	}
	w := c.tp.DotWriter()
	if _, err := w.Write(raw); err != nil {
		return fail("writing the message", err)
	}
	if err := w.Close(); err != nil {
		return fail("finishing the message", err)
	}
	c.note(fmt.Sprintf("C: (the message, %d bytes)", len(raw)))
	if _, err := c.read(250); err != nil {
		err = classify(err)
		return fail("the server did not accept the message", err)
	}
	_, _ = c.cmd(221, "", "QUIT")
	return c.log, nil
}

// auth signs in with PLAIN where the server offers it and LOGIN where it
// offers only that, which some hosted servers still do.
func (c *conversation) auth(cfg Config) error {
	mechs := strings.Fields(strings.ToUpper(c.ext["AUTH"]))
	has := func(want string) bool {
		for _, m := range mechs {
			if m == want {
				return true
			}
		}
		return false
	}
	user, pw := cfg.User, cfg.Password.Reveal()
	switch {
	case has("PLAIN"):
		cred := base64.StdEncoding.EncodeToString([]byte("\x00" + user + "\x00" + pw))
		_, err := c.cmd(235, "AUTH PLAIN (credentials hidden)", "AUTH PLAIN "+cred)
		return err
	case has("LOGIN"):
		if _, err := c.cmd(334, "", "AUTH LOGIN"); err != nil {
			return err
		}
		if _, err := c.cmd(334, "(username)", base64.StdEncoding.EncodeToString([]byte(user))); err != nil {
			return err
		}
		_, err := c.cmd(235, "(password hidden)", base64.StdEncoding.EncodeToString([]byte(pw)))
		return err
	}
	return fmt.Errorf("the server offers no sign-in method reet speaks (it offers %q; reet speaks PLAIN and LOGIN)", c.ext["AUTH"])
}

// classify marks a 5xx reply as permanent. It is applied only to the replies
// that judge the message (RCPT TO and the end of DATA): a 5xx to the
// greeting, AUTH or MAIL FROM is about reet's settings, and bouncing on it
// would blank every queued link. A 4xx is the server asking to be tried later,
// which is what the queue does.
func classify(err error) error {
	var te *textproto.Error
	if errors.As(err, &te) && te.Code >= 500 && te.Code < 600 {
		return &PermanentError{Err: err}
	}
	return err
}

// helloName is the name reet gives in EHLO: its own host, from the address its
// links start with. Some servers score a bare "localhost".
func helloName(c Config) string {
	h := strings.TrimPrefix(strings.TrimPrefix(c.BaseURL, "https://"), "http://")
	h, _, _ = strings.Cut(h, "/")
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	if h == "" {
		return "localhost"
	}
	return h
}
