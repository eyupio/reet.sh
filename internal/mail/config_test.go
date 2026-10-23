package mail

import (
	"net/mail"
	"strings"
	"testing"
)

func parseAddress(t *testing.T, value string) mail.Address {
	t.Helper()
	a, err := mail.ParseAddress(value)
	if err != nil {
		t.Fatal(err)
	}
	return *a
}

func TestValidateDefaultsAndRefusals(t *testing.T) {
	c := Config{Host: "smtp.example.com", TLS: "starttls", User: "me", Password: NewSecret("secret"),
		From: parseAddress(t, "hello@example.com"), BaseURL: "https://reet.example"}
	if err := Validate(&c); err != nil {
		t.Fatal(err)
	}
	if c.Port != 587 || c.From.Name != "reet.sh" {
		t.Fatalf("defaults: %+v", c)
	}
	c.Port = 465
	c.TLS = TLSImplicit
	if err := Validate(&c); err != nil {
		t.Fatal(err)
	}
	c = Config{Host: "smtp.example.com", TLS: TLSNone, From: parseAddress(t, "hello@example.com"), BaseURL: "https://reet.example"}
	if err := Validate(&c); err == nil {
		t.Fatal("plaintext to another machine was accepted")
	}
	c.Host = "127.0.0.1"
	if err := Validate(&c); err != nil {
		t.Fatalf("plaintext to a local catcher was refused: %v", err)
	}
	c = Config{Host: "smtp.example.com", From: parseAddress(t, "hello@example.com")}
	if err := Validate(&c); err == nil || !strings.Contains(err.Error(), "public address") {
		t.Fatalf("missing base URL: %v", err)
	}
	c = Config{From: mail.Address{Address: "Name <hello@example.com>"}, BaseURL: "https://reet.example"}
	if err := Validate(&c); err == nil {
		t.Fatal("a named from address was accepted")
	}
}

func TestSecretNeverPrintsItsValue(t *testing.T) {
	s := NewSecret("hunter2")
	if strings.Contains(s.String(), "hunter2") || strings.Contains(s.GoString(), "hunter2") {
		t.Fatal("secret printed")
	}
	if s.Reveal() != "hunter2" || !s.Set() {
		t.Fatal("secret value lost")
	}
}

func TestEncodeRejectsHeaderInjectionAndWritesBothBodies(t *testing.T) {
	m := Message{From: parseAddress(t, "hello@example.com"), To: "you@example.com", Subject: "Subject", Text: "plain", HTML: "<p>rich</p>"}
	raw, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"hello@example.com", "To: you@example.com", "Subject: Subject", "multipart/alternative", "plain", "rich"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	m.Subject = "hello\r\nBcc: someone@example.com"
	if _, err := Encode(m); err == nil || !Permanent(err) {
		t.Fatal("header injection was accepted")
	}
}

func TestComposeAndRender(t *testing.T) {
	out, err := Compose(TplPasswordReset, Data{Name: "Sam", Email: "sam@example.com", Link: "https://reet.example/reset?token=abc", Minutes: 30, Base: "https://reet.example"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text, "https://reet.example/reset?token=abc") || !strings.Contains(out.HTML, "https://reet.example/reset?token=abc") {
		t.Fatal("reset link missing")
	}
	if strings.Contains(out.HTML, "<script") {
		t.Fatal("unexpected script")
	}
}
