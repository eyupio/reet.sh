package store_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func TestEnrolCodeIsSpentOnceAndRecordsItsMachine(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	future := time.Now().Add(15 * time.Minute).UnixMilli()
	if err = st.CreateEnrolCode(uid, "live", future, 20); err != nil {
		t.Fatal(err)
	}
	// Two machines race for one code: exactly one is enrolled.
	var wg sync.WaitGroup
	results := make([]error, 2)
	agents := make([]store.SyncAgent, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var got int64
			got, agents[i], _, _, results[i] = st.EnrolWithCode("live", []string{"one", "two"}[i], "test", "")
			if results[i] == nil && got != uid {
				t.Errorf("enrolled for account %d, want %d", got, uid)
			}
		}(i)
	}
	wg.Wait()
	won := 0
	for i, err := range results {
		if err == nil {
			won++
			if id, _ := st.EnrolCodeAgent("live"); id != agents[i].ID {
				t.Fatalf("code recorded agent %q, enrolled %q", id, agents[i].ID)
			}
		} else if !errors.Is(err, store.ErrEnrolCodeInvalid) {
			t.Fatalf("losing enrolment: %v", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d enrolments with one code", won)
	}
	if list, _ := st.SyncAgentsFor(uid); len(list) != 1 {
		t.Fatalf("machines: %+v", list)
	}
	// Expired and unknown codes fail the same way.
	if err = st.CreateEnrolCode(uid, "old", time.Now().Add(-time.Second).UnixMilli(), 20); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"old", "unknown", "live"} {
		if _, _, _, _, err = st.EnrolWithCode(code, "three", "test", ""); !errors.Is(err, store.ErrEnrolCodeInvalid) {
			t.Fatalf("%s: %v", code, err)
		}
	}
}

// A code given for a machine already enrolled enrols it again in place, by
// code, with a new secret: the answer never says a name was taken, and the
// code is spent either way. A name that can never be valid leaves it unspent.
func TestEnrolCodeReplacesAMachineOfTheSameName(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	old, oldSecret, err := st.EnrollSyncAgent(uid, "Studio", "test")
	if err != nil {
		t.Fatal(err)
	}
	if old.EnrolledVia != store.EnrolledByToken || !old.LegacyToken {
		t.Fatalf("token enrolment: %+v", old)
	}
	if err = st.CreateEnrolCode(uid, "code", time.Now().Add(time.Minute).UnixMilli(), 20); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err = st.EnrolWithCode("code", "bad\nname", "test", ""); !errors.Is(err, store.ErrMachineName) {
		t.Fatalf("invalid name: %v", err)
	}
	gotUID, a, secret, replaced, err := st.EnrolWithCode("code", "studio", "test", "")
	if err != nil || gotUID != uid || !replaced || a.ID != old.ID || a.Host != "Studio" || secret == oldSecret {
		t.Fatalf("again by code: %+v %v %v", a, replaced, err)
	}
	if _, err = st.AuthenticateSyncAgent(uid, old.ID, oldSecret, "test"); err == nil {
		t.Fatal("the old secret still works")
	}
	if _, a, err := st.AuthenticateRemoteAgent(old.ID, secret, "test"); err != nil || a.EnrolledVia != store.EnrolledByCode {
		t.Fatalf("new secret: %+v %v", a, err)
	}
	if list, _ := st.SyncAgentsFor(uid); len(list) != 1 || list[0].LegacyToken || list[0].EnrolledVia != store.EnrolledByCode {
		t.Fatalf("machines: %+v", list)
	}
	if _, _, _, _, err = st.EnrolWithCode("code", "other", "test", ""); !errors.Is(err, store.ErrEnrolCodeInvalid) {
		t.Fatalf("code reused: %v", err)
	}
}

func TestEnrolCodesPerAccountAreCapped(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	uid, _ := st.DefaultOwner()
	future := time.Now().Add(time.Minute).UnixMilli()
	for i := range 3 {
		if err = st.CreateEnrolCode(uid, string(rune('a'+i)), future, 3); err != nil {
			t.Fatal(err)
		}
	}
	if err = st.CreateEnrolCode(uid, "d", future, 3); !errors.Is(err, store.ErrEnrolCodeLimit) {
		t.Fatalf("fourth code: %v", err)
	}
	// A spent code no longer counts.
	if _, _, _, _, err = st.EnrolWithCode("a", "m", "test", ""); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateEnrolCode(uid, "d", future, 3); err != nil {
		t.Fatal(err)
	}
}
