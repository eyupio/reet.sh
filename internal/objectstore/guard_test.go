package objectstore

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestPublicAddress(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1":         false, // loopback
		"::1":               false,
		"169.254.169.254":   false, // link-local, where cloud metadata lives
		"fe80::1":           false,
		"10.1.2.3":          false, // private
		"172.16.0.1":        false,
		"192.168.1.1":       false,
		"100.64.0.1":        false, // carrier-grade NAT
		"fd00::1":           false, // unique-local
		"0.0.0.0":           false,
		"::ffff:10.0.0.1":   false, // a private address in IPv6 dress
		"64:ff9b::7f00:1":   false,
		"8.8.8.8":           true,
		"2606:4700:4700::1": true,
	} {
		if got := PublicAddress(net.ParseIP(addr)); got != want {
			t.Errorf("PublicAddress(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestPublicOnlyClientRefusesThisMachine(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	config := Config{Endpoint: server.URL, Bucket: "bucket", AccessKey: "key", SecretKey: "secret", PathStyle: boolPtr(true)}

	config.PublicOnly = true
	guardedClient, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := guardedClient.PutObjectRetry(context.Background(), "object", []byte("payload")); !errors.Is(err, ErrAddress) {
		t.Fatalf("put to loopback = %v, want ErrAddress", err)
	}
	if _, err := guardedClient.List(context.Background(), ""); !errors.Is(err, ErrAddress) {
		t.Fatalf("list on loopback = %v, want ErrAddress", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the server on this machine was asked %d times", hits.Load())
	}

	// The instance's own destination is reached as before.
	config.PublicOnly = false
	open, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := open.Put(context.Background(), "object", []byte("payload")); err != nil || hits.Load() != 1 {
		t.Fatalf("the instance's own client: %v after %d requests", err, hits.Load())
	}
}

// A name is checked by the address it resolves to, not by how it is spelled.
func TestPublicOnlyClientRefusesANameForThisMachine(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client, err := New(Config{Endpoint: "http://localhost:" + u.Port(), Bucket: "bucket", AccessKey: "key", SecretKey: "secret",
		PathStyle: boolPtr(true), PublicOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Put(context.Background(), "object", []byte("payload")); !errors.Is(err, ErrAddress) {
		t.Fatalf("put = %v, want ErrAddress", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the server on this machine was asked")
	}
}

func TestPublicOnlyClientChecksTheDestinationBehindAProxy(t *testing.T) {
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxied.Add(1) }))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	inward := guarded{
		proxy: func(*http.Request) (*url.URL, error) { return proxyURL, nil },
		lookup: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.5")}}, nil
		},
	}
	for _, target := range []string{"http://bucket.internal.example/object", "http://169.254.169.254/latest/meta-data"} {
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		if _, err := inward.RoundTrip(req); !errors.Is(err, ErrAddress) {
			t.Fatalf("%s through a proxy = %v, want ErrAddress", target, err)
		}
	}
	if proxied.Load() != 0 {
		t.Fatal("the proxy was asked for a private destination")
	}
}

func TestPublicOnlyClientDoesNotFollowARedirect(t *testing.T) {
	var followed atomic.Int32
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Add(1) }))
	defer inner.Close()
	outer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, inner.URL+"/elsewhere", http.StatusTemporaryRedirect)
	}))
	defer outer.Close()
	client, err := New(Config{Endpoint: outer.URL, Bucket: "bucket", AccessKey: "key", SecretKey: "secret",
		PathStyle: boolPtr(true), PublicOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	// Reach the test server past the dial guard, keeping the redirect rule.
	client.http.Transport = s3Transport
	if _, err := client.Get(context.Background(), "object"); err == nil {
		t.Fatal("a redirected read succeeded")
	}
	if followed.Load() != 0 {
		t.Fatal("the redirect was followed")
	}

	// The instance's own client follows it, as it always has.
	open, err := New(Config{Endpoint: outer.URL, Bucket: "bucket", AccessKey: "key", SecretKey: "secret", PathStyle: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := open.Get(context.Background(), "object"); err != nil || followed.Load() != 1 {
		t.Fatalf("the instance's own client: %v after %d followed", err, followed.Load())
	}
}
