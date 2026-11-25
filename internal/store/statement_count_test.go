package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// statementCounter remembers the text of every statement prepared on a store
// whose handle countStatements swapped, to show that some work is done once.
type statementCounter struct {
	mu    sync.Mutex
	texts []string
}

// count is how many statements containing part were prepared.
func (c *statementCounter) count(part string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, q := range c.texts {
		if strings.Contains(q, part) {
			n++
		}
	}
	return n
}

func (c *statementCounter) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.texts = nil
}

type countingConnector struct {
	dsn     string
	inner   driver.Driver
	counter *statementCounter
}

func (c countingConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.inner.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return countingConn{conn, c.counter}, nil
}

func (c countingConnector) Driver() driver.Driver { return c.inner }

// countingConn has only Prepare, Close and Begin, so database/sql prepares
// every statement through it, where it is counted.
type countingConn struct {
	driver.Conn
	counter *statementCounter
}

func (c countingConn) Prepare(query string) (driver.Stmt, error) {
	c.counter.mu.Lock()
	c.counter.texts = append(c.counter.texts, query)
	c.counter.mu.Unlock()
	return c.Conn.Prepare(query)
}

// countStatements makes s's write handle count the statements it prepares,
// from now on.
func countStatements(t *testing.T, s *Store) *statementCounter {
	t.Helper()
	probe, err := sql.Open("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	inner := probe.Driver()
	probe.Close()
	counter := &statementCounter{}
	db := sql.OpenDB(countingConnector{
		dsn:     filepath.Join(s.dir, "reet.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		inner:   inner,
		counter: counter,
	})
	db.SetMaxOpenConns(1)
	was := s.db
	s.db = db
	t.Cleanup(func() { s.db = was; db.Close() })
	return counter
}
