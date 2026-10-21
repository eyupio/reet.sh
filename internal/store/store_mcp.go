package store

import (
	"database/sql"
	"errors"
	"strings"
)

// MCPConnection stores only a credential hash; its secret is returned once by the API.
type MCPConnection struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Scope      string `json:"scope"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"createdAt"`
	ExpiresAt  int64  `json:"expiresAt"`
	LastUsedAt int64  `json:"lastUsedAt"`
	Kind       string `json:"kind"`
}

func (s *Store) migrateMCP() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS mcp_access (user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE, enabled INTEGER NOT NULL DEFAULT 0, browser_enabled INTEGER NOT NULL DEFAULT 0, browser_scope TEXT NOT NULL DEFAULT 'read')`,
		`CREATE TABLE IF NOT EXISTS mcp_connections (
		 id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		 name TEXT NOT NULL, hash TEXT NOT NULL UNIQUE, scope TEXT NOT NULL CHECK(scope IN ('read','write')), kind TEXT NOT NULL DEFAULT 'bearer',
		 enabled INTEGER NOT NULL DEFAULT 1, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
		 last_used_at INTEGER NOT NULL DEFAULT 0, revoked_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_connections_user ON mcp_connections(user_id)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	// Add columns for installations that first enabled the bearer-token bridge.
	for _, c := range []struct{ table, ddl string }{{"mcp_access", "browser_enabled INTEGER NOT NULL DEFAULT 0"}, {"mcp_access", "browser_scope TEXT NOT NULL DEFAULT 'read'"}, {"mcp_connections", "kind TEXT NOT NULL DEFAULT 'bearer'"}} {
		ok, err := hasColumn(s.db, c.table, strings.Fields(c.ddl)[0])
		if err != nil {
			return err
		}
		if !ok {
			if _, err := s.db.Exec(`ALTER TABLE ` + c.table + ` ADD COLUMN ` + c.ddl); err != nil {
				return err
			}
		}
	}
	return s.migrateMCPOAuth()
}

func (s *Store) MCPEnabled(userID int64) (bool, error) {
	var enabled bool
	err := s.db.QueryRow(`SELECT enabled FROM mcp_access WHERE user_id=?`, userID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return enabled, err
}

func (s *Store) SetMCPEnabled(userID int64, enabled bool) error {
	_, err := s.db.Exec(`INSERT INTO mcp_access(user_id,enabled) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET enabled=excluded.enabled`, userID, enabled)
	return err
}

func validMCP(name, scope string, expiresAt int64) error {
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return errors.New("connection name must be 1–100 characters")
	}
	if scope != "read" && scope != "write" {
		return errors.New("scope must be read or write")
	}
	if expiresAt <= now() || expiresAt > now()+366*24*60*60*1000 {
		return errors.New("expiry must be within the next 365 days")
	}
	return nil
}

func (s *Store) CreateMCPConnection(userID int64, name, scope, hash string, expiresAt int64) (MCPConnection, error) {
	name = strings.TrimSpace(name)
	if err := validMCP(name, scope, expiresAt); err != nil {
		return MCPConnection{}, err
	}
	c := MCPConnection{Name: name, Scope: scope, Kind: "bearer", Enabled: true, CreatedAt: now(), ExpiresAt: expiresAt}
	res, err := s.db.Exec(`INSERT INTO mcp_connections(user_id,name,hash,scope,created_at,expires_at) VALUES(?,?,?,?,?,?)`, userID, name, hash, scope, c.CreatedAt, expiresAt)
	if err != nil {
		return c, err
	}
	c.ID, err = res.LastInsertId()
	return c, err
}

func (s *Store) MCPConnections(userID int64) ([]MCPConnection, error) {
	rows, err := s.db.Query(`SELECT id,name,scope,enabled,created_at,expires_at,last_used_at,kind FROM mcp_connections WHERE user_id=? AND revoked_at=0 ORDER BY created_at DESC,id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MCPConnection{}
	for rows.Next() {
		var c MCPConnection
		if err := rows.Scan(&c.ID, &c.Name, &c.Scope, &c.Enabled, &c.CreatedAt, &c.ExpiresAt, &c.LastUsedAt, &c.Kind); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateMCPConnection(userID, id int64, name, scope string, enabled bool, expiresAt int64) error {
	// Pausing an expired credential is still allowed; callers only extend expiry explicitly.
	if strings.TrimSpace(name) == "" || len(name) > 100 || (scope != "read" && scope != "write") {
		return errors.New("invalid connection name or scope")
	}
	res, err := s.db.Exec(`UPDATE mcp_connections SET name=?,scope=?,enabled=?,expires_at=? WHERE id=? AND user_id=? AND revoked_at=0`, strings.TrimSpace(name), scope, enabled, expiresAt, id, userID)
	return mcpChanged(res, err)
}

func (s *Store) RotateMCPConnection(userID, id int64, hash string) error {
	res, err := s.db.Exec(`UPDATE mcp_connections SET hash=? WHERE id=? AND user_id=? AND revoked_at=0`, hash, id, userID)
	return mcpChanged(res, err)
}

func (s *Store) RevokeMCPConnection(userID, id int64) error {
	res, err := s.db.Exec(`UPDATE mcp_connections SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at=0`, now(), id, userID)
	return mcpChanged(res, err)
}

func (s *Store) RevokeMCPConnections(userID int64) error {
	_, err := s.db.Exec(`UPDATE mcp_connections SET revoked_at=? WHERE user_id=? AND revoked_at=0`, now(), userID)
	return err
}

func mcpChanged(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// MCPUser checks current account, master switch, connection state and expiry on every request.
func (s *Store) MCPUser(hash string, at int64) (User, MCPConnection, bool, error) {
	return s.MCPUserFor(hash, at, "")
}

func (s *Store) MCPUserFor(hash string, at int64, resource string) (User, MCPConnection, bool, error) {
	var u User
	var c MCPConnection
	err := s.db.QueryRow(`SELECT u.id,u.email,u.name,u.role,u.disabled_at,c.id,c.name,
	 CASE WHEN c.kind='oauth' AND (SELECT scope FROM mcp_oauth_access WHERE hash=? AND connection_id=c.id)='read' THEN 'read' ELSE c.scope END,
	 c.enabled,c.created_at,c.expires_at,c.last_used_at,c.kind
	 FROM mcp_connections c JOIN users u ON u.id=c.user_id JOIN mcp_access a ON a.user_id=u.id
	 WHERE ((c.kind='bearer' AND c.hash=?) OR EXISTS (SELECT 1 FROM mcp_oauth_access t WHERE t.connection_id=c.id AND t.hash=? AND t.expires_at>? AND (?='' OR t.resource=?)))
	 AND c.revoked_at=0 AND c.enabled=1 AND c.expires_at>? AND a.enabled=1 AND u.disabled_at=0`, hash, hash, hash, at, resource, resource, at).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.DisabledAt, &c.ID, &c.Name, &c.Scope, &c.Enabled, &c.CreatedAt, &c.ExpiresAt, &c.LastUsedAt, &c.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return u, c, false, nil
	}
	if err != nil {
		return u, c, false, err
	}
	_, err = s.db.Exec(`UPDATE mcp_connections SET last_used_at=? WHERE id=?`, at, c.ID)
	return u, c, err == nil, err
}

func (s *Store) MCPBrowser(userID int64) (bool, string, error) {
	var enabled bool
	var scope string
	err := s.db.QueryRow(`SELECT browser_enabled,browser_scope FROM mcp_access WHERE user_id=?`, userID).Scan(&enabled, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "read", nil
	}
	return enabled, scope, err
}

func (s *Store) SetMCPBrowser(userID int64, enabled bool, scope string) error {
	if scope != "read" && scope != "write" {
		return errors.New("scope must be read or write")
	}
	_, err := s.db.Exec(`INSERT INTO mcp_access(user_id,browser_enabled,browser_scope) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET browser_enabled=excluded.browser_enabled,browser_scope=excluded.browser_scope`, userID, enabled, scope)
	return err
}
