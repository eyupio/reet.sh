package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
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

// unreachable is what a cycle returns when the controller does not answer at
// all, as the HTTP client reports it.
func unreachable() error {
	return &url.Error{Op: "Post", URL: "https://old.example.com/api/v2/sync/poll", Err: errors.New("dial tcp: connection refused")}
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
	if !f.follow(context.Background(), unreachable(), loaded(t, config), config, &out, "test") {
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
	if f.follow(context.Background(), unreachable(), loaded(t, config), config, &out, "test") {
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

// A move that was called off must not be finished by a local failure. Only a
// controller that does not answer at all lets the kept address stand in for it.
func TestALocalFailureDoesNotSendAMachineToTheKeptAddress(t *testing.T) {
	config := enrolledMachine(t, "https://old-local.example.com")
	next := honest(t)
	c := loaded(t, config)
	c.MovingTo = next.URL
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	f := &moveFollower{pending: next.URL}
	var out bytes.Buffer
	for _, local := range []error{errors.New("scan: open /home/dev/.claude: permission denied"), errors.New("invalid character in agent.json.result")} {
		if f.follow(context.Background(), local, loaded(t, config), config, &out, "test") {
			t.Fatalf("%v sent the machine to the kept address", local)
		}
	}
	if len(next.requests()) != 0 {
		t.Fatalf("the kept address was asked: %+v", next.requests())
	}
	// A proxy saying the controller is down counts as not answering.
	if !f.follow(context.Background(), controllerError{Status: http.StatusBadGateway, Message: "bad gateway"}, loaded(t, config), config, &out, "test") {
		t.Fatalf("a 502 did not count as the controller being gone: %s", out.String())
	}
}

// The controller answering without its fence means the fence was lifted: the
// kept address is dropped there and then, even if the rest of the cycle fails.
func TestAControllerAnsweringWithoutItsFenceCallsTheMoveOff(t *testing.T) {
	config := enrolledMachine(t, "https://old-answering.example.com")
	next := honest(t)
	c := loaded(t, config)
	c.MovingTo = next.URL
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	f := &moveFollower{pending: next.URL}
	var out bytes.Buffer
	if f.follow(context.Background(), controllerError{Status: http.StatusForbidden, Message: "no"}, loaded(t, config), config, &out, "test") {
		t.Fatal("followed although the controller answered")
	}
	if loaded(t, config).MovingTo != "" || f.pending != "" {
		t.Fatal("the kept address survived a controller that answered without its fence")
	}
	// Later, with the controller gone, there is nothing to go to.
	if f.follow(context.Background(), unreachable(), loaded(t, config), config, &out, "test") || len(next.requests()) != 0 {
		t.Fatal("the abandoned address was followed")
	}
}

// A fence that stops naming an address has withdrawn the one this machine kept.
func TestAFenceWithNoAddressWithdrawsTheKeptOne(t *testing.T) {
	config := enrolledMachine(t, "https://old-withdrawn.example.com")
	next := honest(t)
	c := loaded(t, config)
	c.MovingTo = next.URL
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	f := &moveFollower{pending: next.URL}
	var out bytes.Buffer
	if f.follow(context.Background(), movedRefusal(""), loaded(t, config), config, &out, "test") {
		t.Fatal("followed a fence with no address")
	}
	if loaded(t, config).MovingTo != "" || f.pending != "" {
		t.Fatal("the withdrawn address was kept")
	}
	if f.follow(context.Background(), unreachable(), loaded(t, config), config, &out, "test") || len(next.requests()) != 0 {
		t.Fatal("the withdrawn address was followed once the controller went away")
	}
}

// A fence that swaps its address for one this machine will not use has still
// replaced the old one: the old address is not followed later.
func TestARefusedReplacementAddressDropsTheKeptOne(t *testing.T) {
	config := enrolledMachine(t, "https://old-refused.example.com")
	next := honest(t)
	c := loaded(t, config)
	c.MovingTo = next.URL
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	f := &moveFollower{pending: next.URL}
	var out bytes.Buffer
	f.follow(context.Background(), movedRefusal("http://plain.example.com"), loaded(t, config), config, &out, "test")
	if loaded(t, config).MovingTo != "" || f.pending != "" {
		t.Fatal("the replaced address was kept")
	}
	if f.follow(context.Background(), unreachable(), loaded(t, config), config, &out, "test") || len(next.requests()) != 0 {
		t.Fatal("the replaced address was followed")
	}
}

// What a machine holds back for is said again for a new address, and again
// after the fence has been lifted and raised anew.
func TestAMachineHoldingBackSaysSoForEachAddressAndEachFence(t *testing.T) {
	const old = "https://old-held-again.example.com"
	config := enrolledMachine(t, old)
	c := loaded(t, config)
	c.FollowOff = true
	f := &moveFollower{}
	var out bytes.Buffer
	f.follow(context.Background(), movedRefusal("https://a.example.com"), c, config, &out, "test")
	f.follow(context.Background(), movedRefusal("https://a.example.com"), c, config, &out, "test")
	f.follow(context.Background(), movedRefusal("https://b.example.com"), c, config, &out, "test")
	if n := len(saidTo(old)); n != 2 {
		t.Fatalf("told %d times for two addresses, want 2", n)
	}
	f.settled(config, c, &out) // the fence was lifted: a good cycle
	f.follow(context.Background(), movedRefusal("https://b.example.com"), c, config, &out, "test")
	if n := len(saidTo(old)); n != 3 {
		t.Fatalf("a new fence was not told: %d", n)
	}
}

// A departure report the old controller did not take is kept and sent again
// until it lands; any answer from the old controller ends it.
func TestADepartureReportIsSentAgainUntilTheOldControllerTakesIt(t *testing.T) {
	const old = "https://old-departure.example.com"
	config := enrolledMachine(t, "https://new-departure.example.com")
	c := loaded(t, config)
	c.LeftFrom, c.LeftAt = old, time.Now().UnixMilli()
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	answer := error(unreachable())
	saved := tellOldFn
	tellOldFn = func(ctx context.Context, c agentConfig, body map[string]string, version string) error {
		_ = saved(ctx, c, body, version)
		return answer
	}
	t.Cleanup(func() { tellOldFn = saved })

	(&departureReport{}).report(context.Background(), config, "test")
	if loaded(t, config).LeftFrom != old {
		t.Fatal("a report that did not land was dropped")
	}
	// Paced: the same report is not sent again within the minute.
	d := &departureReport{}
	d.report(context.Background(), config, "test")
	d.report(context.Background(), config, "test")
	if n := len(saidTo(old)); n != 2 {
		t.Fatalf("sent %d times, want 2", n)
	}
	if got := saidTo(old)[0]; got["to"] != "https://new-departure.example.com" {
		t.Fatalf("said %v", got)
	}
	// The old controller answers (here: it is no longer fenced), which ends it.
	answer = controllerError{Status: http.StatusConflict, Code: "not_fenced"}
	(&departureReport{}).report(context.Background(), config, "test")
	if loaded(t, config).LeftFrom != "" {
		t.Fatal("an answered report was kept")
	}
}

// A week on, the old controller is retired; the machine stops asking.
func TestADepartureReportIsDroppedAfterAWeek(t *testing.T) {
	const old = "https://old-retired.example.com"
	config := enrolledMachine(t, "https://new-retired.example.com")
	c := loaded(t, config)
	c.LeftFrom, c.LeftAt = old, time.Now().Add(-8*24*time.Hour).UnixMilli()
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	(&departureReport{}).report(context.Background(), config, "test")
	if loaded(t, config).LeftFrom != "" || len(saidTo(old)) != 0 {
		t.Fatal("a week-old report was still sent or kept")
	}
}
