package gaffer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

func settingsFixture(t *testing.T) (*Service, *store.Store, Principal) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser("gaffer@example.test", "Owner", "owner", "hash")
	if err != nil {
		t.Fatal(err)
	}
	seal, err := sealed.NewSealer(strings.Repeat("test-key-", 8), "test")
	if err != nil {
		t.Fatal(err)
	}
	service := New(s, seal)
	t.Cleanup(func() { service.Close(); s.Close() })
	return service, s, Principal{UserID: u.ID, CanUseLocal: true}
}

func TestChangingAProviderCancelsItsProbeAndRejectsTheLateReply(t *testing.T) {
	s, _, p := settingsFixture(t)
	started := make(chan struct{})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer fake.Close()
	row, err := s.Save(p, Input{Name: "Local", Kind: "openai_compatible", BaseURL: fake.URL, Model: "m", Enabled: true, LocalOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Models(context.Background(), p, inputFrom(row)); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	// Losing administrator access must still allow a person to pause their
	// existing local provider and cancel a request already in flight.
	p.CanUseLocal = false
	if err := s.SetEnabled(p, row.ID, row.Revision, false); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, store.ErrGafferConflict) {
			t.Fatalf("late reply: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("probe was not cancelled")
	}
}

func TestProviderProbeAndSealingBudgetsAreBounded(t *testing.T) {
	s, _, p := settingsFixture(t)
	for i := 0; i < 12; i++ {
		_, done, err := s.begin(context.Background(), p, "", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		done()
	}
	if _, _, err := s.begin(context.Background(), p, "", time.Second); !errors.Is(err, ErrBusy) {
		t.Fatalf("probe budget: %v", err)
	}
	s.sealSlots <- struct{}{}
	s.sealSlots <- struct{}{}
	key := "synthetic-key"
	if _, err := s.Save(p, Input{Name: "Not saved", Kind: "openai", Model: "m", APIKey: &key}); !errors.Is(err, ErrBusy) {
		t.Fatalf("sealing budget: %v", err)
	}
	<-s.sealSlots
	<-s.sealSlots
	s.Close()
	if _, err := s.Save(p, Input{Name: "Closed", Kind: "openai", Model: "m"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("save after shutdown: %v", err)
	}
}

func TestClosingGafferCancelsAndJoinsAnActiveProbe(t *testing.T) {
	s, _, p := settingsFixture(t)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer fake.Close()
	result := make(chan error, 1)
	go func() {
		_, err := s.Models(context.Background(), p, Input{Name: "Local", Kind: "openai_compatible", BaseURL: fake.URL, LocalOnly: true})
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	s.Close()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled probe succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close left a probe running")
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("close left the provider request running")
	}
	if _, _, err := s.begin(context.Background(), p, "", time.Second); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("probe admitted after close: %v", err)
	}
}

func TestProviderCredentialsAreSealedAndBoundToTheirOwnerAndAddress(t *testing.T) {
	s, db, p := settingsFixture(t)
	key := "synthetic-secret"
	in := Input{Name: "My provider", Kind: "openai", Model: "a-model", Enabled: true, APIKey: &key, MakeDefault: true}
	got, err := s.Save(p, in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasKey || strings.Contains(got.KeySealed, key) {
		t.Fatal("key was not sealed")
	}
	other, _ := db.CreateUser("other@example.test", "Other", "member", "hash")
	if _, err := s.Models(context.Background(), Principal{UserID: other.ID}, Input{ID: got.ID, Revision: got.Revision}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-account probe: %v", err)
	}
	changed := inputFrom(got)
	changed.BaseURL = "https://different.example.test/v1"
	if _, err := s.Models(context.Background(), p, changed); err == nil {
		t.Fatal("saved key could be sent to a new address")
	}
	changed.APIKey = nil
	updated, err := s.Save(p, changed)
	if err != nil {
		t.Fatal(err)
	}
	if updated.HasKey {
		t.Fatal("address change retained the previous address's key")
	}
	// A copied ciphertext also carries its original provider identity.
	copy := updated
	copy.ID = strings.Repeat("f", 32)
	copy.Revision = 0
	copy.Name = "Copied ciphertext"
	copy.KeySealed = got.KeySealed
	copy, err = db.SaveGafferProviderFor(p.UserID, copy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.credential(copy); err == nil {
		t.Fatal("copied ciphertext opened for another provider")
	}
}

func TestModelsAndConnectionTestsUseOnlyTheConfiguredProvider(t *testing.T) {
	s, _, p := settingsFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Error("missing synthetic key")
		}
		if r.Method == "GET" {
			fmt.Fprint(w, `{"data":[{"id":"z"},{"id":"a"},{"id":"a"}]}`)
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	key := "synthetic-key"
	in := Input{Name: "Local", Kind: "openai_compatible", BaseURL: server.URL, Model: "a", Enabled: true, LocalOnly: true, APIKey: &key}
	models, err := s.Models(context.Background(), p, in)
	if err != nil || strings.Join(models, ",") != "a,z" {
		t.Fatalf("models: %v %v", models, err)
	}
	got, err := s.Save(p, in)
	if err != nil {
		t.Fatal(err)
	}
	check, err := s.Test(context.Background(), p, got.ID, got.Revision)
	if err != nil || !check.OK || check.Model != "a" {
		t.Fatalf("check: %+v %v", check, err)
	}
	if _, err := s.Save(p, inputFrom(got)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Test(context.Background(), p, got.ID, got.Revision); !errors.Is(err, store.ErrGafferConflict) {
		t.Fatalf("stale test: %v", err)
	}
}

func TestProviderAddressPolicyRejectsUnapprovedAndMixedDestinations(t *testing.T) {
	for _, url := range []string{"file:///tmp/key", "https://user:password@host.test", "https://host.test/?key=value", "https://host.test/#fragment", "http://public.example.test"} {
		if _, err := normalize(Principal{UserID: 1}, Input{Name: "p", Kind: "openai_compatible", BaseURL: url, Model: "m"}, true); err == nil {
			t.Errorf("accepted %s", url)
		}
	}
	if _, err := normalize(Principal{UserID: 1}, Input{Name: "p", Kind: "openai_compatible", BaseURL: "http://localhost:11434/v1", Model: "m", LocalOnly: true}, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member local access: %v", err)
	}
	for _, tc := range []struct {
		local bool
		ips   []string
		ok    bool
	}{
		{false, []string{"1.1.1.1"}, true}, {false, []string{"1.1.1.1", "127.0.0.1"}, false},
		{true, []string{"127.0.0.1", "::1"}, true}, {true, []string{"10.0.0.2"}, true},
		{true, []string{"100.80.0.2"}, true}, {true, []string{"1.1.1.1"}, false},
		{true, []string{"169.254.169.254"}, false}, {true, []string{"100.100.100.200"}, false},
	} {
		ips := []netip.Addr{}
		for _, ip := range tc.ips {
			ips = append(ips, netip.MustParseAddr(ip))
		}
		if err := checkAddresses(ips, tc.local); (err == nil) != tc.ok {
			t.Errorf("%v local=%v: %v", tc.ips, tc.local, err)
		}
	}
}
