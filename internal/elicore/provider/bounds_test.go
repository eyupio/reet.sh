package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/eyupio/reet.sh/internal/elicore"
)

func apiProviders() map[string]func(Config) elicore.Provider {
	return map[string]func(Config) elicore.Provider{
		"OpenAI compatible": func(c Config) elicore.Provider { return NewOpenAICompatible(c) },
		"Anthropic":         func(c Config) elicore.Provider { return NewAnthropic(c) },
	}
}

func TestProviderRequestsDoNotFollowRedirectsWithCredentials(t *testing.T) {
	for name, open := range apiProviders() {
		t.Run(name, func(t *testing.T) {
			var visits atomic.Int64
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				visits.Add(1)
				fmt.Fprint(w, `{"data":[]}`)
			}))
			t.Cleanup(destination.Close)
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
			}))
			t.Cleanup(source.Close)
			for _, client := range []*http.Client{nil, source.Client()} {
				p := open(Config{BaseURL: source.URL, APIKey: "synthetic-key", Client: client})
				_, err := p.(elicore.ModelLister).Models(context.Background())
				if err == nil || visits.Load() != 0 {
					t.Fatalf("redirect: error=%v, destination requests=%d", err, visits.Load())
				}
			}
		})
	}
}

func TestProviderErrorsDoNotRepeatTheConfiguredKey(t *testing.T) {
	const key = "synthetic-sensitive-provider-key"
	for name, open := range apiProviders() {
		for _, status := range []int{200, 400, 404, 500} {
			t.Run(fmt.Sprintf("%s/%d", name, status), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(status)
					body := `{"type":"error","error":{"message":"Model ` + key + ` was refused"}}`
					if status == 200 {
						fmt.Fprintf(w, "data: %s\n\n", body)
					} else {
						fmt.Fprint(w, body)
					}
				}))
				t.Cleanup(srv.Close)
				p := open(Config{BaseURL: srv.URL, Model: "m", APIKey: key})
				s, err := p.Chat(context.Background(), hello())
				if err == nil {
					defer s.Close()
					e, ok := s.Next(context.Background())
					if !ok {
						t.Fatal("missing provider error event")
					}
					err = e.Err
				}
				if err == nil || strings.Contains(err.Error(), key) {
					t.Fatalf("provider error must be present and hide the configured key: %v", err)
				}
			})
		}
	}
}

func TestReadEventsRejectsOversizedMultilineData(t *testing.T) {
	input := strings.Repeat("data: "+strings.Repeat("x", 512)+"\n", 4096) + "\n"
	for e, err := range ReadEvents(strings.NewReader(input)) {
		if err != nil {
			return
		}
		t.Fatalf("oversized event was returned with %d bytes", len(e.Data))
	}
	t.Fatal("missing size-limit error")
}

func TestOpenAIToolIndexesCannotPanicOrAllocateAnUnboundedList(t *testing.T) {
	for _, index := range []int{-1, 4096} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("provider frame panicked: %v", p)
				}
			}()
			frame := fmt.Sprintf(`{"choices":[{"delta":{"tool_calls":[{"index":%d,"id":"c","function":{"name":"read","arguments":"{}"}}]}}]}`, index)
			if _, err := newOpenAIDecoder()(ServerEvent{Data: frame}); err == nil {
				t.Fatal("invalid tool index accepted")
			}
		})
	}
}

func TestToolArgumentsHaveABoundAcrossStreamFrames(t *testing.T) {
	fragment := strings.Repeat("x", 16<<10)
	encoded, _ := json.Marshal(fragment)
	for name, decode := range map[string]Decoder{"OpenAI": newOpenAIDecoder(), "Anthropic": newAnthropicDecoder()} {
		t.Run(name, func(t *testing.T) {
			frame := `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":` + string(encoded) + `}}]}}]}`
			if name == "Anthropic" {
				if _, err := decode(ServerEvent{Data: `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"c","name":"read"}}`}); err != nil {
					t.Fatal(err)
				}
				frame = `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + string(encoded) + `}}`
			}
			for i := 0; i < 80; i++ {
				if _, err := decode(ServerEvent{Data: frame}); err != nil {
					return
				}
			}
			t.Fatal("oversized tool arguments accepted")
		})
	}
}

func TestCancellationDiscardsBufferedToolCalls(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: one\n\n"))}
	s := NewStream(resp, func(ServerEvent) ([]elicore.Event, error) {
		return []elicore.Event{{Delta: "hello"}, {ToolCall: &elicore.ToolCall{Name: "read"}}}, nil
	})
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e, ok := s.Next(ctx); !ok || e.Delta != "hello" {
		t.Fatalf("first event: %+v %v", e, ok)
	}
	cancel()
	if e, ok := s.Next(ctx); !ok || !errors.Is(e.Err, context.Canceled) || e.ToolCall != nil {
		t.Fatalf("event after cancellation: %+v %v", e, ok)
	}
	if _, ok := s.Next(ctx); ok {
		t.Fatal("event after cancellation error")
	}
}

func TestAnthropicToolBlocksRejectInvalidIndexesDuplicatesAndExcess(t *testing.T) {
	frame := func(index int) ServerEvent {
		return ServerEvent{Data: fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"c","name":"read"}}`, index)}
	}
	if _, err := newAnthropicDecoder()(frame(-1)); err == nil {
		t.Fatal("negative tool-block index accepted")
	}
	decode := newAnthropicDecoder()
	if _, err := decode(frame(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := decode(frame(0)); err == nil {
		t.Fatal("duplicate active tool block accepted")
	}
	decode = newAnthropicDecoder()
	for i := 0; i < 32; i++ {
		if _, err := decode(frame(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := decode(frame(32)); err == nil {
		t.Fatal("excess active tool blocks accepted")
	}
}
