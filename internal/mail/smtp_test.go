package mail

import (
	"context"
	"net/mail"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/mail/smtptest"
)

func TestSMTPConnectsSignsInAndSendsOverStartTLS(t *testing.T) {
	server := smtptest.Start(t, func(s *smtptest.Server) { s.User, s.Password = "me", "secret" })
	c := Config{Host: "127.0.0.1", Port: server.Port(), TLS: TLSStartTLS, User: "me", Password: NewSecret("secret"),
		From: mail.Address{Name: "reet.sh", Address: "hello@example.com"}, BaseURL: "https://reet.example"}
	tr, err := (&SMTP{Config: c, TLSConfig: server.ClientTLS}).Check(context.Background())
	if err != nil {
		t.Fatalf("check: %v\n%s", err, tr)
	}
	if err := (&SMTP{Config: c, TLSConfig: server.ClientTLS}).Send(context.Background(), Message{
		From: c.From, To: "sam@example.com", Subject: "Hello", Text: "plain", HTML: "<p>rich</p>",
	}); err != nil {
		t.Fatal(err)
	}
	msgs := server.Messages()
	if len(msgs) != 1 || msgs[0].To[0] != "sam@example.com" || !strings.Contains(msgs[0].Data, "Subject: Hello") {
		t.Fatalf("messages: %+v", msgs)
	}
	for _, line := range server.Wire() {
		if strings.Contains(line, "secret") {
			t.Fatal("password was sent before TLS")
		}
	}
}

func TestSMTPRefusesA5xxForGood(t *testing.T) {
	server := smtptest.Start(t, func(s *smtptest.Server) { s.FailRcpt("550 5.1.1 no such user", 1) })
	c := Config{Host: "127.0.0.1", Port: server.Port(), TLS: TLSStartTLS,
		From: mail.Address{Name: "reet.sh", Address: "hello@example.com"}, BaseURL: "https://reet.example"}
	err := (&SMTP{Config: c, TLSConfig: server.ClientTLS}).Send(context.Background(), Message{
		From: c.From, To: "sam@example.com", Subject: "Hello", Text: "plain", HTML: "<p>rich</p>",
	})
	if err == nil || !Permanent(err) {
		t.Fatalf("5xx was not permanent: %v", err)
	}
}
