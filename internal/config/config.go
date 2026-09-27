package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Build-time variables optionally injected via -ldflags at compile time by the machine building it.
// Default to empty strings if not specified.
var (
	BuildSensoryURL   = ""
	BuildWorkingURL   = ""
	BuildKnowledgeURL = ""
)

// DefaultMaxInputBytes caps the combined inline/file/stdin payload a single invocation accepts (1 MiB).
const DefaultMaxInputBytes = 1 << 20

// Node 2 context budget defaults. They must match the Node 2 deployment: the context window is
// llama-server's n_ctx (Node 2 ContextLimit) and the output reserve is Node 2's OutputReserve,
// so context - output reserve equals the prompt_window_tokens Node 2 reports. The template
// reserve covers Node 2's prompt template, measured live at ~184 tokens (v1.0.10: 193 prompt
// tokens for a 9-token task) plus a margin.
const (
	DefaultContextTokens       = 4096
	DefaultOutputReserveTokens = 512
	DefaultPromptReserveTokens = 256
)

// DefaultSensoryTimeout is the Node 3 filter deadline when CLUSTER_SENSORY_TIMEOUT_MS is unset.
const DefaultSensoryTimeout = 30 * time.Second

// DefaultConsolidateTimeout is the Node 1 consolidate deadline when CLUSTER_CONSOLIDATE_TIMEOUT_MS
// is unset. Synchronous consolidation of a large payload runs entity extraction on Node 1, so the
// default is generous; tune it per cluster in .env.
const DefaultConsolidateTimeout = 120 * time.Second

// Config represents the fully resolved runtime configuration for the cluster tool.
type Config struct {
	SensoryURL         string        `json:"sensory_url"`
	WorkingURL         string        `json:"working_url"`
	KnowledgeURL       string        `json:"knowledge_url"`
	APIKey             string        `json:"api_key,omitempty"`
	TLSCACert          string        `json:"tls_ca_cert,omitempty"`
	TLSCACertSource    string        `json:"tls_ca_cert_source,omitempty"`
	Insecure           bool          `json:"insecure,omitempty"`
	DefaultTimeout     time.Duration `json:"default_timeout"`
	DeliberateTimeout  time.Duration `json:"deliberate_timeout"`
	SensoryTimeout     time.Duration `json:"sensory_timeout"`
	SensorySource      string        `json:"sensory_timeout_source"`
	ConsolidateTimeout time.Duration `json:"consolidate_timeout"`
	ConsolidateSource  string        `json:"consolidate_timeout_source"`
	SalienceThreshold  float64       `json:"salience_threshold"`
	RecallTopK         int           `json:"recall_top_k"`
	RecallMinSim       float64       `json:"recall_min_sim,omitempty"`
	MaxInputBytes      int           `json:"max_input_bytes"`
	ContextTokens      int           `json:"deliberate_context_tokens"`
	OutputReserve      int           `json:"deliberate_output_reserve_tokens"`
	PromptReserve      int           `json:"deliberate_prompt_reserve_tokens"`
	EnvFileLoaded      string        `json:"env_file_loaded,omitempty"`
}

// FlagOverrides captures CLI arguments for precedence overriding.
type FlagOverrides struct {
	EnvPath            string
	SensoryURL         string
	WorkingURL         string
	KnowledgeURL       string
	OrchestrationURL   string
	APIKey             string
	TLSCACert          string
	Insecure           bool
	Timeout            time.Duration
	DeliberateTimeout  time.Duration
	SensoryTimeout     time.Duration
	ConsolidateTimeout time.Duration
	// SensoryTimeoutFlag / ConsolidateTimeoutFlag name the flag that set the value (e.g.
	// "--consolidate-timeout"), reported as its source in deadline errors.
	SensoryTimeoutFlag     string
	ConsolidateTimeoutFlag string
	SalienceThreshold      float64
	RecallTopK             int
	RecallMinSim           float64
	MaxInputBytes          int
	ContextTokens          int
	OutputReserve          int
	PromptReserve          int
}

// Load resolves configuration across flags, real OS environment variables, .env files, and build variables.
func Load(flags FlagOverrides) (*Config, error) {
	cfg := &Config{
		SensoryURL:         "",
		WorkingURL:         "",
		KnowledgeURL:       "",
		DefaultTimeout:     1500 * time.Millisecond,
		DeliberateTimeout:  8000 * time.Millisecond,
		SensoryTimeout:     DefaultSensoryTimeout,
		SensorySource:      "default",
		ConsolidateTimeout: DefaultConsolidateTimeout,
		ConsolidateSource:  "default",
		SalienceThreshold:  0.45,
		RecallTopK:         5,
		MaxInputBytes:      DefaultMaxInputBytes,
		ContextTokens:      DefaultContextTokens,
		OutputReserve:      DefaultOutputReserveTokens,
		PromptReserve:      DefaultPromptReserveTokens,
	}

	// 1. Check build-time injected variables (from compile flags)
	if BuildSensoryURL != "" {
		cfg.SensoryURL = BuildSensoryURL
	}
	if BuildWorkingURL != "" {
		cfg.WorkingURL = BuildWorkingURL
	}
	if BuildKnowledgeURL != "" {
		cfg.KnowledgeURL = BuildKnowledgeURL
	}

	// 2. Discover and parse .env file
	envFile := resolveEnvPath(flags.EnvPath)
	fileEnv := make(map[string]string)
	if envFile != "" {
		parsed, err := parseDotEnv(envFile)
		if err != nil && flags.EnvPath != "" {
			return nil, fmt.Errorf("failed to parse explicitly specified env file '%s': %w", flags.EnvPath, err)
		}
		if err == nil {
			fileEnv = parsed
			cfg.EnvFileLoaded = envFile
		}
	}

	// Helper to lookup with precedence: OS environment variable > .env file
	getEnv := func(primaryKey, fallbackKey string) string {
		if val, exists := os.LookupEnv(primaryKey); exists && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
		if fallbackKey != "" {
			if val, exists := os.LookupEnv(fallbackKey); exists && strings.TrimSpace(val) != "" {
				return strings.TrimSpace(val)
			}
		}
		if val, exists := fileEnv[primaryKey]; exists && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
		if fallbackKey != "" {
			if val, exists := fileEnv[fallbackKey]; exists && strings.TrimSpace(val) != "" {
				return strings.TrimSpace(val)
			}
		}
		return ""
	}

	// 3. Apply environment variables (.env + OS environment)
	if val := getEnv("CLUSTER_SENSORY_URL", "SEKHA_NODE3_URL"); val != "" {
		cfg.SensoryURL = val
	}
	if val := getEnv("CLUSTER_WORKING_URL", "SEKHA_NODE2_URL"); val != "" {
		cfg.WorkingURL = val
	}
	if val := getEnv("CLUSTER_KNOWLEDGE_URL", "SEKHA_NODE1_URL"); val != "" {
		cfg.KnowledgeURL = val
	}
	if val := getEnv("CLUSTER_API_KEY", "SEKHA_API_KEY"); val != "" {
		cfg.APIKey = val
	}

	if val := getEnv("CLUSTER_DEFAULT_TIMEOUT_MS", ""); val != "" {
		if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
			cfg.DefaultTimeout = time.Duration(ms) * time.Millisecond
		}
	}
	if val := getEnv("CLUSTER_DELIBERATE_TIMEOUT_MS", ""); val != "" {
		if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
			cfg.DeliberateTimeout = time.Duration(ms) * time.Millisecond
		}
	}
	// sourceOf reports where an env key's value came from, so deadline errors can name it.
	sourceOf := func(key string) string {
		if val, exists := os.LookupEnv(key); exists && strings.TrimSpace(val) != "" {
			return "environment variable " + key
		}
		if cfg.EnvFileLoaded != "" {
			return fmt.Sprintf("%s in .env file (%s)", key, cfg.EnvFileLoaded)
		}
		return key + " in .env file"
	}
	if val := getEnv("CLUSTER_SENSORY_TIMEOUT_MS", ""); val != "" {
		if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
			cfg.SensoryTimeout = time.Duration(ms) * time.Millisecond
			cfg.SensorySource = sourceOf("CLUSTER_SENSORY_TIMEOUT_MS")
		}
	}
	if val := getEnv("CLUSTER_CONSOLIDATE_TIMEOUT_MS", ""); val != "" {
		if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
			cfg.ConsolidateTimeout = time.Duration(ms) * time.Millisecond
			cfg.ConsolidateSource = sourceOf("CLUSTER_CONSOLIDATE_TIMEOUT_MS")
		}
	}
	if val := getEnv("CLUSTER_SALIENCE_THRESHOLD", ""); val != "" {
		if th, err := strconv.ParseFloat(val, 64); err == nil && th > 0 {
			cfg.SalienceThreshold = th
		}
	}
	if val := getEnv("CLUSTER_RECALL_TOP_K", ""); val != "" {
		if k, err := strconv.Atoi(val); err == nil && k > 0 {
			cfg.RecallTopK = k
		}
	}

	if val := getEnv("CLUSTER_RECALL_MIN_SIM", ""); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
			cfg.RecallMinSim = f
		}
	}
	if val := getEnv("CLUSTER_MAX_INPUT_BYTES", ""); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxInputBytes = n
		}
	}
	if val := getEnv("CLUSTER_DELIBERATE_CONTEXT_TOKENS", ""); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.ContextTokens = n
		}
	}
	if val := getEnv("CLUSTER_DELIBERATE_OUTPUT_RESERVE", ""); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.OutputReserve = n
		}
	}
	if val := getEnv("CLUSTER_DELIBERATE_PROMPT_RESERVE", ""); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.PromptReserve = n
		}
	}

	if val, exists := os.LookupEnv("CLUSTER_TLS_CA_CERT"); exists && strings.TrimSpace(val) != "" {
		cfg.TLSCACert = strings.TrimSpace(val)
		cfg.TLSCACertSource = "environment variable CLUSTER_TLS_CA_CERT"
	} else if val, exists := fileEnv["CLUSTER_TLS_CA_CERT"]; exists && strings.TrimSpace(val) != "" {
		cfg.TLSCACert = strings.TrimSpace(val)
		if cfg.EnvFileLoaded != "" {
			cfg.TLSCACertSource = fmt.Sprintf(".env file (%s)", cfg.EnvFileLoaded)
		} else {
			cfg.TLSCACertSource = ".env file"
		}
	}
	if val := getEnv("CLUSTER_INSECURE", ""); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.Insecure = b
		}
	}

	// 4. CLI flag overrides (highest precedence)
	if flags.SensoryURL != "" {
		cfg.SensoryURL = flags.SensoryURL
	}
	if flags.WorkingURL != "" {
		cfg.WorkingURL = flags.WorkingURL
	}
	if flags.KnowledgeURL != "" {
		cfg.KnowledgeURL = flags.KnowledgeURL
	}
	if flags.OrchestrationURL != "" && cfg.WorkingURL == "" {
		cfg.WorkingURL = flags.OrchestrationURL
	}
	if flags.APIKey != "" {
		cfg.APIKey = flags.APIKey
	}
	if flags.TLSCACert != "" {
		cfg.TLSCACert = flags.TLSCACert
		cfg.TLSCACertSource = "--tls-ca-cert flag"
	}
	if flags.Insecure {
		cfg.Insecure = true
	}
	if flags.Timeout > 0 {
		cfg.DefaultTimeout = flags.Timeout
	}
	if flags.DeliberateTimeout > 0 {
		cfg.DeliberateTimeout = flags.DeliberateTimeout
	}
	if flags.SensoryTimeout > 0 {
		cfg.SensoryTimeout = flags.SensoryTimeout
		cfg.SensorySource = flagSource(flags.SensoryTimeoutFlag)
	}
	if flags.ConsolidateTimeout > 0 {
		cfg.ConsolidateTimeout = flags.ConsolidateTimeout
		cfg.ConsolidateSource = flagSource(flags.ConsolidateTimeoutFlag)
	}
	if flags.SalienceThreshold > 0 {
		cfg.SalienceThreshold = flags.SalienceThreshold
	}
	if flags.RecallTopK > 0 {
		cfg.RecallTopK = flags.RecallTopK
	}
	if flags.RecallMinSim > 0 {
		cfg.RecallMinSim = flags.RecallMinSim
	}
	if flags.MaxInputBytes > 0 {
		cfg.MaxInputBytes = flags.MaxInputBytes
	}
	if flags.ContextTokens > 0 {
		cfg.ContextTokens = flags.ContextTokens
	}
	if flags.OutputReserve > 0 {
		cfg.OutputReserve = flags.OutputReserve
	}
	if flags.PromptReserve > 0 {
		cfg.PromptReserve = flags.PromptReserve
	}

	return cfg, nil
}

func flagSource(name string) string {
	if name == "" {
		return "command-line flag"
	}
	return name + " flag"
}

// resolveEnvPath finds the appropriate .env file to load.
func resolveEnvPath(explicitPath string) string {
	if explicitPath != "" {
		return explicitPath
	}
	if env := os.Getenv("CLUSTER_ENV_FILE"); env != "" {
		return env
	}

	// 1. Current working directory
	for _, c := range []string{".env.local", ".env"} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}

	// 2. Binary's install directory (e.g. ~/.local/bin/.env)
	if exe, err := os.Executable(); err == nil {
		binEnv := filepath.Join(filepath.Dir(exe), ".env")
		if _, err := os.Stat(binEnv); err == nil {
			return binEnv
		}
	}

	// 3. User standard config directory (~/.config/sekha-cluster-tool/.env)
	if home, err := os.UserHomeDir(); err == nil {
		userConfig := filepath.Join(home, ".config", "sekha-cluster-tool", ".env")
		if _, err := os.Stat(userConfig); err == nil {
			return userConfig
		}
	}

	return ""
}

// parseDotEnv parses a standard KEY=VALUE .env file without external dependencies.
func parseDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Support 'export KEY=VALUE'
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip surrounding quotes
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}

		result[key] = val
	}

	return result, scanner.Err()
}

// ValidateSensory checks that the sensory layer endpoint address is configured.
func (c *Config) ValidateSensory() error {
	if strings.TrimSpace(c.SensoryURL) == "" {
		return errors.New("sensory layer endpoint URL is not configured (specify via --sensory-url, CLUSTER_SENSORY_URL, .env file, or compile flag)")
	}
	return nil
}

// ValidateWorking checks that the working memory layer endpoint address is configured.
func (c *Config) ValidateWorking() error {
	if strings.TrimSpace(c.WorkingURL) == "" {
		return errors.New("working memory layer endpoint URL is not configured (specify via --working-url, CLUSTER_WORKING_URL, .env file, or compile flag)")
	}
	return nil
}

// ValidateKnowledge checks that the long-term knowledge layer endpoint address is configured.
func (c *Config) ValidateKnowledge() error {
	if strings.TrimSpace(c.KnowledgeURL) == "" {
		return errors.New("knowledge layer endpoint URL is not configured (specify via --knowledge-url, CLUSTER_KNOWLEDGE_URL, .env file, or compile flag)")
	}
	return nil
}

// ValidateAll ensures all three cluster node addresses are defined.
func (c *Config) ValidateAll() error {
	var missing []string
	if strings.TrimSpace(c.SensoryURL) == "" {
		missing = append(missing, "sensory_url (--sensory-url / CLUSTER_SENSORY_URL)")
	}
	if strings.TrimSpace(c.WorkingURL) == "" {
		missing = append(missing, "working_url (--working-url / CLUSTER_WORKING_URL)")
	}
	if strings.TrimSpace(c.KnowledgeURL) == "" {
		missing = append(missing, "knowledge_url (--knowledge-url / CLUSTER_KNOWLEDGE_URL)")
	}

	if len(missing) > 0 {
		return fmt.Errorf("cluster endpoints not configured: %s", strings.Join(missing, ", "))
	}
	return nil
}

// TemplateDotEnv returns a blank .env template for operators.
func TemplateDotEnv() string {
	return `# ==============================================================================
# Tri-Node Cognitive Cluster Environment Configuration (.env)
# All endpoint addresses default to blank ("") and must be configured by the machine.
# ==============================================================================

# Sensory Attention Layer (e.g. http://192.168.8.183:8081)
CLUSTER_SENSORY_URL=

# Working Memory Scratchpad Layer (e.g. http://192.168.8.175:8083)
CLUSTER_WORKING_URL=

# Long-Term Knowledge Graph Layer (e.g. http://192.168.8.213:8084)
CLUSTER_KNOWLEDGE_URL=

# Node 1 API Key (optional authentication for protected knowledge endpoints)
CLUSTER_API_KEY=

# Timeout Budgets (milliseconds)
CLUSTER_DEFAULT_TIMEOUT_MS=1500
CLUSTER_DELIBERATE_TIMEOUT_MS=8000
# Node 3 filter deadline (filter, orchestrate stage 1; default 30000)
CLUSTER_SENSORY_TIMEOUT_MS=30000
# Node 1 consolidate deadline (consolidate, orchestrate stage 4; default 120000)
CLUSTER_CONSOLIDATE_TIMEOUT_MS=120000

# Attention & Recall Parameters
CLUSTER_SALIENCE_THRESHOLD=0.45
CLUSTER_RECALL_TOP_K=5
# Relevance gate: minimum sim_score for recalled nodes to reach Node 2 (default 0.50)
# CLUSTER_RECALL_MIN_SIM=0.50

# Payload & Node 2 context budget. Context window and output reserve must match Node 2's
# llama-server n_ctx and Node 2 OutputReserve; the prompt reserve covers Node 2's template.
# CLUSTER_MAX_INPUT_BYTES=1048576
# CLUSTER_DELIBERATE_CONTEXT_TOKENS=4096
# CLUSTER_DELIBERATE_OUTPUT_RESERVE=512
# CLUSTER_DELIBERATE_PROMPT_RESERVE=256

# Transport Encryption & TLS (optional)
# CLUSTER_TLS_CA_CERT=/path/to/ca.crt
# CLUSTER_INSECURE=false
`
}
