package store

import (
	"context"
	"database/sql"
	"fmt"
)

// SchemaVersion is what this build writes to the database's user_version. It
// goes up when a release changes the schema in a way an older release would
// misread, never for an additive migration the older one simply ignores. A
// copy stamped higher than this was taken by a newer reet and is refused
// rather than opened by a binary that does not understand it.
//
// Releases cannot make this comparison: development builds carry no ordered
// version, and a copy's manifest records only the release string.
const SchemaVersion = 1

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

// SealedSecret is one value in a database that only the backup key it was
// sealed under can open.
type SealedSecret struct {
	Where  string
	Sealed string
}

// RestoreSummary is what a copy holds, read without opening it for writing.
type RestoreSummary struct {
	// SchemaVersion is 0 for a copy taken before versions were recorded.
	SchemaVersion  int
	Users          int
	Skills         int
	Lists          int
	Tags           int
	Environments   int
	Sessions       int
	APITokens      int
	MCPConnections int
	Secrets        []SealedSecret
}

// Adaptation is what was changed in a copy so it is safe to run on the
// controller it lands on.
type Adaptation struct {
	SessionsEnded         int
	TokensRevoked         int
	MCPConnectionsRevoked int
	IdentityDropped       bool
}

// AdaptOptions chooses what survives the move.
type AdaptOptions struct {
	// KeepTokens leaves personal API tokens and MCP connections working, for
	// a move where the old controller is being retired.
	KeepTokens bool
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

// countRows tolerates a table an older copy never had.
func countFileRows(ctx context.Context, db *sql.DB, table, where string) int {
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

// InspectDatabaseFile reports what a restored database would bring, so an
// operator is told what came across rather than only an id.
func InspectDatabaseFile(ctx context.Context, path string) (RestoreSummary, error) {
	db, err := openDatabaseFile(ctx, path, true)
	if err != nil {
		return RestoreSummary{}, err
	}
	defer db.Close()
	var sum RestoreSummary
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&sum.SchemaVersion); err != nil {
		return sum, err
	}
	sum.Users = countFileRows(ctx, db, "users", "")
	sum.Skills = countFileRows(ctx, db, "shelf", "")
	sum.Lists = countFileRows(ctx, db, "lists", "")
	sum.Tags = countFileRows(ctx, db, "tags", "")
	sum.Environments = countFileRows(ctx, db, "environments", "")
	sum.Sessions = countFileRows(ctx, db, "sessions", "")
	sum.APITokens = countFileRows(ctx, db, "api_tokens", "revoked_at=0")
	sum.MCPConnections = countFileRows(ctx, db, "mcp_connections", "revoked_at=0")

	collect := func(table, query string, label func(a, b string) string) {
		if !tableExists(ctx, db, table) {
			return
		}
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var a, b string
			if rows.Scan(&a, &b) == nil && b != "" {
				sum.Secrets = append(sum.Secrets, SealedSecret{Where: label(a, b), Sealed: b})
			}
		}
	}
	collect("backup_remotes", `SELECT name,secret_sealed FROM backup_remotes WHERE secret_sealed<>''`,
		func(a, _ string) string { return fmt.Sprintf("the access key of backup destination %q", a) })
	collect("backup_settings", `SELECT CAST(owner_id AS TEXT),password_sealed FROM backup_settings WHERE password_sealed<>''`,
		func(a, _ string) string {
			if a == "0" {
				return "the instance copy password"
			}
			return "a person's copy password"
		})
	collect("mail_settings", `SELECT 'mail',password_sealed FROM mail_settings WHERE password_sealed<>''`,
		func(string, string) string { return "the outgoing mail password" })
	return sum, nil
}

// AdaptDatabaseFile makes a restored copy safe to run beside, or in place of,
// the controller it was taken from. Browser sign-ins and password-reset links
// always end. Personal API tokens and MCP connections are revoked unless the
// operator chose to keep them, because otherwise a credential from the old
// machine opens the new one. The private-connection identity is removed so
// the new controller mints its own: two controllers must never present one.
func AdaptDatabaseFile(ctx context.Context, path string, opts AdaptOptions) (Adaptation, error) {
	var out Adaptation
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
	if out.SessionsEnded, err = exec("sessions", `DELETE FROM sessions`); err != nil {
		return out, err
	}
	if _, err = exec("password_resets", `DELETE FROM password_resets`); err != nil {
		return out, err
	}
	if !opts.KeepTokens {
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
	n, err := exec("private_transport_identity", `DELETE FROM private_transport_identity`)
	if err != nil {
		return out, err
	}
	out.IdentityDropped = n > 0
	if err := tx.Commit(); err != nil {
		return out, err
	}
	// Fold the write-ahead log in so the single file is the whole copy.
	_, _ = db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return out, nil
}
