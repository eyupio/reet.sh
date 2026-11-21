package store

import (
	"errors"
	"testing"
	"time"
)

// A connection may approve Factory merges only once its owner allows it, and
// only its owner can.
func TestMCPConnectionMergeApprovalIsOffUntilItsOwnerAllowsIt(t *testing.T) {
	s := newStore(t)
	owner, err := s.CreateUser("owner-merge@example.test", "Owner", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateUser("other-merge@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetMCPEnabled(owner.ID, true); err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateMCPConnection(owner.ID, "Desk", "write", "merge-hash", time.Now().Add(time.Hour).UnixMilli())
	if err != nil || c.ApproveMerges {
		t.Fatalf("created: %+v %v", c, err)
	}
	// The list the page shows and the credential check a call makes agree.
	allowed := func() bool {
		t.Helper()
		list, err := s.MCPConnections(owner.ID)
		if err != nil || len(list) != 1 {
			t.Fatalf("list: %+v %v", list, err)
		}
		_, got, ok, err := s.MCPUser("merge-hash", time.Now().UnixMilli())
		if err != nil || !ok || got.ApproveMerges != list[0].ApproveMerges {
			t.Fatalf("credential: %+v %v %v, listed %v", got, ok, err, list[0].ApproveMerges)
		}
		return got.ApproveMerges
	}
	if allowed() {
		t.Fatal("a new connection may approve merges")
	}
	if err = s.SetMCPConnectionApproveMerges(other.ID, c.ID, true); !errors.Is(err, ErrNotFound) || allowed() {
		t.Fatalf("another account changed it: %v", err)
	}
	if err = s.SetMCPConnectionApproveMerges(owner.ID, c.ID, true); err != nil || !allowed() {
		t.Fatalf("allow: %v", err)
	}
	if err = s.SetMCPConnectionApproveMerges(other.ID, c.ID, false); !errors.Is(err, ErrNotFound) || !allowed() {
		t.Fatalf("another account changed it back: %v", err)
	}
	if err = s.SetMCPConnectionApproveMerges(owner.ID, c.ID, false); err != nil || allowed() {
		t.Fatalf("disallow: %v", err)
	}
	if err = s.RevokeMCPConnection(owner.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SetMCPConnectionApproveMerges(owner.ID, c.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a revoked connection was changed: %v", err)
	}
}

// An installation from before the permission gets it switched off for the
// connections it already has, and migrating again changes nothing.
func TestMCPMigrationAddsMergeApprovalSwitchedOff(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("old-merge@example.test", "Old", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`ALTER TABLE mcp_connections DROP COLUMN approve_merges`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO mcp_connections(user_id,name,hash,scope,created_at,expires_at) VALUES(?,?,?,?,?,?)`, u.ID, "Old client", "old-merge-hash", "write", 1, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = s.migrateMCP(); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.MCPConnections(u.ID)
	if err != nil || len(list) != 1 || list[0].ApproveMerges {
		t.Fatalf("migrated: %+v %v", list, err)
	}
}
