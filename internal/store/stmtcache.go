package store

import (
	"context"
	"database/sql"
	"sync"
)

// maxCachedStatements bounds the per-handle statement cache. Query shapes are
// mostly stable, but filters and IN-list sizes can vary; dropping the cache
// when it is full keeps memory bounded without an eviction policy.
const maxCachedStatements = 64

// stmtCache keeps prepared statements so repeated hot queries do not pay
// SQLite's parse and VDBE-preparation cost on every call.
type stmtCache struct {
	mu    sync.Mutex
	db    *sql.DB
	stmts map[string]*sql.Stmt
}

func (c *stmtCache) setDB(db *sql.DB) { c.db = db }

func (c *stmtCache) get(query string) (*sql.Stmt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stmts == nil {
		c.stmts = make(map[string]*sql.Stmt)
	}
	if stmt, ok := c.stmts[query]; ok {
		return stmt, nil
	}
	if len(c.stmts) >= maxCachedStatements {
		for _, stmt := range c.stmts {
			_ = stmt.Close()
		}
		c.stmts = make(map[string]*sql.Stmt)
	}
	stmt, err := c.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	c.stmts[query] = stmt
	return stmt, nil
}

func (c *stmtCache) query(query string, args ...any) (*sql.Rows, error) {
	stmt, err := c.get(query)
	if err != nil {
		return nil, err
	}
	return stmt.Query(args...)
}

func (c *stmtCache) queryRow(query string, args ...any) *sql.Row {
	stmt, err := c.get(query)
	if err != nil {
		return c.db.QueryRow(query, args...)
	}
	return stmt.QueryRow(args...)
}

func (c *stmtCache) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	stmt, err := c.get(query)
	if err != nil {
		return c.db.QueryRowContext(ctx, query, args...)
	}
	return stmt.QueryRowContext(ctx, args...)
}

func (c *stmtCache) exec(query string, args ...any) (sql.Result, error) {
	stmt, err := c.get(query)
	if err != nil {
		return nil, err
	}
	return stmt.Exec(args...)
}

func (c *stmtCache) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, stmt := range c.stmts {
		_ = stmt.Close()
	}
	c.stmts = nil
}
