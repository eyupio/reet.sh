package webpush

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKeysSurviveBeingSaved(t *testing.T) {
	keys, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := b64.DecodeString(keys.Public)
	if err != nil || len(raw) != 65 || raw[0] != 4 {
		t.Fatalf("public key is not an uncompressed P-256 point: %d bytes, %v", len(raw), err)
	}
	saved, err := keys.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(saved)
	if err != nil {
		t.Fatal(err)
	}
	if again.Public != keys.Public {
		t.Fatal("a saved identity came back with a different public key")
	}
	if _, err := Parse("not a key"); err == nil {
		t.Fatal("nonsense parsed as a key")
	}
}

func TestOnlyPushServicesAreEndpoints(t *testing.T) {
	for endpoint, want := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                true,
		"https://updates.push.services.mozilla.com/wpush/v2/abc": true,
		"https://web.push.apple.com/abc":                         true,
		"https://db5p.notify.windows.com/w/?token=abc":           true,
		"http://fcm.googleapis.com/fcm/send/abc":                 false,
		"https://fcm.googleapis.com.evil.example/fcm/send/abc":   false,
		"https://evilfcm.googleapis.com/x":                       false,
		"https://push.apple.com.example.org/x":                   false,
		"https://user@fcm.googleapis.com/x":                      false,
		"https://fcm.googleapis.com:8443/x":                      false,
		"https://127.0.0.1/x":                                    false,
		"https://localhost/x":                                    false,
		"https://169.254.169.254/latest/meta-data":               false,
		"": false,
		"https://fcm.googleapis.com/" + strings.Repeat("a", 2100): false,
	} {
		if got := ValidEndpoint(endpoint); got != want {
			t.Errorf("ValidEndpoint(%.60q) = %v, want %v", endpoint, got, want)
		}
	}
}

func TestSendSignsAnEmptyWakeUp(t *testing.T) {
	keys, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	var got *http.Request
	var length int64
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, length = r.Clone(context.Background()), r.ContentLength
		w.WriteHeader(http.StatusCreated)
	}))
	defer service.Close()
	at := time.Unix(1_800_000_000, 0)
	sender := &Sender{Keys: keys, Allow: func(string) bool { return true }, Now: func() time.Time { return at }}
	err = sender.Send(context.Background(), Message{Endpoint: service.URL + "/send/abc", Subject: "mailto:ops@example.test", TTL: time.Hour, Topic: "needs-you"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPost || length != 0 {
		t.Fatalf("sent %s with %d bytes, want an empty POST", got.Method, length)
	}
	if got.Header.Get("TTL") != "3600" || got.Header.Get("Urgency") != "high" || got.Header.Get("Topic") != "needs-you" {
		t.Fatalf("headers: TTL %q Urgency %q Topic %q", got.Header.Get("TTL"), got.Header.Get("Urgency"), got.Header.Get("Topic"))
	}
	auth := got.Header.Get("Authorization")
	token, key, ok := strings.Cut(strings.TrimPrefix(auth, "vapid t="), ", k=")
	if !strings.HasPrefix(auth, "vapid t=") || !ok || key != keys.Public {
		t.Fatalf("Authorization is %q", auth)
	}
	claims, err := verify(keys.Public, token)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if claims["aud"] != service.URL || claims["sub"] != "mailto:ops@example.test" {
		t.Fatalf("claims: %v", claims)
	}
	if exp := int64(claims["exp"].(float64)); exp <= at.Unix() || exp > at.Add(24*time.Hour).Unix() {
		t.Fatalf("exp %d is not within a day of %d", exp, at.Unix())
	}
	other, _ := Generate()
	if _, err := verify(other.Public, token); err == nil {
		t.Fatal("a token verified against another controller's key")
	}
}

func TestSendSaysWhenASubscriptionIsGone(t *testing.T) {
	keys, _ := Generate()
	status := http.StatusGone
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
	defer service.Close()
	sender := &Sender{Keys: keys, Allow: func(string) bool { return true }}
	for code, gone := range map[int]bool{http.StatusGone: true, http.StatusNotFound: true, http.StatusTooManyRequests: false, http.StatusInternalServerError: false} {
		status = code
		err := sender.Send(context.Background(), Message{Endpoint: service.URL, Subject: "mailto:ops@example.test", TTL: time.Minute})
		if err == nil || errors.Is(err, ErrGone) != gone {
			t.Errorf("status %d: %v, gone should be %v", code, err, gone)
		}
	}
}

func TestSendRefusesAnAddressThatIsNotAPushService(t *testing.T) {
	keys, _ := Generate()
	called := false
	service := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer service.Close()
	err := (&Sender{Keys: keys}).Send(context.Background(), Message{Endpoint: service.URL, Subject: "mailto:ops@example.test", TTL: time.Minute})
	if !errors.Is(err, ErrEndpoint) || called {
		t.Fatalf("err %v, called %v: the controller must not post to an address that is not a push service", err, called)
	}
}
