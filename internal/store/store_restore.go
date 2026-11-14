package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// SchemaVersion is what this build writes to the database's user_version. It
// goes up when a release changes the schema in a way an older release would
// misread, never for an additive migration the older one simply ignores. A
// copy stamped higher than this was taken by a newer reet and is refused
// rather than opened by a binary that does not understand it.
//
// Releases cannot make this comparison: development builds carry no ordered
// version, and a copy's manifest records only the release string.
const SchemaVersion = 3

func stampSchemaVersion(db *sql.DB) error {
	var have int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&have); err != nil {
		return err
	}
	// Never lower it: a newer release that opened this database last
	// recorded what it needs, and this build must not hide that.
	if have >= SchemaVersion {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, SchemaVersion))
	return err
}

// RestoreSummary is what a copy holds, read without opening it for writing.
type RestoreSummary struct {
	// SchemaVersion is 0 for a copy taken before versions were recorded.
	SchemaVersion int
	Users         int
	UsersByRole   map[string]int
	// Shelves is the accounts that hold at least one skill on their shelf;
	// Skills is every skill the controller knows.
	Shelves        int
	Skills         int
	LibraryItems   int
	PrivateBundles int
	Lists          int
	Tags           int
	Environments   int
	// MadeTags and MadeEnvironments leave out what a controller creates for
	// itself: the topic tags the daily sync adds, and the report the server
	// makes of its own machine. A new controller holds those before anyone
	// has used it, so they are not data an import would replace.
	MadeTags         int
	MadeEnvironments int
	Machines         int
	Destinations     int
	Sessions         int
	APITokens        int
	MCPConnections   int
	// Owners are the addresses of the owner accounts that can sign in.
	Owners []string
}

// Adaptation is what was changed in a copy so it is safe to run on the
// controller it lands on.
type Adaptation struct {
	SessionsEnded         int
	TokensRevoked         int
	MCPConnectionsRevoked int
	IdentityDropped       bool
	GafferGoalsPaused     int
	// MailBaseRewritten and SyncOriginsRewritten say which saved addresses
	// were changed to the new public address; MCPGrantsStale counts the OAuth
	// grants removed because they were issued for the old one.
	MailBaseRewritten    bool
	SyncOriginsRewritten int
	MCPGrantsStale       int
	// Paused lists, as a person reads them, the work that was held back so the
	// restored controller does not repeat it: project run members, software
	// batches, queued mail. The operator resumes each with the retry the
	// product already has for it.
	Paused []string
	// OwnerMade is the address of the account made owner, as the copy spells
	// it, and OwnersDemoted the owners it replaced, who are now admins.
	OwnerMade     string
	OwnersDemoted []string
	// Cleared names each stored secret that could not be opened on the
	// controller the copy lands on, and what to do about it. The value is
	// removed rather than kept as something nothing can read.
	Cleared []string
}

// AdaptOptions chooses what is changed in the copy.
type AdaptOptions struct {
	// DropSessions ends every browser sign-in and password-reset link. By
	// default they come across.
	DropSessions bool
	// DropTokens revokes personal API tokens and MCP connections. By default
	// they come across.
	DropTokens bool
	// PublicURL, when set, is the new address of the controller: the saved mail
	// base URL and the sync library origin are rewritten to it, and MCP grants
	// issued for the old address are removed. It must already be a plain
	// http(s) origin (see ValidatePublicURL). Empty leaves every address alone.
	PublicURL string
	// MakeOwner, when set, is the address of an account already in the copy
	// that becomes its owner, signing in with OwnerPasswordHash. It is for a
	// copy whose owner nobody on the new machine can sign in as. An address
	// the copy does not hold, or holds disabled, is refused.
	MakeOwner         string
	OwnerPasswordHash string
	// Unreadable says whether a sealed secret cannot be opened by the
	// controller the copy lands on. Nil leaves every secret alone.
	Unreadable func(sealed string) bool
	// EventSource and EventAction, with EventDetail, add one line to the
	// copy's activity log. Nothing secret belongs in the detail.
	EventSource, EventAction string
	EventDetail              map[string]any
}

func openDatabaseFile(ctx context.Context, path string, readOnly bool) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(3000)"
	if readOnly {
		dsn += "&mode=ro"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func tableExists(ctx context.Context, db rowQuerier, name string) bool {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return err == nil && n > 0
}

// InspectDatabaseFile reports what a restored database would bring, so an
// operator is told what came across rather than only an id. The file is
// opened read-only.
func InspectDatabaseFile(ctx context.Context, path string) (RestoreSummary, error) {
	db, err := openDatabaseFile(ctx, path, true)
	if err != nil {
		return RestoreSummary{}, err
	}
	defer db.Close()
	sum := RestoreSummary{UsersByRole: map[string]int{}, Owners: []string{}}
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&sum.SchemaVersion); err != nil {
		return sum, err
	}
	fillSummary(ctx, db, &sum)
	return sum, nil
}

type queryer interface {
	rowQuerier
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func fillSummary(ctx context.Context, db queryer, sum *RestoreSummary) {
	count := func(table, where string) int {
		if !tableExists(ctx, db, table) {
			return 0
		}
		q := `SELECT COUNT(*) FROM ` + table
		if where != "" {
			q += ` WHERE ` + where
		}
		var n int
		if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			return 0
		}
		return n
	}
	sum.Users = count("users", "")
	sum.Skills = count("skills", "")
	sum.LibraryItems = count("library_items", "deleted_at=0")
	sum.PrivateBundles = count("private_skill_bundles", "")
	sum.Lists = count("lists", "")
	sum.Tags = count("tags", "")
	sum.Environments = count("environments", "")
	sum.Machines = count("sync_agents", "")
	sum.Destinations = count("backup_remotes", "")
	sum.Sessions = count("sessions", "")
	sum.APITokens = count("api_tokens", "revoked_at=0")
	sum.MCPConnections = count("mcp_connections", "revoked_at=0")
	sum.MadeEnvironments = count("environments", "agent<>'reet-server'")
	if tableExists(ctx, db, "topic_skills") {
		sum.MadeTags = count("tags", "id NOT IN (SELECT tag_id FROM topic_skills)")
	} else {
		sum.MadeTags = sum.Tags
	}
	if tableExists(ctx, db, "shelf") {
		_ = db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT user_id) FROM shelf`).Scan(&sum.Shelves)
	}
	if tableExists(ctx, db, "users") {
		if rows, err := db.QueryContext(ctx, `SELECT role,COUNT(*) FROM users GROUP BY role`); err == nil {
			for rows.Next() {
				var role string
				var n int
				if rows.Scan(&role, &n) == nil {
					sum.UsersByRole[role] = n
				}
			}
			rows.Close()
		}
		if rows, err := db.QueryContext(ctx, `SELECT email FROM users WHERE role='owner' AND disabled_at=0 ORDER BY id`); err == nil {
			for rows.Next() {
				var e string
				if rows.Scan(&e) == nil {
					sum.Owners = append(sum.Owners, e)
				}
			}
			rows.Close()
		}
	}
}

// Holdings reads the same counts from the live database, for deciding
// whether a controller already holds data.
func (s *Store) Holdings(ctx context.Context) RestoreSummary {
	sum := RestoreSummary{UsersByRole: map[string]int{}, Owners: []string{}}
	fillSummary(ctx, s.db, &sum)
	return sum
}

const unreadableFix = "enter it again"

// AdaptDatabaseFile makes a restored copy safe to run beside, or in place of,
// the controller it was taken from. Sessions and personal tokens come across
// unless the operator drops them: a credential from the old machine opening
// the new one is then a choice, not an accident. Stored secrets this
// controller cannot open are cleared. The private-connection identity is
// always removed so the new controller mints its own: two controllers must
// never present one.
func AdaptDatabaseFile(ctx context.Context, path string, opts AdaptOptions) (Adaptation, error) {
	out := Adaptation{Cleared: []string{}}
	db, err := openDatabaseFile(ctx, path, false)
	if err != nil {
		return out, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	exec := func(table, stmt string, args ...any) (int, error) {
		if !tableExists(ctx, tx, table) {
			return 0, nil
		}
		res, err := tx.ExecContext(ctx, stmt, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	// A copy can predate external worker effects, or run beside its original.
	// Restoring it must not reauthorise background work from that old snapshot.
	if tableExists(ctx, tx, "gaffer_goals") {
		if out.GafferGoalsPaused, err = exec("gaffer_goals", `UPDATE gaffer_goals SET data=json_set(data,'$.state','needs_user','$.revision',json_extract(data,'$.revision')+1,'$.activeTurnId','','$.pendingId','','$.note','This controller was restored from a copy. Check the working conversation and explicitly resume follow-up.','$.updatedAt',?) WHERE json_extract(data,'$.state') NOT IN('stopped','completed')`, now()); err != nil {
			return out, err
		}
		if _, err = exec("terminal_chat_queue", `UPDATE terminal_chat_queue SET state=CASE WHEN state='sending' THEN 'uncertain' ELSE 'removed' END,text='',images='[]',note='Restored copy: review goal delivery before resuming.',revision=revision+1 WHERE state IN('queued','sending') AND origin='companion' AND id IN(SELECT r.queue_id FROM worker_message_receipts r JOIN gaffer_turns t ON t.conversation_id=r.subject AND t.id=r.turn_id WHERE r.kind='gaffer' AND t.goal_id<>'')`); err != nil {
			return out, err
		}
	}
	// An outcome still starting sessions in the copy must not start them again here.
	if tableExists(ctx, tx, "gaffer_outcomes") {
		if _, err = exec("gaffer_outcomes", `UPDATE gaffer_outcomes SET data=json_set(data,'$.state','stopped','$.revision',json_extract(data,'$.revision')+1,'$.note','This controller was restored from a copy, so the sessions not yet started were left unstarted.') WHERE json_extract(data,'$.state')='starting'`); err != nil {
			return out, err
		}
	}
	if opts.DropSessions {
		if out.SessionsEnded, err = exec("sessions", `DELETE FROM sessions`); err != nil {
			return out, err
		}
		if _, err = exec("password_resets", `DELETE FROM password_resets`); err != nil {
			return out, err
		}
	}
	if opts.DropTokens {
		at := now()
		if out.TokensRevoked, err = exec("api_tokens", `UPDATE api_tokens SET revoked_at=? WHERE revoked_at=0`, at); err != nil {
			return out, err
		}
		if out.MCPConnectionsRevoked, err = exec("mcp_connections", `UPDATE mcp_connections SET revoked_at=? WHERE revoked_at=0`, at); err != nil {
			return out, err
		}
		for _, t := range []string{"mcp_oauth_codes", "mcp_oauth_access", "mcp_oauth_refresh"} {
			if _, err = exec(t, `DELETE FROM `+t); err != nil {
				return out, err
			}
		}
	}
	if opts.MakeOwner != "" {
		if out.OwnerMade, out.OwnersDemoted, err = makeOwner(ctx, tx, opts.MakeOwner, opts.OwnerPasswordHash); err != nil {
			return out, err
		}
	}
	// What a copy says about a fence is about the controller it was taken
	// from. The controller it lands on is the one meant to be live.
	if _, err = exec("settings", `DELETE FROM settings WHERE key IN(?,?)`, SettingFenceSince, SettingFenceMovedTo); err != nil {
		return out, err
	}
	if opts.PublicURL != "" {
		n, err := exec("mail_settings", `UPDATE mail_settings SET base_url=? WHERE base_url<>'' AND base_url<>?`, opts.PublicURL, opts.PublicURL)
		if err != nil {
			return out, err
		}
		out.MailBaseRewritten = n > 0
		if out.SyncOriginsRewritten, err = exec("sync_library", `UPDATE sync_library SET origin=? WHERE origin<>'' AND origin<>?`, opts.PublicURL, opts.PublicURL); err != nil {
			return out, err
		}
		// A grant is bound to the address it was issued for (its resource).
		// Removing the OAuth rows makes each client ask again, here.
		for _, t := range []string{"mcp_oauth_codes", "mcp_oauth_access", "mcp_oauth_refresh"} {
			n, err := exec(t, `DELETE FROM `+t)
			if err != nil {
				return out, err
			}
			out.MCPGrantsStale += n
		}
	}
	if out.Paused, err = pauseWork(ctx, tx, out.GafferGoalsPaused); err != nil {
		return out, err
	}
	n, err := exec("private_transport_identity", `DELETE FROM private_transport_identity`)
	if err != nil {
		return out, err
	}
	out.IdentityDropped = n > 0
	if opts.Unreadable != nil {
		if out.Cleared, err = clearUnreadable(ctx, tx, opts.Unreadable); err != nil {
			return out, err
		}
	}
	if opts.EventAction != "" && tableExists(ctx, tx, "events") {
		detail := map[string]any{}
		for k, v := range opts.EventDetail {
			detail[k] = v
		}
		detail["sessionsDropped"] = opts.DropSessions
		detail["tokensDropped"] = opts.DropTokens
		detail["secretsCleared"] = len(out.Cleared)
		detail["gafferGoalsPaused"] = out.GafferGoalsPaused
		detail["workPaused"] = len(out.Paused)
		if out.OwnerMade != "" {
			detail["ownerMade"] = out.OwnerMade
			detail["ownersDemoted"] = len(out.OwnersDemoted)
		}
		if opts.PublicURL != "" {
			detail["publicUrl"] = opts.PublicURL
		}
		raw, _ := json.Marshal(detail)
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(at,user_id,actor,source,action,skill_id,detail) VALUES(?,0,'',?,?,'',?)`,
			now(), opts.EventSource, opts.EventAction, string(raw)); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	// Fold the write-ahead log in so the single file is the whole copy.
	_, _ = db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return out, nil
}

// ErrNoSuchAccount is returned when the account to make owner is not one the
// copy holds and could sign in with.
var ErrNoSuchAccount = errors.New("the copy has no such account")

// makeOwner gives the copy an owner somebody on the new machine can sign in
// as. There is one owner, so those the copy had become admins and lose
// nothing else. The account keeps its two-step sign-in; only its password,
// its browser sign-ins and any reset link are replaced, because whoever held
// the old ones is not necessarily who runs the new machine.
func makeOwner(ctx context.Context, tx *sql.Tx, email, hash string) (string, []string, error) {
	if hash == "" {
		return "", nil, errors.New("a new owner needs a password")
	}
	var id, disabled int64
	var spelled string
	err := tx.QueryRowContext(ctx, `SELECT id,email,disabled_at FROM users WHERE email=?`, strings.TrimSpace(email)).Scan(&id, &spelled, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("%w for %s", ErrNoSuchAccount, email)
	}
	if err != nil {
		return "", nil, err
	}
	if disabled != 0 {
		return "", nil, fmt.Errorf("%w that can sign in: %s is disabled", ErrNoSuchAccount, spelled)
	}
	var demoted []string
	rows, err := tx.QueryContext(ctx, `SELECT email FROM users WHERE role='owner' AND id<>? ORDER BY id`, id)
	if err != nil {
		return "", nil, err
	}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			rows.Close()
			return "", nil, err
		}
		demoted = append(demoted, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET role='admin' WHERE role='owner' AND id<>?`, id); err != nil {
		return "", nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET role='owner',password_hash=? WHERE id=?`, hash, id); err != nil {
		return "", nil, err
	}
	for _, t := range []string{"sessions", "password_resets"} {
		if !tableExists(ctx, tx, t) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE user_id=?`, id); err != nil {
			return "", nil, err
		}
	}
	return spelled, demoted, nil
}

// clearUnreadable removes each sealed secret the caller says cannot be opened
// and marks what owned it as needing the secret again. A destination is also
// switched off so it is not tried with no key.
func clearUnreadable(ctx context.Context, tx *sql.Tx, unreadable func(string) bool) ([]string, error) {
	cleared := []string{}
	type row struct {
		key    string
		label  string
		sealed string
	}
	collect := func(table, query string, label func(a string) string) ([]row, error) {
		if !tableExists(ctx, tx, table) {
			return nil, nil
		}
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.key, &r.sealed); err != nil {
				return nil, err
			}
			r.label = label(r.key)
			out = append(out, r)
		}
		return out, rows.Err()
	}
	remotes, err := collect("backup_remotes", `SELECT id,secret_sealed FROM backup_remotes WHERE secret_sealed<>''`,
		func(string) string { return "" })
	if err != nil {
		return nil, err
	}
	for _, r := range remotes {
		if !unreadable(r.sealed) {
			continue
		}
		var name string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM backup_remotes WHERE id=?`, r.key).Scan(&name); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE backup_remotes SET secret_sealed='',disabled=1,last_error=? WHERE id=?`,
			"the secret access key must be entered again", r.key); err != nil {
			return nil, err
		}
		cleared = append(cleared, fmt.Sprintf("the access key of backup destination %q was sealed under another key; it is cleared and the destination is switched off. Enter its secret access key again under Admin → Backups, then switch it on.", name))
	}
	providers, err := collect("gaffer_providers", `SELECT id,key_sealed FROM gaffer_providers WHERE key_sealed<>''`, func(string) string { return "a Gaffer provider key" })
	if err != nil {
		return nil, err
	}
	for _, r := range providers {
		if !unreadable(r.sealed) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE gaffer_providers SET key_sealed='',enabled=0,revision=revision+1,check_at=0,check_error='' WHERE id=?`, r.key); err != nil {
			return nil, err
		}
		cleared = append(cleared, "A Gaffer provider key could not be opened and was cleared. Re-enter it in Account → Gaffer AI, then enable the provider.")
	}
	pw, err := collect("backup_settings", `SELECT CAST(owner_id AS TEXT),password_sealed FROM backup_settings WHERE password_sealed<>''`,
		func(a string) string {
			if a == "0" {
				return "the instance copy password"
			}
			return "a person's copy password"
		})
	if err != nil {
		return nil, err
	}
	for _, r := range pw {
		if !unreadable(r.sealed) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE backup_settings SET password_sealed='' WHERE CAST(owner_id AS TEXT)=?`, r.key); err != nil {
			return nil, err
		}
		cleared = append(cleared, r.label+" was sealed under another key and is cleared; later copies are sealed under the backup key until you set a password again. Copies already taken with it still need it.")
	}
	mail, err := collect("mail_settings", `SELECT 'mail',password_sealed FROM mail_settings WHERE password_sealed<>''`,
		func(string) string { return "the outgoing mail password" })
	if err != nil {
		return nil, err
	}
	for _, r := range mail {
		if !unreadable(r.sealed) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE mail_settings SET password_sealed=''`); err != nil {
			return nil, err
		}
		cleared = append(cleared, r.label+" was sealed under another key and is cleared; "+unreadableFix+" under Admin → Email.")
	}
	return cleared, nil
}

// ValidatePublicURL accepts a plain http or https origin, such as
// https://reet.example.com or http://10.0.0.5:8080, and returns it without a
// trailing slash. A path, query, fragment or credentials are refused, because
// the saved addresses are origins that paths are added to.
func ValidatePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	plainPath := u != nil && (u.Path == "" || u.Path == "/")
	ok := err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery && plainPath && u.Opaque == ""
	if !ok {
		return "", errors.New("the public URL must be a plain http or https origin, such as https://reet.example.com, with no path, query, fragment or credentials")
	}
	return strings.TrimRight(raw, "/"), nil
}

// PausedByRestore is the code a held-back project run member or software batch
// member carries, and the reason a person reads.
const PausedByRestore = "paused_by_restore"

// pauseWork holds back what a restored copy would otherwise start again by
// itself at the next start, using the states the product already has, so the
// operator resumes each with the retry that already exists: a project run
// member waits as failed with a code, a software batch stops on its next
// member, and queued mail is held. Nothing is deleted. It returns one line per
// thing held, for the report.
func pauseWork(ctx context.Context, tx *sql.Tx, gaffer int) ([]string, error) {
	var lines []string
	if gaffer > 0 {
		lines = append(lines, fmt.Sprintf("%d Gaffer goal(s): paused; each needs an explicit resume.", gaffer))
	}
	rows := func(query string, args ...any) ([][3]string, error) {
		r, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		var out [][3]string
		for r.Next() {
			var a, b, c string
			if err := r.Scan(&a, &b, &c); err != nil {
				return nil, err
			}
			out = append(out, [3]string{a, b, c})
		}
		return out, r.Err()
	}
	if tableExists(ctx, tx, "project_run_members") {
		held, err := rows(`SELECT r.id,r.project_name,CAST(COUNT(*) AS TEXT) FROM project_runs r JOIN project_run_members m ON m.run_id=r.id AND m.user_id=r.user_id
			WHERE m.state='pending' GROUP BY r.user_id,r.id ORDER BY r.created_at,r.rowid`)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE project_run_members SET state='failed',code=? WHERE state='pending'`, PausedByRestore); err != nil {
			return nil, err
		}
		for _, h := range held {
			lines = append(lines, fmt.Sprintf("project run %s (%s): %s member(s) held; retry each member to start it.", h[0], h[1], h[2]))
		}
	}
	if tableExists(ctx, tx, "software_batches") {
		held, err := rows(`SELECT b.id,b.tool,CAST(COUNT(*) AS TEXT) FROM software_batches b JOIN software_batch_members m ON m.batch_id=b.id AND m.user_id=b.user_id
			WHERE b.state IN('pending','running') AND m.state IN('pending','running') GROUP BY b.user_id,b.id ORDER BY b.created_at,b.rowid`)
		if err != nil {
			return nil, err
		}
		const active = `batch_id IN(SELECT id FROM software_batches WHERE state IN('pending','running'))`
		first := `position=(SELECT MIN(x.position) FROM software_batch_members x WHERE x.batch_id=software_batch_members.batch_id AND x.user_id=software_batch_members.user_id AND x.state IN('pending','running'))`
		// A member that was running has an unknown outcome; every other running
		// member goes back to waiting, and the first stops the batch as a failure would.
		if _, err := tx.ExecContext(ctx, `UPDATE software_batch_members SET state='failed',code=?,updated_at=? WHERE state IN('pending','running') AND `+active+` AND `+first, PausedByRestore, now()); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE software_batch_members SET state='pending',updated_at=? WHERE state='running' AND `+active, now()); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE software_batches SET state='stopped',updated_at=? WHERE state IN('pending','running')`, now()); err != nil {
			return nil, err
		}
		for _, h := range held {
			lines = append(lines, fmt.Sprintf("software batch %s (%s): stopped with %s machine(s) waiting; retry the first machine to continue.", h[0], h[1], h[2]))
		}
	}
	if tableExists(ctx, tx, "mail_outbox") {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail_outbox WHERE state IN('queued','sending')`).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			if _, err := tx.ExecContext(ctx, `UPDATE mail_outbox SET state='held',updated_at=? WHERE state IN('queued','sending')`, now()); err != nil {
				return nil, err
			}
			lines = append(lines, fmt.Sprintf("%d queued email(s): held; send each from the outbox once mail is set up here. A held password-reset link points at the old address.", n))
		}
	}
	return lines, nil
}
