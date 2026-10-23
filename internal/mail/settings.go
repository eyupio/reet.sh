package mail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// ErrNoSealer is returned when a password cannot be stored because reet has no
// encryption key to seal it with.
var ErrNoSealer = errors.New("no encryption key is available to store the mail password")

// Sealer seals a credential at rest. The backup key reet already generates is
// one; the Email page never stores a password in the clear.
type Sealer interface {
	SealSecret(string) (string, error)
	OpenSecret(string) (string, error)
}

// Settings is where the mail configuration comes from: what the owner saved
// on Admin, Email, which is the only place a mail server is named. Env holds
// what the database cannot: the local outbox directory and a public address
// from REET_PUBLIC_URL.
type Settings struct {
	Store  *store.Store
	Sealer Sealer
	Env    Config
}

// Snapshot is the configuration in force, where it came from, and what is
// wrong with it, if anything. A problem means messages are held: the queue
// must not try a half-read server.
type Snapshot struct {
	Config  Config
	Source  string
	Problem string
}

// Snapshot reads the saved server. It is deliberately uncached: a save applies
// to the next message, with no restart.
func (s *Settings) Snapshot() Snapshot {
	c := s.Env
	source := "none"
	var problem string
	if s.Store == nil {
		return Snapshot{Config: c, Source: source}
	}
	saved, ok, err := s.Store.GetMailSettings()
	switch {
	case err != nil:
		problem = "The saved email settings could not be read: " + err.Error() + ". Messages are held until they can be."
	case ok:
		source = "database"
		c.Host, c.Port, c.TLS, c.User = saved.Host, saved.Port, saved.TLS, saved.User
		c.From = mail.Address{Name: saved.FromName, Address: saved.From}
		c.ReplyTo, c.BaseURL, c.DKIMSelector = saved.ReplyTo, saved.BaseURL, saved.DKIMSelector
		if saved.PasswordSealed != "" {
			switch {
			case s.Sealer == nil:
				problem = "The saved mail password cannot be opened because there is no encryption key. Messages are held until it is entered again."
			default:
				pw, err := s.Sealer.OpenSecret(saved.PasswordSealed)
				if err != nil {
					problem = "The saved mail password cannot be opened with this key. Enter it again on Admin, Email; messages are held until then."
				} else {
					c.Password = NewSecret(pw)
				}
			}
		}
	}
	if c.BaseURL == "" {
		c.BaseURL = s.Env.BaseURL
	}
	if problem == "" {
		if err := Validate(&c); err != nil {
			problem = "The saved email settings are not usable: " + strings.TrimPrefix(err.Error(), "email: ") + ". Messages are held until they are fixed."
		}
	}
	if problem != "" {
		c = Config{BaseURL: s.Env.BaseURL, OutboxDir: s.Env.OutboxDir}
	}
	return Snapshot{Config: c, Source: source, Problem: problem}
}

// Current is the configuration in force.
func (s *Settings) Current() Config { return s.Snapshot().Config }

// Form is what the Email page sends. Password is read only with
// PasswordAction replace.
type Form struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	TLS            string `json:"tls"`
	User           string `json:"user"`
	Password       string `json:"password"`
	PasswordAction string `json:"passwordAction"`
	From           string `json:"from"`
	FromName       string `json:"fromName"`
	ReplyTo        string `json:"replyTo"`
	BaseURL        string `json:"baseUrl"`
	DKIMSelector   string `json:"dkimSelector"`
}

const (
	PasswordKeep    = "keep"
	PasswordReplace = "replace"
	PasswordClear   = "clear"
)

func (f Form) action() (string, error) {
	switch f.PasswordAction {
	case "", PasswordKeep:
		return PasswordKeep, nil
	case PasswordReplace:
		if f.Password == "" {
			return "", errors.New("type the new password, or keep the one that is set")
		}
		return PasswordReplace, nil
	case PasswordClear:
		return PasswordClear, nil
	}
	return "", errors.New("the password action is keep, replace or clear")
}

func (s *Settings) openSaved(pc string, f Form) (string, string, error) {
	saved, ok, err := s.Store.GetMailSettings()
	if err != nil {
		return "", "", err
	}
	switch pc {
	case PasswordReplace:
		return f.Password, "", nil
	case PasswordClear:
		return "", "", nil
	}
	if !ok {
		return "", "", nil
	}
	if saved.PasswordSet && !strings.EqualFold(strings.TrimSpace(f.Host), saved.Host) {
		return "", "", fmt.Errorf("the saved password is for %s; enter it again to use it with %s, or clear it", saved.Host, strings.TrimSpace(f.Host))
	}
	if !saved.PasswordSet {
		return "", saved.PasswordSealed, nil
	}
	if s.Sealer == nil {
		return "", "", ErrNoSealer
	}
	pw, err := s.Sealer.OpenSecret(saved.PasswordSealed)
	if err != nil {
		return "", "", fmt.Errorf("the saved password cannot be opened; enter it again, or clear it")
	}
	return pw, saved.PasswordSealed, nil
}

// Resolve turns a form into the configuration it would make, with the stored
// password where the form keeps it, and validates it. It is what a save stores
// and what a connection check tries.
func (s *Settings) Resolve(f Form) (Config, string, error) {
	pc, err := f.action()
	if err != nil {
		return Config{}, "", err
	}
	pw, _, err := s.openSaved(pc, f)
	if err != nil {
		return Config{}, pc, err
	}
	c := Config{Host: f.Host, Port: f.Port, TLS: f.TLS, User: f.User, Password: NewSecret(pw),
		From: mail.Address{Name: f.FromName, Address: f.From}, ReplyTo: f.ReplyTo,
		BaseURL: f.BaseURL, DKIMSelector: f.DKIMSelector, OutboxDir: s.Env.OutboxDir}
	if c.BaseURL == "" {
		c.BaseURL = s.Env.BaseURL
	}
	if err := Validate(&c); err != nil {
		return c, pc, err
	}
	return c, pc, nil
}

// Save stores a form. The next message uses it.
func (s *Settings) Save(f Form, by string) (Config, error) {
	c, pc, err := s.Resolve(f)
	if err != nil {
		return c, err
	}
	sealed := ""
	switch pc {
	case PasswordReplace:
		if s.Sealer == nil {
			return c, ErrNoSealer
		}
		if sealed, err = s.Sealer.SealSecret(f.Password); err != nil {
			return c, err
		}
	case PasswordKeep:
		_, kept, err := s.openSaved(pc, f)
		if err != nil {
			return c, err
		}
		sealed = kept
	}
	from := c.From.Address
	if !c.SMTP() {
		from = strings.TrimSpace(f.From)
	}
	if err := s.Store.SaveMailSettings(store.MailSettings{
		Host: c.Host, Port: c.Port, TLS: c.TLS, User: c.User, PasswordSealed: sealed,
		From: from, FromName: c.From.Name, ReplyTo: c.ReplyTo,
		BaseURL: strings.TrimRight(strings.TrimSpace(f.BaseURL), "/"), DKIMSelector: c.DKIMSelector,
	}, by); err != nil {
		return c, err
	}
	return s.Current(), nil
}

// ---------------------------------------------------------------------------
// DNS: SPF, DKIM, DMARC and BIMI for the from domain

// DNSCheck is what the from domain publishes. A missing record is a warning,
// not a refusal: mail still leaves, it is only likelier to be filed as spam or
// refused by a strict receiver.
type DNSCheck struct {
	Domain string    `json:"domain"`
	SPF    DNSRecord `json:"spf"`
	DKIM   DNSRecord `json:"dkim"`
	DMARC  DNSRecord `json:"dmarc"`
	BIMI   DNSRecord `json:"bimi"`
}

// DNSRecord is one of them: found, missing or unknown (the lookup failed),
// with the record and what to do.
type DNSRecord struct {
	State  string `json:"state"`
	Record string `json:"record,omitempty"`
	Advice string `json:"advice"`
}

// Resolver looks up TXT records; net.DefaultResolver is one.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// CheckDNS looks the records up with a short timeout. DKIM needs the selector
// the server signs with, which is the mail provider's to say; with none it is
// left unknown and says where to find it.
func CheckDNS(ctx context.Context, r Resolver, from, selector string) DNSCheck {
	if r == nil {
		r = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	domain := from
	if at := strings.LastIndex(from, "@"); at >= 0 {
		domain = from[at+1:]
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	out := DNSCheck{Domain: domain}
	find := func(name, prefix string) (string, string) {
		txts, err := r.LookupTXT(ctx, name)
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return "missing", ""
		}
		if err != nil {
			return "unknown", ""
		}
		for _, t := range txts {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), strings.ToLower(prefix)) {
				return "found", t
			}
		}
		return "missing", ""
	}
	st, rec := find(domain, "v=spf1")
	out.SPF = DNSRecord{State: st, Record: rec, Advice: map[string]string{
		"found":   "SPF is published. Check it names your mail server, for example with include: or its address.",
		"missing": "No SPF record. Publish a TXT record on " + domain + " naming the servers allowed to send for it, such as \"v=spf1 mx -all\".",
		"unknown": "SPF could not be looked up from this instance just now.",
	}[st]}
	st, rec = find("_dmarc."+domain, "v=DMARC1")
	out.DMARC = DNSRecord{State: st, Record: rec, Advice: map[string]string{
		"found":   "DMARC is published.",
		"missing": "No DMARC record. Publish a TXT record on _dmarc." + domain + ", starting with \"v=DMARC1; p=none; rua=mailto:...\" and tightening once reports look clean.",
		"unknown": "DMARC could not be looked up from this instance just now.",
	}[st]}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		out.DKIM = DNSRecord{State: "unknown", Advice: "DKIM is checked by selector, the name your mail server signs with (its settings or its DKIM record say, for example \"mail\" or \"default\"). Enter it to check."}
	} else {
		st, rec = find(selector+"._domainkey."+domain, "v=DKIM1")
		if st == "missing" {
			// Some providers publish the key without the optional version tag.
			if txts, err := r.LookupTXT(ctx, selector+"._domainkey."+domain); err == nil {
				for _, t := range txts {
					if strings.Contains(t, "p=") {
						st, rec = "found", t
					}
				}
			}
		}
		out.DKIM = DNSRecord{State: st, Record: rec, Advice: map[string]string{
			"found":   "A DKIM key is published for the selector " + selector + ". The server must also sign with it; the test email's headers show whether it does.",
			"missing": "No DKIM key at " + selector + "._domainkey." + domain + ". Publish the public key your mail server gives you there.",
			"unknown": "DKIM could not be looked up from this instance just now.",
		}[st]}
	}
	st, rec = find("default._bimi."+domain, "v=BIMI1;")
	out.BIMI = DNSRecord{State: st, Record: rec, Advice: map[string]string{
		"found":   "BIMI is published. This checks the TXT record only, not the logo, certificate or eligibility. Inbox providers decide whether to show an avatar.",
		"missing": "No BIMI record at default._bimi." + domain + ". A published record and DMARC enforcement are what an inbox avatar needs; many providers also require a mark certificate.",
		"unknown": "BIMI could not be looked up from this instance just now.",
	}[st]}
	return out
}
