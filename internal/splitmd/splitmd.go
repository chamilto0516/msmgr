package splitmd

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"msmgr/internal/llm"
)

const defaultSplitLevel = 2
const defaultMaxHeadingLevel = 3
const defaultMinChars = 220
const defaultMaxChars = 1800

var headingRE = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
var nonAlnumRE = regexp.MustCompile(`[^A-Za-z0-9]+`)
var wordRE = regexp.MustCompile(`[A-Za-z0-9]+`)

type Options struct {
	InputPath       string
	OutputDir       string
	ManifestPath    string
	SplitLevel      int
	MaxHeadingLevel int
	MinChars        int
	MaxChars        int
	DryRun          bool
}

type LLMClient interface {
	ChatJSON(ctx context.Context, messages []llm.Message, schemaName string, schema any) (map[string]any, error)
}

type sectionNode struct {
	level        int
	title        string
	parent       *sectionNode
	children     []*sectionNode
	contentLines []string
}

type chunk struct {
	sourceFile     string
	documentTitle  string
	headingPath    []string
	chunkIndex     int
	chunkText      string
	sectionLevel   int
	outputFilename string
	chunkID        string
}

type outputRecord struct {
	ID             string   `json:"id"`
	SourceFile     string   `json:"source_file"`
	DocumentTitle  string   `json:"document_title"`
	HeadingPath    []string `json:"heading_path"`
	SectionLevel   int      `json:"section_level"`
	ChunkIndex     int      `json:"chunk_index"`
	OutputFilename string   `json:"output_filename"`
	Text           string   `json:"text"`
}

type markdownSource struct {
	path       string
	sourceFile string
}

func Run(ctx context.Context, opts Options, client LLMClient, stdout io.Writer) error {
	normalizeOptions(&opts)

	files, err := discoverMarkdownFiles(opts.InputPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no markdown files found")
	}

	allRecords := make([]outputRecord, 0)
	seenNames := map[string]int{}

	for _, source := range files {
		moniker, records, err := processFile(ctx, source, opts, client, seenNames)
		if err != nil {
			return err
		}
		allRecords = append(allRecords, records...)

		if _, err := fmt.Fprintf(stdout, "Processed %s: %d chunks, moniker=%s, output_dir=%s\n",
			filepath.Base(source.path), len(records), moniker, opts.OutputDir); err != nil {
			return err
		}
	}

	if opts.DryRun {
		for _, record := range allRecords {
			if _, err := fmt.Fprintf(stdout, "[DRY RUN] %s :: %s\n", record.OutputFilename, strings.Join(record.HeadingPath, " > ")); err != nil {
				return err
			}
		}
		return nil
	}

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", opts.OutputDir, err)
	}
	manifestDir := filepath.Dir(opts.ManifestPath)
	if manifestDir != "." && manifestDir != "" {
		if err := os.MkdirAll(manifestDir, 0o755); err != nil {
			return fmt.Errorf("create manifest dir %s: %w", manifestDir, err)
		}
	}

	for _, record := range allRecords {
		chunkPath := filepath.Join(opts.OutputDir, record.OutputFilename)
		if err := os.WriteFile(chunkPath, []byte(strings.TrimSpace(record.Text)+"\n"), 0o644); err != nil {
			return fmt.Errorf("write chunk %s: %w", chunkPath, err)
		}
	}

	manifestFile, err := os.Create(opts.ManifestPath)
	if err != nil {
		return fmt.Errorf("create manifest %s: %w", opts.ManifestPath, err)
	}
	defer manifestFile.Close()

	for _, record := range allRecords {
		line, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("encode manifest record: %w", err)
		}
		if _, err := manifestFile.Write(append(line, '\n')); err != nil {
			return fmt.Errorf("write manifest %s: %w", opts.ManifestPath, err)
		}
	}

	return nil
}

func normalizeOptions(opts *Options) {
	if opts.OutputDir == "" {
		opts.OutputDir = filepath.Join("test_file", "output")
	}
	if opts.ManifestPath == "" {
		opts.ManifestPath = filepath.Join(opts.OutputDir, "manifest.jsonl")
	}
	if opts.SplitLevel <= 0 {
		opts.SplitLevel = defaultSplitLevel
	}
	if opts.MaxHeadingLevel <= 0 {
		opts.MaxHeadingLevel = defaultMaxHeadingLevel
	}
	if opts.MinChars <= 0 {
		opts.MinChars = defaultMinChars
	}
	if opts.MaxChars <= 0 {
		opts.MaxChars = defaultMaxChars
	}
}

func discoverMarkdownFiles(inputPath string) ([]markdownSource, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("stat input path %s: %w", inputPath, err)
	}

	if !info.IsDir() {
		return []markdownSource{{
			path:       inputPath,
			sourceFile: filepath.Base(inputPath),
		}}, nil
	}

	var files []string
	if err := filepath.WalkDir(inputPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("discover markdown files in %s: %w", inputPath, err)
	}

	sort.Strings(files)
	sources := make([]markdownSource, 0, len(files))
	for _, path := range files {
		rel, err := filepath.Rel(inputPath, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		sources = append(sources, markdownSource{
			path:       path,
			sourceFile: filepath.ToSlash(rel),
		})
	}

	return sources, nil
}

func processFile(ctx context.Context, source markdownSource, opts Options, client LLMClient, seenNames map[string]int) (string, []outputRecord, error) {
	fullText, chunks, documentTitle, err := buildChunks(source, opts.SplitLevel, opts.MaxHeadingLevel, opts.MinChars, opts.MaxChars)
	if err != nil {
		return "", nil, err
	}

	moniker := fallbackMoniker(source.sourceFile, fullText)
	if client != nil {
		if generated, err := generateMoniker(ctx, client, source.sourceFile, documentTitle, fullText); err == nil {
			moniker = generated
		}
	}

	records := make([]outputRecord, 0, len(chunks))
	for i := range chunks {
		descriptor := fallbackDescriptor(chunks[i])
		if client != nil {
			if generated, err := generateDescriptor(ctx, client, chunks[i]); err == nil {
				descriptor = generated
			}
		}

		baseName := fmt.Sprintf("%s_%s", moniker, descriptor)
		filename := uniqueFilename(baseName, seenNames)
		chunks[i].outputFilename = filename

		records = append(records, outputRecord{
			ID:             chunks[i].chunkID,
			SourceFile:     chunks[i].sourceFile,
			DocumentTitle:  chunks[i].documentTitle,
			HeadingPath:    append([]string(nil), chunks[i].headingPath...),
			SectionLevel:   chunks[i].sectionLevel,
			ChunkIndex:     chunks[i].chunkIndex,
			OutputFilename: filename,
			Text:           chunks[i].chunkText,
		})
	}

	return moniker, records, nil
}

func buildChunks(source markdownSource, splitLevel, maxHeadingLevel, minChars, maxChars int) (string, []chunk, string, error) {
	textBytes, err := os.ReadFile(source.path)
	if err != nil {
		return "", nil, "", fmt.Errorf("read markdown %s: %w", source.path, err)
	}
	text := string(textBytes)

	root := parseMarkdownSections(text)
	documentTitle := strings.TrimSuffix(filepath.Base(source.path), filepath.Ext(source.path))
	var h1Node *sectionNode
	if len(root.children) > 0 && root.children[0].level == 1 {
		h1Node = root.children[0]
		documentTitle = h1Node.title
	}

	targetNodes := make([]*sectionNode, 0)
	var walk func(node *sectionNode)
	walk = func(node *sectionNode) {
		if node.level == splitLevel {
			targetNodes = append(targetNodes, node)
			return
		}
		for _, child := range node.children {
			walk(child)
		}
	}
	walk(root)
	if len(targetNodes) == 0 {
		if len(root.children) > 0 {
			targetNodes = append(targetNodes, root.children...)
		} else {
			targetNodes = []*sectionNode{root}
		}
	}

	chunks := make([]chunk, 0)
	if h1Node != nil && splitLevel > 1 {
		h1Intro := h1Node.ownBodyText()
		if h1Intro != "" {
			chunks = append(chunks, chunk{
				sourceFile:    source.sourceFile,
				documentTitle: documentTitle,
				headingPath:   h1Node.pathTitles(),
				chunkIndex:    0,
				chunkText:     buildChunkText(h1Node, h1Intro),
				sectionLevel:  h1Node.level,
			})
		}
	}

	for _, node := range targetNodes {
		chunks = append(chunks, splitNode(node, source.sourceFile, documentTitle, maxHeadingLevel, minChars, maxChars)...)
	}

	for i := range chunks {
		chunks[i].chunkIndex = i + 1
		chunks[i].chunkID = stableChunkID(source.sourceFile, chunks[i].headingPath, chunks[i].chunkIndex)
	}

	return text, chunks, documentTitle, nil
}

func parseMarkdownSections(text string) *sectionNode {
	root := &sectionNode{level: 0, title: ""}
	stack := []*sectionNode{root}

	for _, line := range strings.Split(text, "\n") {
		match := headingRE.FindStringSubmatch(line)
		if len(match) == 3 {
			level := len(match[1])
			title := strings.TrimSpace(match[2])
			for len(stack) > 0 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			parent := stack[len(stack)-1]
			node := &sectionNode{level: level, title: title, parent: parent}
			parent.children = append(parent.children, node)
			stack = append(stack, node)
			continue
		}

		stack[len(stack)-1].contentLines = append(stack[len(stack)-1].contentLines, line)
	}

	return root
}

func buildChunkText(node *sectionNode, introBody string) string {
	parts := []string{fmt.Sprintf("%s %s", strings.Repeat("#", node.level), node.title)}
	body := strings.TrimSpace(introBody)
	if body == "" {
		body = node.ownBodyText()
	}
	if body != "" {
		parts = append(parts, body)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func shouldSplitNode(node *sectionNode, maxHeadingLevel, maxChars int) bool {
	if len(node.children) == 0 || node.level >= maxHeadingLevel {
		return false
	}
	subtreeLen := len(node.subtreeMarkdown())
	return subtreeLen > maxChars || len(node.children) > 1
}

func splitNode(node *sectionNode, sourceFile, documentTitle string, maxHeadingLevel, minChars, maxChars int) []chunk {
	subtreeText := node.subtreeMarkdown()
	if !shouldSplitNode(node, maxHeadingLevel, maxChars) {
		return []chunk{{
			sourceFile:    sourceFile,
			documentTitle: documentTitle,
			headingPath:   node.pathTitles(),
			chunkIndex:    0,
			chunkText:     subtreeText,
			sectionLevel:  node.level,
		}}
	}

	chunks := make([]chunk, 0)
	introBody := node.ownBodyText()
	if introBody != "" && len(introBody) >= minChars {
		chunks = append(chunks, chunk{
			sourceFile:    sourceFile,
			documentTitle: documentTitle,
			headingPath:   node.pathTitles(),
			chunkIndex:    0,
			chunkText:     buildChunkText(node, introBody),
			sectionLevel:  node.level,
		})
	}

	for _, child := range node.children {
		chunks = append(chunks, splitNode(child, sourceFile, documentTitle, maxHeadingLevel, minChars, maxChars)...)
	}

	if len(chunks) == 0 {
		chunks = append(chunks, chunk{
			sourceFile:    sourceFile,
			documentTitle: documentTitle,
			headingPath:   node.pathTitles(),
			chunkIndex:    0,
			chunkText:     subtreeText,
			sectionLevel:  node.level,
		})
	}

	return chunks
}

func (n *sectionNode) headingText() string {
	return fmt.Sprintf("%s %s", strings.Repeat("#", n.level), n.title)
}

func (n *sectionNode) pathTitles() []string {
	var titles []string
	for node := n; node != nil && node.level > 0; node = node.parent {
		titles = append(titles, node.title)
	}
	for i, j := 0, len(titles)-1; i < j; i, j = i+1, j-1 {
		titles[i], titles[j] = titles[j], titles[i]
	}
	return titles
}

func (n *sectionNode) ownBodyText() string {
	if len(n.contentLines) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(n.contentLines, "\n"))
}

func (n *sectionNode) subtreeMarkdown() string {
	parts := make([]string, 0, 1+len(n.children))
	if n.level > 0 {
		parts = append(parts, n.headingText())
	}
	if body := strings.TrimSpace(strings.Join(n.contentLines, "\n")); body != "" {
		parts = append(parts, body)
	}
	for _, child := range n.children {
		if childMD := strings.TrimSpace(child.subtreeMarkdown()); childMD != "" {
			parts = append(parts, childMD)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func stableChunkID(sourceFile string, headingPath []string, index int) string {
	raw := fmt.Sprintf("%s|%s|%d", sourceFile, strings.Join(headingPath, " > "), index)
	sum := sha1.Sum([]byte(raw))
	return fmt.Sprintf("%x", sum[:8])
}

func slugWords(value string, maxWords int) string {
	words := wordRE.FindAllString(strings.ToLower(value), -1)
	if len(words) == 0 {
		return "section"
	}
	if maxWords > 0 && len(words) > maxWords {
		words = words[:maxWords]
	}
	return strings.Join(words, "_")
}

func sanitizeMoniker(value string) string {
	cleaned := strings.ToUpper(nonAlnumRE.ReplaceAllString(value, ""))
	if len(cleaned) >= 10 {
		return cleaned[:10]
	}
	sum := sha1.Sum([]byte(value))
	return (cleaned + strings.ToUpper(fmt.Sprintf("%x", sum[:])))[:10]
}

func fallbackMoniker(filename, fullText string) string {
	stem := strings.ToUpper(nonAlnumRE.ReplaceAllString(strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)), ""))
	sum := sha1.Sum([]byte(filename + "\n" + fullText))
	return (stem + strings.ToUpper(fmt.Sprintf("%x", sum[:])))[:10]
}

func fallbackDescriptor(ch chunk) string {
	if len(ch.headingPath) >= 3 {
		return slugWords(strings.Join(ch.headingPath[len(ch.headingPath)-2:], " "), 8)
	}
	if len(ch.headingPath) == 1 {
		return slugWords(ch.headingPath[0], 8)
	}
	if len(ch.headingPath) > 1 {
		return slugWords(strings.Join(ch.headingPath, " "), 8)
	}
	return "section"
}

func uniqueFilename(baseName string, seen map[string]int) string {
	count := seen[baseName] + 1
	seen[baseName] = count
	if count == 1 {
		return baseName + ".md"
	}
	return fmt.Sprintf("%s_%02d.md", baseName, count)
}

func generateMoniker(ctx context.Context, client LLMClient, sourceFile, documentTitle, fullText string) (string, error) {
	messages := []llm.Message{
		{Role: "system", Content: "You generate strict JSON only."},
		{
			Role: "user",
			Content: "Create a stable project moniker for this markdown document.\n" +
				"Requirements:\n" +
				"- Exactly 10 uppercase alphanumeric characters\n" +
				"- No punctuation, spaces, or underscores\n" +
				"- Must reflect the filename and overall document subject\n" +
				"- Prefer pronounceable or mnemonic output when possible\n\n" +
				"Filename: " + sourceFile + "\n" +
				"Document title: " + documentTitle + "\n\n" +
				"Document contents:\n" + truncate(fullText, 12000),
		},
	}

	result, err := client.ChatJSON(ctx, messages, "document_moniker", documentMonikerSchema())
	if err != nil {
		return "", err
	}

	value, _ := result["moniker"].(string)
	if value == "" {
		return "", fmt.Errorf("empty moniker response")
	}
	return sanitizeMoniker(value), nil
}

func generateDescriptor(ctx context.Context, client LLMClient, ch chunk) (string, error) {
	messages := []llm.Message{
		{Role: "system", Content: "You generate strict JSON only."},
		{
			Role: "user",
			Content: "Create a lowercase underscore-separated descriptor for this markdown chunk.\n" +
				"Requirements:\n" +
				"- Up to 8 words\n" +
				"- Only lowercase letters and digits\n" +
				"- Use underscores between words\n" +
				"- Focus on what this section is about, not generic filler\n\n" +
				"Heading path: " + strings.Join(ch.headingPath, " > ") + "\n" +
				"Chunk text:\n" + truncate(ch.chunkText, 6000),
		},
	}

	result, err := client.ChatJSON(ctx, messages, "chunk_descriptor", chunkDescriptorSchema())
	if err != nil {
		return "", err
	}

	value, _ := result["descriptor"].(string)
	if value == "" {
		return "", fmt.Errorf("empty descriptor response")
	}
	return slugWords(value, 8), nil
}

func documentMonikerSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"moniker": map[string]any{
				"type":        "string",
				"description": "Exactly 10 uppercase alphanumeric characters.",
				"pattern":     "^[A-Z0-9]{10}$",
			},
		},
		"required":             []string{"moniker"},
		"additionalProperties": false,
	}
}

func chunkDescriptorSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"descriptor": map[string]any{
				"type":        "string",
				"description": "Up to 8 lowercase underscore-separated descriptive words.",
				"pattern":     "^[a-z0-9]+(?:_[a-z0-9]+){0,7}$",
			},
		},
		"required":             []string{"descriptor"},
		"additionalProperties": false,
	}
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
