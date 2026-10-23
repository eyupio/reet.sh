package mail

import (
	"context"
	"errors"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

type badSealer struct{}

func (badSealer) SealSecret(string) (string, error) { return "", errors.New("no key") }
func (badSealer) OpenSecret(string) (string, error) { return "", errors.New("no key") }

type plainSealer struct{}

func (plainSealer) SealSecret(v string) (string, error) { return "sealed:" + v, nil }
func (plainSealer) OpenSecret(v string) (string, error) {
	if !strings.HasPrefix(v, "sealed:") {
		return "", errors.New("bad seal")
	}
	return strings.TrimPrefix(v, "sealed:"), nil
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSettingsSavesPasswordSealedAndBindsItToTheHost(t *testing.T) {
	st := openStore(t)
	s := &Settings{Store: st, Sealer: plainSealer{}, Env: Config{BaseURL: "https://reet.example"}}
	c, err := s.Save(Form{
		Host: "smtp.example.com", Port: 587, TLS: TLSStartTLS, User: "u", Password: "pw", PasswordAction: PasswordReplace,
		From: "hello@example.com", FromName: "reet.sh", BaseURL: "https://reet.example",
	}, "owner@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !c.SMTP() || c.Password.Reveal() != "pw" {
		t.Fatalf("saved config: %+v", c)
	}
	saved, ok, err := st.GetMailSettings()
	if err != nil || !ok || !strings.HasPrefix(saved.PasswordSealed, "sealed:") || strings.Contains(saved.PasswordSealed, "\n") {
		t.Fatalf("stored settings: %+v %v %v", saved, ok, err)
	}
	snap := s.Snapshot()
	if snap.Problem != "" {
		t.Fatal("unexpected problem")
	}
	if snap.Config.Password.Reveal() != "pw" || snap.Source != "database" {
		t.Fatalf("snapshot: %+v", snap)
	}
	if _, err := s.Save(Form{
		Host: "other.example.com", TLS: TLSStartTLS, User: "u", PasswordAction: PasswordKeep,
		From: "hello@example.com", BaseURL: "https://reet.example",
	}, "owner@example.com"); err == nil || !strings.Contains(err.Error(), "saved password is for") {
		t.Fatalf("password followed a host change: %v", err)
	}
}

func TestQueueSendsRetriesAndBounces(t *testing.T) {
	st := openStore(t)
	fake := &Fake{}
	q := &Queue{Store: st, Sender: fake, Config: Config{
		From: mail.Address{Name: "reet.sh", Address: "hello@example.com"}, BaseURL: "https://reet.example",
	}}
	if _, _, err := st.EnqueueMail(store.Mail{Key: "one", Template: TplWelcome, To: "sam@example.com", Subject: "Hello", Text: "hello", HTML: "<p>hello</p>"}); err != nil {
		t.Fatal(err)
	}
	p, err := q.RunOnce(context.Background())
	if err != nil || p.Sent != 1 {
		t.Fatalf("send: %+v %v", p, err)
	}
	if got := fake.Messages(); len(got) != 1 || got[0].To != "sam@example.com" {
		t.Fatalf("messages: %+v", got)
	}
	ms, _ := st.MailListing("", 10)
	if len(ms) != 1 || ms[0].State != store.MailDelivered {
		t.Fatalf("outbox: %+v", ms)
	}

	// A transient failure is queued again with a later attempt.
	if _, _, err := st.EnqueueMail(store.Mail{Key: "two", Template: TplWelcome, To: "sam@example.com", Subject: "Hello", Text: "hello", HTML: "<p>hello</p>"}); err != nil {
		t.Fatal(err)
	}
	fake.SetFail(errors.New("server said 451"))
	p, err = q.RunOnce(context.Background())
	if err != nil || p.Retried != 1 {
		t.Fatalf("retry: %+v %v", p, err)
	}
	ms, _ = st.MailListing("", 10)
	if ms[0].State != store.MailQueued || ms[0].Attempts != 1 || ms[0].NextAttemptAt == 0 {
		t.Fatalf("retry row: %+v", ms[0])
	}

	fake.SetFail(&PermanentError{Err: errors.New("550 no such user")})
	q.Now = func() time.Time { return time.Now().Add(72 * time.Hour) }
	p, err = q.RunOnce(context.Background())
	if err != nil || p.Bounced != 1 {
		t.Fatalf("bounce: %+v %v", p, err)
	}
}

func TestHeldSecretIsBlankedAndAFinishedSecretCannotBeResent(t *testing.T) {
	st := openStore(t)
	m, _, err := st.EnqueueMail(store.Mail{Key: "reset", Template: TplPasswordReset, To: "sam@example.com", Subject: "Reset", Secret: true,
		Text: "https://reet.example/reset?token=live", HTML: "<a href=\"https://reet.example/reset?token=live\">reset</a>", State: store.MailHeld})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetMail(m.ID)
	if err != nil || got.Text != "https://reet.example/reset?token=live" {
		t.Fatalf("held row: %+v %v", got, err)
	}
	if err := st.MailSent(m.ID, "smtp"); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetMail(m.ID)
	if got.Text != store.RedactedBody || !got.Spent() {
		t.Fatalf("delivered secret kept its body: %+v", got)
	}
	if _, err := st.RequeueMail(m.ID); !errors.Is(err, store.ErrMailSpent) {
		t.Fatalf("spent link was requeued: %v", err)
	}
}

func TestDirWritesEmailsForLocalWork(t *testing.T) {
	dir := t.TempDir()
	sender := Dir{Path: dir}
	err := sender.Send(context.Background(), Message{ID: "one", From: parseAddress(t, "hello@example.com"), To: "sam@example.com", Subject: "Hello", Text: "plain", HTML: "<p>rich</p>"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.eml", "one.html"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestSnapshotHoldsWhenThePasswordCannotBeOpened(t *testing.T) {
	st := openStore(t)
	good := &Settings{Store: st, Sealer: plainSealer{}, Env: Config{BaseURL: "https://reet.example"}}
	if _, err := good.Save(Form{Host: "smtp.example.com", TLS: TLSStartTLS, User: "u", Password: "pw", PasswordAction: PasswordReplace,
		From: "hello@example.com", BaseURL: "https://reet.example"}, "owner@example.com"); err != nil {
		t.Fatal(err)
	}
	bad := &Settings{Store: st, Sealer: badSealer{}, Env: Config{BaseURL: "https://reet.example"}}
	snap := bad.Snapshot()
	if snap.Problem == "" || snap.Config.SMTP() || snap.Config.Delivers() {
		t.Fatalf("unreadable password did not hold mail: %+v", snap)
	}
}
