package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"
)

// Template names. They are recorded in the outbox so an admin can see what
// kind of message a row is, and are stable for that.
const (
	TplPasswordReset   = "password_reset"
	TplPasswordChanged = "password_changed"
	TplInvite          = "invite"
	TplWelcome         = "welcome"
	TplTest            = "test"

	TplTeamMemberJoined = "team_member_joined"
	TplTeamMemberLeft   = "team_member_left"
	TplTeamMemberRole   = "team_member_role"
	TplTeamListShared   = "team_list_shared"
)

// Data is what a message is written from.
type Data struct {
	Name     string
	Email    string
	Team     string
	List     string
	Actor    string
	Role     string
	Link     string
	Base     string
	Inviter  string
	Instance string
	Why      string
	// Audience is managers or members, for a team notice's footer.
	Audience string
	When     time.Time
	Expires  time.Time
	Minutes  int
}

// Content is one message's words, before the layout.
type Content struct {
	Subject   string
	Preheader string
	Heading   string
	Tone      string
	Intro     []string
	Facts     []Fact
	Button    *Link
	After     []string
	Why       string
}

// Fact is one row of the facts table.
type Fact struct{ Label, Value string }

// Link is a button, and the address written out beneath it for a client that
// shows no buttons.
type Link struct{ Label, URL string }

// Rendered is a message ready for the outbox.
type Rendered struct {
	Subject string
	Text    string
	HTML    string
}

// Templates is every message kind, for a page that lists them.
func Templates() []string {
	return []string{
		TplPasswordReset, TplPasswordChanged, TplInvite, TplWelcome, TplTest,
		TplTeamMemberJoined, TplTeamMemberLeft, TplTeamMemberRole, TplTeamListShared,
	}
}

var messages = map[string]func(Data) Content{
	TplPasswordReset: func(d Data) Content {
		return Content{
			Subject:   "Reset your reet.sh password",
			Preheader: "Choose a new password. The link works once, for " + minutes(d.Minutes) + ".",
			Heading:   "Choose a new password",
			Intro: []string{
				hello(d),
				"Somebody asked to reset the password for your reet.sh account. If that was you, choose a new one below.",
			},
			Facts:  []Fact{{"Account", or(d.Email, "your account")}, {"Link works for", minutes(d.Minutes)}},
			Button: &Link{Label: "Choose a new password", URL: d.Link},
			After:  []string{"The link works once. If you did not ask for this, ignore this email: your password has not changed."},
			Why:    "You were sent this because somebody asked to reset this address's reet.sh password. A security notice like this cannot be turned off.",
		}
	},
	TplPasswordChanged: func(d Data) Content {
		return Content{
			Subject:   "Your reet.sh password changed",
			Preheader: "Your password was changed and other sessions were signed out.",
			Heading:   "Your password changed",
			Intro: []string{
				hello(d),
				"The password for your reet.sh account was changed on " + at(d.When) + ". Every other session was signed out.",
			},
			Button: &Link{Label: "Open reet.sh", URL: d.Link},
			After:  []string{"If this was not you, ask your instance administrator to reset it now and check the account's activity."},
			Why:    "You were sent this because this address belongs to a reet.sh account. A security notice like this cannot be turned off.",
		}
	},
	TplInvite: func(d Data) Content {
		inviter := d.Inviter
		if inviter == "" {
			inviter = "An administrator"
		}
		return Content{
			Subject:   "You are invited to " + or(d.Instance, "reet.sh"),
			Preheader: inviter + " invited you as " + roleWords(d.Role) + ".",
			Heading:   "You are invited",
			Intro: []string{
				hello(d),
				inviter + " invited you to " + or(d.Instance, "a reet.sh instance") + " as " + roleWords(d.Role) + ". Your shelf, shared lists and agent setups are waiting.",
			},
			Facts:  []Fact{{"Role", roleWords(d.Role)}, {"Invitation works until", at(d.Expires)}},
			Button: &Link{Label: "Join reet.sh", URL: d.Link},
			After:  []string{"The invitation works once. If you were not expecting it, you can ignore this email."},
			Why:    "You were sent this because somebody invited this address to a reet.sh instance. There is nothing to unsubscribe from: no account exists for the address until the link is used.",
		}
	},
	TplWelcome: func(d Data) Content {
		return Content{
			Subject:   "Welcome to " + or(d.Instance, "reet.sh"),
			Preheader: "Your shelf is ready.",
			Heading:   "Your shelf is ready",
			Intro: []string{
				hello(d),
				"Your reet.sh account is ready. Save the skills you find, give new ones a go, and keep every agent on every machine in step.",
			},
			Button: &Link{Label: "Open your shelf", URL: d.Link},
			After:  []string{"Your account's password and sessions are on your account page. Your instance administrator can help if you get stuck."},
			Why:    "You were sent this because an account was created for this address on a reet.sh instance. A welcome notice like this cannot be turned off.",
		}
	},
	TplTest: func(d Data) Content {
		return Content{
			Subject:   "Test email from reet.sh",
			Preheader: "If you can read this, the instance's email works.",
			Heading:   "The instance's email works",
			Intro: []string{
				hello(d),
				"This is the test email an administrator asked for from Admin, Email.",
				"It was sent through " + d.Instance + ". Check that it arrived in the inbox rather than spam, and look at its headers for spf=pass, dkim=pass and dmarc=pass.",
			},
			Facts: []Fact{{"Sent to", d.Email}, {"Sent through", d.Instance}},
			Why:   "You were sent this because you pressed Send test email. There is nothing to unsubscribe from.",
		}
	},
	TplTeamMemberJoined: func(d Data) Content {
		return Content{
			Subject:   or(d.Name, "Someone") + " joined " + d.Team,
			Preheader: or(d.Name, "Someone") + " is now " + roleWords(d.Role) + " of " + d.Team + ".",
			Heading:   "A new member joined",
			Intro: []string{
				hello(d),
				or(d.Name, "Someone") + " joined " + d.Team + " on " + at(d.When) + " as " + roleWords(d.Role) + ".",
			},
			Facts:  []Fact{{"Team", d.Team}, {"Person", or(d.Name, "an account")}, {"Role", roleWords(d.Role)}},
			Button: &Link{Label: "Open Teams", URL: d.Link},
			Why:    teamWhy(d),
		}
	},
	TplTeamMemberLeft: func(d Data) Content {
		return Content{
			Subject:   or(d.Name, "Someone") + " left " + d.Team,
			Preheader: or(d.Name, "Someone") + " is no longer a member of " + d.Team + ".",
			Heading:   "A member left",
			Intro: []string{
				hello(d),
				or(d.Name, "Someone") + " left " + d.Team + " on " + at(d.When) + ".",
			},
			Facts:  []Fact{{"Team", d.Team}, {"Person", or(d.Name, "an account")}},
			Button: &Link{Label: "Open Teams", URL: d.Link},
			After:  []string{"Lists shared with this team are no longer available through its membership. A direct invitation to a list still applies until it is removed."},
			Why:    teamWhy(d),
		}
	},
	TplTeamMemberRole: func(d Data) Content {
		return Content{
			Subject:   or(d.Name, "Someone") + " is now " + roleWords(d.Role) + " of " + d.Team,
			Preheader: "A team role changed.",
			Heading:   "A team role changed",
			Intro: []string{
				hello(d),
				or(d.Name, "Someone") + " is now " + roleWords(d.Role) + " of " + d.Team + ", from " + at(d.When) + ".",
			},
			Facts:  []Fact{{"Team", d.Team}, {"Person", or(d.Name, "an account")}, {"Role", roleWords(d.Role)}},
			Button: &Link{Label: "Open Teams", URL: d.Link},
			Why:    teamWhy(d),
		}
	},
	TplTeamListShared: func(d Data) Content {
		return Content{
			Subject:   d.List + " is now shared with " + d.Team,
			Preheader: "A list was shared with your team.",
			Heading:   "A list is shared with your team",
			Intro: []string{
				hello(d),
				or(d.Actor, "A team manager") + " shared " + d.List + " with " + d.Team + " on " + at(d.When) + ".",
				"Everyone the audience includes can read it; the access chosen for the team decides whether they can also change its skills and notes.",
			},
			Facts:  []Fact{{"List", d.List}, {"Team", d.Team}},
			Button: &Link{Label: "Open the list", URL: d.Link},
			Why:    teamWhy(d),
		}
	},
}

func or(s, alt string) string {
	if s != "" {
		return s
	}
	return alt
}

func hello(d Data) string {
	if d.Name == "" {
		return "Hello,"
	}
	return "Hello " + d.Name + ","
}

func roleWords(role string) string {
	switch role {
	case "manager":
		return "a manager"
	case "admin":
		return "an administrator"
	case "viewer":
		return "a viewer"
	case "owner":
		return "the owner"
	}
	return "a member"
}

func minutes(n int) string {
	switch {
	case n <= 0:
		return "a short while"
	case n == 1:
		return "1 minute"
	case n < 60:
		return fmt.Sprintf("%d minutes", n)
	case n == 60:
		return "1 hour"
	case n < 24*60:
		return fmt.Sprintf("%d hours", n/60)
	}
	days := n / (24 * 60)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

func at(t time.Time) string {
	if t.IsZero() {
		return "just now"
	}
	return t.UTC().Format("2 January 2006 at 15:04 MST")
}

func teamWhy(d Data) string {
	if d.Audience == "members" {
		return "You were sent this because you are a member of " + d.Team + ". A team manager chose what this team is emailed about."
	}
	return "You were sent this because this address is on " + d.Team + "'s notification list. A team manager chose what it is emailed about."
}

// Compose draws one message from its data.
func Compose(name string, d Data) (Rendered, error) {
	build, ok := messages[name]
	if !ok {
		return Rendered{}, fmt.Errorf("email: no message template named %q", name)
	}
	c := build(d)
	return Render(c, d.Base)
}

// Render draws content as plain text and HTML. base is the instance address,
// used only for the footer; every link in the content is already absolute.
func Render(c Content, base string) (Rendered, error) {
	var htmlOut bytes.Buffer
	if err := htmlTemplate.Execute(&htmlOut, map[string]any{
		"Content": c,
		"Base":    strings.TrimRight(base, "/"),
		"Tone":    toneColour(c.Tone),
	}); err != nil {
		return Rendered{}, err
	}
	return Rendered{Subject: c.Subject, Text: renderText(c, base), HTML: htmlOut.String()}, nil
}

func toneColour(tone string) string {
	switch tone {
	case "warning":
		return "#92400e"
	case "danger":
		return "#b91c1c"
	}
	return "#c2410c"
}

func renderText(c Content, base string) string {
	var b strings.Builder
	b.WriteString("reet.sh\n\n")
	b.WriteString(c.Heading + "\n\n")
	for _, p := range c.Intro {
		b.WriteString(p + "\n\n")
	}
	for _, f := range c.Facts {
		b.WriteString(f.Label + ": " + f.Value + "\n")
	}
	if len(c.Facts) > 0 {
		b.WriteString("\n")
	}
	if c.Button != nil {
		b.WriteString(c.Button.Label + ":\n" + c.Button.URL + "\n\n")
	}
	for _, p := range c.After {
		b.WriteString(p + "\n\n")
	}
	if c.Why != "" {
		b.WriteString(c.Why + "\n\n")
	}
	b.WriteString("reet.sh · your shelf for everything AI")
	if base != "" {
		b.WriteString(" · " + strings.TrimRight(base, "/"))
	}
	return b.String()
}

var htmlTemplate = template.Must(template.New("email").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Content.Subject}}</title>
</head>
<body style="margin:0;padding:0;background:#f7f6f2;color:#1d1c1a;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent;">{{.Content.Preheader}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f7f6f2;">
<tr><td align="center" style="padding:32px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;">
<tr><td style="padding:0 4px 18px;">
<div style="font:700 22px/1.2 Georgia,'Iowan Old Style','Palatino Linotype',serif;letter-spacing:-0.02em;color:#1d1c1a;">reet<span style="color:#c2410c;">.sh</span></div>
</td></tr>
<tr><td style="background:#fffdf9;border:1px solid #dedbd2;border-radius:12px;padding:28px;">
<div style="height:3px;width:48px;border-radius:3px;background:{{.Tone}};margin:0 0 18px;"></div>
<h1 style="margin:0 0 16px;font:600 26px/1.25 Georgia,'Iowan Old Style','Palatino Linotype',serif;letter-spacing:-0.02em;color:#1d1c1a;">{{.Content.Heading}}</h1>
{{range .Content.Intro}}<p style="margin:0 0 14px;font:15px/1.6 system-ui,-apple-system,'Segoe UI',sans-serif;color:#1d1c1a;">{{.}}</p>{{end}}
{{if .Content.Facts}}<table role="presentation" cellpadding="0" cellspacing="0" style="width:100%;margin:18px 0;border-top:1px solid #dedbd2;border-bottom:1px solid #dedbd2;">{{range .Content.Facts}}<tr><td style="padding:9px 0;font:13px/1.5 system-ui,-apple-system,'Segoe UI',sans-serif;color:#5f5c54;width:38%;">{{.Label}}</td><td style="padding:9px 0;font:14px/1.5 system-ui,-apple-system,'Segoe UI',sans-serif;color:#1d1c1a;overflow-wrap:anywhere;">{{.Value}}</td></tr>{{end}}</table>{{end}}
{{if .Content.Button}}<table role="presentation" cellpadding="0" cellspacing="0" style="margin:22px 0 8px;"><tr><td style="border-radius:8px;background:#c2410c;"><a href="{{.Content.Button.URL}}" style="display:inline-block;padding:12px 20px;font:600 14px/1.2 system-ui,-apple-system,'Segoe UI',sans-serif;color:#ffffff;text-decoration:none;border-radius:8px;">{{.Content.Button.Label}}</a></td></tr></table>
<p style="margin:6px 0 16px;font:12px/1.5 system-ui,-apple-system,'Segoe UI',sans-serif;color:#5f5c54;overflow-wrap:anywhere;">Or open <a href="{{.Content.Button.URL}}" style="color:#c2410c;">{{.Content.Button.URL}}</a></p>{{end}}
{{range .Content.After}}<p style="margin:0 0 14px;font:14px/1.6 system-ui,-apple-system,'Segoe UI',sans-serif;color:#5f5c54;">{{.}}</p>{{end}}
</td></tr>
<tr><td style="padding:20px 6px 0;">
<p style="margin:0 0 8px;font:12px/1.6 system-ui,-apple-system,'Segoe UI',sans-serif;color:#5f5c54;">{{.Content.Why}}</p>
<p style="margin:0;font:12px/1.6 system-ui,-apple-system,'Segoe UI',sans-serif;color:#5f5c54;">reet.sh · your shelf for everything AI{{if .Base}} · <a href="{{.Base}}" style="color:#5f5c54;">{{.Base}}</a>{{end}}</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`))
