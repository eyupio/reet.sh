package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// toldOld records what machines said to the controller they were leaving. The
// addresses in these tests do not exist, so nothing is sent for real; each
// test uses its own old address and reads only what was said to it.
var toldOld struct {
	sync.Mutex
	said map[string][]map[string]string
}

func init() {
	tellOldFn = func(_ context.Context, c agentConfig, body map[string]string, _ string) error {
		toldOld.Lock()
		defer toldOld.Unlock()
		if toldOld.said == nil {
			toldOld.said = map[string][]map[string]string{}
		}
		toldOld.said[c.Server] = append(toldOld.said[c.Server], body)
		return nil
	}
}

func saidTo(server string) []map[string]string {
	toldOld.Lock()
	defer toldOld.Unlock()
	return append([]map[string]string(nil), toldOld.said[server]...)
}

// The old controller can be switched off before the new one answers, or the
// agent restarted in between. The address the fence named is kept, so the
// machine still gets there with nobody visiting it.
func TestAMachineFinishesTheMoveAfterTheOldControllerHasGone(t *testing.T) {
	const old = "https://old-gone.example.com"
	config := enrolledMachine(t, old)
	next := newController(t, func(_ *controller, w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	f := &moveFollower{}
	var out bytes.Buffer
	if f.follow(context.Background(), movedRefusal(next.URL), loaded(t, config), config, &out, "test") {
		t.Fatal("followed a controller that is not up yet")
	}
	if loaded(t, config).MovingTo != next.URL {
		t.Fatal("the address the fence named was not kept")
	}

	// The agent restarts and the old controller no longer answers at all;
	// the new one is up by now.
	ready := honest(t)
	c := loaded(t, config)
	c.MovingTo = ready.URL
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	f = &moveFollower{pending: loaded(t, config).MovingTo}
	if !f.follow(context.Background(), errors.New("dial tcp: connection refused"), loaded(t, config), config, &out, "test") {
		t.Fatalf("the kept address was not followed: %s", out.String())
	}
	now := loaded(t, config)
	if now.Server != ready.URL || now.MovingTo != "" {
		t.Fatalf("saved %+v", now)
	}
	if got := saidTo(old); len(got) != 1 || got[0]["to"] != ready.URL {
		t.Fatalf("the old controller was told %v", got)
	}
}

// The kept address is still only a hint: a place that cannot prove the record
// gets no credential, however the old controller went away.
func TestAKeptAddressIsProvedBeforeAnyCredentialIsSent(t *testing.T) {
	config := enrolledMachine(t, "https://old-kept.example.com")
	stranger := newController(t, func(_ *controller, w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"code": "unknown_machine"})
	})
	f := &moveFollower{pending: stranger.URL}
	var out bytes.Buffer
	if f.follow(context.Background(), errors.New("connection refused"), loaded(t, config), config, &out, "test") {
		t.Fatal("followed a place with no record")
	}
	noCredentialEverSent(t, stranger)
	if loaded(t, config).Server != "https://old-kept.example.com" {
		t.Fatal("the address changed")
	}
}

// A fence lifted means the move is off: the machine forgets where it was
// going once its controller takes a whole cycle again.
func TestALiftedFenceForgetsTheKeptAddress(t *testing.T) {
	config := enrolledMachine(t, "https://old-lifted.example.com")
	c := loaded(t, config)
	c.MovingTo = "https://never.example.com"
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	f := &moveFollower{pending: c.MovingTo}
	var out bytes.Buffer
	f.settled(config, c, &out)
	if loaded(t, config).MovingTo != "" || f.pending != "" || !strings.Contains(out.String(), "the move it announced is off") {
		t.Fatalf("kept %q: %s", loaded(t, config).MovingTo, out.String())
	}
	// Without a pending move a good cycle writes nothing.
	before := fileBytes(t, config)
	(&moveFollower{}).settled(config, loaded(t, config), &out)
	if !bytes.Equal(before, fileBytes(t, config)) {
		t.Fatal("a good cycle rewrote the enrollment")
	}
}

// A machine that will not follow on its own says so to the fenced controller,
// once, so its owner sees which machines need a hand.
func TestAMachineThatWillNotFollowTellsTheOldControllerOnce(t *testing.T) {
	const old = "https://old-held.example.com"
	config := enrolledMachine(t, old)
	c := loaded(t, config)
	c.FollowOff = true
	f := &moveFollower{}
	var out bytes.Buffer
	for i := 0; i < 3; i++ {
		f.follow(context.Background(), movedRefusal("https://new-held.example.com"), c, config, &out, "test")
	}
	if got := saidTo(old); len(got) != 1 || got[0]["held"] != "follow-off" {
		t.Fatalf("the old controller was told %v", got)
	}
}

// What a machine says on leaving goes to the controller it already uses, on
// the route the fence lets through, with its own credentials and nothing else.
func TestTellingTheOldControllerUsesItsOwnAddressAndRoute(t *testing.T) {
	old := newController(t, func(_ *controller, w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusNoContent)
	})
	config := enrolledMachine(t, old.URL)
	if err := tellOld(context.Background(), loaded(t, config), map[string]string{"to": "https://next.example.com"}, "test"); err != nil {
		t.Fatal(err)
	}
	reqs := old.requests()
	if len(reqs) != 1 || reqs[0].path != fenceLeftPath || reqs[0].headers.Get("X-Reet-Agent") != testMachineID || !strings.Contains(reqs[0].body, "https://next.example.com") {
		t.Fatalf("requests: %+v", reqs)
	}
}

// set-server by hand ends a pending move too, and the old controller hears of it.
func TestSetServerClearsAPendingMoveAndTellsTheOldController(t *testing.T) {
	const old = "https://old-by-hand.example.com"
	config := enrolledMachine(t, old)
	c := loaded(t, config)
	c.MovingTo = "https://somewhere-else.example.com"
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	withRestart(t, nil)
	next := honest(t)
	var out bytes.Buffer
	if err := runAgentSetServer([]string{next.URL, "--config", config}, &out, "test"); err != nil {
		t.Fatal(err)
	}
	if now := loaded(t, config); now.Server != next.URL || now.MovingTo != "" {
		t.Fatalf("saved %+v", now)
	}
	if got := saidTo(old); len(got) != 1 || got[0]["to"] != next.URL {
		t.Fatalf("the old controller was told %v", got)
	}
}
