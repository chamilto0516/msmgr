package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromEnvDefaultsHTTPAddr(t *testing.T) {
	t.Setenv("MSMGR_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("MEILI_HTTP_ADDR", "")
	t.Setenv("MEILI_API_KEY", "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if cfg.HTTPAddr != "http://localhost:7700" {
		t.Fatalf("unexpected HTTPAddr %q", cfg.HTTPAddr)
	}
}

func TestLoadFromEnvTrimsValues(t *testing.T) {
	t.Setenv("MSMGR_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("MEILI_HTTP_ADDR", " http://example.test:7700/ ")
	t.Setenv("MEILI_API_KEY", " secret ")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if cfg.HTTPAddr != "http://example.test:7700" {
		t.Fatalf("unexpected HTTPAddr %q", cfg.HTTPAddr)
	}

	if cfg.APIKey != "secret" {
		t.Fatalf("unexpected APIKey %q", cfg.APIKey)
	}
}

func TestLoadFromEnvRejectsRelativeURL(t *testing.T) {
	t.Setenv("MSMGR_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("MEILI_HTTP_ADDR", "/tmp/meili")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error for relative URL")
	}
}

func TestLoadFromEnvReadsCurrentEnvironment(t *testing.T) {
	originalAddr, hadAddr := os.LookupEnv("MEILI_HTTP_ADDR")
	originalKey, hadKey := os.LookupEnv("MEILI_API_KEY")
	t.Cleanup(func() {
		if hadAddr {
			os.Setenv("MEILI_HTTP_ADDR", originalAddr)
		} else {
			os.Unsetenv("MEILI_HTTP_ADDR")
		}
		if hadKey {
			os.Setenv("MEILI_API_KEY", originalKey)
		} else {
			os.Unsetenv("MEILI_API_KEY")
		}
	})
	os.Unsetenv("MSMGR_CONFIG")

	os.Setenv("MEILI_HTTP_ADDR", "http://localhost:8800")
	os.Setenv("MEILI_API_KEY", "abc123")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if cfg.HTTPAddr != "http://localhost:8800" || cfg.APIKey != "abc123" {
		t.Fatalf("unexpected config %#v", cfg)
	}
}

func TestLoadFromEnvReadsConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "msmgr.json")
	content := []byte(`{
  "meili": {"http_addr": "http://meili.test:7700", "api_key": "meili-key"},
  "llm": {"base_url": "http://llm.test/v1", "api_key": "llm-key", "model": "bulk-model", "max_tokens": 77}
}`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	t.Setenv("MSMGR_CONFIG", path)
	t.Setenv("MEILI_HTTP_ADDR", "")
	t.Setenv("MEILI_API_KEY", "")
	t.Setenv("MSMGR_LLM_BASE_URL", "")
	t.Setenv("MSMGR_LLM_API_KEY", "")
	t.Setenv("MSMGR_LLM_MODEL", "")
	t.Setenv("MSMGR_LLM_MAX_TOKENS", "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if cfg.HTTPAddr != "http://meili.test:7700" || cfg.APIKey != "meili-key" {
		t.Fatalf("unexpected meili config %#v", cfg)
	}
	if cfg.LLM.BaseURL != "http://llm.test/v1" || cfg.LLM.APIKey != "llm-key" || cfg.LLM.Model != "bulk-model" || cfg.LLM.MaxTokens != 77 {
		t.Fatalf("unexpected llm config %#v", cfg.LLM)
	}
}
