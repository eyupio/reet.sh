package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"github.com/eyupio/reet.sh/internal/webpush"
)

// Push notifications tell a phone that a session needs its person, or has
// ended, while reet is not open. A wake-up carries nothing (see
// internal/webpush): the browser's service worker is woken, asks
// GET /api/v2/push/pending with the person's own sign-in, and shows what
// that answers. So a session's name never passes through a push service.

const (
	pushKeySetting = "push_vapid_key"
	// pushEvery is how often sessions are looked at for something new to tell.
	pushEvery = 5 * time.Second
	// pushSettle is how old a moment must be before it is told. Somebody
	// looking at the session marks it seen within this time, and is not told
	// what they are watching.
	pushSettle = 8 * time.Second
	// pushFresh is how recent a moment must be to be told at all, and how long
	// an ended session stays in what the service worker is given. A device
	// registered while a session has been waiting for an hour hears nothing
	// about that hour.
	pushFresh = 15 * time.Minute
	// pushTTL is how long a push service keeps a wake-up for a phone that is off.
	pushTTL = time.Hour
	// pushTestFor is how long a requested test stays in what the service worker is given.
	pushTestFor = 2 * time.Minute
)

type pushState struct {
	mu   sync.Mutex
	keys *webpush.Keys
	// tests is when each person last asked for a test notification.
	tests map[int64]time.Time
	// send stands in for the push services in tests.
	send func(ctx context.Context, m webpush.Message) error
	// allow overrides the push service address check in tests.
	allow func(endpoint string) bool
}

func (s *server) registerPushRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/push", s.pushStatus)
	mux.HandleFunc("POST /api/v2/push/subscriptions", s.requireWriter(s.pushSubscribe))
	mux.HandleFunc("DELETE /api/v2/push/subscriptions", s.pushUnsubscribe)
	mux.HandleFunc("POST /api/v2/push/test", s.requireWriter(s.pushTest))
	mux.HandleFunc("GET /api/v2/push/pending", s.pushPending)
}

// pushKeys is the controller's push identity, made the first time it is needed.
func (s *server) pushKeys() (*webpush.Keys, error) {
	s.push.mu.Lock()
	defer s.push.mu.Unlock()
	if s.push.keys != nil {
		return s.push.keys, nil
	}
	saved, err := s.st.GetSetting(pushKeySetting)
	if err != nil {
		return nil, err
	}
	if saved != "" {
		keys, err := webpush.Parse(saved)
		if err != nil {
			return nil, err
		}
		s.push.keys = keys
		return keys, nil
	}
	keys, err := webpush.Generate()
	if err != nil {
		return nil, err
	}
	text, err := keys.Marshal()
	if err != nil {
		return nil, err
	}
	if err := s.st.SetSetting(pushKeySetting, text); err != nil {
		return nil, err
	}
	s.push.keys = keys
	return keys, nil
}

// pushPerson is the signed-in person push works for: an account, since a
// device has to belong to somebody.
func (s *server) pushPerson(r *http.Request) (int64, bool) {
	if s.authMode != AuthAccounts {
		return 0, false
	}
	u, ok := userFrom(r)
	return u.ID, ok && u.ID > 0
}

type pushStatusView struct {
	// Available is false on an instance without accounts.
	Available bool                     `json:"available"`
	PublicKey string                   `json:"publicKey,omitempty"`
	Devices   []store.PushSubscription `json:"devices"`
	// Current is the id of the device asking, when it says which it is.
	Current int64 `json:"current,omitempty"`
}

func (s *server) pushStatus(w http.ResponseWriter, r *http.Request) {
	out := pushStatusView{Devices: []store.PushSubscription{}}
	uid, ok := s.pushPerson(r)
	if !ok {
		writeJSON(w, http.StatusOK, out)
		return
	}
	keys, err := s.pushKeys()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "The push identity could not be read.")
		return
	}
	devices, err := s.st.PushSubscriptionsFor(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out.Available, out.PublicKey, out.Devices = true, keys.Public, devices
	if endpoint := r.Header.Get("X-Reet-Push-Endpoint"); endpoint != "" {
		for _, d := range devices {
			if d.Endpoint == endpoint {
				out.Current = d.ID
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) pushEndpointAllowed(endpoint string) bool {
	if s.push.allow != nil {
		return s.push.allow(endpoint)
	}
	return webpush.ValidEndpoint(endpoint)
}

func (s *server) pushSubscribe(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.pushPerson(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "Notifications on a device need an account.")
		return
	}
	var in struct {
		Endpoint string `json:"endpoint"`
		Label    string `json:"label"`
		Waiting  *bool  `json:"waiting"`
		Ended    *bool  `json:"ended"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.pushEndpointAllowed(in.Endpoint) {
		writeErr(w, http.StatusBadRequest, "That is not an address a browser's push service gives out.")
		return
	}
	waiting, ended := in.Waiting == nil || *in.Waiting, in.Ended == nil || *in.Ended
	device, err := s.st.SavePushSubscription(uid, in.Endpoint, strings.TrimSpace(in.Label), waiting, ended)
	if errors.Is(err, store.ErrPushLimit) {
		writeErr(w, http.StatusConflict, "You have "+strconv.Itoa(store.MaxPushSubscriptions)+" devices already. Remove one you no longer use, then turn this one on.")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, device)
}

func (s *server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.pushPerson(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "Notifications on a device need an account.")
		return
	}
	var in struct {
		Endpoint string `json:"endpoint"`
		ID       int64  `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Endpoint == "" && in.ID == 0 {
		writeErr(w, http.StatusBadRequest, "Say which device: its endpoint or its id.")
		return
	}
	removed, err := s.st.DeletePushSubscription(uid, in.Endpoint, in.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": removed})
}

// pushTest wakes every device the person has, so they can see one arrive.
func (s *server) pushTest(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.pushPerson(r)
	if !ok {
		writeErr(w, http.StatusForbidden, "Notifications on a device need an account.")
		return
	}
	devices, err := s.st.PushSubscriptionsFor(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(devices) == 0 {
		writeErr(w, http.StatusConflict, "No device is turned on for notifications yet.")
		return
	}
	s.push.mu.Lock()
	if s.push.tests == nil {
		s.push.tests = map[int64]time.Time{}
	}
	s.push.tests[uid] = time.Now()
	s.push.mu.Unlock()
	sent := s.pushWake(r.Context(), devices, "test")
	writeJSON(w, http.StatusOK, map[string]int{"sent": sent, "devices": len(devices)})
}

// pushNote is one notification the service worker shows.
type pushNote struct {
	Tag   string `json:"tag"`
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// pushPending is what a woken service worker shows: a requested test, the
// sessions waiting for this person, and the ones that ended a moment ago and
// have not been looked at. It reads the sessions as they are now, so a
// wake-up delivered late shows what is still true. A worker that names a
// device which is not this person's is shown nothing.
func (s *server) pushPending(w http.ResponseWriter, r *http.Request) {
	notes := []pushNote{}
	uid, ok := s.pushPerson(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"notifications": notes})
		return
	}
	waiting, ended := true, true
	if endpoint := r.Header.Get("X-Reet-Push-Endpoint"); endpoint != "" {
		devices, err := s.st.PushSubscriptionsFor(uid)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		mine := false
		for _, d := range devices {
			if d.Endpoint == endpoint {
				mine, waiting, ended = true, d.Waiting, d.Ended
			}
		}
		// The browser was woken for somebody who signed out of it, and asks as
		// the person signed in now, who never turned this device on. They are
		// shown nothing, and the worker is told to end the subscription.
		if !mine {
			writeJSON(w, http.StatusOK, map[string]any{"notifications": notes, "unsubscribe": true})
			return
		}
	}
	now := time.Now()
	s.push.mu.Lock()
	tested := s.push.tests[uid]
	s.push.mu.Unlock()
	if !tested.IsZero() && now.Sub(tested) <= pushTestFor {
		notes = append(notes, pushNote{Tag: "reet-test", Title: "Notifications are on", Body: "This device will hear when a session needs you.", URL: "/notifications"})
	}
	sessions, err := s.st.TerminalSessionsFor(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, item := range sessions {
		kind, at := pushMoment(item)
		wanted := kind == "waiting" && waiting || kind != "" && kind != "waiting" && ended
		if !wanted || kind != "waiting" && now.Sub(time.UnixMilli(at)) > pushFresh {
			continue
		}
		notes = append(notes, pushNote{Tag: "reet-session-" + item.ID, Title: pushTitle(item), Body: pushBody(kind), URL: "/sessions?agent=" + item.AgentID + "&session=" + item.ID})
		if len(notes) >= 4 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": notes})
}

func pushTitle(item store.TerminalSession) string {
	if title := strings.TrimSpace(item.Title); title != "" {
		return title
	}
	return "A session"
}

func pushBody(kind string) string {
	switch kind {
	case "waiting":
		return "Needs you."
	case "failed":
		return "Stopped with a problem."
	case "turn-finished":
		return "Finished its turn."
	case "turn-failed":
		return "Its turn stopped with a problem."
	case "turn-unwatched":
		return "Stopped waiting for its background work; it may still be running."
	default:
		return "Finished."
	}
}

// pushMoment is what a session has to tell its person, and when it began:
// waiting for them (a bell or a pending question they have not seen), ended
// unseen, or a chat whose turn is over and unseen. Anything else is "".
func pushMoment(item store.TerminalSession) (kind string, at int64) {
	switch item.Attention {
	case "waiting":
		return "waiting", pushMillis(item.BellAt)
	case "finished", "failed":
		if item.ChangedSinceSeen {
			return item.Attention, pushMillis(item.UpdatedAt)
		}
	}
	// A chat that finished its turn and waits for the next message is still running, so its attention is "quiet":
	// the turn's own end is the moment, until the person has looked.
	turnOver := item.ChatStatus == "finished" || item.ChatStatus == "failed" || item.ChatStatus == "unwatched"
	if done := pushMillis(item.CompletedAt); item.Structured && item.State == terminalproto.Running && turnOver && done > pushMillis(item.SeenAt) {
		return "turn-" + item.ChatStatus, done
	}
	return "", 0
}

func pushMillis(v int64) int64 {
	if v > 0 && v < 100_000_000_000 {
		return v * 1000
	}
	return v
}

// pushSubject is who runs this controller, for a push service that needs to
// reach an operator: the contact address, or the controller's own address.
func (s *server) pushSubject() string {
	if s.contactEmail != "" {
		return "mailto:" + s.contactEmail
	}
	if origin := s.publicOrigin(); strings.HasPrefix(origin, "https://") {
		return origin
	}
	return "https://reet.sh"
}

// pushWake wakes each device and reports how many the push services accepted.
// A device the push service no longer knows is forgotten.
func (s *server) pushWake(ctx context.Context, devices []store.PushSubscription, topic string) int {
	keys, err := s.pushKeys()
	if err != nil {
		log.Printf("push: identity: %v", err)
		return 0
	}
	send := s.push.send
	if send == nil {
		send = (&webpush.Sender{Keys: keys, Allow: s.push.allow}).Send
	}
	subject, sent := s.pushSubject(), 0
	for _, device := range devices {
		err := send(ctx, webpush.Message{Endpoint: device.Endpoint, Subject: subject, TTL: pushTTL, Topic: topic})
		switch {
		case err == nil:
			sent++
			_ = s.st.MarkPushDelivered(device.Endpoint)
		case errors.Is(err, webpush.ErrGone), errors.Is(err, webpush.ErrEndpoint):
			_ = s.st.DropPushEndpoint(device.Endpoint)
		default:
			log.Printf("push: device %d: %v", device.ID, err)
		}
	}
	return sent
}

func (s *server) pushWorker() {
	ticker := time.NewTicker(pushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.pushSweep(time.Now())
		}
	}
}

// pushSweep tells each person with a device about anything new: one wake-up
// per device per sweep, however many sessions changed. Each moment (a bell,
// an ending) is told once; a fenced controller tells nobody, since the copy
// that took over does.
func (s *server) pushSweep(now time.Time) {
	if s.st.Fenced() {
		return
	}
	users, err := s.st.PushUsers()
	if err != nil {
		return
	}
	for _, uid := range users {
		sessions, err := s.st.TerminalSessionsFor(uid)
		if err != nil {
			continue
		}
		told, err := s.st.PushTold(uid)
		if err != nil {
			continue
		}
		newWaiting, newEnded := false, false
		for _, item := range sessions {
			kind, at := pushMoment(item)
			if kind == "" {
				continue
			}
			marker := kind + ":" + strconv.FormatInt(at, 10)
			age := now.Sub(time.UnixMilli(at))
			if told[item.ID] == marker || age < pushSettle {
				continue
			}
			if err := s.st.SetPushTold(uid, item.ID, marker); err != nil {
				continue
			}
			// An old moment is recorded as told, and tells nobody.
			if age > pushFresh {
				continue
			}
			if kind == "waiting" {
				newWaiting = true
			} else {
				newEnded = true
			}
		}
		if !newWaiting && !newEnded {
			continue
		}
		devices, err := s.st.PushSubscriptionsFor(uid)
		if err != nil {
			continue
		}
		wake := devices[:0:0]
		for _, device := range devices {
			if newWaiting && device.Waiting || newEnded && device.Ended {
				wake = append(wake, device)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		s.pushWake(ctx, wake, "sessions")
		cancel()
	}
}
