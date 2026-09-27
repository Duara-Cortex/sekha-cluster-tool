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
	// Version is injected at link time via -ldflags or defaults to the release version.
	Version = "v1.0.11"
)

// GlobalFlags captures CLI arguments specified globally across any subcommand position.
type GlobalFlags struct {
	EnvPath           string
	SensoryURL        string
	WorkingURL        string
	KnowledgeURL      string
	OrchestrationURL  string
	APIKey            string
	TLSCACert         string
	Insecure          bool
	Secret            bool
	AllowSecret       bool
	TraceID           string
	Verbose           bool
	Timeout           time.Duration
	Anchors           []string
	AnchorMode        string
	IncludeEmbeddings bool
	Format            string
	EntityType        string
	MinScore          float64
	JSON              bool
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

// concatFlag accumulates repeated occurrences of a payload flag (e.g. --input "part1" --input "part2")
// and joins them in order with no separator, so callers can split a payload at arbitrary byte
// offsets to stay under per-argument OS limits.
type concatFlag struct {
	parts []string
}

func (f *concatFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(f.parts, "")
}

func (f *concatFlag) Set(val string) error {
	f.parts = append(f.parts, val)
	return nil
}

// Len returns the combined byte length of every occurrence.
func (f *concatFlag) Len() int {
	n := 0
	for _, p := range f.parts {
		n += len(p)
	}
	return n
}

// payloadFlags take a free-text value that must reach the subcommand verbatim, even when the
// value itself looks like a global flag (e.g. --input "--verbose output from syslog").
var payloadFlags = []string{"input", "text", "trace", "file", "directive", "task", "goal", "query", "context", "observation"}

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

		if m, _, inline := matchStringFlag(arg, payloadFlags...); m && subcommand != "" {
			subcommandArgs = append(subcommandArgs, arg)
			i++
			if !inline && i < len(rawArgs) {
				subcommandArgs = append(subcommandArgs, rawArgs[i])
				i++
			}
			continue
		}

		if m, val, _ := matchBoolFlag(arg, "verbose"); m {
			global.Verbose = val
			i++
			continue
		}

		if m, val, _ := matchBoolFlag(arg, "json"); m {
			global.JSON = val
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

		if m, val, inline := matchStringFlag(arg, "api-key", "key", "token"); m {
			if inline {
				global.APIKey = val
				i++
			} else if i+1 < len(rawArgs) {
				global.APIKey = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "tls-ca-cert"); m {
			if inline {
				global.TLSCACert = val
				i++
			} else if i+1 < len(rawArgs) {
				global.TLSCACert = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, _ := matchBoolFlag(arg, "insecure"); m {
			global.Insecure = val
			i++
			continue
		}

		if m, val, _ := matchBoolFlag(arg, "secret"); m {
			global.Secret = val
			i++
			continue
		}

		if m, val, _ := matchBoolFlag(arg, "allow-secret"); m {
			global.AllowSecret = val
			i++
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

		if m, val, inline := matchStringFlag(arg, "format"); m {
			if inline {
				global.Format = val
				i++
			} else if i+1 < len(rawArgs) {
				global.Format = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "type", "t"); m {
			if inline {
				global.EntityType = val
				i++
			} else if i+1 < len(rawArgs) {
				global.EntityType = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			continue
		}

		if m, val, inline := matchStringFlag(arg, "min-score"); m {
			var scoreStr string
			if inline {
				scoreStr = val
				i++
			} else if i+1 < len(rawArgs) {
				scoreStr = rawArgs[i+1]
				i += 2
			} else {
				i++
			}
			if s, err := strconv.ParseFloat(scoreStr, 64); err == nil {
				global.MinScore = s
			}
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
	case "ping":
		os.Exit(runPing(global, args))
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
  ping          Preflight connectivity check across cluster endpoints (exit 0/1)
  env           Manage and inspect cluster environment configuration (init, show)
  version       Display tool version and build information

Global Flags:
  --env-file <path>          Path to .env configuration file (searches ./.env, ./.env.local by default)
  --sensory-url <url>        Sensory filter endpoint URL (legacy alias: --node3-url)
  --working-url <url>        Working memory scratchpad URL (legacy alias: --node2-url)
  --knowledge-url <url>      Knowledge store & recall URL (legacy alias: --node1-url)
  --orchestration-url <url>  Orchestration working memory URL alias
  --api-key, --token <key>   Node 1 API key for protected knowledge endpoints
  --tls-ca-cert <path>       Custom root CA certificate file for cluster HTTPS
  --insecure                 Skip TLS certificate verification (InsecureSkipVerify: true)
  --secret                   Enforce payload encryption at rest (consolidate)
  --allow-secret             Permit persisting raw credentials/secrets (consolidate)
  --anchor, -a <tag>         Target anchor tag (repeatable or comma-delimited, e.g. -a "#project:kestrel")
  --anchor-mode <mode>       Anchor recall mode: boost or filter (default: boost)
  --include-embeddings       Include vector embeddings in recall responses (default: false)
  --format <format>          Output format for recall: json, concise, or markdown (default: json)
  --type, -t <type>          Filter recall results by entity type
  --min-score <score>        Minimum recall candidate score threshold (default: 0.0)
  --verbose                  Enable diagnostic step logs on stderr (stdout remains clean JSON)
  --json                     Output result in structured JSON format (for status, ping)
  --trace-id <id>            Specify or propagate an explicit X-Trace-ID
  --timeout <duration>       Operation or probe timeout budget (e.g. 500ms, 2s)

Large Payloads (filter/orchestrate/consolidate --input/--text, consolidate --trace):
  Payload flags are repeatable; occurrences are concatenated in order with no separator,
  e.g. --input "$PART1" --input "$PART2". Use this to stay under per-argument OS limits
  (Linux MAX_ARG_STRLEN = 128 KiB). Combined input is capped by --max-input-bytes
  (default 1 MiB, env CLUSTER_MAX_INPUT_BYTES); oversized input fails and is never truncated.

Orchestrate Relevance & Budget:
  --min-sim <score>          Minimum recall sim_score passed to Node 2 (default 0.50, env CLUSTER_RECALL_MIN_SIM)
  --context-tokens <n>       Node 2 context window (default 4096, env CLUSTER_DELIBERATE_CONTEXT_TOKENS)
  --prompt-reserve <n>       Tokens reserved for Node 2 prompt template (default 384, env CLUSTER_DELIBERATE_PROMPT_RESERVE)

Environment Variables (.env / OS):
  CLUSTER_ENV_FILE       Path to custom .env configuration file
  CLUSTER_SENSORY_URL    Sensory Buffer base URL (fallback: SEKHA_NODE3_URL)
  CLUSTER_WORKING_URL    Working Scratchpad base URL (fallback: SEKHA_NODE2_URL)
  CLUSTER_KNOWLEDGE_URL  Knowledge Store base URL (fallback: SEKHA_NODE1_URL)
  CLUSTER_API_KEY        Node 1 API Key (fallback: SEKHA_API_KEY)
  CLUSTER_TLS_CA_CERT    Custom root CA certificate path for HTTPS
  CLUSTER_INSECURE       Skip TLS certificate verification (true/false)
  CLUSTER_MAX_INPUT_BYTES            Combined payload cap in bytes (default 1048576)
  CLUSTER_RECALL_MIN_SIM             Relevance gate sim_score floor (default 0.50)
  CLUSTER_DELIBERATE_CONTEXT_TOKENS  Node 2 context window in tokens (default 4096)
  CLUSTER_DELIBERATE_PROMPT_RESERVE  Node 2 prompt template reserve in tokens (default 384)
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

var osExit = os.Exit

// Format401Diagnostic enriches HTTP 401 error messages with actionable operator advice.
func Format401Diagnostic(message string) string {
	if strings.Contains(message, "401") || strings.Contains(strings.ToLower(message), "unauthorized") {
		if !strings.Contains(message, "CLUSTER_API_KEY") || !strings.Contains(message, "--token") {
			return fmt.Sprintf("HTTP 401 Unauthorized: API key is invalid or missing. Check .env (CLUSTER_API_KEY / SEKHA_API_KEY) or pass --token (details: %s)", message)
		}
	}
	return message
}

func outputError(traceID, message string) {
	message = Format401Diagnostic(message)
	outputJSON(map[string]interface{}{
		"status":   "error",
		"error":    message,
		"trace_id": traceID,
	})
	osExit(1)
}

func setDoubleDashUsage(fs *flag.FlagSet) {
	fs.SetOutput(os.Stdout)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of %s:\n", fs.Name())
		fs.VisitAll(func(f *flag.Flag) {
			s := fmt.Sprintf("  --%s", f.Name)
			name, usage := flag.UnquoteUsage(f)
			if len(name) > 0 {
				s += " " + name
			}
			if len(s) <= 4 {
				s += "\t"
			} else {
				s += "\n    \t"
			}
			s += strings.ReplaceAll(usage, "\n", "\n    \t")
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
				s += fmt.Sprintf(" (default %s)", f.DefValue)
			}
			fmt.Fprintf(fs.Output(), "%s\n", s)
		})
	}
}

// parseFlagSetWithPositionals parses flags from args while allowing positional arguments
// to appear before, between, or after flags. Returns the positional arguments in order.
func parseFlagSetWithPositionals(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	remaining := args
	for len(remaining) > 0 {
		if err := fs.Parse(remaining); err != nil {
			return nil, err
		}
		parsedArgs := fs.Args()
		if len(parsedArgs) == 0 {
			break
		}
		if len(remaining) > 0 && remaining[0] == "--" {
			positional = append(positional, parsedArgs...)
			break
		}
		positional = append(positional, parsedArgs[0])
		remaining = parsedArgs[1:]
	}
	return positional, nil
}

func validateFormat(raw string) (string, error) {
	if raw == "" {
		return "json", nil
	}
	f := strings.ToLower(strings.TrimSpace(raw))
	switch f {
	case "json", "concise", "markdown":
		return f, nil
	default:
		return "", fmt.Errorf("invalid --format '%s': must be 'json', 'concise', or 'markdown'", raw)
	}
}

func renderRecallOutput(w io.Writer, resp *model.RecallResponse, format string) error {
	switch format {
	case "concise", "markdown":
		_, err := fmt.Fprintln(w, resp.FormatMarkdown())
		return err
	case "json", "":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	default:
		return fmt.Errorf("invalid --format '%s': must be 'json', 'concise', or 'markdown'", format)
	}
}

// errInputTooLarge builds the actionable error returned when a payload exceeds the input cap.
// Input is never truncated: the invocation fails before anything is sent to the cluster.
func errInputTooLarge(flagName string, size, limit int) error {
	if strings.Contains(flagName, " ") {
		return fmt.Errorf("%s payload is %d bytes, exceeding the %d-byte input limit; nothing was sent and input is never truncated. "+
			"Raise the limit with --max-input-bytes or CLUSTER_MAX_INPUT_BYTES, or pass large text via repeated --input flags",
			flagName, size, limit)
	}
	return fmt.Errorf("--%s payload is %d bytes, exceeding the %d-byte input limit; nothing was sent and input is never truncated. "+
		"Raise the limit with --max-input-bytes or CLUSTER_MAX_INPUT_BYTES, or split the work across invocations. "+
		"To pass more than the OS allows in one argument (Linux: 128 KiB per argument), repeat the flag: --%s \"part1\" --%s \"part2\" (parts are joined in order)",
		flagName, size, limit, flagName, flagName)
}

// readLimited reads r up to limit bytes, failing rather than truncating if more is available.
func readLimited(r io.Reader, source string, limit int) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return "", err
	}
	if len(data) > limit {
		return "", fmt.Errorf("%s exceeds the %d-byte input limit; nothing was sent and input is never truncated. Raise the limit with --max-input-bytes or CLUSTER_MAX_INPUT_BYTES", source, limit)
	}
	return string(data), nil
}

// readInput resolves the payload from repeated inline flags or --file ('-' for stdin), enforcing
// maxBytes across whichever source is used.
func readInput(inline *concatFlag, inlineName, fileFlag string, maxBytes int) (string, error) {
	if inline.Len() > 0 && fileFlag != "" {
		return "", fmt.Errorf("--%s and --file are mutually exclusive; pass one input source", inlineName)
	}
	if inline.Len() > 0 {
		if inline.Len() > maxBytes {
			return "", errInputTooLarge(inlineName, inline.Len(), maxBytes)
		}
		return inline.String(), nil
	}
	if fileFlag == "-" {
		return readLimited(os.Stdin, "stdin input", maxBytes)
	}
	if fileFlag != "" {
		f, err := os.Open(fileFlag)
		if err != nil {
			return "", err
		}
		defer f.Close()
		return readLimited(f, fmt.Sprintf("input file '%s'", fileFlag), maxBytes)
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
		apiKeyFlag := fs.String("api-key", "", "API key for Node 1 authentication")
		tokenFlag := fs.String("token", "", "Token alias for Node 1 authentication")
		keyFlag := fs.String("key", "", "Legacy alias for API key")
		tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
		insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
		_ = fs.Parse(actionArgs)

		cfg, err := config.Load(config.FlagOverrides{
			EnvPath:      pickURL(*envFileFlag, global.EnvPath),
			SensoryURL:   global.SensoryURL,
			WorkingURL:   global.WorkingURL,
			KnowledgeURL: global.KnowledgeURL,
			APIKey:       pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
			TLSCACert:    pickURL(*tlsCACertFlag, global.TLSCACert),
			Insecure:     *insecureFlag || global.Insecure,
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
	var textFlag concatFlag
	fs.Var(&textFlag, "text", "Raw text input to filter (repeatable; occurrences are concatenated in order)")
	fs.Var(&textFlag, "input", "Alias for --text (repeatable)")
	fileFlag := fs.String("file", "", "Path to text file (or '-' for stdin); convenience for human operators")
	maxInputFlag := fs.Int("max-input-bytes", 0, "Maximum combined input size in bytes (default 1048576)")
	directiveFlag := fs.String("directive", "", "Task directive to guide salience scoring")
	thresholdFlag := fs.Float64("threshold", 0.0, "Salience retention threshold (0.0 to 1.0)")
	fromBufferFlag := fs.Bool("from-buffer", false, "Filter directly from in-memory ring buffer")
	sensoryURLFlag := fs.String("sensory-url", "", "Sensory filter endpoint URL")
	node3URLFlag := fs.String("node3-url", "", "Legacy alias for --sensory-url")
	apiKeyFlag := fs.String("api-key", "", "API key for cluster authentication")
	tokenFlag := fs.String("token", "", "Token alias for cluster authentication")
	keyFlag := fs.String("key", "", "Legacy alias for API key")
	tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
	insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
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
		APIKey:            pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
		TLSCACert:         pickURL(*tlsCACertFlag, global.TLSCACert),
		Insecure:          *insecureFlag || global.Insecure,
		Timeout:           timeout,
		SalienceThreshold: *thresholdFlag,
		MaxInputBytes:     *maxInputFlag,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateTLS(); err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateSensory(); err != nil {
		outputError(traceID, err.Error())
	}

	text, err := readInput(&textFlag, "text", *fileFlag, cfg.MaxInputBytes)
	if err != nil {
		outputError(traceID, fmt.Sprintf("Error reading input: %v", err))
	}

	if text == "" && !*fromBufferFlag {
		outputError(traceID, "Must provide either --text, --file, or --from-buffer")
	}

	requestTimeout := client.PayloadTimeout(cfg.DefaultTimeout, len(text))
	sensoryClient := client.NewSensoryClient(cfg.SensoryURL, requestTimeout, cfg.TLSCACert, cfg.Insecure)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
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
	setDoubleDashUsage(fs)
	queryFlag := fs.String("query", "", "Search query for associative knowledge recall")
	topKFlag := fs.Int("top-k", 0, "Number of ranked nodes to retrieve")
	alphaFlag := fs.Float64("alpha", 0.6, "Similarity weight")
	betaFlag := fs.Float64("beta", 0.2, "Access frequency weight")
	gammaFlag := fs.Float64("gamma", 0.2, "Recency decay weight")
	hopsFlag := fs.Int("hops", 1, "Graph relational expansion hops")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	apiKeyFlag := fs.String("api-key", "", "API key for Node 1 authentication")
	tokenFlag := fs.String("token", "", "Token alias for Node 1 authentication")
	keyFlag := fs.String("key", "", "Legacy alias for API key")
	tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
	insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	timeoutFlag := fs.Duration("timeout", 0, "Operation timeout budget")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	var anchorFlags anchorSliceFlag
	fs.Var(&anchorFlags, "anchor", "Target anchor tag (repeatable or comma-delimited, e.g. -a \"#project:kestrel\")")
	fs.Var(&anchorFlags, "a", "Target anchor tag alias")
	anchorModeFlag := fs.String("anchor-mode", "boost", "Anchor recall mode: boost or filter (default: boost)")
	includeEmbeddingsFlag := fs.Bool("include-embeddings", false, "Include vector embeddings in recall results (default: false)")
	formatFlag := fs.String("format", "", "Output format: json, concise, or markdown (default: json)")
	typeFlag := fs.String("type", "", "Filter recall results by entity type")
	fs.StringVar(typeFlag, "t", "", "Filter recall results by entity type (alias)")
	minScoreFlag := fs.Float64("min-score", 0.0, "Minimum recall candidate score threshold (default: 0.0)")
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

	rawFormat := *formatFlag
	if rawFormat == "" {
		rawFormat = global.Format
	}
	format, err := validateFormat(rawFormat)
	if err != nil {
		outputError(traceID, err.Error())
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
		APIKey:       pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
		TLSCACert:    pickURL(*tlsCACertFlag, global.TLSCACert),
		Insecure:     *insecureFlag || global.Insecure,
		Timeout:      timeout,
		RecallTopK:   *topKFlag,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateTLS(); err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateKnowledge(); err != nil {
		outputError(traceID, err.Error())
	}

	knowledgeClient := client.NewKnowledgeClient(cfg.KnowledgeURL, cfg.DefaultTimeout, cfg.APIKey, cfg.TLSCACert, cfg.Insecure)
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

	entityType := *typeFlag
	if entityType == "" {
		entityType = global.EntityType
	}
	minScore := *minScoreFlag
	if minScore == 0.0 && global.MinScore > 0 {
		minScore = global.MinScore
	}

	resp, err := knowledgeClient.Recall(ctx, req, traceID)
	if err != nil {
		outputError(traceID, err.Error())
	}

	resp.Filter(entityType, minScore)

	switch format {
	case "concise", "markdown":
		fmt.Println(resp.FormatMarkdown())
	default:
		outputJSON(resp)
	}
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
	apiKeyFlag := fs.String("api-key", "", "API key for cluster authentication")
	tokenFlag := fs.String("token", "", "Token alias for cluster authentication")
	keyFlag := fs.String("key", "", "Legacy alias for API key")
	tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
	insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
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
		OrchestrationURL:  pickURL(*orchestrationURLFlag, global.OrchestrationURL),
		APIKey:            pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
		TLSCACert:         pickURL(*tlsCACertFlag, global.TLSCACert),
		Insecure:          *insecureFlag || global.Insecure,
		DeliberateTimeout: timeout,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateTLS(); err != nil {
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

	workingClient := client.NewWorkingClient(cfg.WorkingURL, cfg.DeliberateTimeout, cfg.TLSCACert, cfg.Insecure)
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
	setDoubleDashUsage(fs)
	origUsage := fs.Usage
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage:\n")
		fmt.Fprintf(fs.Output(), "  sekha-cluster-tool consolidate \"<label>\" \"<summary>\" [options...]\n")
		fmt.Fprintf(fs.Output(), "  sekha-cluster-tool consolidate --trace '<json>' [options...]\n\n")
		origUsage()
	}

	var traceJSONFlag concatFlag
	fs.Var(&traceJSONFlag, "trace", "Inline trace JSON or path to trace file (repeatable; occurrences are concatenated in order)")
	var inputFlag concatFlag
	fs.Var(&inputFlag, "input", "Raw sensory text to consolidate as sensory_context (repeatable; occurrences are concatenated in order)")
	fs.Var(&inputFlag, "text", "Alias for --input (repeatable)")
	maxInputFlag := fs.Int("max-input-bytes", 0, "Maximum combined input size in bytes (default 1048576)")
	sessionIDFlag := fs.String("session-id", "", "Session identifier")
	goalFlag := fs.String("goal", "", "Task goal description")
	outcomeFlag := fs.String("outcome", "success", "Outcome: success or failure")
	syncFlag := fs.Bool("sync", false, "Execute synchronous consolidation cycle")
	secretFlag := fs.Bool("secret", false, "Mark consolidated trace as secret/encrypted at rest")
	allowSecretFlag := fs.Bool("allow-secret", false, "Confirm persistence of detected raw credentials")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	apiKeyFlag := fs.String("api-key", "", "API key for Node 1 authentication")
	tokenFlag := fs.String("token", "", "Token alias for Node 1 authentication")
	keyFlag := fs.String("key", "", "Legacy alias for API key")
	tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
	insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	timeoutFlag := fs.Duration("timeout", 0, "Operation timeout budget")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	var anchorFlags anchorSliceFlag
	fs.Var(&anchorFlags, "anchor", "Target anchor tag (repeatable or comma-delimited, e.g. -a \"#project:kestrel\")")
	fs.Var(&anchorFlags, "a", "Target anchor tag alias")

	positionalArgs, _ := parseFlagSetWithPositionals(fs, args)

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
		EnvPath:       pickURL(*envFileFlag, global.EnvPath),
		KnowledgeURL:  pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		APIKey:        pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
		TLSCACert:     pickURL(*tlsCACertFlag, global.TLSCACert),
		Insecure:      *insecureFlag || global.Insecure,
		Timeout:       timeout,
		MaxInputBytes: *maxInputFlag,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateTLS(); err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateKnowledge(); err != nil {
		outputError(traceID, err.Error())
	}

	traceArg := traceJSONFlag.String()
	positionalBytes := 0
	for _, p := range positionalArgs {
		positionalBytes += len(p)
	}
	if total := traceJSONFlag.Len() + inputFlag.Len() + positionalBytes; total > cfg.MaxInputBytes {
		source := "input"
		if traceJSONFlag.Len() >= inputFlag.Len() && traceJSONFlag.Len() >= positionalBytes {
			source = "trace"
		} else if positionalBytes > inputFlag.Len() {
			source = "positional arguments"
		}
		outputError(traceID, errInputTooLarge(source, total, cfg.MaxInputBytes).Error())
	}

	var traceContent []byte
	var req model.ConsolidateRequest
	if traceArg != "" {
		if strings.HasPrefix(strings.TrimSpace(traceArg), "{") {
			traceContent = []byte(traceArg)
		} else {
			f, err := os.Open(traceArg)
			if err != nil {
				outputError(traceID, fmt.Sprintf("Error reading trace file: %v", err))
			}
			content, err := readLimited(f, fmt.Sprintf("trace file '%s'", traceArg), cfg.MaxInputBytes)
			f.Close()
			if err != nil {
				outputError(traceID, fmt.Sprintf("Error reading trace file: %v", err))
			}
			traceContent = []byte(content)
		}
		if err := json.Unmarshal(traceContent, &req); err != nil {
			outputError(traceID, fmt.Sprintf("Error unmarshalling trace JSON: %v", err))
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
		} else if len(positionalArgs) == 1 && req.TaskGoal == "" {
			req.TaskGoal = positionalArgs[0]
		}
		if *outcomeFlag != "" {
			req.Outcome = *outcomeFlag
		}
		req.Synchronous = *syncFlag
		req.TraceID = traceID
	} else if len(positionalArgs) >= 2 {
		req = model.BuildPositionalConsolidation(
			positionalArgs[0],
			positionalArgs[1],
			*sessionIDFlag,
			*goalFlag,
			*outcomeFlag,
			anchors,
			*syncFlag,
			traceID,
		)
	} else {
		if len(anchors) > 0 {
			req.Anchors = anchors
		}
		if *sessionIDFlag != "" {
			req.SessionID = *sessionIDFlag
		}
		if req.SessionID == "" {
			req.SessionID = fmt.Sprintf("session-%d", time.Now().Unix())
		}
		if *goalFlag != "" {
			req.TaskGoal = *goalFlag
		} else if len(positionalArgs) == 1 {
			req.TaskGoal = positionalArgs[0]
		}
		if *outcomeFlag != "" {
			req.Outcome = *outcomeFlag
		}
		req.Synchronous = *syncFlag
		req.TraceID = traceID
	}

	inputText := inputFlag.String()
	if inputText != "" {
		req.SensoryContext = append(req.SensoryContext, model.SensoryItem{
			ID:        fmt.Sprintf("cli-input-%d", time.Now().UnixNano()),
			Text:      inputText,
			Salience:  1.0,
			Source:    "cli_input",
			Timestamp: time.Now().UTC(),
		})
	}

	allowSecret := *allowSecretFlag || global.AllowSecret
	isSecret := *secretFlag || global.Secret

	// Credential persistence safeguards: detect raw secrets in arguments, trace content, or request fields
	var textsToScan []string
	textsToScan = append(textsToScan, positionalArgs...)
	if inputText != "" {
		textsToScan = append(textsToScan, inputText)
	}
	if traceArg != "" {
		textsToScan = append(textsToScan, traceArg)
		// Inline traces are the argument itself; only file contents need a second scan.
		if len(traceContent) > 0 && string(traceContent) != traceArg {
			textsToScan = append(textsToScan, string(traceContent))
		}
	}

	detected, secretType := model.DetectRawSecretsInStrings(textsToScan...)
	if !detected {
		detected, secretType = req.DetectSecrets()
	}

	if detected {
		if !allowSecret {
			outputError(traceID, fmt.Sprintf("raw credentials detected (%s) in consolidation payload; persistence rejected. Use external references (e.g. vault:// or env://) or pass --allow-secret to confirm encryption at rest", secretType))
		}
		req.IsSecret = true
	} else if isSecret || allowSecret {
		req.IsSecret = true
	}

	requestTimeout := client.PayloadTimeout(cfg.DefaultTimeout, len(traceContent)+len(inputText)+positionalBytes)
	knowledgeClient := client.NewKnowledgeClient(cfg.KnowledgeURL, requestTimeout, cfg.APIKey, cfg.TLSCACert, cfg.Insecure)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
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
	setDoubleDashUsage(fs)
	var inputFlag concatFlag
	fs.Var(&inputFlag, "input", "Raw sensory stream or log text (repeatable; occurrences are concatenated in order)")
	fs.Var(&inputFlag, "text", "Alias for --input (repeatable)")
	fileFlag := fs.String("file", "", "Path to raw input file (or '-' for stdin); convenience for human operators")
	maxInputFlag := fs.Int("max-input-bytes", 0, "Maximum combined input size in bytes (default 1048576)")
	minSimFlag := fs.Float64("min-sim", 0, "Relevance gate: minimum recall sim_score to reach Node 2 (default 0.50)")
	contextTokensFlag := fs.Int("context-tokens", 0, "Node 2 context window in tokens (default 4096)")
	promptReserveFlag := fs.Int("prompt-reserve", 0, "Tokens reserved for the Node 2 prompt template (default 384)")
	directiveFlag := fs.String("directive", "", "High-level cognitive goal or task directive")
	taskFlag := fs.String("task", "", "Alias for --directive: High-level cognitive goal or task directive")
	thresholdFlag := fs.Float64("threshold", 0.0, "Salience retention threshold")
	topKFlag := fs.Int("top-k", 0, "Number of associative knowledge nodes to recall")
	maxTokensFlag := fs.Int("max-tokens", 256, "Max tokens for working memory deliberation")
	sessionIDFlag := fs.String("session-id", "", "Unique session identifier")
	syncFlag := fs.Bool("sync", false, "Synchronous memory consolidation")
	sensoryURLFlag := fs.String("sensory-url", "", "Sensory filter endpoint URL")
	workingURLFlag := fs.String("working-url", "", "Working memory scratchpad endpoint URL")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	apiKeyFlag := fs.String("api-key", "", "API key for Node 1 authentication")
	tokenFlag := fs.String("token", "", "Token alias for Node 1 authentication")
	keyFlag := fs.String("key", "", "Legacy alias for API key")
	tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
	insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
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
	_, _ = parseFlagSetWithPositionals(fs, args)

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
		EnvPath:           pickURL(*envFileFlag, global.EnvPath),
		SensoryURL:        pickURL(*sensoryURLFlag, *node3URLFlag, global.SensoryURL),
		WorkingURL:        pickURL(*workingURLFlag, *node2URLFlag, *orchestrationURLFlag, global.WorkingURL, global.OrchestrationURL),
		KnowledgeURL:      pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		OrchestrationURL:  pickURL(*orchestrationURLFlag, global.OrchestrationURL),
		APIKey:            pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
		TLSCACert:         pickURL(*tlsCACertFlag, global.TLSCACert),
		Insecure:          *insecureFlag || global.Insecure,
		Timeout:           timeout,
		SalienceThreshold: *thresholdFlag,
		RecallTopK:        *topKFlag,
		RecallMinSim:      *minSimFlag,
		MaxInputBytes:     *maxInputFlag,
		ContextTokens:     *contextTokensFlag,
		PromptReserve:     *promptReserveFlag,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateTLS(); err != nil {
		outputError(traceID, err.Error())
	}

	if err := cfg.ValidateAll(); err != nil {
		outputError(traceID, err.Error())
	}

	rawText, err := readInput(&inputFlag, "input", *fileFlag, cfg.MaxInputBytes)
	if err != nil {
		outputError(traceID, fmt.Sprintf("Error reading input: %v", err))
	}

	directive := strings.TrimSpace(*directiveFlag)
	if directive == "" {
		directive = strings.TrimSpace(*taskFlag)
	}

	if rawText == "" && directive == "" {
		outputError(traceID, "Must provide either --input/--file or --task/--directive")
	}

	orch := orchestrator.NewOrchestrator(*cfg)
	ctx := context.Background()

	req := model.OrchestrateRequest{
		RawInput:               rawText,
		TaskDirective:          directive,
		FilterThreshold:        cfg.SalienceThreshold,
		RecallTopK:             cfg.RecallTopK,
		MaxTokens:              *maxTokensFlag,
		SessionID:              *sessionIDFlag,
		SynchronousConsolidate: *syncFlag,
		Anchors:                anchors,
		AnchorMode:             anchorMode,
		IncludeEmbeddings:      includeEmbeddings,
		RecallMinSim:           cfg.RecallMinSim,
		ContextTokens:          cfg.ContextTokens,
		PromptReserveTokens:    cfg.PromptReserve,
	}

	resp, err := orch.RunCycle(ctx, req, traceID)
	if err != nil {
		outputError(traceID, err.Error())
	}

	outputJSON(resp)
}

// runStatus queries and aggregates operational health across configured cluster endpoints.
func runStatus(global GlobalFlags, args []string) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	setDoubleDashUsage(fs)
	sensoryURLFlag := fs.String("sensory-url", "", "Sensory filter endpoint URL")
	workingURLFlag := fs.String("working-url", "", "Working memory scratchpad endpoint URL")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	apiKeyFlag := fs.String("api-key", "", "API key for Node 1 authentication")
	tokenFlag := fs.String("token", "", "Token alias for Node 1 authentication")
	keyFlag := fs.String("key", "", "Legacy alias for API key")
	tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
	insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
	node3URLFlag := fs.String("node3-url", "", "Legacy alias for --sensory-url")
	node2URLFlag := fs.String("node2-url", "", "Legacy alias for --working-url")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	orchestrationURLFlag := fs.String("orchestration-url", "", "Alias for working memory URL")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	jsonFlag := fs.Bool("json", false, "Emit cluster status as structured JSON")
	formatFlag := fs.String("format", "", "Output format: text or json")

	timeout := 500 * time.Millisecond
	if global.Timeout > 0 {
		timeout = global.Timeout
	}
	fs.DurationVar(&timeout, "timeout", timeout, "Probe timeout per node (default: 500ms)")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:          pickURL(*envFileFlag, global.EnvPath),
		SensoryURL:       pickURL(*sensoryURLFlag, *node3URLFlag, global.SensoryURL),
		WorkingURL:       pickURL(*workingURLFlag, *node2URLFlag, *orchestrationURLFlag, global.WorkingURL, global.OrchestrationURL),
		KnowledgeURL:     pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		OrchestrationURL: pickURL(*orchestrationURLFlag, global.OrchestrationURL),
		APIKey:           pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
		TLSCACert:        pickURL(*tlsCACertFlag, global.TLSCACert),
		Insecure:         *insecureFlag || global.Insecure,
		Timeout:          timeout,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	probeResult := client.ProbeClusterHealth(context.Background(), *cfg, timeout, traceID)

	isJSON := *jsonFlag || global.JSON || strings.EqualFold(*formatFlag, "json") || strings.EqualFold(global.Format, "json")
	if isJSON {
		outputJSON(probeResult.ToStatusJSON())
	} else {
		fmt.Print(probeResult.FormatDashboard())
	}
	return 0
}

// runPing performs a lightweight preflight connectivity probe across all cluster nodes.
func runPing(global GlobalFlags, args []string) int {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	setDoubleDashUsage(fs)
	sensoryURLFlag := fs.String("sensory-url", "", "Sensory filter endpoint URL")
	workingURLFlag := fs.String("working-url", "", "Working memory scratchpad endpoint URL")
	knowledgeURLFlag := fs.String("knowledge-url", "", "Knowledge store endpoint URL")
	apiKeyFlag := fs.String("api-key", "", "API key for Node 1 authentication")
	tokenFlag := fs.String("token", "", "Token alias for Node 1 authentication")
	keyFlag := fs.String("key", "", "Legacy alias for API key")
	tlsCACertFlag := fs.String("tls-ca-cert", "", "Path to custom root CA certificate")
	insecureFlag := fs.Bool("insecure", false, "Skip TLS certificate verification")
	node3URLFlag := fs.String("node3-url", "", "Legacy alias for --sensory-url")
	node2URLFlag := fs.String("node2-url", "", "Legacy alias for --working-url")
	node1URLFlag := fs.String("node1-url", "", "Legacy alias for --knowledge-url")
	orchestrationURLFlag := fs.String("orchestration-url", "", "Alias for working memory URL")
	envFileFlag := fs.String("env-file", "", "Path to .env configuration file")
	traceIDFlag := fs.String("trace-id", "", "Distributed trace ID")
	verboseFlag := fs.Bool("verbose", false, "Enable stderr diagnostic logs")
	jsonFlag := fs.Bool("json", false, "Emit connectivity status as structured JSON")
	formatFlag := fs.String("format", "", "Output format: text or json")

	timeout := 500 * time.Millisecond
	if global.Timeout > 0 {
		timeout = global.Timeout
	}
	fs.DurationVar(&timeout, "timeout", timeout, "Probe timeout per node (default: 500ms)")
	_ = fs.Parse(args)

	verbose := global.Verbose || *verboseFlag
	telemetry.SetVerbose(verbose)
	traceID := pickURL(*traceIDFlag, global.TraceID)
	if traceID == "" {
		traceID = telemetry.GenerateTraceID()
	}

	cfg, err := config.Load(config.FlagOverrides{
		EnvPath:          pickURL(*envFileFlag, global.EnvPath),
		SensoryURL:       pickURL(*sensoryURLFlag, *node3URLFlag, global.SensoryURL),
		WorkingURL:       pickURL(*workingURLFlag, *node2URLFlag, *orchestrationURLFlag, global.WorkingURL, global.OrchestrationURL),
		KnowledgeURL:     pickURL(*knowledgeURLFlag, *node1URLFlag, global.KnowledgeURL),
		OrchestrationURL: pickURL(*orchestrationURLFlag, global.OrchestrationURL),
		APIKey:           pickURL(*apiKeyFlag, *tokenFlag, *keyFlag, global.APIKey),
		TLSCACert:        pickURL(*tlsCACertFlag, global.TLSCACert),
		Insecure:         *insecureFlag || global.Insecure,
		Timeout:          timeout,
	})
	if err != nil {
		outputError(traceID, err.Error())
	}

	probeResult := client.ProbeClusterHealth(context.Background(), *cfg, timeout, traceID)

	isJSON := *jsonFlag || global.JSON || strings.EqualFold(*formatFlag, "json") || strings.EqualFold(global.Format, "json")
	if isJSON {
		outputJSON(probeResult.ToPingJSON())
	} else {
		fmt.Print(probeResult.FormatPing())
	}

	if probeResult.AllHealthy {
		return 0
	}
	return 1
}
