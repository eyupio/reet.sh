package backup

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

// controllerWithAMember is a controller whose owner nobody here can sign in
// as, with a member who should run it from now on.
func controllerWithAMember(t *testing.T) (svc *Service, st *store.Store, dir, id string) {
	t.Helper()
	svc, st, dir, _ = liveController(t)
	uid, err := st.CreateUser("sam@example.com", "Sam", "member", "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("gone@example.com", "Gone", "member", "x"); err != nil {
		t.Fatal(err)
	}
	gone, _, _ := st.UserByEmail("gone@example.com")
	if err := st.SetUserDisabled(gone.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSession("old-session", uid.ID, 1<<60, "test"); err != nil {
		t.Fatal(err)
	}
	item, err := svc.RunInstance(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return svc, st, dir, item.ID
}

// A copy from a controller whose owner belonged to whoever ran it before is
// no use to the person restoring it unless somebody can be the owner.
func TestARestoreCanMakeAnAccountInTheCopyItsOwnerWithANewPassword(t *testing.T) {
	svc, st, dir, id := controllerWithAMember(t)
	res, err := RestoreInstance(context.Background(), svc, dir, id, "",
		RestoreOptions{AllowLiveSource: true, MakeOwner: " SAM@example.com ", OwnerPassword: "a-new-long-password"}, st.Close)
	if err != nil {
		t.Fatal(err)
	}
	if res.Report.OwnerMade != "sam@example.com" || len(res.Report.OwnersDemoted) != 1 || res.Report.OwnersDemoted[0] != "owner@example.com" {
		t.Fatalf("report: %+v", res.Report)
	}
	if !strings.Contains(strings.Join(res.Report.Outcome(), "\n"), "Made sam@example.com the owner") {
		t.Fatalf("the outcome does not say who the owner is: %v", res.Report.Outcome())
	}
	if strings.Contains(res.Message, "a-new-long-password") {
		t.Fatal("the password was printed")
	}
	after, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	sam, ok, err := after.UserByEmail("sam@example.com")
	if err != nil || !ok || sam.Role != "owner" || !auth.VerifyPassword(sam.PasswordHash, "a-new-long-password") {
		t.Fatalf("sam: %+v %v", sam, err)
	}
	was, ok, err := after.UserByEmail("owner@example.com")
	if err != nil || !ok || was.Role != "admin" {
		t.Fatalf("the old owner: %+v %v", was, err)
	}
	// Whoever held the member's old sign-in must not hold the owner's.
	if _, ok, _ := after.SessionUser("old-session", 1); ok {
		t.Fatal("the new owner's earlier sign-in survived")
	}
}

func TestARestoreRefusesToMakeAnOwnerOfAnAccountTheCopyLacksOrHoldsDisabled(t *testing.T) {
	for _, address := range []string{"nobody@example.com", "gone@example.com"} {
		svc, st, dir, id := controllerWithAMember(t)
		before := liveBytes(t, dir)
		_, err := RestoreInstance(context.Background(), svc, dir, id, "",
			RestoreOptions{AllowLiveSource: true, MakeOwner: address, OwnerPassword: "a-new-long-password"}, st.Close)
		if err == nil || !strings.Contains(err.Error(), address) || !strings.Contains(err.Error(), "nothing was changed") {
			t.Fatalf("%s: want a refusal naming the address, got %v", address, err)
		}
		if !bytes.Equal(before, liveBytes(t, dir)) {
			t.Fatalf("%s: a refused restore changed reet.db", address)
		}
		noAsideCopy(t, dir)
		st.Close()
	}
}

func TestARestoreRefusesAShortOwnerPasswordAndAnOwnerCheckBesideIt(t *testing.T) {
	for name, opts := range map[string]RestoreOptions{
		"short password":        {MakeOwner: "sam@example.com", OwnerPassword: "short"},
		"no password":           {MakeOwner: "sam@example.com"},
		"a check beside it":     {MakeOwner: "sam@example.com", OwnerPassword: "a-new-long-password", OwnerEmail: "owner@example.com"},
		"a password for no-one": {OwnerPassword: "a-new-long-password"},
	} {
		svc, st, dir, id := controllerWithAMember(t)
		before := liveBytes(t, dir)
		opts.AllowLiveSource = true
		if _, err := RestoreInstance(context.Background(), svc, dir, id, "", opts, st.Close); err == nil {
			t.Fatalf("%s was accepted", name)
		}
		if !bytes.Equal(before, liveBytes(t, dir)) {
			t.Fatalf("%s: a refused restore changed reet.db", name)
		}
		st.Close()
	}
}

// Looking first must show who would become the owner and change nothing.
func TestACheckOnlyRestoreSaysWhoWouldBecomeTheOwner(t *testing.T) {
	svc, st, dir, id := controllerWithAMember(t)
	defer st.Close()
	before := liveBytes(t, dir)
	res, err := RestoreInstance(context.Background(), svc, dir, id, "",
		RestoreOptions{AllowLiveSource: true, CheckOnly: true, MakeOwner: "sam@example.com", OwnerPassword: "a-new-long-password"}, st.Close)
	if err != nil || res.Report.OwnerMade != "sam@example.com" {
		t.Fatalf("%+v %v", res.Report, err)
	}
	if !bytes.Equal(before, liveBytes(t, dir)) {
		t.Fatal("a check changed reet.db")
	}
}
