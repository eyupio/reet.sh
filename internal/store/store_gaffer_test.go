package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGafferProvidersAreOwnedVersionedAndHaveOneExplicitDefault(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a, _ := s.CreateUser("gaffer-a@example.test", "A", "member", "hash")
	b, _ := s.CreateUser("gaffer-b@example.test", "B", "member", "hash")
	p := GafferProvider{ID: strings.Repeat("a", 32), Name: "My model", Kind: "openai", BaseURL: "https://api.openai.com/v1", Model: "a-model", Enabled: true, Default: true, KeySealed: "ciphertext"}
	p, err = s.SaveGafferProviderFor(a.ID, p)
	if err != nil || p.Revision != 1 {
		t.Fatalf("create: %+v %v", p, err)
	}
	if _, err := s.GafferProviderFor(b.ID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account read: %v", err)
	}
	p.Name = "Changed"
	updated, err := s.SaveGafferProviderFor(a.ID, p)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := s.SaveGafferProviderFor(a.ID, p); !errors.Is(err, ErrGafferConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	if err := s.RecordGafferCheckFor(a.ID, p.ID, p.Revision, GafferCheck{At: 1, OK: true}); !errors.Is(err, ErrGafferConflict) {
		t.Fatalf("stale probe: %v", err)
	}
	q, err := s.SaveGafferProviderFor(a.ID, GafferProvider{ID: strings.Repeat("b", 32), Name: "Other", Kind: "anthropic", Model: "m", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveGafferProviderFor(a.ID, GafferProvider{ID: strings.Repeat("e", 32), Name: "Other", Kind: "openai", Model: "m", Enabled: true, Default: true}); !errors.Is(err, ErrGafferName) {
		t.Fatalf("duplicate name: %v", err)
	}
	if err := s.DefaultGafferProviderFor(a.ID, q.ID, q.Revision); err != nil {
		t.Fatal(err)
	}
	list, _ := s.GafferProvidersFor(a.ID)
	defaults := 0
	for _, row := range list {
		if row.Default {
			defaults++
			if row.ID != q.ID {
				t.Fatal("wrong default")
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults: %d", defaults)
	}
	if err := s.DeleteGafferProviderFor(b.ID, q.ID, q.Revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-account delete: %v", err)
	}
	if err := s.DeleteGafferProviderFor(a.ID, q.ID, q.Revision); err != nil {
		t.Fatal(err)
	}
	list, _ = s.GafferProvidersFor(a.ID)
	if len(list) != 1 || list[0].Default {
		t.Fatalf("silent default substitution: %+v", list)
	}
	if err := s.DeleteUser(a.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.GafferProvidersFor(a.ID)
	if len(list) != 0 {
		t.Fatal("account deletion retained provider credentials")
	}
}

func TestRestoreClearsAnUnreadableGafferKeyAndPausesItsProvider(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u, _ := s.CreateUser("restore-gaffer@example.test", "Person", "member", "hash")
	p, err := s.SaveGafferProviderFor(u.ID, GafferProvider{ID: strings.Repeat("d", 32), Name: "Saved", Kind: "openai", Model: "m", Enabled: true, Default: true, KeySealed: "unreadable"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	cleared, err := clearUnreadable(context.Background(), tx, func(value string) bool { return value == "unreadable" })
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := s.GafferProviderFor(u.ID, p.ID)
	if err != nil || got.Enabled || got.HasKey || got.Revision != 2 || len(cleared) != 1 {
		t.Fatalf("restored provider: %+v %v, messages %v", got, err, cleared)
	}
}

func TestGafferProviderCheckCannotResurrectADeletedProvider(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u, _ := s.CreateUser("gaffer@example.test", "A", "member", "hash")
	p, err := s.SaveGafferProviderFor(u.ID, GafferProvider{ID: strings.Repeat("c", 32), Name: "Local", Kind: "openai_compatible", Model: "m", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordGafferCheckFor(u.ID, p.ID, p.Revision, GafferCheck{At: 5, OK: true, Model: "m"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GafferProviderFor(u.ID, p.ID)
	if got.LastCheck == nil || !got.LastCheck.OK {
		t.Fatal("missing test result")
	}
	if err := s.DeleteGafferProviderFor(u.ID, p.ID, p.Revision); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordGafferCheckFor(u.ID, p.ID, p.Revision, GafferCheck{At: 6, OK: true}); !errors.Is(err, ErrGafferConflict) {
		t.Fatalf("late result: %v", err)
	}
}
