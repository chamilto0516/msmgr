package splitmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"msmgr/internal/llm"
)

type fakeLLMClient struct {
	responses map[string]map[string]any
}

func (f fakeLLMClient) ChatJSON(context.Context, []llm.Message, string, any) (map[string]any, error) {
	if f.responses == nil {
		return map[string]any{}, nil
	}
	if value, ok := f.responses["default"]; ok {
		return value, nil
	}
	return map[string]any{}, nil
}

func TestRunWritesChunksAndManifest(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "notes.md")
	outputDir := filepath.Join(dir, "out")
	content := "# Project Notes\n\nIntro paragraph.\n\n## Section One\n\nBody text.\n"
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout strings.Builder
	if err := Run(context.Background(), Options{
		InputPath: inputPath,
		OutputDir: outputDir,
	}, nil, &stdout); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	manifestPath := filepath.Join(outputDir, "manifest.jsonl")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(manifestBytes)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(lines))
	}

	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if first["source_file"] != "notes.md" {
		t.Fatalf("unexpected source_file %#v", first["source_file"])
	}
	if first["document_title"] != "Project Notes" {
		t.Fatalf("unexpected document_title %#v", first["document_title"])
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}

	mdCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") && entry.Name() != "manifest.jsonl" {
			mdCount++
		}
	}
	if mdCount != 2 {
		t.Fatalf("expected 2 chunk files, got %d", mdCount)
	}

	if got := stdout.String(); !strings.Contains(got, "Processed notes.md: 2 chunks") {
		t.Fatalf("unexpected stdout %q", got)
	}
}

func TestRunHandlesDocumentsWithoutHeadings(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "notes.md")
	outputDir := filepath.Join(dir, "out")
	if err := os.WriteFile(inputPath, []byte("Plain body text only.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := Run(context.Background(), Options{
		InputPath: inputPath,
		OutputDir: outputDir,
	}, nil, &strings.Builder{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	manifestPath := filepath.Join(outputDir, "manifest.jsonl")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest file, got error: %v", err)
	}
}
