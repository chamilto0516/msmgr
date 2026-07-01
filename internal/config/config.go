package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultHTTPAddr = "http://localhost:7700"
const defaultConfigPath = "msmgr.json"

type Config struct {
	HTTPAddr string
	APIKey   string
	LLM      LLMConfig
}

type LLMConfig struct {
	BaseURL   string
	APIKey    string
	Model     string
	MaxTokens int
}

func LoadFromEnv() (Config, error) {
	cfg, err := loadFromFile()
	if err != nil {
		return Config{}, err
	}

	if value := strings.TrimSpace(os.Getenv("MEILI_HTTP_ADDR")); value != "" {
		cfg.HTTPAddr = value
	}
	if value := strings.TrimSpace(os.Getenv("MEILI_API_KEY")); value != "" {
		cfg.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("MSMGR_LLM_BASE_URL")); value != "" {
		cfg.LLM.BaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("MSMGR_LLM_API_KEY")); value != "" {
		cfg.LLM.APIKey = value
	}
	if value := strings.TrimSpace(os.Getenv("MSMGR_LLM_MODEL")); value != "" {
		cfg.LLM.Model = value
	}
	if value := strings.TrimSpace(os.Getenv("MSMGR_LLM_MAX_TOKENS")); value != "" {
		maxTokens, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse MSMGR_LLM_MAX_TOKENS: %w", err)
		}
		cfg.LLM.MaxTokens = maxTokens
	}

	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
	}

	httpAddr, err := normalizeURL(cfg.HTTPAddr, "MEILI_HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}
	cfg.HTTPAddr = httpAddr

	if cfg.LLM.BaseURL != "" {
		baseURL, err := normalizeURL(cfg.LLM.BaseURL, "LLM base URL")
		if err != nil {
			return Config{}, err
		}
		cfg.LLM.BaseURL = baseURL
	}

	return cfg, nil
}

func loadFromFile() (Config, error) {
	path := strings.TrimSpace(os.Getenv("MSMGR_CONFIG"))
	if path == "" {
		path = defaultConfigPath
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	type fileConfig struct {
		Meili struct {
			HTTPAddr string `json:"http_addr"`
			APIKey   string `json:"api_key"`
		} `json:"meili"`
		LLM struct {
			BaseURL   string `json:"base_url"`
			APIKey    string `json:"api_key"`
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		} `json:"llm"`
	}

	var raw fileConfig
	if err := json.Unmarshal(content, &raw); err != nil {
		return Config{}, fmt.Errorf("parse config file %s: %w", path, err)
	}

	return Config{
		HTTPAddr: strings.TrimSpace(raw.Meili.HTTPAddr),
		APIKey:   strings.TrimSpace(raw.Meili.APIKey),
		LLM: LLMConfig{
			BaseURL:   strings.TrimSpace(raw.LLM.BaseURL),
			APIKey:    strings.TrimSpace(raw.LLM.APIKey),
			Model:     strings.TrimSpace(raw.LLM.Model),
			MaxTokens: raw.LLM.MaxTokens,
		},
	}, nil
}

func normalizeURL(value, label string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", label, err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%s must be an absolute URL", label)
	}

	return strings.TrimRight(parsed.String(), "/"), nil
}
