// Package gaffer owns account provider configuration and bounded provider probes.
// Worker-session permissions and conversation coordination remain separate.
package gaffer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/elicore"
	"github.com/eyupio/reet.sh/internal/elicore/provider"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

var (
	ErrForbidden   = errors.New("only an owner or administrator can use providers on the controller's local network")
	ErrUnavailable = errors.New("the controller cannot unlock provider keys; check its instance key configuration")
	ErrBusy        = errors.New("Gaffer has enough provider requests on just now; try again shortly")
)

type Invalid struct {
	Field   string
	Message string
}

type ProbeError struct{ Message string }

func (e *ProbeError) Error() string { return e.Message }

func (e *Invalid) Error() string          { return e.Message }
func invalid(field, message string) error { return &Invalid{field, message} }

type Principal struct {
	UserID      int64
	CanUseLocal bool
}

type Input struct {
	ID              string  `json:"id"`
	Revision        int64   `json:"revision"`
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	BaseURL         string  `json:"baseUrl"`
	Model           string  `json:"model"`
	ReasoningEffort string  `json:"reasoningEffort"`
	Enabled         bool    `json:"enabled"`
	LocalOnly       bool    `json:"localOnly"`
	AllowStatus     bool    `json:"allowStatus"`
	AllowSessions   bool    `json:"allowSessions"`
	AllowCode       bool    `json:"allowCode"`
	APIKey          *string `json:"apiKey,omitempty"`
	ClearKey        bool    `json:"clearKey"`
	MakeDefault     bool    `json:"makeDefault"`
}

type Preset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	BaseURL   string `json:"baseUrl"`
	Help      string `json:"help"`
	LocalOnly bool   `json:"localOnly"`
}

func Presets() []Preset {
	return []Preset{
		{"openai", "OpenAI", "openai", "https://api.openai.com/v1", "Use an OpenAI API key. Your ChatGPT subscription and API billing are separate.", false},
		{"anthropic", "Anthropic", "anthropic", "https://api.anthropic.com", "Use an Anthropic API key for the Messages API.", false},
		{"ollama-cloud", "Ollama Cloud", "openai_compatible", "https://ollama.com/v1", "Use your Ollama Cloud key and a cloud model.", false},
		{"opencode-zen", "OpenCode Zen", "openai_compatible", "https://opencode.ai/zen/v1", "Choose a model served through the OpenAI chat protocol. Models requiring Messages or Responses need a different backend.", false},
		{"opencode-go", "OpenCode Go", "openai_compatible", "https://opencode.ai/zen/go/v1", "Use your OpenCode key and a model supported by its OpenAI chat endpoint.", false},
		{"ollama-local", "Local Ollama", "openai_compatible", "http://localhost:11434/v1", "Localhost means the reet controller. In Docker, use an address reachable from the container.", true},
		{"compatible", "Other compatible server", "openai_compatible", "", "An OpenAI chat-compatible endpoint, such as OpenRouter, LM Studio or vLLM. Include its API prefix.", false},
	}
}

func normalize(p Principal, in Input, requireModel bool) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.Model = strings.TrimSpace(in.Model)
	clean := func(s string, max int) bool {
		return utf8.ValidString(s) && len(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
	}
	if in.Name == "" || !clean(in.Name, 320) || utf8.RuneCountInString(in.Name) > 80 {
		return in, invalid("name", "Give this provider a name of up to 80 characters.")
	}
	switch in.Kind {
	case "openai":
		if in.BaseURL == "" {
			in.BaseURL = "https://api.openai.com/v1"
		}
	case "anthropic":
		if in.BaseURL == "" {
			in.BaseURL = "https://api.anthropic.com"
		}
	case "openai_compatible":
	default:
		return in, invalid("kind", "Choose an available API provider.")
	}
	u, err := url.Parse(in.BaseURL)
	if err != nil || u == nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !clean(in.BaseURL, 2048) || (u.Scheme != "http" && u.Scheme != "https") {
		return in, invalid("baseUrl", "Use an HTTP or HTTPS API address without a sign-in, query string or fragment.")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return in, invalid("baseUrl", "Use a valid port in the API address.")
		}
	}
	if in.LocalOnly && !p.CanUseLocal {
		return in, ErrForbidden
	}
	if !in.LocalOnly && u.Scheme != "https" {
		return in, invalid("baseUrl", "Use HTTPS for a hosted provider, or select Local network only for a local server.")
	}
	u.Host = strings.ToLower(u.Host)
	in.BaseURL = strings.TrimRight(u.String(), "/")
	if (requireModel && in.Model == "") || !clean(in.Model, 256) {
		return in, invalid("model", "Choose a model, or enter its exact name.")
	}
	if in.ReasoningEffort != "" && in.ReasoningEffort != "none" && in.ReasoningEffort != "low" && in.ReasoningEffort != "medium" && in.ReasoningEffort != "high" {
		return in, invalid("reasoningEffort", "Choose one of the available reasoning levels.")
	}
	if in.Kind == "anthropic" && in.ReasoningEffort != "" {
		return in, invalid("reasoningEffort", "This backend does not accept the OpenAI reasoning-effort setting.")
	}
	if in.AllowCode && !in.AllowSessions {
		return in, invalid("allowCode", "Enable followed-session context before allowing its code and logs.")
	}
	if in.MakeDefault && !in.Enabled {
		return in, invalid("enabled", "Enable this provider before making it the default.")
	}
	if in.APIKey != nil {
		key := strings.TrimSpace(*in.APIKey)
		if !clean(key, 4096) {
			return in, invalid("apiKey", "Use a key of at most 4096 bytes without control characters.")
		}
		in.APIKey = &key
	}
	if in.ClearKey && in.APIKey != nil && *in.APIKey != "" {
		return in, invalid("apiKey", "Choose either a replacement key or Remove the saved key.")
	}
	return in, nil
}

func inputFrom(p store.GafferProvider) Input {
	return Input{ID: p.ID, Revision: p.Revision, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, Model: p.Model, ReasoningEffort: p.ReasoningEffort, Enabled: p.Enabled, LocalOnly: p.LocalOnly, AllowStatus: p.AllowStatus, AllowSessions: p.AllowSessions, AllowCode: p.AllowCode}
}

type activeProbe struct {
	user     int64
	provider string
	cancel   context.CancelFunc
}
type Service struct {
	store     *store.Store
	sealer    *sealed.Sealer
	limiter   *auth.Limiter
	writes    *auth.Limiter
	sealSlots chan struct{}
	mu        sync.Mutex
	closed    bool
	serial    uint64
	active    map[uint64]activeProbe
	wg        sync.WaitGroup
}

func New(st *store.Store, s *sealed.Sealer) *Service {
	return &Service{store: st, sealer: s, limiter: auth.NewLimiter(12, time.Minute), writes: auth.NewLimiter(30, time.Minute), sealSlots: make(chan struct{}, 2), active: map[uint64]activeProbe{}}
}
func (s *Service) Available() bool { return s != nil && s.sealer != nil }

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	for _, a := range s.active {
		a.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) cancelProvider(uid int64, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.active {
		if a.user == uid && a.provider == id {
			a.cancel()
		}
	}
}

func (s *Service) begin(parent context.Context, p Principal, id string, timeout time.Duration) (context.Context, func(), error) {
	if !s.Available() {
		return nil, nil, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strconv.FormatInt(p.UserID, 10)
	if !s.limiter.Allow(key) {
		return nil, nil, ErrBusy
	}
	count := 0
	for _, a := range s.active {
		if a.user == p.UserID {
			count++
		}
	}
	if s.closed {
		return nil, nil, ErrUnavailable
	}
	if count >= 2 || len(s.active) >= 4 {
		return nil, nil, ErrBusy
	}
	s.limiter.Hit(key)
	ctx, cancel := context.WithTimeout(parent, timeout)
	s.serial++
	idn := s.serial
	s.active[idn] = activeProbe{p.UserID, id, cancel}
	s.wg.Add(1)
	return ctx, func() { cancel(); s.mu.Lock(); delete(s.active, idn); s.mu.Unlock(); s.wg.Done() }, nil
}

func (s *Service) List(p Principal) ([]store.GafferProvider, error) {
	return s.store.GafferProvidersFor(p.UserID)
}

func (s *Service) owned(p Principal, in Input) (store.GafferProvider, error) {
	if in.ID == "" {
		return store.GafferProvider{}, nil
	}
	row, err := s.store.GafferProviderFor(p.UserID, in.ID)
	if err != nil {
		return row, err
	}
	if in.Revision != row.Revision {
		return row, store.ErrGafferConflict
	}
	return row, nil
}

type keyEnvelope struct {
	Owner                  int64
	ID, Kind, Address, Key string
}

func (s *Service) credential(p store.GafferProvider) (string, error) {
	if p.KeySealed == "" {
		return "", nil
	}
	if !s.Available() {
		return "", ErrUnavailable
	}
	select {
	case s.sealSlots <- struct{}{}:
		defer func() { <-s.sealSlots }()
	default:
		return "", ErrBusy
	}
	raw, err := s.sealer.OpenSecret(p.KeySealed)
	if err != nil {
		return "", ErrUnavailable
	}
	var e keyEnvelope
	if json.Unmarshal([]byte(raw), &e) != nil || e.Owner != p.OwnerID || e.ID != p.ID || e.Kind != p.Kind || e.Address != p.BaseURL {
		return "", ErrUnavailable
	}
	return e.Key, nil
}

func (s *Service) Save(p Principal, in Input) (store.GafferProvider, error) {
	if !s.Available() {
		return store.GafferProvider{}, ErrUnavailable
	}
	old, err := s.owned(p, in)
	if err != nil {
		return old, err
	}
	s.mu.Lock()
	keyID := strconv.FormatInt(p.UserID, 10)
	if s.closed {
		s.mu.Unlock()
		return old, ErrUnavailable
	}
	if !s.writes.Allow(keyID) {
		s.mu.Unlock()
		return old, ErrBusy
	}
	s.writes.Hit(keyID)
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	in, err = normalize(p, in, true)
	if err != nil {
		return old, err
	}
	id := in.ID
	if id == "" {
		var b [16]byte
		if _, err = rand.Read(b[:]); err != nil {
			return old, err
		}
		id = hex.EncodeToString(b[:])
	}
	row := store.GafferProvider{ID: id, OwnerID: p.UserID, Revision: in.Revision, Name: in.Name, Kind: in.Kind, BaseURL: in.BaseURL, Model: in.Model, ReasoningEffort: in.ReasoningEffort, Enabled: in.Enabled, Default: in.MakeDefault, LocalOnly: in.LocalOnly, AllowStatus: in.AllowStatus, AllowSessions: in.AllowSessions, AllowCode: in.AllowCode}
	if in.APIKey == nil && !in.ClearKey && old.Kind == in.Kind && old.BaseURL == in.BaseURL {
		row.KeySealed = old.KeySealed
	}
	if in.APIKey != nil && *in.APIKey != "" && !in.ClearKey {
		select {
		case s.sealSlots <- struct{}{}:
			defer func() { <-s.sealSlots }()
		default:
			return old, ErrBusy
		}
		raw, _ := json.Marshal(keyEnvelope{p.UserID, id, in.Kind, in.BaseURL, *in.APIKey})
		row.KeySealed, err = s.sealer.SealSecret(string(raw))
		if err != nil {
			return old, err
		}
	}
	row, err = s.store.SaveGafferProviderFor(p.UserID, row)
	if err == nil {
		s.cancelProvider(p.UserID, id)
	}
	return row, err
}

func (s *Service) Delete(p Principal, id string, revision int64) error {
	if err := s.store.DeleteGafferProviderFor(p.UserID, id, revision); err != nil {
		return err
	}
	s.cancelProvider(p.UserID, id)
	return nil
}
func (s *Service) SetDefault(p Principal, id string, revision int64) error {
	row, err := s.owned(p, Input{ID: id, Revision: revision})
	if err != nil {
		return err
	}
	if !row.Enabled {
		return invalid("enabled", "Enable this provider before making it the default.")
	}
	if _, err = normalize(p, inputFrom(row), true); err != nil {
		return err
	}
	return s.store.DefaultGafferProviderFor(p.UserID, id, revision)
}
func (s *Service) SetEnabled(p Principal, id string, revision int64, on bool) error {
	row, err := s.owned(p, Input{ID: id, Revision: revision})
	if err != nil {
		return err
	}
	if on {
		if !s.Available() {
			return ErrUnavailable
		}
		if _, err = normalize(p, inputFrom(row), true); err != nil {
			return err
		}
	}
	row.Enabled = on
	_, err = s.store.SaveGafferProviderFor(p.UserID, row)
	if err == nil {
		s.cancelProvider(p.UserID, id)
	}
	return err
}

func (s *Service) open(p Principal, in Input, old store.GafferProvider) (elicore.Provider, *http.Client, error) {
	key := ""
	if in.APIKey != nil {
		key = *in.APIKey
	} else if !in.ClearKey && old.KeySealed != "" {
		if in.Kind != old.Kind || in.BaseURL != old.BaseURL {
			return nil, nil, invalid("apiKey", "Enter a key for the changed API address, or remove the saved key.")
		}
		var err error
		key, err = s.credential(old)
		if err != nil {
			return nil, nil, err
		}
	}
	client := newProviderClient(in.LocalOnly)
	config := provider.Config{BaseURL: in.BaseURL, APIKey: key, Model: in.Model, ReasoningEffort: in.ReasoningEffort, Client: client}
	if in.Kind == "anthropic" {
		return provider.NewAnthropic(config), client, nil
	}
	return provider.NewOpenAICompatible(config), client, nil
}

func (s *Service) current(p Principal, old store.GafferProvider) error {
	if old.ID == "" {
		return nil
	}
	now, err := s.store.GafferProviderFor(p.UserID, old.ID)
	if err != nil || now.Revision != old.Revision {
		return store.ErrGafferConflict
	}
	return nil
}

func (s *Service) Models(parent context.Context, p Principal, in Input) ([]string, error) {
	old, err := s.owned(p, in)
	if err != nil {
		return nil, err
	}
	if old.ID != "" && in.Kind == "" {
		in = inputFrom(old)
	}
	in, err = normalize(p, in, false)
	if err != nil {
		return nil, err
	}
	ctx, done, err := s.begin(parent, p, in.ID, 20*time.Second)
	if err != nil {
		return nil, err
	}
	defer done()
	if err = s.current(p, old); err != nil {
		return nil, err
	}
	backend, client, err := s.open(p, in, old)
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	models, err := backend.(elicore.ModelLister).Models(ctx)
	if changed := s.current(p, old); changed != nil {
		return nil, changed
	}
	if err != nil {
		return nil, &ProbeError{Message: err.Error()}
	}
	return models, err
}

func (s *Service) Test(parent context.Context, p Principal, id string, revision int64) (store.GafferCheck, error) {
	var check store.GafferCheck
	old, err := s.owned(p, Input{ID: id, Revision: revision})
	if err != nil {
		return check, err
	}
	in, err := normalize(p, inputFrom(old), true)
	if err != nil {
		return check, err
	}
	ctx, done, err := s.begin(parent, p, id, 45*time.Second)
	if err != nil {
		return check, err
	}
	defer done()
	if err = s.current(p, old); err != nil {
		return check, err
	}
	backend, client, err := s.open(p, in, old)
	if err != nil {
		return check, err
	}
	defer client.CloseIdleConnections()
	result, probeErr := backend.Check(ctx)
	if err = s.current(p, old); err != nil {
		return check, err
	}
	if parent.Err() != nil {
		return check, parent.Err()
	}
	check = store.GafferCheck{At: time.Now().UnixMilli(), OK: probeErr == nil, Model: result.Model, LatencyMS: result.Latency.Milliseconds(), UsageReported: result.UsageReported}
	if probeErr != nil {
		check.Error = probeErr.Error()
		if len(check.Error) > 1000 {
			check.Error = strings.ToValidUTF8(check.Error[:1000], "") + "…"
		}
	}
	if err = s.store.RecordGafferCheckFor(p.UserID, id, revision, check); err != nil {
		return check, err
	}
	return check, nil
}
