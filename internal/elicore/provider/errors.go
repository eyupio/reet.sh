// SPDX-License-Identifier: AGPL-3.0-only
// Adapted from eyupio/zoomies, 3c4d09627f78afab72a6575e94624fb0e7710f3f.
// Original source: internal/assistant/provider/. See ../NOTICE.md for provenance.
package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/eyupio/reet.sh/internal/elicore"
)

// HTTPError is a response that was not a success, with the status kept so a
// caller can tell a missing route from a refusal without parsing the words.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) HTTPStatus() int { return e.Status }

func (e *HTTPError) Error() string { return e.Message }

// StatusError turns a response that is not a success into an error written
// for the person who will read it on the settings page. The request is
// never part of it: its headers hold the key, and a 401's body often quotes
// the key back, so the two statuses that mean "the key" say only that.
// hasKey is whether a key was sent at all: a refusal with no key configured
// sends the person to add one, not to rotate one they never set.
func StatusError(resp *http.Response, hasKey bool, keys ...string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return &HTTPError{Status: resp.StatusCode, Message: statusMessage(resp, hasKey, keys...)}
}

// A gateway can quote a key in a streamed error or a transport failure as
// well as in an authentication refusal. Keep cancellation/status inspection
// available without repeating that key in a user-facing error.
func hideKey(err error, key string) error {
	if err == nil || key == "" || !strings.Contains(err.Error(), key) {
		return err
	}
	return &hiddenKeyError{cause: err, message: strings.ReplaceAll(err.Error(), key, "[redacted]")}
}

type hiddenKeyError struct {
	cause   error
	message string
}

func (e *hiddenKeyError) Error() string { return e.message }
func (e *hiddenKeyError) Unwrap() error { return e.cause }

func keySafeDecoder(decode Decoder, key string) Decoder {
	return func(event ServerEvent) ([]elicore.Event, error) {
		events, err := decode(event)
		return events, hideKey(err, key)
	}
}

func statusMessage(resp *http.Response, hasKey bool, keys ...string) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	hide := func(value string) string {
		for _, key := range keys {
			if key != "" {
				value = strings.ReplaceAll(value, key, "[redacted]")
			}
		}
		return value
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		if !hasKey {
			return fmt.Sprintf("the provider needs an API key (HTTP %d)", resp.StatusCode)
		}
		return fmt.Sprintf("the provider refused the key (HTTP %d)", resp.StatusCode)
	case http.StatusNotFound:
		if m := modelName.FindSubmatch(body); m != nil {
			return fmt.Sprintf("no model named %s at the provider", hide(string(m[1])))
		}
		return "the provider answered 404: check the base URL and the model name"
	case http.StatusTooManyRequests:
		return "the provider rate limited the request (HTTP 429); try again shortly"
	}
	if msg := errorMessage(body); msg != "" {
		return fmt.Sprintf("the provider answered %d: %s", resp.StatusCode, hide(msg))
	}
	return fmt.Sprintf("the provider answered %d", resp.StatusCode)
}

// modelName finds the quoted model in the sentences both APIs write for an
// unknown model, so the error can say which name was wrong.
var modelName = regexp.MustCompile(`[mM]odel[: ]+['"\x60]?([A-Za-z0-9._:/-]+)['"\x60]?`)

// errorMessage reads the message both APIs put at error.message, and nothing
// when the body is not that shape.
func errorMessage(body []byte) string {
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return env.Error.Message
}
