package skillfetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
)

// The text of a failed connection names the addresses it tried: this server's
// resolver, its proxy, the host a writer typed. None of that is the writer's to
// read, so a source that cannot be reached says only that.
func TestFetchHidesWhereAConnectionFailed(t *testing.T) {
	// A proxy that is down: the dial error names the proxy's address.
	down := httptest.NewServer(http.NotFoundHandler())
	downURL, _ := url.Parse(down.URL)
	down.Close()
	for _, tc := range []struct {
		name string
		f    Fetcher
		leak []string
	}{
		{"dial", Fetcher{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 443}, Err: errors.New("connect: connection refused")}
		})}}, []string{"10.0.0.7", "dial", "refused"}},
		{"resolve", Fetcher{
			proxy: func(*http.Request) (*url.URL, error) { return downURL, nil },
			lookup: func(context.Context, string) ([]net.IP, error) {
				return nil, errors.New("lookup intranet.example on 10.0.0.2:53: no such host")
			},
		}, []string{"10.0.0.2", ":53", "lookup"}},
		{"proxy", Fetcher{
			proxy:  func(*http.Request) (*url.URL, error) { return downURL, nil },
			lookup: fakeLookup(map[string]string{"example.test": "140.82.112.3"}),
		}, []string{downURL.Host, "127.0.0.1"}},
	} {
		_, err := tc.f.Fetch(context.Background(), ref.Ref{Source: "example.test", Slug: "demo", SourceType: "well-known", URL: "https://x"})
		if !errors.Is(err, ErrUnreachable) {
			t.Errorf("%s: err = %v", tc.name, err)
			continue
		}
		for _, leak := range tc.leak {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: %q is in %q", tc.name, leak, err)
			}
		}
	}
}

// The guard's refusal is still told as what it is, and a deadline as one.
func TestFetchStillSaysWhyTheGuardRefusedAndWhenItTimedOut(t *testing.T) {
	_, r := wellKnownServer(t, demoHandler())
	_, err := Fetcher{}.Fetch(context.Background(), r)
	if !errors.Is(err, errAddress) || !strings.Contains(err.Error(), "not one this server may connect to") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("loopback: %v", err)
	}
	stuck := Fetcher{timeout: time.Millisecond, Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 443}, Err: req.Context().Err()}
	})}}
	if _, err = stuck.Fetch(context.Background(), r); !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "10.0.0.7") {
		t.Fatalf("deadline: %v", err)
	}
}
