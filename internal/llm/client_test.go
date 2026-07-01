package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"msmgr/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestGenerateTitle(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer key123" {
				t.Fatalf("unexpected auth header %q", got)
			}

			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload["model"] != "bulk-model" {
				t.Fatalf("unexpected model %#v", payload["model"])
			}
			if payload["max_tokens"] != float64(64) {
				t.Fatalf("unexpected max_tokens %#v", payload["max_tokens"])
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"Context Growth Notes"}}]}`)),
				Request:    r,
			}, nil
		}),
	}

	client, err := NewClient(config.Config{
		LLM: config.LLMConfig{
			BaseURL:   "http://example.test/v1",
			APIKey:    "key123",
			Model:     "bulk-model",
			MaxTokens: 64,
		},
	}, httpClient)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	title, err := client.GenerateTitle(context.Background(), "Some long content")
	if err != nil {
		t.Fatalf("GenerateTitle returned error: %v", err)
	}

	if title != "Context Growth Notes" {
		t.Fatalf("unexpected title %q", title)
	}
}

func TestGenerateTitleCapsMaxTokensAt128(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload["max_tokens"] != float64(128) {
				t.Fatalf("unexpected max_tokens %#v", payload["max_tokens"])
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"Context Growth Notes"}}]}`)),
				Request:    r,
			}, nil
		}),
	}

	client, err := NewClient(config.Config{
		LLM: config.LLMConfig{
			BaseURL:   "http://example.test/v1",
			APIKey:    "key123",
			Model:     "bulk-model",
			MaxTokens: 65536,
		},
	}, httpClient)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	if _, err := client.GenerateTitle(context.Background(), "Some long content"); err != nil {
		t.Fatalf("GenerateTitle returned error: %v", err)
	}
}

func TestChatJSON(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if payload["model"] != "bulk-model" {
				t.Fatalf("unexpected model %#v", payload["model"])
			}
			if _, ok := payload["response_format"]; !ok {
				t.Fatal("expected response_format in payload")
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"descriptor\":\"alpha_section\"}"}}]}`)),
				Request:    r,
			}, nil
		}),
	}

	client, err := NewClient(config.Config{
		LLM: config.LLMConfig{
			BaseURL:   "http://example.test/v1",
			APIKey:    "key123",
			Model:     "bulk-model",
			MaxTokens: 64,
		},
	}, httpClient)
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	result, err := client.ChatJSON(context.Background(), []Message{{Role: "user", Content: "hello"}}, "chunk_descriptor", map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("ChatJSON returned error: %v", err)
	}

	if result["descriptor"] != "alpha_section" {
		t.Fatalf("unexpected result %#v", result)
	}
}
