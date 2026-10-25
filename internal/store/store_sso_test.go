package store

import (
	"errors"
	"testing"
	"time"
)

const idp = "https://id.example.com"

func ssoUser(t *testing.T, s *Store, email, role string) User {
	t.Helper()
	u, err := s.CreateUser(email, "", role, "")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAnIdentityBelongsToOnePersonAndAPersonHasOnePerProvider(t *testing.T) {
	s := newStore(t)
	ada, bob := ssoUser(t, s, "ada@example.com", "member"), ssoUser(t, s, "bob@example.com", "member")
	if err := s.LinkSSOIdentity(idp, "sub-ada", ada.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkSSOIdentity(idp, "sub-ada", ada.ID); err != nil {
		t.Fatalf("linking the same pair twice should be a no-op: %v", err)
	}
	if err := s.LinkSSOIdentity(idp, "sub-ada", bob.ID); !errors.Is(err, ErrSSOLinkedElsewhere) {
		t.Fatalf("a subject taken by someone else: %v", err)
	}
	if err := s.LinkSSOIdentity(idp, "sub-other", ada.ID); !errors.Is(err, ErrSSOAlreadyLinked) {
		t.Fatalf("a second subject for one person at one provider: %v", err)
	}
	if err := s.LinkSSOIdentity("https://other.example.com", "sub-other", ada.ID); err != nil {
		t.Fatalf("another provider is a separate link: %v", err)
	}
	u, ok, err := s.SSOIdentityUser(idp, "sub-ada")
	if err != nil || !ok || u.ID != ada.ID {
		t.Fatalf("lookup = %v %v %v", u, ok, err)
	}
	if _, ok, _ := s.SSOIdentityUser(idp, "nobody"); ok {
		t.Fatal("an unknown subject found someone")
	}
	if err := s.UnlinkSSOIdentity(ada.ID, idp); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.SSOIdentityUser(idp, "sub-ada"); ok {
		t.Fatal("unlinked, still found")
	}
	if err := s.UnlinkSSOIdentity(ada.ID, idp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlinking nothing: %v", err)
	}
}

func TestDeletingAPersonDeletesTheirIdentities(t *testing.T) {
	s := newStore(t)
	ada := ssoUser(t, s, "ada@example.com", "member")
	if err := s.LinkSSOIdentity(idp, "sub-ada", ada.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ada.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.SSOIdentityUser(idp, "sub-ada"); ok {
		t.Fatal("the identity outlived the account")
	}
}

func TestACreatedPersonAndTheirIdentityAppearTogetherOrNotAtAll(t *testing.T) {
	s := newStore(t)
	ssoUser(t, s, "taken@example.com", "member")
	if _, err := s.CreateSSOUser(idp, "sub-1", "taken@example.com", "Taken", "member"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate email: %v", err)
	}
	if _, ok, _ := s.SSOIdentityUser(idp, "sub-1"); ok {
		t.Fatal("an identity was left behind by a refused account")
	}
	u, err := s.CreateSSOUser(idp, "sub-2", "new@example.com", "New", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if u.PasswordHash != "" || u.Role != "viewer" {
		t.Fatalf("created %+v: a single sign-on account has no password and the role it was given", u)
	}
	if got, ok, _ := s.SSOIdentityUser(idp, "sub-2"); !ok || got.ID != u.ID {
		t.Fatal("the new account is not linked")
	}
	if _, err := s.CreateSSOUser(idp, "sub-3", "owner@example.com", "Owner", "owner"); err == nil {
		t.Fatal("an account made by single sign-on can never be the owner")
	}
}

func TestALoginAttemptCanBeSpentOnce(t *testing.T) {
	s := newStore(t)
	at := time.Now().UnixMilli()
	l := SSOLogin{StateHash: "s1", BindingHash: "b1", Nonce: "n", Verifier: "v", Next: "/lists", Intent: "signin", ExpiresAt: at + 60_000}
	if err := s.CreateSSOLogin(l, at); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.ConsumeSSOLogin("s1", at)
	if err != nil || !ok || got.Nonce != "n" || got.Verifier != "v" || got.Next != "/lists" || got.BindingHash != "b1" {
		t.Fatalf("consume = %+v %v %v", got, ok, err)
	}
	if _, ok, _ := s.ConsumeSSOLogin("s1", at); ok {
		t.Fatal("the same state worked twice")
	}
	if _, ok, _ := s.ConsumeSSOLogin("never", at); ok {
		t.Fatal("an unknown state worked")
	}
	if err := s.CreateSSOLogin(SSOLogin{StateHash: "s2", BindingHash: "b", Nonce: "n", Verifier: "v", ExpiresAt: at + 1000}, at); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ConsumeSSOLogin("s2", at+1000); ok {
		t.Fatal("an expired attempt was accepted")
	}
}

func TestLoginAttemptsAreCappedAndTheExpiredOnesFreeRoom(t *testing.T) {
	s := newStore(t)
	at := time.Now().UnixMilli()
	for i := 0; i < SSOLoginCap; i++ {
		l := SSOLogin{StateHash: "s" + string(rune('a'+i%26)) + itoa(i), BindingHash: "b", Nonce: "n", Verifier: "v", ExpiresAt: at + 60_000}
		if err := s.CreateSSOLogin(l, at); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if err := s.CreateSSOLogin(SSOLogin{StateHash: "over", BindingHash: "b", Nonce: "n", Verifier: "v", ExpiresAt: at + 60_000}, at); !errors.Is(err, ErrSSOBusy) {
		t.Fatalf("over the cap: %v", err)
	}
	if err := s.CreateSSOLogin(SSOLogin{StateHash: "later", BindingHash: "b", Nonce: "n", Verifier: "v", ExpiresAt: at + 200_000}, at+120_000); err != nil {
		t.Fatalf("after the first batch expired: %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

func TestLogoutEndsTheSessionsItNamesAndNoOthers(t *testing.T) {
	s := newStore(t)
	ada, bob := ssoUser(t, s, "ada@example.com", "member"), ssoUser(t, s, "bob@example.com", "member")
	exp := time.Now().Add(time.Hour).UnixMilli()
	mk := func(id string, u User, iss, sid, sub string) {
		t.Helper()
		if err := s.CreateSSOSession(id, u.ID, exp, "ua", iss, sid, sub); err != nil {
			t.Fatal(err)
		}
	}
	mk("a1", ada, idp, "sid-1", "sub-ada")
	mk("a2", ada, idp, "sid-2", "sub-ada")
	mk("a3", ada, "https://other.example.com", "sid-1", "sub-ada")
	mk("b1", bob, idp, "sid-1", "sub-bob")
	if err := s.CreateSession("plain", ada.ID, exp, "ua"); err != nil {
		t.Fatal(err)
	}
	live := func(id string) bool { _, ok, _ := s.SessionUser(id, time.Now().UnixMilli()); return ok }

	// One provider session of one person.
	ids, err := s.DeleteSSOSessions(idp, "sub-ada", "sid-1")
	if err != nil || len(ids) != 1 || ids[0] != "a1" {
		t.Fatalf("by sid and subject = %v %v", ids, err)
	}
	if live("a1") || !live("a2") || !live("a3") || !live("b1") || !live("plain") {
		t.Fatal("a logout for one provider session ended others")
	}
	// A session id alone names that provider session, whoever has it.
	ids, _ = s.DeleteSSOSessions(idp, "", "sid-1")
	if len(ids) != 1 || ids[0] != "b1" {
		t.Fatalf("by sid alone = %v", ids)
	}
	// A subject alone names all of a person's sessions at that provider only.
	ids, _ = s.DeleteSSOSessions(idp, "sub-ada", "")
	if len(ids) != 1 || ids[0] != "a2" || !live("a3") || !live("plain") {
		t.Fatalf("by subject alone = %v", ids)
	}
	if ids, _ := s.DeleteSSOSessions(idp, "", ""); len(ids) != 0 {
		t.Fatal("a logout naming nobody ended sessions")
	}
}

func TestAPersonsSessionIdsAreListedSoTheirTerminalsCanBeClosed(t *testing.T) {
	s := newStore(t)
	ada := ssoUser(t, s, "ada@example.com", "member")
	exp := time.Now().Add(time.Hour).UnixMilli()
	_ = s.CreateSession("one", ada.ID, exp, "ua")
	_ = s.CreateSession("two", ada.ID, exp, "ua")
	ids, err := s.UserSessionIDs(ada.ID)
	if err != nil || len(ids) != 2 {
		t.Fatalf("ids = %v %v", ids, err)
	}
}
