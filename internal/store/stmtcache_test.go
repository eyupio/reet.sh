package store

import "testing"

func TestStmtCacheReusesStatements(t *testing.T) {
	s := newStore(t)
	first, err := s.readStmts.get(`SELECT 1`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.readStmts.get(`SELECT 1`)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("same query prepared twice")
	}
	if _, err := s.readStmts.get(`SELECT * FROM no_such_table`); err == nil {
		t.Fatal("invalid query was cached")
	}
	var n int
	if err := s.readStmts.queryRow(`SELECT 1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cached query = %d, %v", n, err)
	}
}
