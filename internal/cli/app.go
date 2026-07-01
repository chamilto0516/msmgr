package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"msmgr/internal/config"
	"msmgr/internal/llm"
	"msmgr/internal/meili"
)

const defaultDocumentListLimit = 20
const defaultSearchLimit = 5
const migrationBatchSize = 100
const taskPollInterval = 200 * time.Millisecond
const defaultRequestTimeout = 30 * time.Second
const defaultTaskWaitTimeout = 60 * time.Second
const maxDocumentIDCollisionAttempts = 100
const migrationCachePath = "cache.txt"

type App struct {
	stdout            io.Writer
	loadConfig        func() (config.Config, error)
	newClient         func(config.Config) apiClient
	newTitleGenerator func(config.Config) (titleGenerator, error)
	now               func() time.Time
	sleep             func(time.Duration)
	cachePath         string
	requestTimeout    time.Duration
	taskWaitTimeout   time.Duration
	randomIDSuffix    func() (string, error)
}

type apiClient interface {
	Health(ctx context.Context) (meili.Health, error)
	ListIndexes(ctx context.Context) ([]meili.Index, error)
	GetIndex(ctx context.Context, uid string) (meili.Index, error)
	ListDocuments(ctx context.Context, indexUID string, limit, offset int) ([]meili.Document, error)
	GetDocument(ctx context.Context, indexUID, documentID string) (meili.Document, error)
	SearchIndex(ctx context.Context, indexUID, query string, limit int) (meili.SearchResult, error)
	CreateIndex(ctx context.Context, uid, primaryKey string) (meili.Task, error)
	DeleteIndex(ctx context.Context, uid string) (meili.Task, error)
	DeleteDocument(ctx context.Context, indexUID, documentID string) (meili.Task, error)
	AddDocuments(ctx context.Context, indexUID string, documents []meili.Document) (meili.Task, error)
	GetTask(ctx context.Context, taskUID int) (meili.Task, error)
}

type titleGenerator interface {
	GenerateTitle(ctx context.Context, content string) (string, error)
}

func NewApp(stdout io.Writer) *App {
	app := &App{
		stdout:          stdout,
		loadConfig:      config.LoadFromEnv,
		now:             time.Now,
		sleep:           time.Sleep,
		cachePath:       migrationCachePath,
		requestTimeout:  defaultRequestTimeout,
		taskWaitTimeout: defaultTaskWaitTimeout,
		randomIDSuffix:  random4DigitSuffix,
	}
	app.newClient = func(cfg config.Config) apiClient {
		return meili.NewClient(cfg, &http.Client{Timeout: app.effectiveRequestTimeout()})
	}
	app.newTitleGenerator = func(cfg config.Config) (titleGenerator, error) {
		return llm.NewClient(cfg, &http.Client{Timeout: app.effectiveRequestTimeout()})
	}
	return app
}

func (a *App) Run(args []string) error {
	var err error
	args, err = a.applyTimeoutOption(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		_, err := fmt.Fprint(a.stdout, usageText)
		return err
	}

	switch args[0] {
	case "hello":
		_, err := fmt.Fprintln(a.stdout, "Hello from MeiliSearch Manager CLI")
		return err
	case "health":
		return a.runHealth()
	case "indexes":
		return a.runIndexes(args[1:])
	case "documents":
		return a.runDocuments(args[1:])
	case "split-markdown":
		return a.runSplitMarkdown(args[1:])
	case "search":
		return a.runSearch(args[1:])
	case "help", "--help", "-h":
		_, err := fmt.Fprint(a.stdout, usageText)
		return err
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText)
	}
}

func (a *App) applyTimeoutOption(args []string) ([]string, error) {
	if a.requestTimeout <= 0 {
		a.requestTimeout = defaultRequestTimeout
	}
	if a.taskWaitTimeout <= 0 {
		a.taskWaitTimeout = defaultTaskWaitTimeout
	}

	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := ""
		switch {
		case arg == "--timeout":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing seconds for --timeout\n\n%s", usageText)
			}
			i++
			value = args[i]
		case strings.HasPrefix(arg, "--timeout="):
			value = strings.TrimPrefix(arg, "--timeout=")
		default:
			filtered = append(filtered, arg)
			continue
		}

		seconds, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || seconds <= 0 {
			return nil, fmt.Errorf("--timeout must be a positive number of seconds")
		}
		a.requestTimeout = time.Duration(seconds) * time.Second
	}

	return filtered, nil
}

func (a *App) effectiveRequestTimeout() time.Duration {
	if a.requestTimeout <= 0 {
		return defaultRequestTimeout
	}
	return a.requestTimeout
}

func (a *App) runHealth() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}

	health, err := a.newClient(cfg).Health(context.Background())
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(a.stdout, health.Status)
	return err
}

func (a *App) runIndexes(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing indexes subcommand\n\n%s", usageText)
	}

	switch args[0] {
	case "create":
		if len(args) < 2 {
			return fmt.Errorf("missing index UID for indexes create\n\n%s", usageText)
		}

		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		primaryKey := ""
		if len(args) > 2 {
			primaryKey = args[2]
		}

		task, err := a.newClient(cfg).CreateIndex(context.Background(), args[1], primaryKey)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(a.stdout, formatTask(task))
		return err
	case "delete":
		if len(args) < 2 {
			return fmt.Errorf("missing index UID for indexes delete\n\n%s", usageText)
		}

		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		task, err := a.newClient(cfg).DeleteIndex(context.Background(), args[1])
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(a.stdout, formatTask(task))
		return err
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("missing index UID for indexes get\n\n%s", usageText)
		}

		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		index, err := a.newClient(cfg).GetIndex(context.Background(), args[1])
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(a.stdout, "uid: %s\nprimaryKey: %s\ncreatedAt: %s\nupdatedAt: %s\n",
			valueOrDash(index.UID),
			valueOrDash(index.PrimaryKey),
			valueOrDash(index.CreatedAt),
			valueOrDash(index.UpdatedAt),
		); err != nil {
			return err
		}

		return nil
	case "list":
		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		indexes, err := a.newClient(cfg).ListIndexes(context.Background())
		if err != nil {
			return err
		}

		for _, index := range indexes {
			if _, err := fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", index.UID, valueOrDash(index.PrimaryKey), valueOrDash(index.CreatedAt)); err != nil {
				return err
			}
		}

		return nil
	default:
		return fmt.Errorf("unknown indexes subcommand %q\n\n%s", args[0], usageText)
	}
}

func (a *App) runDocumentsMigrateIDs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing index UID for documents migrate-ids\n\n%s", usageText)
	}

	indexUID := args[0]
	apply := false
	for _, arg := range args[1:] {
		if arg == "--apply" {
			apply = true
			continue
		}
		return fmt.Errorf("unknown migrate-ids option %q\n\n%s", arg, usageText)
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}

	client := a.newClient(cfg)
	documents, err := a.listAllDocuments(client, indexUID)
	if err != nil {
		return err
	}

	planEntries := map[string]migrationPlanEntry{}
	if apply {
		plan, err := a.readMigrationPlan()
		if err != nil {
			return err
		}
		if plan.IndexUID != indexUID {
			return fmt.Errorf("cache.txt targets index %q, not %q", plan.IndexUID, indexUID)
		}
		for _, entry := range plan.Entries {
			planEntries[entry.OldID] = entry
		}
	} else {
		var titleGenerator titleGenerator
		if generator, err := a.newTitleGenerator(cfg); err == nil {
			titleGenerator = generator
		}
		for _, document := range documents {
			oldID := documentField(document, "id")
			if matchesGeneratedIDPattern(oldID) {
				continue
			}

			content := strings.TrimSpace(fmt.Sprint(document["content"]))
			if content == "" {
				continue
			}

			title := a.generateTitleOrFallback(context.Background(), titleGenerator, content, fallbackTitleForDocument(document))

			planEntries[oldID] = migrationPlanEntry{
				OldID: oldID,
				NewID: buildDocumentID(a.now(), title, ""),
				Title: title,
			}
		}
	}

	migrated := 0
	for _, document := range documents {
		oldID := documentField(document, "id")
		if matchesGeneratedIDPattern(oldID) {
			continue
		}

		content := strings.TrimSpace(fmt.Sprint(document["content"]))
		if content == "" {
			if _, err := fmt.Fprintf(a.stdout, "skip\t%s\tmissing content\n", oldID); err != nil {
				return err
			}
			continue
		}

		entry, ok := planEntries[oldID]
		if !ok {
			if apply {
				if _, err := fmt.Fprintf(a.stdout, "skip\t%s\tmissing cache entry\n", oldID); err != nil {
					return err
				}
			}
			continue
		}

		newDocument := cloneDocument(document)
		newDocument["title"] = entry.Title
		newDocument["id"] = entry.NewID
		if _, ok := newDocument["source_filename"]; !ok {
			if pathValue := documentField(document, "path"); pathValue != "-" {
				newDocument["source_filename"] = filepath.Base(pathValue)
			}
		}

		if _, err := fmt.Fprintf(a.stdout, "%s\t%s\t%s\n", ternary(apply, "apply", "dry-run"), oldID, entry.NewID); err != nil {
			return err
		}

		if !apply {
			migrated++
			continue
		}

		addTask, err := client.AddDocuments(context.Background(), indexUID, []meili.Document{newDocument})
		if err != nil {
			return fmt.Errorf("add migrated document %s: %w", oldID, err)
		}
		if err := a.waitForTask(client, addTask.TaskUID); err != nil {
			return fmt.Errorf("wait for add task %d: %w", addTask.TaskUID, err)
		}

		deleteTask, err := client.DeleteDocument(context.Background(), indexUID, oldID)
		if err != nil {
			return fmt.Errorf("delete original document %s: %w", oldID, err)
		}
		if err := a.waitForTask(client, deleteTask.TaskUID); err != nil {
			return fmt.Errorf("wait for delete task %d: %w", deleteTask.TaskUID, err)
		}

		migrated++
	}

	if !apply {
		plan := migrationPlan{
			IndexUID: indexUID,
			Entries:  make([]migrationPlanEntry, 0, len(planEntries)),
		}
		for _, document := range documents {
			oldID := documentField(document, "id")
			if entry, ok := planEntries[oldID]; ok {
				plan.Entries = append(plan.Entries, entry)
			}
		}
		if err := a.writeMigrationPlan(plan); err != nil {
			return err
		}
	} else {
		if err := os.Remove(a.cachePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove cache file: %w", err)
		}
	}

	if _, err := fmt.Fprintf(a.stdout, "complete\tcount=%d\tmode=%s\n", migrated, ternary(apply, "apply", "dry-run")); err != nil {
		return err
	}

	return nil
}

func (a *App) runDocuments(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing documents subcommand\n\n%s", usageText)
	}

	switch args[0] {
	case "create":
		if len(args) < 3 {
			return fmt.Errorf("missing index UID or file path for documents create\n\n%s", usageText)
		}

		wait := false
		for _, arg := range args[3:] {
			switch arg {
			case "--wait":
				wait = true
			default:
				return fmt.Errorf("unknown documents create option %q\n\n%s", arg, usageText)
			}
		}

		source, err := loadDocumentSource(args[2])
		if err != nil {
			return err
		}

		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		var titleGenerator titleGenerator
		if generator, err := a.newTitleGenerator(cfg); err == nil {
			titleGenerator = generator
		}
		title := a.generateTitleOrFallback(context.Background(), titleGenerator, source.Content, fallbackTitleForSource(source))

		client := a.newClient(cfg)
		document, err := a.buildUniqueDocument(context.Background(), client, args[1], source, title, a.now())
		if err != nil {
			return err
		}

		task, err := client.AddDocuments(context.Background(), args[1], []meili.Document{document})
		if err != nil {
			return err
		}

		if wait {
			if err := a.waitForTask(client, task.TaskUID); err != nil {
				return fmt.Errorf("wait for create task %d: %w", task.TaskUID, err)
			}
		}

		_, err = fmt.Fprintf(a.stdout, "created\tindex=%s\tid=%s\ttask=%d\tstatus=%s\twait=%t\n", args[1], documentField(document, "id"), task.TaskUID, ternary(wait, "succeeded", task.Status), wait)
		return err
	case "migrate-ids":
		return a.runDocumentsMigrateIDs(args[1:])
	case "get":
		if len(args) < 3 {
			return fmt.Errorf("missing index UID or document ID for documents get\n\n%s", usageText)
		}

		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		document, err := a.newClient(cfg).GetDocument(context.Background(), args[1], args[2])
		if err != nil {
			return err
		}

		encoded, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			return fmt.Errorf("encode document: %w", err)
		}

		_, err = fmt.Fprintln(a.stdout, string(encoded))
		return err
	case "delete":
		if len(args) < 3 {
			return fmt.Errorf("missing index UID or document ID for documents delete\n\n%s", usageText)
		}

		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		task, err := a.newClient(cfg).DeleteDocument(context.Background(), args[1], args[2])
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(a.stdout, formatTask(task))
		return err
	case "list":
		if len(args) < 2 {
			return fmt.Errorf("missing index UID for documents list\n\n%s", usageText)
		}

		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		documents, err := a.newClient(cfg).ListDocuments(context.Background(), args[1], defaultDocumentListLimit, 0)
		if err != nil {
			return err
		}

		for _, document := range documents {
			if _, err := fmt.Fprintf(a.stdout, "%s\t%s\n", documentField(document, "id"), documentField(document, "title")); err != nil {
				return err
			}
		}

		return nil
	default:
		return fmt.Errorf("unknown documents subcommand %q\n\n%s", args[0], usageText)
	}
}

func (a *App) runSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing query for search\n\n%s", usageText)
	}

	options, err := parseSearchArgs(args)
	if err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(options.queryParts, " "))
	if query == "" {
		return fmt.Errorf("missing query for search\n\n%s", usageText)
	}

	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}

	client := a.newClient(cfg)
	indexes := []meili.Index{{UID: options.indexUID}}
	if options.indexUID == "" {
		indexes, err = client.ListIndexes(context.Background())
		if err != nil {
			return err
		}
	}

	foundAny := false
	overallContentBytes := 0
	overallJSONBytes := 0
	overallReturned := 0
	for _, index := range indexes {
		result, err := client.SearchIndex(context.Background(), index.UID, query, options.limit)
		if err != nil {
			return fmt.Errorf("search index %q: %w", index.UID, err)
		}

		if result.EstimatedTotalHits == 0 || len(result.Hits) == 0 {
			continue
		}

		foundAny = true
		totalContentBytes := 0
		totalJSONBytes := 0

		type line struct {
			id           string
			title        string
			contentBytes int
			jsonBytes    int
		}

		lines := make([]line, 0, len(result.Hits))
		for _, hit := range result.Hits {
			contentBytes := documentContentBytes(hit)
			jsonBytes, err := documentJSONBytes(hit)
			if err != nil {
				return err
			}

			totalContentBytes += contentBytes
			totalJSONBytes += jsonBytes
			lines = append(lines, line{
				id:           documentField(hit, "id"),
				title:        documentField(hit, "title"),
				contentBytes: contentBytes,
				jsonBytes:    jsonBytes,
			})
		}

		overallContentBytes += totalContentBytes
		overallJSONBytes += totalJSONBytes
		overallReturned += len(result.Hits)

		sort.Slice(lines, func(i, j int) bool {
			if lines[i].title == lines[j].title {
				return lines[i].id < lines[j].id
			}
			return lines[i].title < lines[j].title
		})

		if _, err := fmt.Fprintf(a.stdout, "%s\thits=%s\treturned=%s\tcontentBytes=%s\tjsonBytes=%s\n",
			index.UID,
			formatNumber(result.EstimatedTotalHits),
			formatNumber(len(result.Hits)),
			formatByteSize(totalContentBytes),
			formatByteSize(totalJSONBytes),
		); err != nil {
			return err
		}

		for _, line := range lines {
			if _, err := fmt.Fprintf(a.stdout, "  %s\t%s\tcontentBytes=%s\tjsonBytes=%s\n", line.id, line.title, formatByteSize(line.contentBytes), formatByteSize(line.jsonBytes)); err != nil {
				return err
			}
		}
	}

	if !foundAny {
		_, err := fmt.Fprintln(a.stdout, "no matches")
		return err
	}

	if _, err := fmt.Fprintf(a.stdout, "TOTAL\treturned=%s\tcontentBytes=%s\tjsonBytes=%s\n",
		formatNumber(overallReturned),
		formatByteSize(overallContentBytes),
		formatByteSize(overallJSONBytes),
	); err != nil {
		return err
	}

	return nil
}

type searchOptions struct {
	queryParts []string
	indexUID   string
	limit      int
}

func parseSearchArgs(args []string) (searchOptions, error) {
	options := searchOptions{limit: defaultSearchLimit}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--index":
			if i+1 >= len(args) {
				return searchOptions{}, fmt.Errorf("missing value for --index\n\n%s", usageText)
			}
			i++
			options.indexUID = strings.TrimSpace(args[i])
			if options.indexUID == "" {
				return searchOptions{}, fmt.Errorf("--index must not be empty")
			}
		case strings.HasPrefix(arg, "--index="):
			options.indexUID = strings.TrimSpace(strings.TrimPrefix(arg, "--index="))
			if options.indexUID == "" {
				return searchOptions{}, fmt.Errorf("--index must not be empty")
			}
		case arg == "--limit":
			if i+1 >= len(args) {
				return searchOptions{}, fmt.Errorf("missing value for --limit\n\n%s", usageText)
			}
			i++
			limit, err := parsePositiveIntFlag("--limit", args[i])
			if err != nil {
				return searchOptions{}, err
			}
			options.limit = limit
		case strings.HasPrefix(arg, "--limit="):
			limit, err := parsePositiveIntFlag("--limit", strings.TrimPrefix(arg, "--limit="))
			if err != nil {
				return searchOptions{}, err
			}
			options.limit = limit
		case strings.HasPrefix(arg, "--"):
			return searchOptions{}, fmt.Errorf("unknown search option %q\n\n%s", arg, usageText)
		default:
			options.queryParts = append(options.queryParts, arg)
		}
	}
	return options, nil
}

func parsePositiveIntFlag(name, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}

	return value
}

func formatNumber(value int) string {
	if value == 0 {
		return "0"
	}

	negative := value < 0
	if negative {
		value = -value
	}

	digits := fmt.Sprintf("%d", value)
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)

	result := strings.Join(parts, ",")
	if negative {
		return "-" + result
	}
	return result
}

func formatByteSize(value int) string {
	if value < 1024 {
		return formatNumber(value) + " bytes"
	}

	kb := float64(value) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.1f KB", kb)
	}

	mb := kb / 1024
	return fmt.Sprintf("%.1f MB", mb)
}

func formatTask(task meili.Task) string {
	return fmt.Sprintf("enqueued\t%s\ttask=%d\tstatus=%s\ttype=%s", valueOrDash(task.IndexUID), task.TaskUID, valueOrDash(task.Status), valueOrDash(task.Type))
}

func documentField(document meili.Document, key string) string {
	value, ok := document[key]
	if !ok || value == nil {
		return "-"
	}

	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return "-"
	}

	return text
}

func documentContentBytes(document meili.Document) int {
	value, ok := document["content"]
	if !ok || value == nil {
		return 0
	}

	return len(fmt.Sprint(value))
}

func documentJSONBytes(document meili.Document) (int, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return 0, fmt.Errorf("encode document: %w", err)
	}

	return len(encoded), nil
}

func (a *App) listAllDocuments(client apiClient, indexUID string) ([]meili.Document, error) {
	offset := 0
	var documents []meili.Document

	for {
		batch, err := client.ListDocuments(context.Background(), indexUID, migrationBatchSize, offset)
		if err != nil {
			return nil, err
		}
		documents = append(documents, batch...)
		if len(batch) < migrationBatchSize {
			return documents, nil
		}
		offset += len(batch)
	}
}

func (a *App) waitForTask(client apiClient, taskUID int) error {
	waitTimeout := a.taskWaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = defaultTaskWaitTimeout
	}
	deadline := a.now().Add(waitTimeout)

	for {
		task, err := client.GetTask(context.Background(), taskUID)
		if err != nil {
			return err
		}
		switch task.Status {
		case "succeeded":
			return nil
		case "failed", "canceled":
			return fmt.Errorf("task %d ended with status %s", taskUID, task.Status)
		}
		if !a.now().Before(deadline) {
			return fmt.Errorf("task %d did not finish within %s; last status %s", taskUID, waitTimeout, valueOrDash(task.Status))
		}
		a.sleep(taskPollInterval)
	}
}

type documentSource struct {
	BaseName string
	Content  string
}

type migrationPlan struct {
	IndexUID string               `json:"index_uid"`
	Entries  []migrationPlanEntry `json:"entries"`
}

type migrationPlanEntry struct {
	OldID string `json:"old_id"`
	NewID string `json:"new_id"`
	Title string `json:"title"`
}

func loadDocumentSource(path string) (documentSource, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".txt" && ext != ".md" {
		return documentSource{}, fmt.Errorf("unsupported document file type %q", ext)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return documentSource{}, fmt.Errorf("read document file: %w", err)
	}

	return documentSource{
		BaseName: filepath.Base(path),
		Content:  string(content),
	}, nil
}

func buildDocument(indexUID string, source documentSource, title string, now time.Time) meili.Document {
	baseTitle := strings.TrimSpace(title)
	if baseTitle == "" {
		baseTitle = fallbackTitleForSource(source)
	}

	id := buildDocumentID(now, baseTitle, "")
	return meili.Document{
		"id":              id,
		"title":           baseTitle,
		"source_filename": source.BaseName,
		"path":            filepath.ToSlash(filepath.Join(indexUID, source.BaseName)),
		"content":         source.Content,
	}
}

func cloneDocument(document meili.Document) meili.Document {
	cloned := make(meili.Document, len(document))
	for key, value := range document {
		cloned[key] = value
	}
	return cloned
}

func (a *App) buildUniqueDocument(ctx context.Context, client apiClient, indexUID string, source documentSource, title string, now time.Time) (meili.Document, error) {
	document := buildDocument(indexUID, source, title, now)
	baseTitle := documentField(document, "title")
	baseID := buildDocumentID(now, baseTitle, "")

	for attempt := 0; attempt <= maxDocumentIDCollisionAttempts; attempt++ {
		candidateID := baseID
		if attempt > 0 {
			suffix, err := a.generateRandomIDSuffix()
			if err != nil {
				return nil, err
			}
			candidateID = buildDocumentID(now, baseTitle, suffix)
		}
		document["id"] = candidateID

		_, err := client.GetDocument(ctx, indexUID, candidateID)
		if err == nil {
			continue
		}
		if isDocumentNotFound(err) {
			return document, nil
		}
		return nil, fmt.Errorf("check document id %q: %w", candidateID, err)
	}

	return nil, fmt.Errorf("could not find unique document id for %q after %d random attempts", baseID, maxDocumentIDCollisionAttempts)
}

func (a *App) generateTitleOrFallback(ctx context.Context, titleGenerator titleGenerator, content, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		fallback = "document"
	}
	if titleGenerator == nil {
		return fallback
	}
	title, err := titleGenerator.GenerateTitle(ctx, content)
	if err != nil || strings.TrimSpace(title) == "" {
		return fallback
	}
	return title
}

func fallbackTitleForSource(source documentSource) string {
	title := strings.TrimSuffix(source.BaseName, filepath.Ext(source.BaseName))
	if strings.TrimSpace(title) == "" {
		return "document"
	}
	return title
}

func fallbackTitleForDocument(document meili.Document) string {
	for _, key := range []string{"title", "source_filename", "path", "id"} {
		value := documentField(document, key)
		if value == "-" {
			continue
		}
		if key == "source_filename" || key == "path" {
			value = strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
		}
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "document"
}

func (a *App) generateRandomIDSuffix() (string, error) {
	if a.randomIDSuffix != nil {
		return a.randomIDSuffix()
	}
	return random4DigitSuffix()
}

func (a *App) writeMigrationPlan(plan migrationPlan) error {
	encoded, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode migration plan: %w", err)
	}
	if err := os.WriteFile(a.cachePath, encoded, 0o644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}
	return nil
}

func (a *App) readMigrationPlan() (migrationPlan, error) {
	content, err := os.ReadFile(a.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return migrationPlan{}, fmt.Errorf("cache file %s not found; run documents migrate-ids without --apply first", a.cachePath)
		}
		return migrationPlan{}, fmt.Errorf("read cache file: %w", err)
	}

	var plan migrationPlan
	if err := json.Unmarshal(content, &plan); err != nil {
		return migrationPlan{}, fmt.Errorf("parse cache file: %w", err)
	}

	return plan, nil
}

func matchesGeneratedIDPattern(value string) bool {
	if len(value) < 10 {
		return false
	}
	for i := 0; i < 8; i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	if value[8] != '-' {
		return false
	}
	slug := value[9:]
	if slug == "" {
		return false
	}
	for i := 0; i < len(slug); i++ {
		ch := slug[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func buildDocumentID(now time.Time, title, suffix string) string {
	id := now.Format("20060102") + "-" + slugify(title)
	if strings.TrimSpace(suffix) == "" {
		return id
	}
	return id + "-" + suffix
}

func random4DigitSuffix() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(9000))
	if err != nil {
		return "", fmt.Errorf("generate random document id suffix: %w", err)
	}
	return fmt.Sprintf("%04d", value.Int64()+1000), nil
}

func isDocumentNotFound(err error) bool {
	var httpErr *meili.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

func ternary[T any](condition bool, left, right T) T {
	if condition {
		return left
	}
	return right
}

func slugify(value string) string {
	var b strings.Builder
	lastUnderscore := false

	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}

		if !lastUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	result := strings.Trim(b.String(), "_")
	if result == "" {
		return "document"
	}

	return result
}

const usageText = `MeiliSearch Manager CLI

Usage:
  msmgr [--timeout seconds] <command>
  msmgr hello
  msmgr health
  msmgr search <query> [--index uid] [--limit n]
  msmgr indexes create <uid> [primaryKey]
  msmgr indexes delete <uid>
  msmgr indexes get <uid>
  msmgr indexes list
  msmgr documents create <index> <path> [--wait]
  msmgr documents get <index> <id>
  msmgr documents migrate-ids <index> [--apply]
  msmgr documents delete <index> <id>
  msmgr documents list <index>
  msmgr split-markdown <input-path> [--output-dir dir] [--manifest path] [--split-level n] [--max-heading-level n] [--min-chars n] [--max-chars n] [--use-llm] [--dry-run]
  msmgr help
`
