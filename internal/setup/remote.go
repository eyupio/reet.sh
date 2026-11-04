package setup

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

	"github.com/eyupio/reet.sh/internal/clientconfig"
	"github.com/eyupio/reet.sh/internal/profilehistory"
)

// HTTPRemote pushes and pulls named setup profiles through the controller API.
type HTTPRemote struct{}

func profileEndpoint(server, name string) (string, error) {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if server == "" {
		return "", errors.New("profile sync needs a reet server URL; set REET_URL or pass --server")
	}
	return server + "/api/v2/setup/profiles/" + url.PathEscape(name), nil
}

// serverMessage is the controller's own reason: its JSON "error" field, or the
// plain text body when it is not JSON.
func serverMessage(data []byte) string {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && body.Error != "" {
		return body.Error
	}
	return strings.TrimSpace(string(data))
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
		if authErr := clientconfig.NewAuthError(resp.StatusCode, serverMessage(data)); authErr != nil {
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("%w: %w", ErrProfileUnauthorized, authErr)
			}
			return nil, authErr
		}
		if message == "" {
			return nil, fmt.Errorf("server replied %s", resp.Status)
		}
		return nil, fmt.Errorf("server replied %s: %s", resp.Status, message)
	}
	return data, nil
}

func (HTTPRemote) Push(server, token string, profile SetupProfile) error {
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

func (HTTPRemote) Pull(server, token, name string) (SetupProfile, error) {
	endpoint, err := profileEndpoint(server, name)
	if err != nil {
		return SetupProfile{}, err
	}
	data, err := profileRequest(http.MethodGet, endpoint, token, nil)
	if err != nil {
		return SetupProfile{}, err
	}
	profile, err := ParseSetupProfile(data, name)
	if err != nil {
		return SetupProfile{}, fmt.Errorf("server returned an invalid setup profile: %w", err)
	}
	return profile, nil
}
