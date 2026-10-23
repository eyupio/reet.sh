package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

// Message is one email to one person, ready to send.
type Message struct {
	// ID is the outbox row's, and becomes the Message-ID, so a message
	// sent twice after a crash is recognisably one message.
	ID      string
	From    mail.Address
	To      string
	ReplyTo string
	Subject string
	Text    string
	HTML    string
	Date    time.Time
	// Headers are any others.
	Headers map[string]string
}

// Sender delivers a message. An error that is Permanent will not succeed on
// a retry (the server refused the address); anything else is tried again.
type Sender interface {
	Send(ctx context.Context, m Message) error
	// Name is what the outbox records as how a message went.
	Name() string
}

// PermanentError is a refusal a retry will not change, such as an SMTP 5xx
// to RCPT TO. The outbox records the message as bounced.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent reports whether err is one.
func Permanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// Encode renders a message as RFC 5322 bytes: multipart/alternative with the
// plain text first and the HTML second, as mail clients expect, each
// quoted-printable so no line is too long for a server to accept.
func Encode(m Message) ([]byte, error) {
	for _, v := range []string{m.To, m.ReplyTo, m.Subject, m.From.Name, m.From.Address} {
		if strings.ContainsAny(v, "\r\n") {
			// Header injection. Nothing reet builds has a line break in a
			// header; one that does came from somebody else.
			return nil, &PermanentError{Err: fmt.Errorf("email: a header value carries a line break")}
		}
	}
	if _, err := mail.ParseAddress(m.To); err != nil {
		return nil, &PermanentError{Err: fmt.Errorf("email: %q is not an address: %v", m.To, err)}
	}
	var b bytes.Buffer
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ typ, content string }{{"text/plain; charset=utf-8", m.Text}, {"text/html; charset=utf-8", m.HTML}} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", part.typ)
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		w, err := mw.CreatePart(h)
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(strings.ReplaceAll(part.content, "\n", "\r\n"))); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	date := m.Date
	if date.IsZero() {
		date = time.Now()
	}
	domain := "localhost"
	if at := strings.LastIndex(m.From.Address, "@"); at >= 0 {
		domain = m.From.Address[at+1:]
	}
	id := m.ID
	if id == "" {
		var r [12]byte
		_, _ = rand.Read(r[:])
		id = hex.EncodeToString(r[:])
	}
	head := [][2]string{
		{"From", m.From.String()},
		{"To", m.To},
		{"Subject", mime.QEncoding.Encode("utf-8", m.Subject)},
		{"Date", date.UTC().Format(time.RFC1123Z)},
		{"Message-ID", "<" + id + "@" + domain + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", "multipart/alternative; boundary=" + mw.Boundary()},
		// RFC 3834: an automatic message, so an out-of-office reply is
		// not sent back to it.
		{"Auto-Submitted", "auto-generated"},
	}
	if m.ReplyTo != "" {
		head = append(head, [2]string{"Reply-To", m.ReplyTo})
	}
	extra := make([]string, 0, len(m.Headers))
	for k := range m.Headers {
		extra = append(extra, k)
	}
	sort.Strings(extra)
	for _, k := range extra {
		v := m.Headers[k]
		if strings.ContainsAny(k+v, "\r\n") {
			return nil, &PermanentError{Err: fmt.Errorf("email: a header value carries a line break")}
		}
		head = append(head, [2]string{k, v})
	}
	for _, h := range head {
		fmt.Fprintf(&b, "%s: %s\r\n", h[0], h[1])
	}
	b.WriteString("\r\n")
	b.Write(body.Bytes())
	return b.Bytes(), nil
}
