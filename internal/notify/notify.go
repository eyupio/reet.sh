// Package notify is what reet tells people, and how: the catalogue of every
// kind of notification, grouped by who it is for; each person's choices over
// the owner's defaults; a row in the notification centre for everything; and
// an email now, when they chose one.
//
// Nothing here sends mail directly. An email is a keyed row in the outbox
// (internal/mail), so an event reported twice tells nobody twice.
//
// Security notices are the exception to every choice: they are always on and
// always immediate, because they are how somebody finds out that another
// person is using their account.
package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/store"
)

// Audiences: who a kind of notification is for.
const (
	Staff = "staff"
	User  = "user"
)

// Type is one kind of notification.
type Type struct {
	Key         string `json:"key"`
	Audience    string `json:"audience"`
	Group       string `json:"group"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Template    string `json:"-"`
	// Locked is why the kind cannot be turned off, when it cannot.
	Locked string `json:"locked,omitempty"`
	// Security marks a security notice.
	Security bool `json:"security,omitempty"`
	// OwnerDefault and AdminDefault are the staff roles a staff kind reaches
	// until the owner changes the default.
	OwnerDefault bool   `json:"ownerDefault,omitempty"`
	AdminDefault bool   `json:"adminDefault,omitempty"`
	Tone         string `json:"tone,omitempty"`
}

const securityLocked = "Security notices are always sent: they are how you would find out that somebody else is using your account."

var catalogue = []Type{
	{Key: "staff_ticket_opened", Audience: Staff, Group: "Support", Label: "New support ticket", Description: "A person or team opened a support ticket.", Template: mail.TplTicketOpened, OwnerDefault: true, AdminDefault: true},
	{Key: "staff_ticket_replied", Audience: Staff, Group: "Support", Label: "Requester replied to a ticket", Description: "A requester replied to a support ticket.", Template: mail.TplTicketReply, OwnerDefault: true, AdminDefault: true},
	{Key: "staff_ticket_assigned", Audience: Staff, Group: "Support", Label: "Ticket assigned to you", Description: "Somebody assigned a support ticket to you.", Template: mail.TplTicketAssigned, OwnerDefault: true, AdminDefault: true},
	{Key: "staff_new_account", Audience: Staff, Group: "Accounts", Label: "New account", Description: "A person created an account or accepted an invitation.", Template: mail.TplStaffNewAccount, OwnerDefault: true, AdminDefault: true},
	{Key: "ticket_opened", Audience: User, Group: "Support", Label: "New team ticket", Description: "A member opened a support ticket for a team you manage.", Template: mail.TplTicketOpened},
	{Key: "ticket_replied", Audience: User, Group: "Support", Label: "Support ticket reply", Description: "Somebody replied to a support ticket you opened or manage.", Template: mail.TplTicketReply},
	{Key: "security_password_changed", Audience: User, Group: "Security", Label: "Password changed", Description: "Your password was changed or reset.", Template: mail.TplPasswordChanged, Locked: securityLocked, Security: true, Tone: "warning"},
}

var byKey = func() map[string]Type {
	m := make(map[string]Type, len(catalogue))
	for _, t := range catalogue {
		if _, dup := m[t.Key]; dup {
			panic("notify: two kinds share the key " + t.Key)
		}
		m[t.Key] = t
	}
	return m
}()

// Lookup finds a kind by key.
func Lookup(key string) (Type, bool) {
	t, ok := byKey[key]
	return t, ok
}

// Catalogue is every kind, in the catalogue's order.
func Catalogue() []Type { return append([]Type(nil), catalogue...) }

// For lists the kinds a person with this instance role sees in their
// preferences: user kinds for everybody, staff kinds for the owner and
// administrators.
func For(role string) []Type {
	out := make([]Type, 0, len(catalogue))
	for _, t := range catalogue {
		if t.Audience == User || ((role == "owner" || role == "admin") && t.Audience == Staff) {
			out = append(out, t)
		}
	}
	return out
}

// Recipient is one person an event reaches.
type Recipient struct {
	UserID int64
	Email  string
	Name   string
	Role   string
	Admin  bool
	// Manager marks a team owner or manager.
	Manager bool
	// Explicit marks somebody named by the event rather than found through
	// its audience, so a staff kind reaches them unless they chose
	// otherwise, whatever their role receives by default.
	Explicit bool
}

// Event is something that happened that somebody should hear about.
type Event struct {
	// Type is a catalogue key.
	Type string
	// Key identifies this occurrence. The same key for the same person is
	// the same event, reported twice, and tells them nothing new.
	Key string
	// TeamID is the team the event is about, when there is one.
	TeamID int64
	// Title and Body are the centre's words. Link opens the requester's
	// page; AdminLink, when set, is where an administrator opens the same.
	Title     string
	Body      string
	Link      string
	AdminLink string
	Tone      string
	Data      mail.Data
	// To names recipients outright. Empty is the kind's audience: every
	// active instance administrator, for a staff kind. A user kind needs
	// To.
	To []Recipient
	// Except leaves somebody out, for the admin who did the thing.
	Except int64
}

// Service records notifications and sends their email.
type Service struct {
	Store  *store.Store
	Mail   *mail.Mailer
	Logger *slog.Logger
	Now    func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// Emit records an event for everybody it reaches and sends their email as
// each chose. A failure to reach one person does not stop the others.
func (s *Service) Emit(ctx context.Context, ev Event) error {
	if s == nil {
		return nil
	}
	t, ok := Lookup(ev.Type)
	if !ok {
		return fmt.Errorf("notify: %q is not a kind of notification", ev.Type)
	}
	if ev.Key == "" {
		return fmt.Errorf("notify: an event of kind %s needs a key", ev.Type)
	}
	recipients, err := s.recipients(t, ev)
	if err != nil {
		return err
	}
	defaults, err := s.Store.PlatformNotifyDefaults()
	if err != nil {
		return err
	}
	prefs := map[int64]map[string]store.NotifyPref{}
	var errs []error
	for _, r := range recipients {
		pref, source, err := s.resolve(t, r, defaults, prefs)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !pref.Enabled {
			continue
		}
		if err := s.deliver(ctx, t, ev, r, pref, source); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Service) recipients(t Type, ev Event) ([]Recipient, error) {
	var out []Recipient
	if len(ev.To) > 0 {
		for i := range ev.To {
			ev.To[i].Explicit = true
			out = append(out, ev.To[i])
		}
	} else if t.Audience == Staff {
		users, err := s.Store.ListUsers()
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			if u.DisabledAt != 0 || (u.Role != "owner" && u.Role != "admin") {
				continue
			}
			out = append(out, Recipient{UserID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, Admin: true})
		}
	} else {
		return nil, fmt.Errorf("notify: %s is about one person and the event names nobody", t.Key)
	}
	seen := map[int64]bool{}
	unique := out[:0]
	for _, r := range out {
		if r.UserID == 0 || r.UserID == ev.Except || seen[r.UserID] {
			continue
		}
		seen[r.UserID] = true
		unique = append(unique, r)
	}
	return unique, nil
}

func (s *Service) resolve(t Type, r Recipient, defaults map[string]store.NotifyDefault, cache map[int64]map[string]store.NotifyPref) (store.NotifyPref, string, error) {
	if t.Locked != "" {
		return store.NotifyPref{Enabled: true, Mode: store.NotifyImmediate}, "locked", nil
	}
	mine, ok := cache[r.UserID]
	if !ok {
		var err error
		mine, err = s.Store.NotificationPrefs(r.UserID)
		if err != nil {
			return store.NotifyPref{}, "", err
		}
		cache[r.UserID] = mine
	}
	if p, ok := mine[t.Key]; ok {
		return clean(t, p), "you", nil
	}
	if t.Audience == Staff && r.Explicit {
		return store.NotifyPref{Enabled: true, Mode: store.NotifyImmediate}, "you", nil
	}
	d, has := defaults[t.Key]
	if t.Audience == Staff {
		enabled, mode := t.OwnerDefault, store.NotifyImmediate
		if r.Role == "admin" {
			enabled = t.AdminDefault
		}
		if has {
			enabled = (r.Role == "owner" && d.Owner) || (r.Role == "admin" && d.Admin)
			if !d.Enabled {
				enabled = false
			}
			mode = d.Mode
		}
		return clean(t, store.NotifyPref{Enabled: enabled, Mode: mode}), "role", nil
	}
	if has {
		return clean(t, store.NotifyPref{Enabled: d.Enabled, Mode: d.Mode}), "platform", nil
	}
	return clean(t, store.NotifyPref{Enabled: true, Mode: store.NotifyImmediate}), "default", nil
}

func clean(t Type, p store.NotifyPref) store.NotifyPref {
	if t.Locked != "" {
		return store.NotifyPref{Enabled: true, Mode: store.NotifyImmediate}
	}
	if p.Mode != store.NotifyImmediate && p.Mode != store.NotifyNone {
		p.Mode = store.NotifyImmediate
	}
	return p
}

func (s *Service) deliver(ctx context.Context, t Type, ev Event, r Recipient, pref store.NotifyPref, source string) error {
	link := ev.Link
	if r.Admin && ev.AdminLink != "" {
		link = ev.AdminLink
	}
	row, created, err := s.Store.RecordNotification(store.Notification{
		UserID: r.UserID, Kind: t.Key, TeamID: ev.TeamID, Title: ev.Title, Body: ev.Body, Link: link, Tone: ev.Tone, Key: ev.Key,
	})
	if err != nil {
		return fmt.Errorf("recording %s for %d: %w", t.Key, r.UserID, err)
	}
	if !created || pref.Mode != store.NotifyImmediate || s.Mail == nil || !s.Mail.Delivers() {
		return nil
	}
	d := ev.Data
	d.Name = r.Name
	d.Link = s.Mail.Base() + link
	d.Why = why(t, r, ev.Data.Team)
	if _, _, err := s.Mail.Send(ctx, "notify:"+ev.Key+":"+fmt.Sprint(r.UserID), t.Template, r.Email, ev.TeamID, false, d); err != nil {
		return err
	}
	return s.Store.SetNotificationEmailed(row.ID)
}

func why(t Type, r Recipient, team string) string {
	if t.Security || t.Locked != "" {
		return "You were sent this because it is a security notice about your account. It cannot be turned off."
	}
	switch {
	case r.Admin:
		return "You were sent this because you are on support for this reet instance. You can change your own notification choices under Account."
	case r.Manager && team != "":
		return "You were sent this because you manage " + team + ". You can change your own notification choices under Account."
	}
	return "You were sent this because this is your ticket. You can change your own notification choices under Account."
}

// PreferenceView is one kind as a person's settings page draws it, with the
// choice in force and where it came from.
type PreferenceView struct {
	Type
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Source  string `json:"source"`
}

// Preferences resolves every kind this person may receive.
func (s *Service) Preferences(role string) ([]PreferenceView, error) {
	defaults, err := s.Store.PlatformNotifyDefaults()
	if err != nil {
		return nil, err
	}
	out := []PreferenceView{}
	for _, t := range For(role) {
		r := Recipient{Role: role, Admin: role == "owner" || role == "admin"}
		p, source, err := s.resolve(t, r, defaults, map[int64]map[string]store.NotifyPref{})
		if err != nil {
			return nil, err
		}
		out = append(out, PreferenceView{Type: t, Enabled: p.Enabled, Mode: p.Mode, Source: source})
	}
	return out, nil
}

// Preference is one choice a person sends back.
type Preference struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
}

// SetPreferences records a person's choices, refusing a kind they cannot
// receive and a locked one.
func (s *Service) SetPreferences(userID int64, role string, prefs []Preference) error {
	allowed := map[string]Type{}
	for _, t := range For(role) {
		allowed[t.Key] = t
	}
	for _, p := range prefs {
		t, ok := allowed[p.Key]
		if !ok {
			return fmt.Errorf("%q is not a notification you can choose", p.Key)
		}
		if t.Locked != "" {
			return fmt.Errorf("%s cannot be turned off", t.Label)
		}
		if p.Mode != store.NotifyImmediate && p.Mode != store.NotifyNone {
			return errors.New("a notification is emailed immediately or kept in the centre only")
		}
		if err := s.Store.SetNotificationPref(userID, p.Key, store.NotifyPref{Enabled: p.Enabled, Mode: p.Mode}); err != nil {
			return err
		}
	}
	return nil
}

// DefaultView is one kind as the owner's defaults page draws it.
type DefaultView struct {
	Type
	Default store.NotifyDefault `json:"default"`
}

// Defaults is every kind with the owner's default, or the catalogue's when
// none is saved.
func (s *Service) Defaults() ([]DefaultView, error) {
	saved, err := s.Store.PlatformNotifyDefaults()
	if err != nil {
		return nil, err
	}
	out := []DefaultView{}
	for _, t := range catalogue {
		d, ok := saved[t.Key]
		if !ok {
			d = store.NotifyDefault{Enabled: true, Mode: store.NotifyImmediate, Owner: t.OwnerDefault, Admin: t.AdminDefault}
		}
		out = append(out, DefaultView{Type: t, Default: d})
	}
	return out, nil
}

// Default is one default the owner sends back.
type Default struct {
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Owner   bool   `json:"owner"`
	Admin   bool   `json:"admin"`
}

// SetDefaults stores the owner's platform defaults.
func (s *Service) SetDefaults(by string, defaults []Default) error {
	for _, d := range defaults {
		t, ok := Lookup(d.Kind)
		if !ok {
			return fmt.Errorf("%q is not a kind of notification", d.Kind)
		}
		if t.Locked != "" {
			continue
		}
		if d.Mode != store.NotifyImmediate && d.Mode != store.NotifyNone {
			return errors.New("a notification is emailed immediately or kept in the centre only")
		}
		if err := s.Store.SetPlatformNotifyDefault(d.Kind, store.NotifyDefault{
			Enabled: d.Enabled, Mode: d.Mode, Owner: d.Owner, Admin: d.Admin,
		}, by); err != nil {
			return err
		}
	}
	return nil
}

// Loop prunes old read notifications until ctx ends.
func (s *Service) Loop(ctx context.Context, every time.Duration) {
	if s == nil {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	if _, err := s.Store.PruneNotifications(90 * 24 * time.Hour); err != nil {
		s.log().Error("notifications: pruning", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.Store.PruneNotifications(90 * 24 * time.Hour); err != nil {
				s.log().Error("notifications: pruning", "error", err)
			}
		}
	}
}
