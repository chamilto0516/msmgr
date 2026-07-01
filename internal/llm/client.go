package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"msmgr/internal/config"
)

type Client struct {
	baseURL    string
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type chatJSONRequest struct {
	Model          string               `json:"model"`
	Messages       []Message            `json:"messages"`
	Temperature    float64              `json:"temperature,omitempty"`
	ResponseFormat chatJSONResponseForm `json:"response_format"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

type chatJSONResponseForm struct {
	Type       string         `json:"type"`
	JSONSchema chatJSONSchema `json:"json_schema"`
}

type chatJSONSchema struct {
	Name   string `json:"name"`
	Schema any    `json:"schema"`
}

func NewClient(cfg config.Config, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		return nil, fmt.Errorf("LLM base URL is required")
	}
	if strings.TrimSpace(cfg.LLM.APIKey) == "" {
		return nil, fmt.Errorf("LLM API key is required")
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		return nil, fmt.Errorf("LLM model is required")
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	maxTokens := cfg.LLM.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 64
	} else if maxTokens > 128 {
		maxTokens = 128
	}

	return &Client{
		baseURL:    cfg.LLM.BaseURL,
		apiKey:     cfg.LLM.APIKey,
		model:      cfg.LLM.Model,
		maxTokens:  maxTokens,
		httpClient: httpClient,
	}, nil
}

func (c *Client) GenerateTitle(ctx context.Context, content string) (string, error) {
	payload, err := json.Marshal(chatCompletionRequest{
		Model: c.model,
		Messages: []Message{
			{
				Role:    "system",
				Content: "Generate a concise document title. Return only the title text with no date, quotes, markdown, or explanation.",
			},
			{
				Role:    "user",
				Content: "Create a short descriptive title for this document. Keep it under 8 words.\n\n" + content,
			},
		},
		MaxTokens: c.maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("encode chat completion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build chat completion request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return "", fmt.Errorf("POST %s: unexpected status %s", req.URL.Path, resp.Status)
		}
		message := strings.TrimSpace(string(body))
		if message == "" {
			return "", fmt.Errorf("POST %s: unexpected status %s", req.URL.Path, resp.Status)
		}
		return "", fmt.Errorf("POST %s: unexpected status %s: %s", req.URL.Path, resp.Status, message)
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("POST %s: decode response: %w", req.URL.Path, err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("POST %s: empty choices", req.URL.Path)
	}

	title := cleanTitle(decoded.Choices[0].Message.Content)
	if title == "" {
		return "", fmt.Errorf("POST %s: empty title response", req.URL.Path)
	}

	return title, nil
}

func (c *Client) ChatJSON(ctx context.Context, messages []Message, schemaName string, schema any) (map[string]any, error) {
	payload, err := json.Marshal(chatJSONRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0.2,
		ResponseFormat: chatJSONResponseForm{
			Type: "json_schema",
			JSONSchema: chatJSONSchema{
				Name:   schemaName,
				Schema: schema,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode chat json request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build chat json request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return nil, fmt.Errorf("POST %s: unexpected status %s", req.URL.Path, resp.Status)
		}
		message := strings.TrimSpace(string(body))
		if message == "" {
			return nil, fmt.Errorf("POST %s: unexpected status %s", req.URL.Path, resp.Status)
		}
		return nil, fmt.Errorf("POST %s: unexpected status %s: %s", req.URL.Path, resp.Status, message)
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("POST %s: decode response: %w", req.URL.Path, err)
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("POST %s: empty choices", req.URL.Path)
	}

	content := decoded.Choices[0].Message.Content
	content = strings.TrimSpace(content)
	content = strings.Trim(content, "\"'")

	var result map[string]any
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("POST %s: decode json content: %w", req.URL.Path, err)
	}

	return result, nil
}

func cleanTitle(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'")
	lines := strings.Split(value, "\n")
	if len(lines) > 0 {
		value = strings.TrimSpace(lines[0])
	}
	return strings.Trim(value, "\"'")
}
