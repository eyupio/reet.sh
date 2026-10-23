package privateconn

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

type memIdentityStore struct {
	mu         sync.Mutex
	version    int
	ciphertext string
	found      bool
	saves      int
}

func (m *memIdentityStore) load() (int, string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version, m.ciphertext, m.found, nil
}

func (m *memIdentityStore) save(version int, ciphertext string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.found {
		if m.version != version || m.ciphertext != ciphertext {
			return errors.New("identity already exists")
		}
		return nil
	}
	m.version, m.ciphertext, m.found = version, ciphertext, true
	m.saves++
	return nil
}

func (m *memIdentityStore) snapshot() (int, string, bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version, m.ciphertext, m.found, m.saves
}

func (m *memIdentityStore) set(version int, ciphertext string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.version, m.ciphertext, m.found = version, ciphertext, true
}

func runtimeFixture() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsPrivateTransport(r.Context()) {
			http.Error(w, "request did not arrive through the private listener", http.StatusInternalServerError)
			return
		}
		if r.Header.Get("Authorization") != fixtureCredential {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, "ready")
	})
}

func testRuntime(t *testing.T, store *memIdentityStore, sealer *sealed.Sealer, discover func(context.Context, string) (*Identity, error)) *Runtime {
	t.Helper()
	r := NewRuntime(RuntimeConfig{
		Sealer: sealer, LoadIdentity: store.load, SaveIdentity: store.save,
		Handler: runtimeFixture(), DiscoverIdentity: discover,
		SetupTimeout: 15 * time.Second, RetryMin: 10 * time.Millisecond, RetryMax: 40 * time.Millisecond,
	})
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before the deadline")
}

func TestRuntimeLazilyStartsAndRestoresIdentity(t *testing.T) {
	store := &memIdentityStore{}
	sealer := testSealer(t, "a")
	discovered := 0
	first := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		discovered++
		return testIdentity(t), nil
	})
	address, err := first.Ensure(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if discovered != 1 {
		t.Fatalf("discovery calls = %d", discovered)
	}
	if _, ciphertext, found, saves := store.snapshot(); !found || ciphertext == "" || saves != 1 {
		t.Fatal("new identity was not persisted once")
	}
	client, err := NewClient(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if got := request(t, client, "/agent/ping", fixtureCredential).StatusCode; got != 200 {
		t.Fatalf("initial private request status = %d", got)
	}
	if strings.Contains(first.String()+first.GoString(), address) {
		t.Fatal("runtime formatting exposed the capability")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	secondDiscoveries := 0
	second := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		secondDiscoveries++
		return nil, ErrUnavailable
	})
	restored, err := second.Ensure(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if restored != address || secondDiscoveries != 0 {
		t.Fatalf("restored address changed=%t discoveries=%d", restored != address, secondDiscoveries)
	}
	reconnected, err := NewClient(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Close()
	if got := request(t, reconnected, "/agent/ping", fixtureCredential).StatusCode; got != 200 {
		t.Fatalf("restored private request status = %d", got)
	}
	if _, _, _, saves := store.snapshot(); saves != 1 {
		t.Fatal("restoration wrote another identity")
	}
}

func TestRuntimeFailsClosedWithoutReplacingIdentity(t *testing.T) {
	store := &memIdentityStore{}
	sealer := testSealer(t, "a")
	first := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		return testIdentity(t), nil
	})
	if _, err := first.Ensure(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	_, ciphertext, _, saves := store.snapshot()

	wrongDiscoveries := 0
	wrong := testRuntime(t, store, testSealer(t, "b"), func(context.Context, string) (*Identity, error) {
		wrongDiscoveries++
		return testIdentity(t), nil
	})
	if _, err := wrong.Ensure(t.Context()); !errors.Is(err, ErrIdentity) {
		t.Fatalf("wrong key: %v", err)
	}
	if wrongDiscoveries != 0 {
		t.Fatal("wrong key triggered identity replacement")
	}
	if _, stored, _, after := store.snapshot(); stored != ciphertext || after != saves {
		t.Fatal("wrong-key failure changed the persisted identity")
	}
	if err := wrong.Close(); err != nil {
		t.Fatal(err)
	}

	damaged := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		t.Fatal("damaged identity triggered discovery")
		return nil, ErrIdentity
	})
	store.set(1, "damaged-ciphertext")
	if _, err := damaged.Ensure(t.Context()); !errors.Is(err, ErrIdentity) {
		t.Fatalf("damaged identity: %v", err)
	}
	if _, stored, _, _ := store.snapshot(); stored != "damaged-ciphertext" {
		t.Fatal("damaged identity was replaced")
	}
}

func TestRuntimeRestartsListenerWithSameIdentity(t *testing.T) {
	store := &memIdentityStore{}
	sealer := testSealer(t, "a")
	runtime := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		return testIdentity(t), nil
	})
	address, err := runtime.Ensure(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	runtime.mu.Lock()
	old := runtime.listener
	runtime.mu.Unlock()
	if old == nil {
		t.Fatal("missing listener")
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return runtime.listener != nil && runtime.listener != old
	})
	restored, err := runtime.Ensure(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if restored != address {
		t.Fatal("listener restart changed the capability")
	}
	client, err := NewClient(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if got := request(t, client, "/agent/ping", fixtureCredential).StatusCode; got != 200 {
		t.Fatalf("request after listener restart = %d", got)
	}
	if _, _, _, saves := store.snapshot(); saves != 1 {
		t.Fatal("listener restart replaced the stored identity")
	}
}

func TestRuntimeBoundsRetries(t *testing.T) {
	store := &memIdentityStore{}
	sealer := testSealer(t, "a")
	var mu sync.Mutex
	calls := 0
	runtime := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls < 3 {
			return nil, ErrUnavailable
		}
		return testIdentity(t), nil
	})
	runtime.cfg.RetryMin, runtime.cfg.RetryMax = 200*time.Millisecond, 300*time.Millisecond
	if _, err := runtime.Ensure(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("initial failure: %v", err)
	}
	if _, err := runtime.Ensure(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("retry during backoff: %v", err)
	}
	mu.Lock()
	if calls != 1 {
		t.Fatalf("immediate retry ignored backoff: calls=%d", calls)
	}
	mu.Unlock()
	waitFor(t, 3*time.Second, func() bool {
		address, err := runtime.Ensure(t.Context())
		return err == nil && address != ""
	})
	mu.Lock()
	got := calls
	mu.Unlock()
	if got != 3 {
		t.Fatalf("discovery calls = %d, want 3", got)
	}
}

func TestRuntimeCloseStopsListenerAndRejectsFurtherStarts(t *testing.T) {
	store := &memIdentityStore{}
	sealer := testSealer(t, "a")
	runtime := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		return testIdentity(t), nil
	})
	address, err := runtime.Ensure(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(address)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if got := request(t, client, "/agent/ping", fixtureCredential).StatusCode; got != 200 {
		t.Fatalf("pre-close request = %d", got)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Ensure(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ensure after close: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, Origin+"/agent/ping", nil)
	req.Header.Set("Authorization", fixtureCredential)
	if resp, err := client.HTTPClient().Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("private request succeeded after close")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestRuntimeConcurrentEnsureStartsOneListener(t *testing.T) {
	store := &memIdentityStore{}
	sealer := testSealer(t, "a")
	var discoveries int
	runtime := testRuntime(t, store, sealer, func(context.Context, string) (*Identity, error) {
		discoveries++
		return testIdentity(t), nil
	})
	const callers = 8
	results := make(chan string, callers)
	errs := make(chan error, callers)
	for range callers {
		go func() {
			address, err := runtime.Ensure(t.Context())
			results <- address
			errs <- err
		}()
	}
	var address string
	for range callers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		got := <-results
		if address == "" {
			address = got
		} else if got != address {
			t.Fatal("concurrent Ensure returned different capabilities")
		}
	}
	if discoveries != 1 {
		t.Fatalf("discovery calls = %d", discoveries)
	}
	if _, _, _, saves := store.snapshot(); saves != 1 {
		t.Fatal("concurrent Ensure persisted more than one identity")
	}
}
