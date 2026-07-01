package cli

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"msmgr/internal/llm"
	"msmgr/internal/splitmd"
)

type splitMarkdownOptions struct {
	inputPath       string
	outputDir       string
	manifestPath    string
	splitLevel      int
	maxHeadingLevel int
	minChars        int
	maxChars        int
	useLLM          bool
	dryRun          bool
}

func (a *App) runSplitMarkdown(args []string) error {
	opts, err := parseSplitMarkdownArgs(args)
	if err != nil {
		return err
	}

	var client splitmd.LLMClient
	if opts.useLLM {
		cfg, err := a.loadConfig()
		if err != nil {
			return err
		}

		generator, err := llm.NewClient(cfg, &http.Client{Timeout: a.effectiveRequestTimeout()})
		if err != nil {
			return err
		}
		client = generator
	}

	return splitmd.Run(context.Background(), splitmd.Options{
		InputPath:       opts.inputPath,
		OutputDir:       opts.outputDir,
		ManifestPath:    opts.manifestPath,
		SplitLevel:      opts.splitLevel,
		MaxHeadingLevel: opts.maxHeadingLevel,
		MinChars:        opts.minChars,
		MaxChars:        opts.maxChars,
		DryRun:          opts.dryRun,
	}, client, a.stdout)
}

func parseSplitMarkdownArgs(args []string) (splitMarkdownOptions, error) {
	opts := splitMarkdownOptions{
		splitLevel:      2,
		maxHeadingLevel: 3,
		minChars:        220,
		maxChars:        1800,
	}
	if len(args) == 0 {
		return opts, fmt.Errorf("missing input path for split-markdown\n\n%s", usageText)
	}

	opts.inputPath = strings.TrimSpace(args[0])
	if opts.inputPath == "" {
		return opts, fmt.Errorf("missing input path for split-markdown\n\n%s", usageText)
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--output-dir":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --output-dir\n\n%s", usageText)
			}
			i++
			opts.outputDir = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--output-dir="):
			opts.outputDir = strings.TrimSpace(strings.TrimPrefix(arg, "--output-dir="))
		case arg == "--manifest":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --manifest\n\n%s", usageText)
			}
			i++
			opts.manifestPath = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--manifest="):
			opts.manifestPath = strings.TrimSpace(strings.TrimPrefix(arg, "--manifest="))
		case arg == "--split-level":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --split-level\n\n%s", usageText)
			}
			i++
			level, err := parseSplitMarkdownLevel("--split-level", args[i])
			if err != nil {
				return opts, err
			}
			opts.splitLevel = level
		case strings.HasPrefix(arg, "--split-level="):
			level, err := parseSplitMarkdownLevel("--split-level", strings.TrimPrefix(arg, "--split-level="))
			if err != nil {
				return opts, err
			}
			opts.splitLevel = level
		case arg == "--max-heading-level":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --max-heading-level\n\n%s", usageText)
			}
			i++
			level, err := parseSplitMarkdownLevel("--max-heading-level", args[i])
			if err != nil {
				return opts, err
			}
			opts.maxHeadingLevel = level
		case strings.HasPrefix(arg, "--max-heading-level="):
			level, err := parseSplitMarkdownLevel("--max-heading-level", strings.TrimPrefix(arg, "--max-heading-level="))
			if err != nil {
				return opts, err
			}
			opts.maxHeadingLevel = level
		case arg == "--min-chars":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --min-chars\n\n%s", usageText)
			}
			i++
			value, err := parsePositiveIntFlag("--min-chars", args[i])
			if err != nil {
				return opts, err
			}
			opts.minChars = value
		case strings.HasPrefix(arg, "--min-chars="):
			value, err := parsePositiveIntFlag("--min-chars", strings.TrimPrefix(arg, "--min-chars="))
			if err != nil {
				return opts, err
			}
			opts.minChars = value
		case arg == "--max-chars":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("missing value for --max-chars\n\n%s", usageText)
			}
			i++
			value, err := parsePositiveIntFlag("--max-chars", args[i])
			if err != nil {
				return opts, err
			}
			opts.maxChars = value
		case strings.HasPrefix(arg, "--max-chars="):
			value, err := parsePositiveIntFlag("--max-chars", strings.TrimPrefix(arg, "--max-chars="))
			if err != nil {
				return opts, err
			}
			opts.maxChars = value
		case arg == "--use-llm":
			opts.useLLM = true
		case arg == "--dry-run":
			opts.dryRun = true
		case arg == "--help" || arg == "-h":
			return opts, fmt.Errorf("%s", usageText)
		default:
			return opts, fmt.Errorf("unknown split-markdown option %q\n\n%s", arg, usageText)
		}
	}

	return opts, nil
}

func parseSplitMarkdownLevel(name, value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 || parsed > 6 {
		return 0, fmt.Errorf("%s must be between 1 and 6", name)
	}
	return parsed, nil
}
