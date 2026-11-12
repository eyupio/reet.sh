// SPDX-License-Identifier: AGPL-3.0-only
// Adapted from eyupio/zoomies, 3c4d09627f78afab72a6575e94624fb0e7710f3f.
// Original source: internal/assistant/provider/. See ../NOTICE.md for provenance.
package provider

import "net/http"

// Config is what an adapter needs: where, with what key, which model, and
// the client to use, which is where the assistant's dialer lives.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	// ReasoningEffort is sent as reasoning_effort on every chat-completion
	// request when set, the check included, so a model that does not take it
	// says so under Test and not in the middle of a conversation.
	ReasoningEffort string
	// PromptCacheKey is sent as prompt_cache_key on chat-completion requests
	// when set, for servers that route cached prompts by a stable key. Leave it
	// empty for servers that may refuse the field. Anthropic takes cache
	// breakpoints instead and ignores it.
	PromptCacheKey string
	Client         *http.Client
}

func (c Config) client() *http.Client {
	client := *http.DefaultClient
	if c.Client != nil {
		client = *c.Client
	}
	// Model APIs have fixed endpoints. In particular, x-api-key is not one
	// of the headers net/http removes when following a redirect to a new host.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client
}
