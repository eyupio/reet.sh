package mail

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/mail"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// Queue sends what is in the outbox. A message is claimed (marked sending, so
// a second worker cannot take it), sent, and then recorded as delivered,
// retried later with a longer wait each time, or given up on: failed once its
// attempts are used, bounced at once when the server refuses the address for
// good.
type Queue struct {
	Store *store.Store
	// Settings is where the configuration comes from on each pass, so a save
	// on the Email page applies to the next message. Nil uses Config.
	Settings *Settings
	Config   Config
	// Sender, when set, delivers every message whatever the configuration
	// says: tests set it. Nil builds the sender from the configuration --
	// SMTP, the local outbox directory, or nothing, in which case nothing is
	// claimed and messages wait.
	Sender Sender
	Logger *slog.Logger
	Now    func() time.Time
	// MaxAttempts is how many sends a message gets before it is failed. Zero
	// is 10, which with the backoff below is about six hours of trying: long
	// enough to outlast a provider's bad afternoon, short enough that a
	// failure is on the admin page the same day.
	MaxAttempts int
	// Batch is how many messages one pass claims. Zero is 20.
	Batch int

	once sync.Once
	kick chan struct{}
}

func (q *Queue) init() { q.once.Do(func() { q.kick = make(chan struct{}, 1) }) }

func (q *Queue) now() time.Time {
	if q.Now != nil {
		return q.Now().UTC()
	}
	return time.Now().UTC()
}

func (q *Queue) log() *slog.Logger {
	if q.Logger != nil {
		return q.Logger
	}
	return slog.New(slog.DiscardHandler)
}

func (q *Queue) maxAttempts() int {
	if q.MaxAttempts > 0 {
		return q.MaxAttempts
	}
	return 10
}

// Backoff is how long to wait after the nth failed attempt: a minute,
// doubling, capped at two hours, with a tenth either way of jitter so a
// provider's outage does not end with every message retried in one second.
func Backoff(attempt int) time.Duration {
	d := time.Minute
	for i := 1; i < attempt && d < 2*time.Hour; i++ {
		d *= 2
	}
	d = min(d, 2*time.Hour)
	j := time.Duration(rand.Int64N(int64(d/5))) - d/10
	return d + j
}

func (q *Queue) config() Config {
	if q.Settings != nil {
		return q.Settings.Current()
	}
	return q.Config
}

// SenderFor is the sender a configuration makes: SMTP when a server is set,
// the local outbox directory when one is, and nil when neither is.
func SenderFor(c Config) Sender {
	switch c.Mode() {
	case "smtp":
		return &SMTP{Config: c}
	case "outbox":
		return Dir{Path: c.OutboxDir}
	}
	return nil
}

// Kick asks the loop to look now rather than at its next tick: a reset link
// should not wait a minute for a timer.
func (q *Queue) Kick() {
	q.init()
	select {
	case q.kick <- struct{}{}:
	default:
	}
}

// Pass is what one RunOnce did.
type Pass struct {
	Sent, Retried, Failed, Bounced int
}

// RunOnce sends every due message it can claim. A pass that stops partway --
// the database refused a write, or the instance is shutting down -- puts the
// messages it had not yet tried back in the queue, rather than leaving them
// marked sending, and unsendable and unresendable, until a restart.
func (q *Queue) RunOnce(ctx context.Context) (Pass, error) {
	var claimed []store.Mail
	tried := 0
	p, err := q.runOnce(ctx, &claimed, &tried)
	if err != nil && tried < len(claimed) {
		ids := make([]string, 0, len(claimed)-tried)
		for _, m := range claimed[tried:] {
			ids = append(ids, m.ID)
		}
		if rerr := q.Store.ReleaseMail(ids); rerr != nil {
			q.log().Error("email: putting untried messages back in the queue", "error", rerr)
		}
	}
	return p, err
}

func (q *Queue) runOnce(ctx context.Context, claimed *[]store.Mail, tried *int) (Pass, error) {
	var p Pass
	cfg := q.config()
	sender := q.Sender
	if sender == nil {
		sender = SenderFor(cfg)
	}
	if sender == nil {
		return p, nil
	}
	batch := q.Batch
	if batch <= 0 {
		batch = 20
	}
	ms, err := q.Store.ClaimMail(batch, q.now().UnixMilli())
	if err != nil {
		return p, err
	}
	*claimed = ms
	for i, m := range ms {
		*tried = i + 1
		if err := ctx.Err(); err != nil {
			*tried = i
			return p, err
		}
		msg := Message{ID: m.ID, From: cfg.From, To: m.To, ReplyTo: cfg.ReplyTo, Subject: m.Subject,
			Text: m.Text, HTML: m.HTML, Date: time.UnixMilli(m.CreatedAt)}
		if msg.From.Address == "" {
			msg.From = mail.Address{Name: "reet.sh", Address: "no-reply@localhost"}
		}
		err := sender.Send(ctx, msg)
		attempt := m.Attempts + 1
		switch {
		case err == nil:
			if err := q.Store.MailSent(m.ID, sender.Name()); err != nil {
				return p, err
			}
			p.Sent++
			// The address is who it went to, which admins need; the body is
			// never logged.
			q.log().Info("email: sent", "id", m.ID, "template", m.Template, "to", m.To, "via", sender.Name())
		case Permanent(err):
			if err := q.Store.MailGiveUp(m.ID, store.MailBounced, err.Error()); err != nil {
				return p, err
			}
			p.Bounced++
			q.log().Warn("email: refused for good; it will not be retried", "id", m.ID, "to", m.To, "error", err)
		case attempt >= q.maxAttempts():
			if err := q.Store.MailGiveUp(m.ID, store.MailFailed, err.Error()); err != nil {
				return p, err
			}
			p.Failed++
			q.log().Error("email: out of attempts; resend it from Admin, Email once the server is back", "id", m.ID, "to", m.To, "attempts", attempt, "error", err)
		default:
			next := q.now().Add(Backoff(attempt))
			if err := q.Store.MailRetry(m.ID, err.Error(), next.UnixMilli()); err != nil {
				return p, err
			}
			p.Retried++
			q.log().Warn("email: not sent; it will be tried again", "id", m.ID, "attempt", attempt, "next", next.Format(time.RFC3339), "error", err)
		}
	}
	return p, nil
}

// Loop runs RunOnce every interval, and at once when kicked. Messages left
// marked sending by an instance that stopped mid-send are put back first.
func (q *Queue) Loop(ctx context.Context, every time.Duration) {
	q.init()
	if n, err := q.Store.RecoverSendingMail(); err != nil {
		q.log().Error("email: recovering messages left sending", "error", err)
	} else if n > 0 {
		q.log().Warn("email: messages were being sent when reet last stopped; they will be sent again", "count", n)
	}
	t := time.NewTicker(every)
	defer t.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	if _, err := q.Store.PruneMail(); err != nil {
		q.log().Error("email: pruning the outbox", "error", err)
	}
	for {
		for {
			p, err := q.RunOnce(ctx)
			if err != nil {
				q.log().Error("email: sending from the outbox", "error", err)
				break
			}
			// A full batch may mean more are due; an empty one means rest.
			if p.Sent+p.Retried+p.Failed+p.Bounced == 0 || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-prune.C:
			if _, err := q.Store.PruneMail(); err != nil {
				q.log().Error("email: pruning the outbox", "error", err)
			}
		case <-q.kick:
		}
	}
}

// LinkRetention is how long a development outbox keeps the files it writes.
const LinkRetention = 30 * 24 * time.Hour
