package api

import (
	"strings"

	"github.com/eyupio/reet.sh/internal/agentupdate"
	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/mail"
	"github.com/eyupio/reet.sh/internal/notify"
	"github.com/eyupio/reet.sh/internal/sealed"
)

// AuthMode selects how the instance authenticates requests.
type AuthMode string

const (
	AuthToken    AuthMode = "token"
	AuthAccounts AuthMode = "accounts"
	AuthNone     AuthMode = "none"
)

// Option configures the server.
type Option func(*server)

// WithRemoteTerminals opts into remote shells. Health remains independent.
func WithRemoteTerminals(enabled bool) Option { return func(s *server) { s.remoteTerminals = enabled } }

// WithPublicURL sets the canonical origin used by MCP OAuth discovery behind a proxy.
func WithPublicURL(origin string) Option {
	return func(s *server) { s.publicURL = strings.TrimRight(origin, "/") }
}

// WithVersion sets the build version and commit reported by /api/version and /healthz.
func WithVersion(version, commit string) Option {
	return func(s *server) {
		if version != "" {
			s.version = version
		}
		if commit != "" {
			s.commit = commit
		}
	}
}

// WithReleaseChecker enables published-channel lookups for agent update status.
// Without it the controller reports no available versions and never auto-updates.
func WithReleaseChecker(c *agentupdate.Checker) Option {
	return func(s *server) { s.releases = c }
}

// WithDebug enables per-fetch debug logging.
func WithDebug(debug bool) Option {
	return func(s *server) { s.debug = debug }
}

// WithTopics enables the daily skills.sh topic seed.
func WithTopics(enabled bool) Option {
	return func(s *server) { s.topics = enabled }
}

// WithMaintenance enables the scheduled machine maintenance sweeps (off until an admin turns them on).
func WithMaintenance(enabled bool) Option {
	return func(s *server) { s.maintenanceJob = enabled }
}

// WithRefresh enables the six-hourly metadata refresh.
func WithRefresh(enabled bool) Option {
	return func(s *server) { s.refreshJob = enabled }
}

// WithAuthMode selects token, accounts or no authentication.
func WithAuthMode(mode AuthMode) Option {
	return func(s *server) { s.authMode = mode }
}

// WithSignup opens self-serve account creation in accounts mode. It is off by
// default so an instance stays invite-only unless the operator opts in.
func WithSignup(enabled bool) Option {
	return func(s *server) { s.signup = enabled }
}

// WithSupportDesk turns on support tickets: the Support page, the ticket API and
// the administrators' queue. It is off by default because a ticket needs
// someone other than the requester to answer it; a team operating a controller
// for other teams opts in.
func WithSupportDesk(enabled bool) Option {
	return func(s *server) { s.supportDesk = enabled }
}

// WithContactEmail publishes an optional admin contact for account and
// password-reset requests. It is shown on the sign-in screen.
func WithContactEmail(email string) Option {
	return func(s *server) { s.contactEmail = strings.TrimSpace(email) }
}

// WithBackups gives the API the backup service. Without one the backup pages
// answer that no key is configured.
func WithBackups(service *backup.Service) Option {
	return func(s *server) { s.backups = service }
}

// WithMail gives the API the outbox, for account email and team notices.
// Without one, email-dependent features explain that it is not wired in.
func WithMail(m *mail.Mailer) Option {
	return func(s *server) { s.mail = m }
}

// WithNotify gives the API the notification centre and its email delivery.
func WithNotify(n *notify.Service) Option {
	return func(s *server) { s.notify = n }
}

// WithPrivateConnection enables the lazily started Tailcat listener. The
// sealing key is the existing instance backup key; without it the listener is
// disabled and bootstrap requests explain that it is unavailable. The relay map
// URL is optional and must be HTTPS.
func WithPrivateConnection(sealer *sealed.Sealer, relayMapURL string) Option {
	return func(s *server) {
		s.privateSealer = sealer
		s.privateRelayURL = strings.TrimSpace(relayMapURL)
	}
}
