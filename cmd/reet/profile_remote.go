package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/profilehistory"
	"github.com/eyupio/reet.sh/internal/setup"
)

type profileRemote struct{}

func profileEndpoint(server, name string) (string, error) {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		return "", errors.New("profile sync needs a reet server URL; set REET_URL or pass --server")
	}
	return server + "/api/v2/setup/profiles/" + url.PathEscape(name), nil
}

func profileRequest(method, endpoint, token string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Reet-Token", token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(data))
		if resp.StatusCode == http.StatusConflict {
			return nil, profilehistory.ErrConflict
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("%w: server replied 401 Unauthorized; use a personal API token from Account on the target server via REET_TOKEN or --token", setup.ErrProfileUnauthorized)
		}
		if message == "" {
			return nil, fmt.Errorf("server replied %s", resp.Status)
		}
		return nil, fmt.Errorf("server replied %s: %s", resp.Status, message)
	}
	return data, nil
}

func (profileRemote) Push(server, token string, profile setup.SetupProfile) error {
	endpoint, err := profileEndpoint(server, profile.Name)
	if err != nil {
		return err
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	_, err = profileRequest(http.MethodPut, endpoint, token, data)
	return err
}

func (profileRemote) Pull(server, token, name string) (setup.SetupProfile, error) {
	endpoint, err := profileEndpoint(server, name)
	if err != nil {
		return setup.SetupProfile{}, err
	}
	data, err := profileRequest(http.MethodGet, endpoint, token, nil)
	if err != nil {
		return setup.SetupProfile{}, err
	}
	profile, err := setup.ParseSetupProfile(data, name)
	if err != nil {
		return setup.SetupProfile{}, fmt.Errorf("server returned an invalid setup profile: %w", err)
	}
	return profile, nil
}
