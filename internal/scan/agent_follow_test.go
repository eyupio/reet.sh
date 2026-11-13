package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/syncproof"
)

// fencedAt is a controller that has moved: it refuses every write as the real
// one does, naming where it went.
func fencedAt(t *testing.T, movedTo string) *controller {
	return newController(t, func(_ *controller, w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusLocked)
		json.NewEncoder(w).Encode(map[string]any{"error": "This controller has moved", "code": "controller_moved", "fenced": true, "movedTo": movedTo})
	})
}

func movedRefusal(to string) error {
	return controllerError{Status: http.StatusLocked, Message: "This controller has moved", Code: "controller_moved", MovedTo: to}
}

func loaded(t *testing.T, config string) agentConfig {
	t.Helper()
	c, err := loadAgentConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The point of the fence's address: a machine left running follows the
// controller without anyone visiting it, and only its address changes.
func TestAnAgentFollowsAMovedControllerThatProvesItHoldsTheRecord(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	next := honest(t)
	var out bytes.Buffer
	if !(&moveFollower{}).follow(context.Background(), movedRefusal(next.URL+"/"), loaded(t, config), config, &out, "test") {
		t.Fatalf("the move was not followed: %s", out.String())
	}
	var was, now map[string]any
	json.Unmarshal(before, &was)
	json.Unmarshal(fileBytes(t, config), &now)
	if now["server"] != next.URL {
		t.Fatalf("server is %v", now["server"])
	}
	delete(was, "server")
	delete(now, "server")
	if !jsonEqual(was, now) {
		t.Fatalf("something other than the address changed:\nwas %v\nnow %v", was, now)
	}
	reqs := next.requests()
	if len(reqs) != 2 || reqs[0].path != syncproof.Path || reqs[1].path != "/api/v2/sync/identity" {
		t.Fatalf("requests: %+v", reqs)
	}
	if reqs[0].headers.Get("X-Reet-Agent-Key") != "" || reqs[0].headers.Get("X-Reet-Token") != "" {
		t.Fatal("the proof request carried a credential")
	}
}

// The address in a refusal is a hint anyone could have written. A place that
// cannot prove it holds the record gets no credential, and is not asked again
// every five seconds while a real destination is still being set up.
func TestAnAgentSendsNoCredentialToAMovedToAddressThatCannotProveTheRecord(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	stranger := newController(t, func(_ *controller, w http.ResponseWriter, _ *http.Request, _ []byte) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"code": "unknown_machine"})
	})
	f := &moveFollower{}
	var out bytes.Buffer
	for i := 0; i < 3; i++ {
		if f.follow(context.Background(), movedRefusal(stranger.URL), loaded(t, config), config, &out, "test") {
			t.Fatal("a controller with no record was followed")
		}
	}
	if !bytes.Equal(before, fileBytes(t, config)) {
		t.Fatal("the enrollment changed")
	}
	noCredentialEverSent(t, stranger)
	if n := len(stranger.requests()); n != 1 {
		t.Fatalf("the address was asked %d times within a minute, want once", n)
	}
	if strings.Count(out.String(), "nothing was changed") != 1 {
		t.Fatalf("the reason should be said once: %s", out.String())
	}
}

func TestAnAgentToldNotToFollowAsksNobodyAndSaysHowToFollowByHand(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	c := loaded(t, config)
	c.FollowOff = true
	next := honest(t)
	var out bytes.Buffer
	if (&moveFollower{}).follow(context.Background(), movedRefusal(next.URL), c, config, &out, "test") {
		t.Fatal("followed although turned off")
	}
	if len(next.requests()) != 0 {
		t.Fatalf("the new address was asked: %+v", next.requests())
	}
	if !strings.Contains(out.String(), "reet agent set-server "+next.URL) {
		t.Fatalf("output: %s", out.String())
	}
}

// Credentials must not cross the network in the clear because an old
// controller said so: the same rule enrollment keeps.
func TestAnAgentDoesNotFollowToAPlainHTTPAddressElsewhere(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	before := fileBytes(t, config)
	var out bytes.Buffer
	if (&moveFollower{}).follow(context.Background(), movedRefusal("http://reet.example.com"), loaded(t, config), config, &out, "test") {
		t.Fatal("followed to plain HTTP")
	}
	if !bytes.Equal(before, fileBytes(t, config)) || !strings.Contains(out.String(), "HTTPS") {
		t.Fatalf("output: %s", out.String())
	}
}

// allow-apply or a terminal switch flipped while the agent runs is read fresh
// by the agent; following a move must not write the stale copy back.
func TestFollowingAMoveKeepsASettingChangedSinceTheAgentStarted(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	started := loaded(t, config)
	changed := started
	changed.AllowApply = false
	if err := writePrivate(config, changed); err != nil {
		t.Fatal(err)
	}
	next := honest(t)
	var out bytes.Buffer
	if !(&moveFollower{}).follow(context.Background(), movedRefusal(next.URL), started, config, &out, "test") {
		t.Fatalf("not followed: %s", out.String())
	}
	if now := loaded(t, config); now.AllowApply || now.Server != next.URL {
		t.Fatalf("saved %+v", now)
	}
}

func TestOnlyAMovedControllersRefusalIsFollowed(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	next := honest(t)
	for name, err := range map[string]error{
		"another refusal":     controllerError{Status: http.StatusForbidden, Message: "no", MovedTo: next.URL},
		"a lock of some kind": controllerError{Status: http.StatusLocked, Message: "busy", MovedTo: next.URL},
		"a network error":     errors.New("connection refused"),
	} {
		var out bytes.Buffer
		if (&moveFollower{}).follow(context.Background(), err, loaded(t, config), config, &out, "test") || out.Len() != 0 {
			t.Fatalf("%s was treated as a move: %s", name, out.String())
		}
	}
	if len(next.requests()) != 0 {
		t.Fatal("the address was asked")
	}
}

// The wiring: a single run against a fenced controller ends up talking to the
// new one, with its credentials, after the proof.
func TestASingleRunFollowsTheMoveAndCarriesOnAgainstTheNewController(t *testing.T) {
	next := honest(t)
	old := fencedAt(t, next.URL)
	config := enrolledMachine(t, old.URL)
	var out bytes.Buffer
	_ = RunAgentInput([]string{"run", "--once", "--config", config}, strings.NewReader(""), &out, "test")
	if loaded(t, config).Server != next.URL {
		t.Fatalf("the enrollment still points at the old controller: %s", out.String())
	}
	reqs := next.requests()
	if len(reqs) < 3 || reqs[0].path != syncproof.Path || reqs[1].path != "/api/v2/sync/identity" {
		t.Fatalf("requests to the new controller: %+v", reqs)
	}
	if reqs[len(reqs)-1].headers.Get("X-Reet-Agent-Key") != testMachineSecret {
		t.Fatal("the run did not carry on against the new controller")
	}
}

func TestFollowMovesIsALocalSwitchThatDefaultsToOn(t *testing.T) {
	config := enrolledMachine(t, "https://old.example.com")
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := RunAgentInput(append([]string{"follow-moves"}, append(args, "--config", config)...), strings.NewReader(""), &out, "test"); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}
	if got := run("status"); !strings.Contains(got, "on") {
		t.Fatalf("default: %s", got)
	}
	run("off")
	if !loaded(t, config).FollowOff || !strings.Contains(run("status"), "off") {
		t.Fatal("off was not saved")
	}
	run("on")
	if loaded(t, config).FollowOff {
		t.Fatal("on was not saved")
	}
	var out bytes.Buffer
	if err := RunAgentInput([]string{"follow-moves", "sideways", "--config", config}, strings.NewReader(""), &out, "test"); err == nil {
		t.Fatal("an unknown word was accepted")
	}
}

// While the new controller is being set up the old one refuses the agent every
// five seconds. The log says so once, not four lines a cycle for an hour.
func TestAWaitingAgentSaysItsControllerMovedOnceAndAsksItOncePerCycle(t *testing.T) {
	old := fencedAt(t, "")
	config := enrolledMachine(t, old.URL)
	c := loaded(t, config)
	var out bytes.Buffer
	last := time.Time{}
	echo := &movedEcho{}
	said := 0
	for i := 0; i < 3; i++ {
		err := agentCycle(context.Background(), c, config, &out, "test", &last, false)
		if _, moved := movedTo(err); !moved {
			t.Fatalf("cycle %d: want the moved refusal, got %v", i, err)
		}
		if echo.worthSaying(err) {
			said++
		}
	}
	if said != 1 {
		t.Fatalf("the refusal was worth saying %d times, want once", said)
	}
	if out.Len() != 0 {
		t.Fatalf("the cycle printed about work it could not do: %s", out.String())
	}
	if n := len(old.requests()); n != 3 {
		t.Fatalf("the fenced controller was asked %d times in three cycles, want one each", n)
	}
	// A different refusal, and the same one after a good cycle, are news again.
	if !echo.worthSaying(movedRefusal("https://reet.new.example")) || !echo.worthSaying(errors.New("connection refused")) {
		t.Fatal("a changed reason was kept quiet")
	}
	echo.worthSaying(nil)
	if !echo.worthSaying(movedRefusal("https://reet.new.example")) {
		t.Fatal("the refusal was kept quiet after the agent had got through")
	}
}
