package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"msmgr/internal/config"
	"msmgr/internal/meili"
)

type collisionClient struct {
	apiClient
	missingAfter map[string]bool
}

func (c collisionClient) GetDocument(_ context.Context, _, documentID string) (meili.Document, error) {
	if c.missingAfter[documentID] {
		return nil, &meili.HTTPError{
			Method:     http.MethodGet,
			Path:       fmt.Sprintf("/indexes/test/documents/%s", documentID),
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Body:       `{"message":"not found"}`,
		}
	}
	return meili.Document{"id": documentID}, nil
}

type stubHealthClient struct {
	health meili.Health
	err    error
}

func (s stubHealthClient) Health(context.Context) (meili.Health, error) {
	return s.health, s.err
}

func (s stubHealthClient) ListIndexes(context.Context) ([]meili.Index, error) {
	return nil, s.err
}

func (s stubHealthClient) GetIndex(context.Context, string) (meili.Index, error) {
	return meili.Index{}, s.err
}

func (s stubHealthClient) ListDocuments(context.Context, string, int, int) ([]meili.Document, error) {
	return nil, s.err
}

func (s stubHealthClient) GetDocument(context.Context, string, string) (meili.Document, error) {
	return nil, s.err
}

func (s stubHealthClient) SearchIndex(context.Context, string, string, int) (meili.SearchResult, error) {
	return meili.SearchResult{}, s.err
}

func (s stubHealthClient) CreateIndex(context.Context, string, string) (meili.Task, error) {
	return meili.Task{}, s.err
}

func (s stubHealthClient) DeleteIndex(context.Context, string) (meili.Task, error) {
	return meili.Task{}, s.err
}

func (s stubHealthClient) DeleteDocument(context.Context, string, string) (meili.Task, error) {
	return meili.Task{}, s.err
}

func (s stubHealthClient) DeleteAllDocuments(context.Context, string) (meili.Task, error) {
	return meili.Task{}, s.err
}

func (s stubHealthClient) AddDocuments(context.Context, string, []meili.Document) (meili.Task, error) {
	return meili.Task{}, s.err
}

func (s stubHealthClient) GetTask(context.Context, int) (meili.Task, error) {
	return meili.Task{}, s.err
}

type stubAPIClient struct {
	health     meili.Health
	indexes    []meili.Index
	index      meili.Index
	documents  []meili.Document
	document   meili.Document
	searches   map[string]meili.SearchResult
	searchLog  *[]searchCall
	created    meili.Task
	deleted    meili.Task
	deletedAll meili.Task
	added      *[]meili.Document
	taskQueue  *[]meili.Task
	err        error
}

type searchCall struct {
	indexUID string
	query    string
	limit    int
}

func (s stubAPIClient) Health(context.Context) (meili.Health, error) {
	return s.health, s.err
}

func (s stubAPIClient) ListIndexes(context.Context) ([]meili.Index, error) {
	return s.indexes, s.err
}

func (s stubAPIClient) GetIndex(context.Context, string) (meili.Index, error) {
	return s.index, s.err
}

func (s stubAPIClient) ListDocuments(context.Context, string, int, int) ([]meili.Document, error) {
	return s.documents, s.err
}

func (s stubAPIClient) GetDocument(context.Context, string, string) (meili.Document, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.document == nil {
		return nil, &meili.HTTPError{
			Method:     http.MethodGet,
			Path:       "/indexes/test/documents/missing",
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Body:       `{"message":"not found"}`,
		}
	}
	return s.document, s.err
}

func (s stubAPIClient) SearchIndex(_ context.Context, indexUID, query string, limit int) (meili.SearchResult, error) {
	if s.err != nil {
		return meili.SearchResult{}, s.err
	}
	if s.searchLog != nil {
		*s.searchLog = append(*s.searchLog, searchCall{indexUID: indexUID, query: query, limit: limit})
	}

	return s.searches[indexUID], nil
}

func (s stubAPIClient) CreateIndex(context.Context, string, string) (meili.Task, error) {
	return s.created, s.err
}

func (s stubAPIClient) DeleteIndex(context.Context, string) (meili.Task, error) {
	return s.deleted, s.err
}

func (s stubAPIClient) DeleteDocument(context.Context, string, string) (meili.Task, error) {
	return s.deleted, s.err
}

func (s stubAPIClient) DeleteAllDocuments(context.Context, string) (meili.Task, error) {
	if s.deletedAll != (meili.Task{}) {
		return s.deletedAll, s.err
	}
	return s.deleted, s.err
}

func (s stubAPIClient) AddDocuments(_ context.Context, _ string, documents []meili.Document) (meili.Task, error) {
	if s.added != nil {
		*s.added = append([]meili.Document(nil), documents...)
	}
	return s.created, s.err
}

func (s stubAPIClient) GetTask(context.Context, int) (meili.Task, error) {
	if s.taskQueue != nil && len(*s.taskQueue) > 0 {
		task := (*s.taskQueue)[0]
		*s.taskQueue = (*s.taskQueue)[1:]
		return task, nil
	}
	return meili.Task{Status: "succeeded"}, s.err
}

type stubTitleGenerator struct {
	title string
	err   error
}

func (s stubTitleGenerator) GenerateTitle(context.Context, string) (string, error) {
	return s.title, s.err
}

type stuckTaskClient struct {
	stubAPIClient
	status string
}

func (s stuckTaskClient) GetTask(context.Context, int) (meili.Task, error) {
	return meili.Task{TaskUID: 73, Status: s.status}, nil
}

func TestRunDocumentsCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "My Note.md")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout bytes.Buffer
	var added []meili.Document
	client := &stubAPIClient{
		created: meili.Task{TaskUID: 73, IndexUID: "test", Status: "enqueued", Type: "documentAdditionOrUpdate"},
		added:   &added,
	}
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return client
		},
		newTitleGenerator: func(config.Config) (titleGenerator, error) {
			return stubTitleGenerator{title: "Generated Note Title"}, nil
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
	}

	if err := app.Run([]string{"documents", "create", "test", path}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "created\tindex=test\tid=20260629-generated_note_title\ttask=73\tstatus=enqueued\twait=false") {
		t.Fatalf("unexpected output %q", got)
	}

	if len(added) != 1 {
		t.Fatalf("unexpected added document count %d", len(added))
	}

	document := added[0]
	if document["id"] != "20260629-generated_note_title" {
		t.Fatalf("unexpected id %#v", document["id"])
	}
	if document["title"] != "Generated Note Title" {
		t.Fatalf("unexpected title %#v", document["title"])
	}
	if document["source_filename"] != "My Note.md" {
		t.Fatalf("unexpected source filename %#v", document["source_filename"])
	}
	if document["path"] != "test/My Note.md" {
		t.Fatalf("unexpected path %#v", document["path"])
	}
	if document["content"] != "hello world" {
		t.Fatalf("unexpected content %#v", document["content"])
	}
}

func TestRunAppliesTimeoutOption(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout)
	app.loadConfig = func() (config.Config, error) {
		return config.Config{HTTPAddr: "http://localhost:7700"}, nil
	}
	app.newClient = func(config.Config) apiClient {
		if app.requestTimeout != 7*time.Second {
			t.Fatalf("unexpected timeout %s", app.requestTimeout)
		}
		return stubAPIClient{health: meili.Health{Status: "available"}}
	}

	if err := app.Run([]string{"--timeout", "7", "health"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); got != "available\n" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunRejectsInvalidTimeoutOption(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"--timeout", "0", "health"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "--timeout must be a positive number of seconds") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestWaitForTaskTimesOut(t *testing.T) {
	current := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	app := &App{
		now: func() time.Time {
			return current
		},
		sleep: func(duration time.Duration) {
			current = current.Add(duration)
		},
		taskWaitTimeout: time.Second,
	}

	err := app.waitForTask(stuckTaskClient{status: "processing"}, 73)
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "task 73 did not finish within 1s; last status processing") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDocumentsCreateWaitsForTask(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "My Note.md")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout bytes.Buffer
	taskQueue := []meili.Task{{TaskUID: 73, Status: "succeeded"}}
	client := &stubAPIClient{
		created:   meili.Task{TaskUID: 73, IndexUID: "test", Status: "enqueued", Type: "documentAdditionOrUpdate"},
		taskQueue: &taskQueue,
	}
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return client
		},
		newTitleGenerator: func(config.Config) (titleGenerator, error) {
			return stubTitleGenerator{title: "Generated Note Title"}, nil
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
		sleep: func(time.Duration) {},
	}

	if err := app.Run([]string{"documents", "create", "test", path, "--wait"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "created\tindex=test\tid=20260629-generated_note_title\ttask=73\tstatus=succeeded\twait=true") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDocumentsCreateAddsSuffixWhenIDExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "My Note.md")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout bytes.Buffer
	var added []meili.Document
	client := &stubAPIClient{
		document: meili.Document{"id": "20260629-generated_note_title"},
		created:  meili.Task{TaskUID: 73, IndexUID: "test", Status: "enqueued", Type: "documentAdditionOrUpdate"},
		added:    &added,
	}
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return collisionClient{apiClient: client, missingAfter: map[string]bool{"20260629-generated_note_title-4821": true}}
		},
		newTitleGenerator: func(config.Config) (titleGenerator, error) {
			return stubTitleGenerator{title: "Generated Note Title"}, nil
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
		randomIDSuffix: func() (string, error) {
			return "4821", nil
		},
	}

	if err := app.Run([]string{"documents", "create", "test", path}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(added) != 1 || added[0]["id"] != "20260629-generated_note_title-4821" {
		t.Fatalf("unexpected added documents %#v", added)
	}
}

func TestRunDocumentsCreateFallsBackWhenLLMUnavailable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "My Note.md")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout bytes.Buffer
	var added []meili.Document
	client := &stubAPIClient{
		created: meili.Task{TaskUID: 73, IndexUID: "test", Status: "enqueued", Type: "documentAdditionOrUpdate"},
		added:   &added,
	}
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return client
		},
		newTitleGenerator: func(config.Config) (titleGenerator, error) {
			return nil, errors.New("LLM unavailable")
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
	}

	if err := app.Run([]string{"documents", "create", "test", path}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(added) != 1 {
		t.Fatalf("unexpected added document count %d", len(added))
	}
	if added[0]["title"] != "My Note" || added[0]["id"] != "20260629-my_note" {
		t.Fatalf("unexpected fallback document %#v", added[0])
	}
	if !strings.Contains(stdout.String(), "id=20260629-my_note") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunDocumentsCreateRequiresArguments(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"documents", "create", "test"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing index UID or file path") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDocumentsCreateRejectsUnknownOption(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"documents", "create", "test", "note.md", "--bad"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "unknown documents create option") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestLoadDocumentSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "My Note.md")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	source, err := loadDocumentSource(path)
	if err != nil {
		t.Fatalf("loadDocumentSource returned error: %v", err)
	}

	if source.BaseName != "My Note.md" {
		t.Fatalf("unexpected basename %#v", source.BaseName)
	}
	if source.Content != "hello world" {
		t.Fatalf("unexpected content %#v", source.Content)
	}
}

func TestBuildDocument(t *testing.T) {
	document := buildDocument("test", documentSource{
		BaseName: "My Note.md",
		Content:  "hello world",
	}, "Context Growth Notes", time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))

	if document["id"] != "20260629-context_growth_notes" {
		t.Fatalf("unexpected id %#v", document["id"])
	}
	if document["title"] != "Context Growth Notes" {
		t.Fatalf("unexpected title %#v", document["title"])
	}
	if document["source_filename"] != "My Note.md" {
		t.Fatalf("unexpected source filename %#v", document["source_filename"])
	}
	if document["path"] != "test/My Note.md" {
		t.Fatalf("unexpected path %#v", document["path"])
	}
	if document["content"] != "hello world" {
		t.Fatalf("unexpected content %#v", document["content"])
	}
}

func TestBuildDocumentID(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)

	if got := buildDocumentID(now, "Context Growth Notes", ""); got != "20260629-context_growth_notes" {
		t.Fatalf("unexpected id %q", got)
	}
	if got := buildDocumentID(now, "Context Growth Notes", "4821"); got != "20260629-context_growth_notes-4821" {
		t.Fatalf("unexpected id %q", got)
	}
}

func TestLoadDocumentSourceRejectsUnsupportedExtension(t *testing.T) {
	_, err := loadDocumentSource("note.pdf")
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "unsupported document file type") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDefaultsToHelp(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout)

	if err := app.Run(nil); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "Usage:") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunHelp(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout)

	if err := app.Run([]string{"help"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "Usage:") {
		t.Fatalf("expected usage text, got %q", got)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	app := NewApp(&stdout)

	err := app.Run([]string{"missing"})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}

	if !strings.Contains(err.Error(), `unknown command "missing"`) {
		t.Fatalf("unexpected error %q", err.Error())
	}
}

func TestRunHealth(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{health: meili.Health{Status: "available"}}
		},
	}

	if err := app.Run([]string{"health"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); got != "available\n" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunHealthReturnsConfigError(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		loadConfig: func() (config.Config, error) {
			return config.Config{}, errors.New("bad config")
		},
		newClient: func(config.Config) apiClient {
			t.Fatal("newClient should not be called")
			return nil
		},
	}

	err := app.Run([]string{"health"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "bad config") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunHealthReturnsClientError(t *testing.T) {
	app := &App{
		stdout: &bytes.Buffer{},
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{err: errors.New("request failed")}
		},
	}

	err := app.Run([]string{"health"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunIndexesList(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				indexes: []meili.Index{
					{UID: "books", PrimaryKey: "id", CreatedAt: "2026-06-29T11:00:00Z"},
					{UID: "notes", CreatedAt: "2026-06-29T11:05:00Z"},
				},
			}
		},
	}

	if err := app.Run([]string{"indexes", "list"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "books\tid\t2026-06-29T11:00:00Z") {
		t.Fatalf("unexpected output %q", got)
	}

	if !strings.Contains(got, "notes\t-\t2026-06-29T11:05:00Z") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunIndexesGet(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				index: meili.Index{
					UID:        "books",
					PrimaryKey: "id",
					CreatedAt:  "2026-06-29T11:00:00Z",
					UpdatedAt:  "2026-06-29T11:05:00Z",
				},
			}
		},
	}

	if err := app.Run([]string{"indexes", "get", "books"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "uid: books") || !strings.Contains(got, "updatedAt: 2026-06-29T11:05:00Z") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunIndexesGetRequiresUID(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"indexes", "get"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing index UID for indexes get") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDocumentsList(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				documents: []meili.Document{
					{"id": float64(1), "title": "Book One", "content": "secret body"},
					{"id": float64(2), "title": "Book Two", "content": "more body"},
				},
			}
		},
	}

	if err := app.Run([]string{"documents", "list", "books"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "1\tBook One") {
		t.Fatalf("unexpected output %q", got)
	}

	if !strings.Contains(got, "2\tBook Two") {
		t.Fatalf("unexpected output %q", got)
	}

	if strings.Contains(got, "secret body") || strings.Contains(got, "more body") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunSplitMarkdownDryRun(t *testing.T) {
	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	inputPath := filepath.Join(inputDir, "notes.md")
	content := "# Project Notes\n\nIntro paragraph.\n\n## Section One\n\nBody text.\n"
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout bytes.Buffer
	app := &App{stdout: &stdout}

	if err := app.Run([]string{"split-markdown", inputPath, "--output-dir", outputDir, "--dry-run"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "Processed notes.md: 2 chunks") {
		t.Fatalf("unexpected output %q", got)
	}
	if !strings.Contains(got, "[DRY RUN]") {
		t.Fatalf("unexpected output %q", got)
	}

	if _, err := os.Stat(filepath.Join(outputDir, "manifest.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("expected no manifest file, got err=%v", err)
	}
}

func TestRunDocumentsListUsesDashForMissingFields(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				documents: []meili.Document{
					{"title": "Untitled"},
					{"id": float64(2)},
				},
			}
		},
	}

	if err := app.Run([]string{"documents", "list", "books"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "-\tUntitled") {
		t.Fatalf("unexpected output %q", got)
	}

	if !strings.Contains(got, "2\t-") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDocumentsGet(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				document: meili.Document{
					"id":              "doc1",
					"title":           "Book One",
					"source_filename": "Book One.md",
					"content":         "hello world",
				},
			}
		},
	}

	if err := app.Run([]string{"documents", "get", "books", "doc1"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, `"id": "doc1"`) || !strings.Contains(got, `"content": "hello world"`) {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDocumentsGetRequiresIDs(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"documents", "get", "books"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing index UID or document ID for documents get") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDocumentsListRequiresIndexUID(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"documents", "list"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing index UID") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDocumentsDelete(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				deleted: meili.Task{TaskUID: 61, IndexUID: "test", Status: "enqueued", Type: "documentDeletion"},
			}
		},
	}

	if err := app.Run([]string{"documents", "delete", "test", "doc1"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "enqueued\ttest\ttask=61\tstatus=enqueued\ttype=documentDeletion") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDocumentsDeleteRequiresIDs(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"documents", "delete", "test"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing index UID or document ID") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDocumentsDeleteAll(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				deletedAll: meili.Task{TaskUID: 62, IndexUID: "test", Status: "enqueued", Type: "documentDeletion"},
			}
		},
	}

	if err := app.Run([]string{"documents", "delete-all", "test"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "enqueued\ttest\ttask=62\tstatus=enqueued\ttype=documentDeletion\twait=false") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDocumentsDeleteAllWaitsForTask(t *testing.T) {
	var stdout bytes.Buffer
	taskQueue := []meili.Task{{TaskUID: 62, Status: "succeeded"}}
	client := &stubAPIClient{
		deletedAll: meili.Task{TaskUID: 62, IndexUID: "test", Status: "enqueued", Type: "documentDeletion"},
		taskQueue:  &taskQueue,
	}
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return client
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
		sleep: func(time.Duration) {},
	}

	if err := app.Run([]string{"documents", "delete-all", "test", "--wait"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "enqueued\ttest\ttask=62\tstatus=succeeded\ttype=documentDeletion\twait=true") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDocumentsDeleteAllRequiresIndex(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"documents", "delete-all"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing index UID for documents delete-all") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunDocumentsMigrateIDsDryRun(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				documents: []meili.Document{
					{"id": "doc1", "title": "Old", "content": "alpha content", "path": "test/doc1.md"},
					{"id": "20260629-existing_pattern", "title": "Current", "content": "beta", "path": "test/doc2.md"},
				},
			}
		},
		newTitleGenerator: func(config.Config) (titleGenerator, error) {
			return stubTitleGenerator{title: "Fresh Title"}, nil
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
		sleep:     func(time.Duration) {},
		cachePath: filepath.Join(t.TempDir(), "cache.txt"),
	}

	if err := app.Run([]string{"documents", "migrate-ids", "test"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "dry-run\tdoc1\t20260629-fresh_title") {
		t.Fatalf("unexpected output %q", got)
	}
	if strings.Contains(got, "existing_pattern") {
		t.Fatalf("unexpected output %q", got)
	}
	if !strings.Contains(got, "complete\tcount=1\tmode=dry-run") {
		t.Fatalf("unexpected output %q", got)
	}

	cacheContent, err := os.ReadFile(app.cachePath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(cacheContent), `"old_id": "doc1"`) {
		t.Fatalf("unexpected cache content %q", string(cacheContent))
	}
	if !strings.Contains(string(cacheContent), `"new_id": "20260629-fresh_title"`) {
		t.Fatalf("unexpected cache content %q", string(cacheContent))
	}
}

func TestRunDocumentsMigrateIDsFallsBackWhenLLMUnavailable(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				documents: []meili.Document{
					{"id": "doc1", "title": "Existing Title", "content": "alpha content", "path": "test/doc1.md"},
				},
			}
		},
		newTitleGenerator: func(config.Config) (titleGenerator, error) {
			return nil, errors.New("LLM unavailable")
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
		sleep:     func(time.Duration) {},
		cachePath: filepath.Join(t.TempDir(), "cache.txt"),
	}

	if err := app.Run([]string{"documents", "migrate-ids", "test"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "dry-run\tdoc1\t20260629-existing_title") {
		t.Fatalf("unexpected output %q", got)
	}

	cacheContent, err := os.ReadFile(app.cachePath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(cacheContent), `"title": "Existing Title"`) {
		t.Fatalf("unexpected cache content %q", string(cacheContent))
	}
}

func TestRunDocumentsMigrateIDsApply(t *testing.T) {
	var stdout bytes.Buffer
	var added []meili.Document
	taskQueue := []meili.Task{
		{TaskUID: 1, Status: "succeeded"},
		{TaskUID: 2, Status: "succeeded"},
	}
	client := &stubAPIClient{
		documents: []meili.Document{
			{"id": "doc1", "title": "Old", "content": "alpha content", "path": "test/doc1.md"},
		},
		created:   meili.Task{TaskUID: 1, IndexUID: "test", Status: "enqueued", Type: "documentAdditionOrUpdate"},
		deleted:   meili.Task{TaskUID: 2, IndexUID: "test", Status: "enqueued", Type: "documentDeletion"},
		added:     &added,
		taskQueue: &taskQueue,
	}

	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return client
		},
		newTitleGenerator: func(config.Config) (titleGenerator, error) {
			t.Fatal("newTitleGenerator should not be called during apply")
			return nil, nil
		},
		now: func() time.Time {
			return time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
		},
		sleep:     func(time.Duration) {},
		cachePath: filepath.Join(t.TempDir(), "cache.txt"),
	}

	cacheContent := `{
  "index_uid": "test",
  "entries": [
    {
      "old_id": "doc1",
      "new_id": "20260629-fresh_title",
      "title": "Fresh Title"
    }
  ]
}`
	if err := os.WriteFile(app.cachePath, []byte(cacheContent), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := app.Run([]string{"documents", "migrate-ids", "test", "--apply"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(added) != 1 || added[0]["id"] != "20260629-fresh_title" {
		t.Fatalf("unexpected added documents %#v", added)
	}
	if got := stdout.String(); !strings.Contains(got, "apply\tdoc1\t20260629-fresh_title") {
		t.Fatalf("unexpected output %q", got)
	}
	if !strings.Contains(stdout.String(), "complete\tcount=1\tmode=apply") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
	if _, err := os.Stat(app.cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removal, stat err=%v", err)
	}
}

func TestRunSearch(t *testing.T) {
	var stdout bytes.Buffer
	var searchLog []searchCall
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				indexes: []meili.Index{
					{UID: "books"},
					{UID: "notes"},
				},
				searches: map[string]meili.SearchResult{
					"books": {
						EstimatedTotalHits: 2,
						Hits: []meili.Document{
							{"id": "b2", "title": "Second", "content": "abcdef"},
							{"id": "b1", "title": "First", "content": "abc"},
						},
					},
					"notes": {
						EstimatedTotalHits: 0,
						Hits:               nil,
					},
				},
				searchLog: &searchLog,
			}
		},
	}

	if err := app.Run([]string{"search", "history", "of", "rome"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "books\thits=2\treturned=2\tcontentBytes=9 bytes") {
		t.Fatalf("unexpected output %q", got)
	}

	if !strings.Contains(got, "b1\tFirst\tcontentBytes=3 bytes") {
		t.Fatalf("unexpected output %q", got)
	}

	if !strings.Contains(got, "b2\tSecond\tcontentBytes=6 bytes") {
		t.Fatalf("unexpected output %q", got)
	}

	if !strings.Contains(got, "TOTAL\treturned=2\tcontentBytes=9 bytes\tjsonBytes=") {
		t.Fatalf("unexpected output %q", got)
	}

	if strings.Contains(got, "notes") {
		t.Fatalf("unexpected output %q", got)
	}

	if len(searchLog) != 2 {
		t.Fatalf("unexpected search calls %#v", searchLog)
	}
	if searchLog[0] != (searchCall{indexUID: "books", query: "history of rome", limit: defaultSearchLimit}) {
		t.Fatalf("unexpected first search call %#v", searchLog[0])
	}
}

func TestRunSearchSupportsIndexAndLimitFlags(t *testing.T) {
	var stdout bytes.Buffer
	var searchLog []searchCall
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				searches: map[string]meili.SearchResult{
					"notes": {
						EstimatedTotalHits: 1,
						Hits: []meili.Document{
							{"id": "n1", "title": "First Note", "content": "abc"},
						},
					},
				},
				searchLog: &searchLog,
			}
		},
	}

	if err := app.Run([]string{"search", "--index", "notes", "--limit=25", "history", "of", "rome"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(searchLog) != 1 {
		t.Fatalf("unexpected search calls %#v", searchLog)
	}
	if searchLog[0] != (searchCall{indexUID: "notes", query: "history of rome", limit: 25}) {
		t.Fatalf("unexpected search call %#v", searchLog[0])
	}
	if !strings.Contains(stdout.String(), "notes\thits=1\treturned=1") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunSearchNoMatches(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				indexes: []meili.Index{{UID: "books"}},
				searches: map[string]meili.SearchResult{
					"books": {EstimatedTotalHits: 0},
				},
			}
		},
	}

	if err := app.Run([]string{"search", "missing"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "no matches") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunSearchRejectsInvalidLimit(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"search", "--limit", "0", "history"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "--limit must be a positive integer") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunSearchRequiresQuery(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"search"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing query") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestRunCreateIndex(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				created: meili.Task{TaskUID: 42, IndexUID: "test", Status: "enqueued", Type: "indexCreation"},
			}
		},
	}

	if err := app.Run([]string{"indexes", "create", "test", "id"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, "enqueued\ttest\ttask=42\tstatus=enqueued\ttype=indexCreation") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDeleteIndex(t *testing.T) {
	var stdout bytes.Buffer
	app := &App{
		stdout: &stdout,
		loadConfig: func() (config.Config, error) {
			return config.Config{HTTPAddr: "http://localhost:7700"}, nil
		},
		newClient: func(config.Config) apiClient {
			return stubAPIClient{
				deleted: meili.Task{TaskUID: 52, IndexUID: "test", Status: "enqueued", Type: "indexDeletion"},
			}
		},
	}

	if err := app.Run([]string{"indexes", "delete", "test"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "enqueued\ttest\ttask=52\tstatus=enqueued\ttype=indexDeletion") {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestRunDeleteIndexRequiresUID(t *testing.T) {
	app := NewApp(&bytes.Buffer{})

	err := app.Run([]string{"indexes", "delete"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "missing index UID for indexes delete") {
		t.Fatalf("unexpected error %q", err)
	}
}
