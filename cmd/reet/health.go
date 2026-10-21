package main

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// healthURL turns a listen address such as ":8080" into the local /healthz URL.
func healthURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = "8080"
	}
	return "http://127.0.0.1:" + port + "/healthz"
}

// healthcheck returns nil when the server at url answers 200; used by the container HEALTHCHECK.
func healthcheck(url string) error {
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %s", resp.Status)
	}
	return nil
}
