package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Duara-Cortex/sekha-cluster-tool/internal/client"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/config"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/model"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/orchestrator"
	"github.com/Duara-Cortex/sekha-cluster-tool/internal/telemetry"
)

var (
	// Version is injected at link time via -ldflags.
	Version = "v1.0.0-dev"
)

// GlobalFlags captures CLI arguments specified globally across any subcommand position.
type GlobalFlags struct {
	EnvPath           string
	SensoryURL        string
	WorkingURL        string
	KnowledgeURL      string
	OrchestrationURL  string
	TraceID           string
	Verbose           bool
	Timeout           time.Duration
	Anchors           []string
	AnchorMode        string
	IncludeEmbeddings bool
}

// anchorSliceFlag enables repeatable and comma-delimited string flags.
type anchorSliceFlag []string

func (f *anchorSliceFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *anchorSliceFlag) Set(val string) error {
	*f = append(*f, val)
	return nil
}

func matchStringFlag(arg string, names ...string) (bool, string, bool) {
	for _, name := range names {
		prefix1 := "-" + name
		prefix2 := "--" + name

		if arg == prefix1 || arg == prefix2 {
			return true, "", false
		}
		if strings.HasPrefix(arg, prefix1+"=") {
			return true, strings.TrimPrefix(arg, prefix1+"="), true
		}
		if strings.HasPrefix(arg, prefix2+"=") {
			return true, strings.TrimPrefix(arg, prefix2+"="), true
		}
	}
	return false, "", false
}

func matchBoolFlag(arg string, names ...string) (bool, bool, bool) {
	for _, name := range names {
		prefix1 := "-" + name
		prefix2 := "--" + name

		if arg == prefix1 || arg == prefix2 {
			return true, true, false
		}
		if strings.HasPrefix(arg, prefix1+"=") {
			v, err := strconv.ParseBool(strings.TrimPrefix(arg, prefix1+"="))
			if err == nil {
				return true, v, true
			}
			return true, true, true
		}
		if strings.HasPrefix(arg, prefix2+"=") {
			v, err := strconv.ParseBool(strings.TrimPrefix(arg, prefix2+"="))
			if err == nil {
				return true, v, true
			}
			return true, true, true
		}
	}
	return false, false, false
}

func parseArgs(rawArgs []string) (GlobalFlags, string, []string) {
	var global GlobalFlags
	var subcommand string
	var subcommandArgs []string

	i := 0
	for i < len(rawArgs) {
		arg := rawArgs[i]

		if arg == "--" {
			i++
			if subcommand == "" && i < len(rawArgs) {
				subcommand = rawArgs[i]
				i++
			}
			for i < len(rawArgs) {
				subcommandArgs = append(subcommandArgs, rawArgs[i])
				i++
			}
			break
		}

		if (arg == "-h" || arg == "--help" || arg == "-help") && subcommand == "" {
			subcommand = "help"
			i++
			continue
		}

		if (arg == "-v" || arg == "--version" || arg == "-version") && subcommand == "" {
			subcommand = "version"
			i++
			continue
		}

		if m, val, _ := matchBoolFlag(arg, "verbose"); m {
			global.Verbose = val
			i++
			continue
		}

		if m, val, inline := matchStringFlag(arg, "sensory-url", "node3-url"); m {
			if inline {
				global.SensoryURL = val
				i++
			} else if i+1 < len(rawArgs) {
				global.SensoryURL = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "working-url", "node2-url"); m {
			if inline {
				global.WorkingURL = val
				i++
			} else if i+1 < len(rawArgs) {
				global.WorkingURL = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "knowledge-url", "node1-url"); m {
			if inline {
				global.KnowledgeURL = val
				i++
			} else if i+1 < len(rawArgs) {
				global.KnowledgeURL = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "orchestration-url", "orchestrator-url"); m {
			if inline {
				global.OrchestrationURL = val
				i++
			} else if i+1 < len(rawArgs) {
				global.OrchestrationURL = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "env-file"); m {
			if inline {
				global.EnvPath = val
				i++
			} else if i+1 < len(rawArgs) {
				global.EnvPath = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "trace-id"); m {
			if inline {
				global.TraceID = val
				i++
			} else if i+1 < len(rawArgs) {
				global.TraceID = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "timeout"); m {
			var durStr string
			if inline {
				durStr = val
				i++
			} else if i+1 < len(rawArgs) {
				durStr = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			if d, err := time.ParseDuration(durStr); err == nil {
				global.Timeout = d
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "anchor", "a"); m {
			if inline {
				global.Anchors = append(global.Anchors, val)
				i++
			} else if i+1 < len(rawArgs) {
				global.Anchors = append(global.Anchors, rawArgs[i+1])
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "anchor-mode"); m {
			if inline {
				global.AnchorMode = val
				i++
			} else if i+1 < len(rawArgs) {
				global.AnchorMode = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, _ := matchBoolFlag(arg, "include-embeddings"); m {
			global.IncludeEmbeddings = val
			i++
			continue
		}

		if subcommand == "" && !strings.HasPrefix(arg, "-") {
			subcommand = arg
			i++
			continue
		}

		subcommandArgs = append(subcommandArgs, arg)
		i++
	}

	if global.OrchestrationURL != "" && global.WorkingURL == "" {
		global.WorkingURL = global.OrchestrationURL
	}

	return global, subcommand, subcommandArgs
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	global, command, args := parseArgs(os.Args[1:])

	if global.Verbose {
		telemetry.SetVerbose(true)
	}

	switch command {
	case "filter":
		runFilter(global, args)
	case "recall":
		runRecall(global, args)
	case "deliberate":
		runDeliberate(global, args)
	case "consolidate":
		runConsolidate(global, args)
	case "orchestrate", "run-loop":
		runOrchestrate(global, args)
	case "status", "health":
		runStatus(global, args)
	case "env", "config":
		runEnv(global, args)
	case "version", "--version", "-v":
		outputJSON(map[string]string{
			"tool":    "sekha-cluster-tool",
			"version": Version,
		})
	case "help", "--help", "-h":
		printUsage()
	case "":
		printUsage()
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown subcommand '%s'\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `sekha-cluster-tool - Tri-Node Cognitive Cluster Orchestration CLI (%s)

Usage:
  sekha-cluster-tool [global flags] <command> [arguments...]
  sekha-cluster-tool <command> [global flags] [arguments...]

Subcommands:
  filter        Dispatch raw sensory streams to sensory attention gate (:8081)
  recall        Retrieve associative knowledge subgraphs from knowledge layer (:8084)
  deliberate    Dispatch active working memory to working scratchpad (:8083)
  consolidate   Commit resolved episodic deliberation traces to consolidation layer (:8084)
  orchestrate   Execute the full 4-stage closed-loop cognitive turn (Sensory -> Recall -> Deliberate -> Consolidate)
  status        Check operational health and ping latency across cluster endpoints
  env           Manage and inspect cluster environment configuration (init, show)
  version       Display tool version and build information

Global Flags:
  --env-file <path>          Path to .env configuration file (searches ./.env, ./.env.local by default)
  --sensory-url <url>        Sensory filter endpoint URL (legacy alias: --node3-url)
  --working-url <url>        Working memory scratchpad URL (legacy alias: --node2-url)
  --knowledge-url <url>      Knowledge store & recall URL (legacy alias: --node1-url)
  --orchestration-url <url>  Orchestration working memory URL alias
  --anchor, -a <tag>         Target anchor tag (repeatable or comma-delimited, e.g. -a "#project:kestrel")
  --anchor-mode <mode>       Anchor recall mode: boost or filter (default: boost)
  --include-embeddings       Include vector embeddings in recall responses (default: false)
  --verbose                  Enable diagnostic step logs on stderr (stdout remains clean JSON)
  --trace-id <id>            Specify or propagate an explicit X-Trace-ID
  --timeout <duration>       Operation or probe timeout budget (e.g. 1500ms, 2s)

Environment Variables (.env / OS):
  CLUSTER_ENV_FILE       Path to custom .env configuration file
  CLUSTER_SENSORY_URL    Sensory Buffer base URL (fallback: SEKHA_NODE3_URL)
  CLUSTER_WORKING_URL    Working Scratchpad base URL (fallback: SEKHA_NODE2_URL)
  CLUSTER_KNOWLEDGE_URL  Knowledge Store base URL (fallback: SEKHA_NODE1_URL)
`, Version)
}

func outputJSON(data interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to serialise output JSON: %v\n", err)
		os.Exit(1)
	}
}

func outputError(traceID, message string) {
	outputJSON(map[string]interface{}{
		"status":   "error",
		"error":    message,
		"trace_id": traceID,
	})
	os.Exit(1)
}

func readInput(textFlag, fileFlag string) (string, error) {
	if textFlag != "" {
		return textFlag, nil
	}
	if fileFlag == "-" {
		bytes, err := io.ReadAll(os.Stdin)
		return string(bytes), err
	}
	if fileFlag != "" {
		bytes, err := os.ReadFile(fileFlag)
		return string(bytes), err
	}
	return "", nil
}

// pickURL returns the first non-empty URL among candidates.
func pickURL(candidates ...string) string {
	for _, c := range candidates {
		if c != "" {
			return c
		}
	}
	return ""
}

// runEnv handles the 'env' / 'config' subcommand (init, show).
func runEnv(global GlobalFlags, args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: sekha-cluster-tool env [init|show] [options...]\n")
		os.Exit(1)
	}

	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "init":
		fs := flag.NewFlagSet("env init", flag.ExitOnError)
		outputFlag := fs.String("output", ".env", "Target path to write starter .env template")
		_ = fs.Parse(actionArgs)

		if err := os.WriteFile(*outputFlag, []byte(config.TemplateDotEnv()), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing .env file '%s': %v\n", *outputFlag, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Starter .env template written to: %s\n", *outputFlag)
		fmt.Fprintf(os.Stderr, "Note: Endpoint addresses default to blank. Please configure URLs for your cluster nodes.\n")

	case "show":
		fs := flag.NewFlagSet("env show", flag.ExitOnError)
		envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
		_ = fs.Parse(actionArgs)

		cfg, err := config.Load(config.FlagOverrides{
			EnvPath:      pickURL(*envFileFlag, global.EnvPath),
			SensoryURL:   global.SensoryURL,
			WorkingURL:   global.WorkingURL,
			KnowledgeURL: global.KnowledgeURL,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
			os.Exit(1)
		}
		outputJSON(cfg)

	default:
		fmt.Fprintf(os.Stderr, "Unknown env action '%s'. Supported: init, show\n", action)
		os.Exit(1)
	}
}

// runFilter handles the 'filter' subcommand.
func runFilter(global GlobalFlags, args []string) {
	fs := flag.NewFlagSet("filter", flag.ExitOnError)
	textFlag := fs.String("text", "", "Raw text input to filter")
	fileFlag := fs.String("file", "", "Path to text file (or '-' for stdin)")
	directiveFlag := fs.String("directive", "", "Task directive to guide salience scoring")
	thresholdFlag := fs.Float64("threshold", 0.0, "Salience retention threshold (0.0 to 1.0)")
	fromBufferFlag := fs.Bool("from-buffer", false, "Filter directly from in-memory ring buffer")
	sensoryURLFlag := fs.String("sensory-url", "", "Sensory filter endpoint URL")
	node3URLFlag := fs.String("node3-url", "", "Legacy alias for --sensory-url")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	timeoutFlag := fs.Duration("timeout", 0, "Operation timeout budget")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	timeout := *timeoutFlag
	if timeout == 0 && global.Timeout > 0 {
		timeout = global.Timeout
	}

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:           pickURL(*envFileFlag, global.EnvPath),
		SensoryURL:        pickURL(*sensoryURLFlag, *node3URLFlag, global.SensoryURL),
		Timeout:           timeout,
		SalienceThreshold: *thresholdFlag,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateSensory(); err != nil {
		outputError(traceID, err.Error())
	}

	text, err := readInput(*textFlag, *fileFlag)
	if err != nil {
		outputError(traceID, fmt.Sprintf("Error reading input: %v", err))
	}

	if text == "" && !*fromBufferFlag {
		outputError(traceID, "Must provide either --text, --file, or --from-buffer")
	}

	sensoryClient := client.NewSensoryClient(cfg.SensoryURL, cfg.DefaultTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DefaultTimeout)
	defer cancel()

	req := model.FilterRequest{
		Text:          text,
		TaskDirective: *directiveFlag,
		Threshold:     cfg.SalienceThreshold,
		FromBuffer:    *fromBufferFlag,
	}

	resp, err := sensoryClient.Filter(ctx, req, traceID)
	if err != nil {
		outputError(traceID, err.Error())
	}

	outputJSON(resp)
}

// runRecall handles the 'recall' subcommand.
func runRecall(global GlobalFlags, args []string) {
	fs := flag.NewFlagSet("recall", flag.ExitOnError)
	queryFlag := fs.String("query", "", "Search query for associative knowledge recall")
	topKFlag := fs.Int("top-k", 0, "Number of ranked nodes to retrieve")
	alphaFlag := fs.Float64("alpha", 0.6, "Similarity weight")
	betaFlag := fs.Float64("beta", 0.2, "Access frequency weight")
	gammaFlag := fs.Float64("gamma", 0.2, "Recency decay weight")
	hopsFlag := fs.Int("hops", 1, "Graph relational expansion hops")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	timeoutFlag := fs.Duration("timeout", 0, "Operation timeout budget")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	var anchorFlags anchorSliceFlag
	fs.Var(&anchorFlags, "anchor", "Target anchor tag (repeatable or comma-delimited, e.g. -a \"#project:kestrel\")")
	fs.Var(&anchorFlags, "a", "Target anchor tag alias")
	anchorModeFlag := fs.String("anchor-mode", "boost", "Anchor recall mode: boost or filter (default: boost)")
	includeEmbeddingsFlag := fs.Bool("include-embeddings", false, "Include vector embeddings in recall results (default: false)")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	if *queryFlag == "" {
		outputError(traceID, "--query flag is required")
	}

	anchors := model.ParseAnchors(append(global.Anchors, anchorFlags...)...)

	anchorMode := strings.ToLower(strings.TrimSpace(*anchorModeFlag))
	if anchorMode == "boost" && global.AnchorMode != "" {
		anchorMode = strings.ToLower(strings.TrimSpace(global.AnchorMode))
	}
	if anchorMode == "" {
		anchorMode = "boost"
	}
	if anchorMode != "boost" && anchorMode != "filter" {
		outputError(traceID, fmt.Sprintf("invalid --anchor-mode '%s': must be 'boost' or 'filter'", *anchorModeFlag))
	}

	includeEmbeddings := *includeEmbeddingsFlag || global.IncludeEmbeddings

	timeout := *timeoutFlag
	if timeout == 0 && global.Timeout > 0 {
		timeout = global.Timeout
	}

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:      pickURL(*envFileFlag, global.EnvPath),
		KnowledgeURL: pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		Timeout:      timeout,
		RecallTopK:   *topKFlag,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateKnowledge(); err != nil {
		outputError(traceID, err.Error())
	}

	knowledgeClient := client.NewKnowledgeClient(cfg.KnowledgeURL, cfg.DefaultTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DefaultTimeout)
	defer cancel()

	req := model.RecallRequest{
		Query:             *queryFlag,
		TopK:              cfg.RecallTopK,
		Alpha:             *alphaFlag,
		Beta:              *betaFlag,
		Gamma:             *gammaFlag,
		ExpandHops:        *hopsFlag,
		Anchors:           anchors,
		AnchorMode:        anchorMode,
		IncludeEmbeddings: includeEmbeddings,
	}

	resp, err := knowledgeClient.Recall(ctx, req, traceID)
	if err != nil {
		outputError(traceID, err.Error())
	}

	outputJSON(resp)
}

// runDeliberate handles the 'deliberate' subcommand.
func runDeliberate(global GlobalFlags, args []string) {
	fs := flag.NewFlagSet("deliberate", flag.ExitOnError)
	taskFlag := fs.String("task", "", "Deliberation objective or prompt task")
	inputFlag := fs.String("input", "", "Salient input text or sensory chunks JSON")
	contextFlag := fs.String("context", "", "Comma-separated long-term context facts or JSON array")
	observationFlag := fs.String("observation", "", "External tool observation from previous step")
	maxTokensFlag := fs.Int("max-tokens", 256, "Maximum token generation budget")
	temperatureFlag := fs.Float64("temperature", 0.2, "Sampling temperature")
	workingURLFlag := fs.String("working-url", "", "Working memory scratchpad endpoint URL")
	node2URLFlag := fs.String("node2-url", "", "Legacy alias for --working-url")
	orchestrationURLFlag := fs.String("orchestration-url", "", "Alias for working memory URL")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	timeoutFlag := fs.Duration("timeout", 0, "Operation timeout budget")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	if *taskFlag == "" {
		outputError(traceID, "--task flag is required")
	}

	timeout := *timeoutFlag
	if timeout == 0 && global.Timeout > 0 {
		timeout = global.Timeout
	}

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:           pickURL(*envFileFlag, global.EnvPath),
		WorkingURL:        pickURL(*workingURLFlag, *node2URLFlag, *orchestrationURLFlag, global.WorkingURL, global.OrchestrationURL),
		OrchestrationURL: pickURL(*orchestrationURLFlag, global.OrchestrationURL),
		DeliberateTimeout: timeout,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateWorking(); err != nil {
		outputError(traceID, err.Error())
	}

	// Parse sensory chunks if provided
	var sensoryChunks []model.SensoryChunk
	if *inputFlag != "" {
		if strings.HasPrefix(strings.TrimSpace(*inputFlag), "[") {
			_ = json.Unmarshal([]byte(*inputFlag), &sensoryChunks)
		} else {
			sensoryChunks = append(sensoryChunks, model.SensoryChunk{
				ID:        fmt.Sprintf("chk-%d", time.Now().UnixNano()),
				Text:      *inputFlag,
				Salience:  0.80,
				Timestamp: time.Now(),
			})
		}
	}

	// Parse long-term facts
	var longTermFacts []string
	if *contextFlag != "" {
		if strings.HasPrefix(strings.TrimSpace(*contextFlag), "[") {
			_ = json.Unmarshal([]byte(*contextFlag), &longTermFacts)
		} else {
			parts := strings.Split(*contextFlag, ";")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					longTermFacts = append(longTermFacts, trimmed)
				}
			}
		}
	}

	workingClient := client.NewWorkingClient(cfg.WorkingURL, cfg.DeliberateTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DeliberateTimeout)
	defer cancel()

	req := model.DeliberateRequest{
		Objective:       *taskFlag,
		SensoryChunks:   sensoryChunks,
		LongTermContext: longTermFacts,
		Observation:     *observationFlag,
		MaxTokens:       *maxTokensFlag,
		Temperature:     *temperatureFlag,
	}

	resp, err := workingClient.Deliberate(ctx, req, traceID)
	if err != nil {
		outputError(traceID, err.Error())
	}

	outputJSON(resp)
}

// runConsolidate handles the 'consolidate' subcommand.
func runConsolidate(global GlobalFlags, args []string) {
	fs := flag.NewFlagSet("consolidate", flag.ExitOnError)
	traceJSONFlag := fs.String("trace", "", "JSON string or path to episodic trace file")
	sessionIDFlag := fs.String("session-id", "", "Session identifier")
	goalFlag := fs.String("goal", "", "Task goal description")
	outcomeFlag := fs.String("outcome", "success", "Outcome: success or failure")
	syncFlag := fs.Bool("sync", false, "Execute synchronous consolidation cycle")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	timeoutFlag := fs.Duration("timeout", 0, "Operation timeout budget")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	var anchorFlags anchorSliceFlag
	fs.Var(&anchorFlags, "anchor", "Target anchor tag (repeatable or comma-delimited, e.g. -a \"#project:kestrel\")")
	fs.Var(&anchorFlags, "a", "Target anchor tag alias")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	anchors := model.ParseAnchors(append(global.Anchors, anchorFlags...)...)

	timeout := *timeoutFlag
	if timeout == 0 && global.Timeout > 0 {
		timeout = global.Timeout
	}

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:      pickURL(*envFileFlag, global.EnvPath),
		KnowledgeURL: pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		Timeout:      timeout,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateKnowledge(); err != nil {
		outputError(traceID, err.Error())
	}

	var req model.ConsolidateRequest
	if *traceJSONFlag != "" {
		var content []byte
		var err error
		if strings.HasPrefix(strings.TrimSpace(*traceJSONFlag), "{") {
			content = []byte(*traceJSONFlag)
		} else {
			content, err = os.ReadFile(*traceJSONFlag)
			if err != nil {
				outputError(traceID, fmt.Sprintf("Error reading trace file: %v", err))
			}
		}
		if err := json.Unmarshal(content, &req); err != nil {
			outputError(traceID, fmt.Sprintf("Error unmarshalling trace JSON: %v", err))
		}
	}

	if len(anchors) > 0 {
		req.Anchors = model.ParseAnchors(append(req.Anchors, anchors...)...)
	}

	if *sessionIDFlag != "" {
		req.SessionID = *sessionIDFlag
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("session-%d", time.Now().Unix())
	}
	if *goalFlag != "" {
		req.TaskGoal = *goalFlag
	}
	if *outcomeFlag != "" {
		req.Outcome = *outcomeFlag
	}
	req.Synchronous = *syncFlag
	req.TraceID = traceID

	knowledgeClient := client.NewKnowledgeClient(cfg.KnowledgeURL, cfg.DefaultTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DefaultTimeout)
	defer cancel()

	resp, err := knowledgeClient.Consolidate(ctx, req, traceID)
	if err != nil {
		outputError(traceID, err.Error())
	}

	outputJSON(resp)
}

// runOrchestrate handles the 'orchestrate' / 'run-loop' subcommand.
func runOrchestrate(global GlobalFlags, args []string) {
	fs := flag.NewFlagSet("orchestrate", flag.ExitOnError)
	inputFlag := fs.String("input", "", "Raw sensory stream or log text")
	fileFlag := fs.String("file", "", "Path to raw input file (or '-' for stdin)")
	directiveFlag := fs.String("directive", "", "High-level cognitive goal or task directive")
	thresholdFlag := fs.Float64("threshold", 0.0, "Salience retention threshold")
	topKFlag := fs.Int("top-k", 0, "Number of associative knowledge nodes to recall")
	maxTokensFlag := fs.Int("max-tokens", 256, "Max tokens for working memory deliberation")
	sessionIDFlag := fs.String("session-id", "", "Unique session identifier")
	syncFlag := fs.Bool("sync", false, "Synchronous memory consolidation")
	sensoryURLFlag := fs.String("sensory-url", "", "Sensory filter endpoint URL")
	workingURLFlag := fs.String("working-url", "", "Working memory scratchpad endpoint URL")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	node3URLFlag := fs.String("node3-url", "", "Legacy alias for --sensory-url")
	node2URLFlag := fs.String("node2-url", "", "Legacy alias for --working-url")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	orchestrationURLFlag := fs.String("orchestration-url", "", "Alias for working memory URL")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	var anchorFlags anchorSliceFlag
	fs.Var(&anchorFlags, "anchor", "Target anchor tag (repeatable or comma-delimited, e.g. -a \"#project:kestrel\")")
	fs.Var(&anchorFlags, "a", "Target anchor tag alias")
	anchorModeFlag := fs.String("anchor-mode", "boost", "Anchor recall mode: boost or filter (default: boost)")
	includeEmbeddingsFlag := fs.Bool("include-embeddings", false, "Include vector embeddings in recall results (default: false)")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	anchors := model.ParseAnchors(append(global.Anchors, anchorFlags...)...)

	anchorMode := strings.ToLower(strings.TrimSpace(*anchorModeFlag))
	if anchorMode == "boost" && global.AnchorMode != "" {
		anchorMode = strings.ToLower(strings.TrimSpace(global.AnchorMode))
	}
	if anchorMode == "" {
		anchorMode = "boost"
	}
	if anchorMode != "boost" && anchorMode != "filter" {
		outputError(traceID, fmt.Sprintf("invalid --anchor-mode '%s': must be 'boost' or 'filter'", *anchorModeFlag))
	}

	includeEmbeddings := *includeEmbeddingsFlag || global.IncludeEmbeddings

	timeout := global.Timeout

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:          pickURL(*envFileFlag, global.EnvPath),
		SensoryURL:       pickURL(*sensoryURLFlag, *node3URLFlag, global.SensoryURL),
		WorkingURL:       pickURL(*workingURLFlag, *node2URLFlag, *orchestrationURLFlag, global.WorkingURL, global.OrchestrationURL),
		KnowledgeURL:     pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		OrchestrationURL: pickURL(*orchestrationURLFlag, global.OrchestrationURL),
		Timeout:          timeout,
		SalienceThreshold: *thresholdFlag,
		RecallTopK:       *topKFlag,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateAll(); err != nil {
		outputError(traceID, err.Error())
	}

	rawText, err := readInput(*inputFlag, *fileFlag)
	if err != nil {
		outputError(traceID, fmt.Sprintf("Error reading input: %v", err))
	}

	if rawText == "" && *directiveFlag == "" {
		outputError(traceID, "Must provide either --input/--file or --directive")
	}

	orch := orchestrator.NewOrchestrator(*cfg)
	ctx := context.Background()

	req := model.OrchestrateRequest{
		RawInput:               rawText,
		TaskDirective:          *directiveFlag,
		FilterThreshold:        cfg.SalienceThreshold,
		RecallTopK:             cfg.RecallTopK,
		MaxTokens:              *maxTokensFlag,
		SessionID:              *sessionIDFlag,
		SynchronousConsolidate: *syncFlag,
		Anchors:                anchors,
		AnchorMode:             anchorMode,
		IncludeEmbeddings:      includeEmbeddings,
	}

	resp, err := orch.RunCycle(ctx, req, traceID)
	if err != nil {
		outputError(traceID, err.Error())
	}

	outputJSON(resp)
}

// runStatus queries and aggregates operational health across configured cluster endpoints.
func runStatus(global GlobalFlags, args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	sensoryURLFlag := fs.String("sensory-url", "", "Sensory filter endpoint URL")
	workingURLFlag := fs.String("working-url", "", "Working memory scratchpad endpoint URL")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	node3URLFlag := fs.String("node3-url", "", "Legacy alias for --sensory-url")
	node2URLFlag := fs.String("node2-url", "", "Legacy alias for --working-url")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	orchestrationURLFlag := fs.String("orchestration-url", "", "Alias for working memory URL")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	timeoutFlag := fs.Duration("timeout", 1500*time.Millisecond, "Probe timeout per node")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	timeout := *timeoutFlag
	if global.Timeout > 0 {
		timeout = global.Timeout
	}

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:          pickURL(*envFileFlag, global.EnvPath),
		SensoryURL:       pickURL(*sensoryURLFlag, *node3URLFlag, global.SensoryURL),
		WorkingURL:       pickURL(*workingURLFlag, *node2URLFlag, *orchestrationURLFlag, global.WorkingURL, global.OrchestrationURL),
		KnowledgeURL:     pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		OrchestrationURL: pickURL(*orchestrationURLFlag, global.OrchestrationURL),
		Timeout:          timeout,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	type NodeStatus struct {
		Name     string      `json:"name"`
		Role     string      `json:"role"`
		URL      string      `json:"url"`
		Status   string      `json:"status"`
		Duration float64     `json:"ping_ms"`
		Details  interface{} `json:"details,omitempty"`
		Error    string      `json:"error,omitempty"`
	}

	results := make([]NodeStatus, 3)

	// 1. Probe Sensory Layer
	if cfg.SensoryURL == "" {
		results[0] = NodeStatus{
			Name:   "Sensory Layer",
			Role:   "Sensory Buffer & Attention Filter",
			URL:    "(not configured)",
			Status: "unconfigured",
			Error:  "Endpoint address not set in .env, environment, flags, or build",
		}
	} else {
		sClient := client.NewSensoryClient(cfg.SensoryURL, timeout)
		t0 := time.Now()
		ctx3, cancel3 := context.WithTimeout(context.Background(), timeout)
		sStats, err := sClient.GetStats(ctx3, traceID)
		cancel3()
		d3 := float64(time.Since(t0).Microseconds()) / 1000.0
		if err != nil {
			results[0] = NodeStatus{
				Name:     "Sensory Layer",
				Role:     "Sensory Buffer & Attention Filter",
				URL:      cfg.SensoryURL,
				Status:   "unreachable",
				Duration: d3,
				Error:    err.Error(),
			}
		} else {
			results[0] = NodeStatus{
				Name:     "Sensory Layer",
				Role:     "Sensory Buffer & Attention Filter",
				URL:      cfg.SensoryURL,
				Status:   "healthy",
				Duration: d3,
				Details:  sStats,
			}
		}
	}

	// 2. Probe Working Memory Layer
	if cfg.WorkingURL == "" {
		results[1] = NodeStatus{
			Name:   "Working Memory Layer",
			Role:   "Working Memory & Inference Engine",
			URL:    "(not configured)",
			Status: "unconfigured",
			Error:  "Endpoint address not set in .env, environment, flags, or build",
		}
	} else {
		wClient := client.NewWorkingClient(cfg.WorkingURL, timeout)
		t1 := time.Now()
		ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
		wHealth, err := wClient.GetHealth(ctx2, traceID)
		cancel2()
		d2 := float64(time.Since(t1).Microseconds()) / 1000.0
		if err != nil {
			results[1] = NodeStatus{
				Name:     "Working Memory Layer",
				Role:     "Working Memory & Inference Engine",
				URL:      cfg.WorkingURL,
				Status:   "unreachable",
				Duration: d2,
				Error:    err.Error(),
			}
		} else {
			results[1] = NodeStatus{
				Name:     "Working Memory Layer",
				Role:     "Working Memory & Inference Engine",
				URL:      cfg.WorkingURL,
				Status:   "healthy",
				Duration: d2,
				Details:  wHealth,
			}
		}
	}

	// 3. Probe Knowledge Layer
	if cfg.KnowledgeURL == "" {
		results[2] = NodeStatus{
			Name:   "Knowledge Layer",
			Role:   "Knowledge Graph & Consolidation Store",
			URL:    "(not configured)",
			Status: "unconfigured",
			Error:  "Endpoint address not set in .env, environment, flags, or build",
		}
	} else {
		kClient := client.NewKnowledgeClient(cfg.KnowledgeURL, timeout)
		t2 := time.Now()
		ctx1, cancel1 := context.WithTimeout(context.Background(), timeout)
		kHealth, err := kClient.GetHealth(ctx1, traceID)
		cancel1()
		d1 := float64(time.Since(t2).Microseconds()) / 1000.0
		if err != nil {
			results[2] = NodeStatus{
				Name:     "Knowledge Layer",
				Role:     "Knowledge Graph & Consolidation Store",
				URL:      cfg.KnowledgeURL,
				Status:   "unreachable",
				Duration: d1,
				Error:    err.Error(),
			}
		} else {
			results[2] = NodeStatus{
				Name:     "Knowledge Layer",
				Role:     "Knowledge Graph & Consolidation Store",
				URL:      cfg.KnowledgeURL,
				Status:   "healthy",
				Duration: d1,
				Details:  kHealth,
			}
		}
	}

	clusterHealthy := true
	for _, r := range results {
		if r.Status != "healthy" {
			clusterHealthy = false
			break
		}
	}

	overallStatus := "all_nodes_healthy"
	if !clusterHealthy {
		overallStatus = "degraded_or_partially_offline"
	}

	outputJSON(map[string]interface{}{
		"cluster_status":   overallStatus,
		"env_file_loaded":  cfg.EnvFileLoaded,
		"trace_id":         traceID,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"nodes":            results,
	})
}
