package store

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var ErrMCPGrant = errors.New("invalid, expired or already used authorization grant")

type MCPClient struct {
	ID           string   `json:"client_id"`
	Name         string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}
type MCPCode struct {
	Hash, ClientID, RedirectURI, Challenge, Resource, Scope string
	ConnectionID, ExpiresAt                                 int64
}

func (s *Store) migrateMCPOAuth() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS mcp_oauth_clients (id TEXT PRIMARY KEY,name TEXT NOT NULL,redirect_uris TEXT NOT NULL,created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS mcp_oauth_codes (hash TEXT PRIMARY KEY,client_id TEXT NOT NULL,connection_id INTEGER NOT NULL REFERENCES mcp_connections(id) ON DELETE CASCADE,redirect_uri TEXT NOT NULL,challenge TEXT NOT NULL,resource TEXT NOT NULL,scope TEXT NOT NULL,expires_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS mcp_oauth_access (hash TEXT PRIMARY KEY,connection_id INTEGER NOT NULL REFERENCES mcp_connections(id) ON DELETE CASCADE,scope TEXT NOT NULL,resource TEXT NOT NULL,expires_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS mcp_oauth_refresh (hash TEXT PRIMARY KEY,connection_id INTEGER NOT NULL REFERENCES mcp_connections(id) ON DELETE CASCADE,client_id TEXT NOT NULL,resource TEXT NOT NULL,scope TEXT NOT NULL,expires_at INTEGER NOT NULL,used_at INTEGER NOT NULL DEFAULT 0)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	ok, err := hasColumn(s.db, "mcp_oauth_access", "resource")
	if err != nil {
		return err
	}
	if !ok {
		_, err = s.db.Exec(`ALTER TABLE mcp_oauth_access ADD COLUMN resource TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

func (s *Store) SaveMCPClient(c MCPClient) error {
	b, err := json.Marshal(c.RedirectURIs)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO mcp_oauth_clients(id,name,redirect_uris,created_at) VALUES(?,?,?,?)`, c.ID, c.Name, string(b), now())
	return err
}

func (s *Store) MCPClient(id string) (MCPClient, error) {
	var c MCPClient
	var redirects string
	err := s.db.QueryRow(`SELECT id,name,redirect_uris FROM mcp_oauth_clients WHERE id=?`, id).Scan(&c.ID, &c.Name, &redirects)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(redirects), &c.RedirectURIs)
	return c, err
}

func (s *Store) SaveMCPCode(c MCPCode) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE mcp_connections SET kind='oauth' WHERE id=?`, c.ConnectionID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO mcp_oauth_codes(hash,client_id,connection_id,redirect_uri,challenge,resource,scope,expires_at) VALUES(?,?,?,?,?,?,?,?)`, c.Hash, c.ClientID, c.ConnectionID, c.RedirectURI, c.Challenge, c.Resource, c.Scope, c.ExpiresAt)
		return err
	})
}

func activeMCPGrant(tx *sql.Tx, id int64, at int64) (int64, error) {
	var expires int64
	err := tx.QueryRow(`SELECT c.expires_at FROM mcp_connections c JOIN users u ON u.id=c.user_id JOIN mcp_access a ON a.user_id=u.id WHERE c.id=? AND c.enabled=1 AND c.revoked_at=0 AND c.expires_at>? AND a.enabled=1 AND u.disabled_at=0`, id, at).Scan(&expires)
	if err != nil {
		return 0, ErrMCPGrant
	}
	return expires, nil
}

func insertMCPAccess(tx *sql.Tx, connectionID int64, scope, resource, accessHash string) error {
	for _, table := range []string{"mcp_oauth_codes", "mcp_oauth_access", "mcp_oauth_refresh"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE expires_at<?`, now()); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`INSERT INTO mcp_oauth_access(hash,connection_id,scope,resource,expires_at) VALUES(?,?,?,?,?)`, accessHash, connectionID, scope, resource, time.Now().Add(time.Hour).UnixMilli())
	return err
}

// ExchangeMCPCode binds one-use codes to the client, exact redirect, resource and PKCE verifier.
func (s *Store) ExchangeMCPCode(hash, clientID, redirect, resource, verifier, accessHash, refreshHash string) (string, error) {
	var scope string
	err := s.withTx(func(tx *sql.Tx) error {
		var c MCPCode
		err := tx.QueryRow(`SELECT connection_id,challenge,scope FROM mcp_oauth_codes WHERE hash=? AND client_id=? AND redirect_uri=? AND resource=? AND expires_at>?`, hash, clientID, redirect, resource, now()).Scan(&c.ConnectionID, &c.Challenge, &c.Scope)
		if err != nil {
			return ErrMCPGrant
		}
		sum := sha256.Sum256([]byte(verifier))
		challenge := base64.RawURLEncoding.EncodeToString(sum[:])
		if subtle.ConstantTimeCompare([]byte(challenge), []byte(c.Challenge)) != 1 {
			return ErrMCPGrant
		}
		expires, err := activeMCPGrant(tx, c.ConnectionID, now())
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM mcp_oauth_codes WHERE hash=?`, hash); err != nil {
			return err
		}
		if err = insertMCPAccess(tx, c.ConnectionID, c.Scope, resource, accessHash); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO mcp_oauth_refresh(hash,connection_id,client_id,resource,scope,expires_at) VALUES(?,?,?,?,?,?)`, refreshHash, c.ConnectionID, clientID, resource, c.Scope, expires)
		scope = c.Scope
		return err
	})
	return scope, err
}

func (s *Store) RefreshMCPGrant(hash, clientID, resource, requestedScope, accessHash, refreshHash string) (string, error) {
	var scope string
	replay := false
	err := s.withTx(func(tx *sql.Tx) error {
		var id, expires, used int64
		err := tx.QueryRow(`SELECT connection_id,scope,expires_at,used_at FROM mcp_oauth_refresh WHERE hash=? AND client_id=? AND resource=? AND expires_at>?`, hash, clientID, resource, now()).Scan(&id, &scope, &expires, &used)
		if err != nil {
			return ErrMCPGrant
		}
		if used != 0 {
			replay = true
			_, err = tx.Exec(`UPDATE mcp_connections SET revoked_at=? WHERE id=?`, now(), id)
			return err
		}
		if requestedScope == "write" && scope != "write" {
			return ErrMCPGrant
		}
		if requestedScope == "read" {
			scope = "read"
		}
		if _, err = activeMCPGrant(tx, id, now()); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE mcp_oauth_refresh SET used_at=? WHERE hash=?`, now(), hash); err != nil {
			return err
		}
		if err = insertMCPAccess(tx, id, scope, resource, accessHash); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO mcp_oauth_refresh(hash,connection_id,client_id,resource,scope,expires_at) VALUES(?,?,?,?,?,?)`, refreshHash, id, clientID, resource, scope, expires)
		return err
	})
	if replay {
		return "", ErrMCPGrant
	}
	return scope, err
}

func (s *Store) RevokeMCPOAuth(hash, clientID string) error {
	_, err := s.db.Exec(`UPDATE mcp_connections SET revoked_at=? WHERE id IN (SELECT connection_id FROM mcp_oauth_refresh WHERE hash=? AND client_id=?)`, now(), hash, clientID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`DELETE FROM mcp_oauth_access WHERE hash=? AND connection_id IN (SELECT connection_id FROM mcp_oauth_refresh WHERE client_id=?)`, hash, clientID)
	return err
}
