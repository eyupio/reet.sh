// Package webpush wakes a browser through its push service (RFC 8030) with a
// VAPID-signed request (RFC 8292) and no payload.
//
// Nothing about a session leaves the controller this way: the push service
// learns only that this controller pinged a subscription. The browser's
// service worker then asks the controller, over the person's own signed-in
// connection, what to show. That also means no payload encryption (RFC 8291)
// and no subscription keys to store.
package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrGone means the push service no longer knows the subscription (the
// browser unsubscribed, or the site's data was cleared): forget it.
var ErrGone = errors.New("webpush: subscription is gone")

// ErrEndpoint means the endpoint is not one a browser's push service gives out.
var ErrEndpoint = errors.New("webpush: not a push service endpoint")

var b64 = base64.RawURLEncoding

// Keys is the controller's VAPID identity: one P-256 key for every subscription.
type Keys struct {
	private *ecdsa.PrivateKey
	// Public is the uncompressed point, base64url: what the browser is given
	// as applicationServerKey and what each request names as k=.
	Public string
}

// Generate makes a new identity.
func Generate() (*Keys, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return fromPrivate(key)
}

// Parse reads an identity saved by Marshal.
func Parse(saved string) (*Keys, error) {
	der, err := b64.DecodeString(saved)
	if err != nil {
		return nil, fmt.Errorf("webpush: saved key: %w", err)
	}
	key, err := x509.ParseECPrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("webpush: saved key: %w", err)
	}
	if key.Curve != elliptic.P256() {
		return nil, errors.New("webpush: saved key is not P-256")
	}
	return fromPrivate(key)
}

func fromPrivate(key *ecdsa.PrivateKey) (*Keys, error) {
	pub, err := key.PublicKey.ECDH()
	if err != nil {
		return nil, err
	}
	return &Keys{private: key, Public: b64.EncodeToString(pub.Bytes())}, nil
}

// Marshal is the identity as one line of text, for the controller's settings.
func (k *Keys) Marshal() (string, error) {
	der, err := x509.MarshalECPrivateKey(k.private)
	if err != nil {
		return "", err
	}
	return b64.EncodeToString(der), nil
}

// pushHosts are the push services browsers use: Chrome and Edge (FCM), Firefox
// (Mozilla autopush), Safari (Apple) and Edge on Windows (WNS). The controller
// posts to an address a browser handed it, so anything else is refused: a
// subscription must never be a way to make the controller call an address of
// somebody's choosing.
var pushHosts = []string{
	"fcm.googleapis.com",
	".push.services.mozilla.com",
	".push.apple.com",
	".notify.windows.com",
}

// ValidEndpoint reports whether endpoint is an https address at a known push service.
func ValidEndpoint(endpoint string) bool {
	if len(endpoint) > 2048 {
		return false
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range pushHosts {
		if host == allowed || strings.HasPrefix(allowed, ".") && strings.HasSuffix(host, allowed) {
			return true
		}
	}
	return false
}

// token signs the VAPID claim for one push service origin.
func (k *Keys) token(audience, subject string, now time.Time) (string, error) {
	header := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{"aud": audience, "exp": now.Add(12 * time.Hour).Unix(), "sub": subject})
	if err != nil {
		return "", err
	}
	signing := header + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, k.private, digest[:])
	if err != nil {
		return "", err
	}
	// JWS wants the two 32-byte integers side by side, not ASN.1.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + b64.EncodeToString(sig), nil
}

// Message is one wake-up.
type Message struct {
	Endpoint string
	// Subject is who runs this controller, as a mailto: or https: address; push
	// services use it to reach an operator whose pushes misbehave.
	Subject string
	// TTL is how long the push service keeps the wake-up for a device that is off.
	TTL time.Duration
	// Topic lets a newer wake-up replace an undelivered older one (up to 32
	// base64url characters). Empty sends each one.
	Topic string
}

// Sender posts wake-ups. Client is the HTTP client to use; nil means a
// default with a ten-second limit.
type Sender struct {
	Keys   *Keys
	Client *http.Client
	// Allow overrides ValidEndpoint, for tests that stand in for a push service.
	Allow func(endpoint string) bool
	Now   func() time.Time
}

// Send wakes one subscription. ErrGone means it should be forgotten; any
// other error is worth another try later.
func (s *Sender) Send(ctx context.Context, m Message) error {
	allow := s.Allow
	if allow == nil {
		allow = ValidEndpoint
	}
	if !allow(m.Endpoint) {
		return ErrEndpoint
	}
	u, err := url.Parse(m.Endpoint)
	if err != nil {
		return ErrEndpoint
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	jwt, err := s.Keys.token(u.Scheme+"://"+u.Host, m.Subject, now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.Endpoint, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+s.Keys.Public)
	req.Header.Set("TTL", strconv.Itoa(int(m.TTL/time.Second)))
	req.Header.Set("Urgency", "high")
	if m.Topic != "" {
		req.Header.Set("Topic", m.Topic)
	}
	client := s.Client
	if client == nil {
		client = defaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		return nil
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return ErrGone
	default:
		return fmt.Errorf("webpush: push service answered %d", res.StatusCode)
	}
}

// A push service address never redirects; following one would undo the host check.
var defaultClient = &http.Client{
	Timeout:       10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// verify checks a token against the identity's public key, for tests.
func verify(public, token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("not three parts")
	}
	raw, err := b64.DecodeString(public)
	if err != nil {
		return nil, err
	}
	point, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return nil, err
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), point.Bytes()) //nolint:staticcheck // the uncompressed point is the wire format
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, errors.New("signature is not 64 bytes")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	if !ecdsa.VerifyASN1(pub, digest[:], asn1Signature(sig)) {
		return nil, errors.New("signature does not verify")
	}
	body, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	return claims, json.Unmarshal(body, &claims)
}
