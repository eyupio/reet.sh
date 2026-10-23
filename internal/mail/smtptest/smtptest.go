// Package smtptest is a fake mail server for tests, the way httptest is a
// fake web server: it listens on a real socket on 127.0.0.1, speaks enough
// SMTP for reet's client (EHLO, STARTTLS, AUTH PLAIN and LOGIN, MAIL,
// RCPT, DATA, QUIT), keeps what it was sent, and can be told to refuse.
// Only tests import it.
package smtptest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// Message is one message the server accepted.
type Message struct {
	From string
	To   []string
	Data string
}

// Server is the fake.
type Server struct {
	Addr string
	// User and Password, when User is set, are what AUTH must present.
	User, Password string
	// StartTLS offers STARTTLS; Implicit speaks TLS from the first byte.
	StartTLS, Implicit bool
	// Mechs is what AUTH offers; empty is "PLAIN LOGIN".
	Mechs string

	ln  net.Listener
	tls *tls.Config
	// ClientTLS trusts this server's certificate, for the client under test.
	ClientTLS *tls.Config

	mu       sync.Mutex
	messages []Message
	// failRcpt answers RCPT TO with this reply the next n times.
	failRcpt     string
	failRcptLeft int
	// wire is every line the server read, for a test that the password
	// was never sent in the clear.
	wire []string
	wg   sync.WaitGroup
}

// Start runs a server; opts configure it before it listens.
func Start(t *testing.T, opts func(*Server)) *Server {
	t.Helper()
	s := &Server{StartTLS: true}
	if opts != nil {
		opts(s)
	}
	cert, pool := selfSigned(t)
	s.tls = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	s.ClientTLS = &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
	var err error
	if s.Implicit {
		s.ln, err = tls.Listen("tcp", "127.0.0.1:0", s.tls)
	} else {
		s.ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	s.Addr = s.ln.Addr().String()
	s.wg.Add(1)
	go s.serve()
	t.Cleanup(func() { s.ln.Close(); s.wg.Wait() })
	return s
}

// Port is the port it listens on.
func (s *Server) Port() int { return s.ln.Addr().(*net.TCPAddr).Port }

// FailRcpt answers the next n RCPT TO commands with reply, such as
// "451 4.3.0 try later" or "550 5.1.1 no such user".
func (s *Server) FailRcpt(reply string, n int) {
	s.mu.Lock()
	s.failRcpt, s.failRcptLeft = reply, n
	s.mu.Unlock()
}

// Messages is what it accepted.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

// Wire is every line it read before TLS, in the clear.
func (s *Server) Wire() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.wire...)
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.session(c)
		}()
	}
}

func (s *Server) session(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	secure := s.Implicit
	tp := textproto.NewConn(c)
	reply := func(l string) { _ = tp.PrintfLine("%s", l) }
	reply("220 fake.test ESMTP ready")
	var cur Message
	authed := s.User == ""
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		if !secure {
			s.mu.Lock()
			s.wire = append(s.wire, line)
			s.mu.Unlock()
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			ext := []string{"250-fake.test hello"}
			if s.StartTLS && !secure {
				ext = append(ext, "250-STARTTLS")
			}
			mechs := s.Mechs
			if mechs == "" {
				mechs = "PLAIN LOGIN"
			}
			if secure || !s.StartTLS {
				ext = append(ext, "250-AUTH "+mechs)
			}
			ext = append(ext, "250 8BITMIME")
			for _, l := range ext {
				reply(l)
			}
		case "STARTTLS":
			reply("220 2.0.0 go ahead")
			tc := tls.Server(c, s.tls)
			if err := tc.Handshake(); err != nil {
				return
			}
			c, secure = tc, true
			tp = textproto.NewConn(c)
			reply = func(l string) { _ = tp.PrintfLine("%s", l) }
		case "AUTH":
			mech, rest, _ := strings.Cut(arg, " ")
			var user, pw string
			switch strings.ToUpper(mech) {
			case "PLAIN":
				raw, _ := base64.StdEncoding.DecodeString(rest)
				parts := strings.Split(string(raw), "\x00")
				if len(parts) == 3 {
					user, pw = parts[1], parts[2]
				}
			case "LOGIN":
				reply("334 VXNlcm5hbWU6")
				l, _ := tp.ReadLine()
				u, _ := base64.StdEncoding.DecodeString(l)
				reply("334 UGFzc3dvcmQ6")
				l, _ = tp.ReadLine()
				p, _ := base64.StdEncoding.DecodeString(l)
				user, pw = string(u), string(p)
			}
			if user == s.User && pw == s.Password {
				authed = true
				reply("235 2.7.0 Authentication successful")
			} else {
				reply("535 5.7.8 Authentication credentials invalid")
			}
		case "MAIL":
			if !authed {
				reply("530 5.7.0 Authentication required")
				continue
			}
			cur = Message{From: strings.Trim(strings.TrimPrefix(arg, "FROM:"), "<>")}
			reply("250 2.1.0 Ok")
		case "RCPT":
			s.mu.Lock()
			fail := ""
			if s.failRcptLeft > 0 {
				fail = s.failRcpt
				s.failRcptLeft--
			}
			s.mu.Unlock()
			if fail != "" {
				reply(fail)
				continue
			}
			cur.To = append(cur.To, strings.Trim(strings.TrimPrefix(arg, "TO:"), "<>"))
			reply("250 2.1.5 Ok")
		case "DATA":
			reply("354 End data with <CR><LF>.<CR><LF>")
			b, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			cur.Data = string(b)
			s.mu.Lock()
			s.messages = append(s.messages, cur)
			s.mu.Unlock()
			reply("250 2.0.0 Ok: queued as FAKE123")
		case "QUIT":
			reply("221 2.0.0 Bye")
			return
		case "RSET", "NOOP":
			reply("250 2.0.0 Ok")
		default:
			reply("502 5.5.2 Command not recognised")
		}
	}
}

func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}
