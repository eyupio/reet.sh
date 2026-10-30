// Package health checks a reet server's /healthz endpoint.
package health

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Result is one health check. Status and Latency are zero when the request
// could not be made at all.
type Result struct {
	URL       string
	Status    int
	OK        bool
	Latency   time.Duration
	CheckedAt time.Time
}

// URLFromAddr turns a listen address such as ":8080" into the local /healthz
// URL used by the container health check and reet healthcheck --local.
func URLFromAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "8080"
	}
	return "http://127.0.0.1:" + port + "/healthz"
}

// Endpoint accepts a server base URL or a /healthz URL and returns the exact
// endpoint to request. Credentials, queries and fragments are refused.
func Endpoint(target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("--url must be an HTTP(S) server URL without credentials, query or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	if strings.HasSuffix(u.Path, "/healthz") {
		return u.String(), nil
	}
	endpoint, err := url.JoinPath(u.String(), "healthz")
	if err != nil {
		return "", errors.New("invalid healthcheck URL")
	}
	return endpoint, nil
}

// Check requests endpoint and returns what it answered. A non-200 answer is
// both a Result and an error so callers can show the status that was seen.
func Check(ctx context.Context, endpoint string) (Result, error) {
	result := Result{URL: endpoint, CheckedAt: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	start := time.Now()
	resp, err := client.Do(req)
	result.Latency = time.Since(start)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	result.Status = resp.StatusCode
	result.OK = resp.StatusCode == http.StatusOK
	if !result.OK {
		return result, fmt.Errorf("health check returned %s", resp.Status)
	}
	return result, nil
}
