package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

// Lifetimes of the links a message carries.
const (
	// ResetTTL is a reset somebody asked for themselves: long enough to find
	// the email, short enough that an old inbox is not a way in.
	ResetTTL = 30 * time.Minute
	// StaffResetTTL is one an administrator started, often while the person
	// is not at their desk.
	StaffResetTTL = 24 * time.Hour
)

// Mailer turns what happened into messages in the outbox. It never sends: the
// Queue does, so a caller never waits on a mail server.
type Mailer struct {
	Store    *store.Store
	Settings *Settings
	Queue    *Queue
	Logger   *slog.Logger
}

func (m *Mailer) log() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// Current is the configuration in force.
func (m *Mailer) Current() Config {
	if m == nil || m.Settings == nil {
		return Config{}
	}
	return m.Settings.Current()
}

// Delivers says whether messages reach anybody; see Config.Delivers.
func (m *Mailer) Delivers() bool { return m != nil && m.Current().Delivers() }

// Base is the instance address every link in a message starts with.
func (m *Mailer) Base() string { return m.Current().BaseURL }

// Send renders a template and puts it in the outbox under key, once: the same
// key again returns the message already there and queues nothing. secret marks
// a message carrying a sign-in link. With nothing configured the message is
// held, and a secret one is held without its body -- nobody will receive it,
// so there is no reason to keep a live link at rest.
func (m *Mailer) Send(ctx context.Context, key, tpl, to string, teamID int64, secret bool, d Data) (store.Mail, bool, error) {
	return m.send(ctx, key, tpl, to, teamID, secret, d)
}

func (m *Mailer) send(ctx context.Context, key, tpl, to string, teamID int64, secret bool, d Data) (store.Mail, bool, error) {
	cfg := m.Current()
	d.Base = cfg.BaseURL
	if d.Instance == "" && cfg.From.Name != "" && cfg.From.Name != "reet.sh" {
		d.Instance = cfg.From.Name
	}
	r, err := Compose(tpl, d)
	if err != nil {
		return store.Mail{}, false, err
	}
	row := store.Mail{Key: key, Template: tpl, To: to, Subject: r.Subject, Text: r.Text, HTML: r.HTML, TeamID: teamID, Secret: secret}
	if !cfg.Delivers() {
		row.State = store.MailHeld
		if secret {
			row.Text, row.HTML = store.RedactedBody, store.RedactedBody
		}
	}
	got, created, err := m.Store.EnqueueMail(row)
	if err != nil {
		return store.Mail{}, false, err
	}
	if created && m.Queue != nil && cfg.Delivers() {
		m.Queue.Kick()
	}
	if created && !cfg.Delivers() {
		m.log().Warn("email: no mail server is configured, so this message is held and nobody receives it",
			"id", got.ID, "template", tpl, "to", to, "configure", "Admin, Email")
	}
	return got, created, err
}

// PasswordReset issues a reset link for an account and queues it. It says
// nothing about whether the account exists: the caller answers the same
// either way, and calls this only when it does.
func (m *Mailer) PasswordReset(ctx context.Context, u store.User, ttl time.Duration) error {
	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	key := auth.HashToken(token)
	expires := time.Now().Add(ttl)
	if err := m.Store.CreatePasswordReset(key, u.ID, expires.UnixMilli()); err != nil {
		return err
	}
	_, _, err = m.Send(ctx, "reset:"+key, TplPasswordReset, u.Email, 0, true, Data{
		Name: u.Name, Email: u.Email, Link: m.Base() + "/reset?token=" + token, Minutes: int(ttl / time.Minute),
	})
	return err
}

// PasswordChanged tells a person their password changed, keyed on when it
// happened so a retried request is one email.
func (m *Mailer) PasswordChanged(ctx context.Context, u store.User, at time.Time) error {
	if at.IsZero() {
		at = time.Now()
	}
	key := fmt.Sprintf("pwchanged:%d:%d", u.ID, at.UnixMilli())
	_, _, err := m.Send(ctx, key, TplPasswordChanged, u.Email, 0, false, Data{
		Name: u.Name, Email: u.Email, Link: m.Base() + "/account/profile", When: at,
	})
	return err
}

// Invite queues an invitation. The key carries the invitation's expiry, so a
// resend is a new message and a retried request is not.
func (m *Mailer) Invite(ctx context.Context, email, role, link string, expires time.Time, inviter string) error {
	key := fmt.Sprintf("invite:%s:%d", strings.ToLower(strings.TrimSpace(email)), expires.UnixMilli())
	_, _, err := m.Send(ctx, key, TplInvite, email, 0, true, Data{
		Email: email, Role: role, Link: link, Expires: expires, Inviter: inviter,
	})
	return err
}

// Welcome says hello after an account is created, whether by sign-up or by
// accepting an invitation.
func (m *Mailer) Welcome(ctx context.Context, u store.User) error {
	_, _, err := m.Send(ctx, fmt.Sprintf("welcome:%d", u.ID), TplWelcome, u.Email, 0, false, Data{
		Name: u.Name, Email: u.Email, Link: m.Base() + "/",
	})
	return err
}

// TeamNotice sends one team event to each recipient, each under a key that
// includes their address. dedupe identifies the occurrence, so a replayed
// request is one email.
func (m *Mailer) TeamNotice(ctx context.Context, groupID int64, event, dedupe string, recipients []store.MailRecipient, d Data) error {
	tpl := ""
	switch event {
	case "member_joined":
		tpl = TplTeamMemberJoined
	case "member_left":
		tpl = TplTeamMemberLeft
	case "member_role":
		tpl = TplTeamMemberRole
	case "list_shared":
		tpl = TplTeamListShared
	default:
		return fmt.Errorf("email: no team notice for %q", event)
	}
	if len(recipients) == 0 {
		m.log().Warn("email: a team notice has nobody to go to", "team", groupID, "event", event)
		return nil
	}
	var errs []error
	for _, r := range recipients {
		d.Name = r.Name
		key := fmt.Sprintf("team:%d:%s:%s:%s", groupID, event, dedupe, strings.ToLower(r.Email))
		if _, _, err := m.Send(ctx, key, tpl, r.Email, groupID, false, d); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// TicketNotice tells one person about a support ticket opening or a reply on
// it. dedupe identifies the message, so a retried request is one email.
// Admins are told they are on support; a team manager is told they manage the
// team; and the requester is told the ticket is theirs.
func (m *Mailer) TicketNotice(ctx context.Context, ticketID, groupID int64, event, dedupe string, r store.MailRecipient, d Data) error {
	tpl := ""
	switch event {
	case "opened":
		tpl = TplTicketOpened
	case "reply":
		tpl = TplTicketReply
	default:
		return fmt.Errorf("email: no ticket notice for %q", event)
	}
	switch {
	case r.Admin:
		d.Why = "You were sent this because you are on support for this reet instance. Support notices cannot be turned off."
	case r.Manager && d.Team != "":
		d.Why = "You were sent this because you manage " + d.Team + ". Support notices cannot be turned off."
	default:
		d.Why = "You were sent this because this support ticket is yours. Support notices cannot be turned off."
	}
	key := fmt.Sprintf("ticket:%d:%s:%s:%s", ticketID, event, dedupe, strings.ToLower(r.Email))
	_, _, err := m.Send(ctx, key, tpl, r.Email, groupID, false, d)
	return err
}
