package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfig_BlankDefaults(t *testing.T) {
	// Clean environment
	os.Unsetenv("CLUSTER_SENSORY_URL")
	os.Unsetenv("CLUSTER_WORKING_URL")
	os.Unsetenv("CLUSTER_KNOWLEDGE_URL")
	os.Unsetenv("SEKHA_NODE3_URL")
	os.Unsetenv("SEKHA_NODE2_URL")
	os.Unsetenv("SEKHA_NODE1_URL")

	cfg, err := Load(FlagOverrides{})
	if err != nil {
		t.Fatalf("unexpected error loading blank config: %v", err)
	}

	if cfg.SensoryURL != "" {
		t.Errorf("expected blank SensoryURL by default, got '%s'", cfg.SensoryURL)
	}
	if cfg.WorkingURL != "" {
		t.Errorf("expected blank WorkingURL by default, got '%s'", cfg.WorkingURL)
	}
	if cfg.KnowledgeURL != "" {
		t.Errorf("expected blank KnowledgeURL by default, got '%s'", cfg.KnowledgeURL)
	}

	if err := cfg.ValidateSensory(); err == nil {
		t.Errorf("expected error when validating blank sensory URL")
	}
	if err := cfg.ValidateAll(); err == nil {
		t.Errorf("expected error when validating all blank cluster endpoints")
	}
}

func TestConfig_DotEnvLoading(t *testing.T) {
	// Clean environment
	os.Unsetenv("CLUSTER_SENSORY_URL")
	os.Unsetenv("CLUSTER_WORKING_URL")
	os.Unsetenv("CLUSTER_KNOWLEDGE_URL")

	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `# Test .env file
export CLUSTER_SENSORY_URL="http://dotenv-sensory:8081"
CLUSTER_WORKING_URL='http://dotenv-working:8083'
CLUSTER_KNOWLEDGE_URL=http://dotenv-knowledge:8084
CLUSTER_DEFAULT_TIMEOUT_MS=2200
CLUSTER_SALIENCE_THRESHOLD=0.55
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env file: %v", err)
	}

	cfg, err := Load(FlagOverrides{EnvPath: envPath})
	if err != nil {
		t.Fatalf("unexpected error loading .env: %v", err)
	}

	if cfg.SensoryURL != "http://dotenv-sensory:8081" {
		t.Errorf("expected sensory url from .env, got '%s'", cfg.SensoryURL)
	}
	if cfg.WorkingURL != "http://dotenv-working:8083" {
		t.Errorf("expected working url from .env, got '%s'", cfg.WorkingURL)
	}
	if cfg.KnowledgeURL != "http://dotenv-knowledge:8084" {
		t.Errorf("expected knowledge url from .env, got '%s'", cfg.KnowledgeURL)
	}
	if cfg.DefaultTimeout != 2200*time.Millisecond {
		t.Errorf("expected 2200ms timeout, got %v", cfg.DefaultTimeout)
	}
	if cfg.SalienceThreshold != 0.55 {
		t.Errorf("expected threshold 0.55, got %v", cfg.SalienceThreshold)
	}
	if err := cfg.ValidateAll(); err != nil {
		t.Errorf("expected ValidateAll to succeed, got: %v", err)
	}
}

func TestConfig_OSEnvPrecedenceOverDotEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")

	content := `CLUSTER_SENSORY_URL=http://from-file:8081`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test .env file: %v", err)
	}

	os.Setenv("CLUSTER_SENSORY_URL", "http://from-os-env:8081")
	defer os.Unsetenv("CLUSTER_SENSORY_URL")

	cfg, err := Load(FlagOverrides{EnvPath: envPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SensoryURL != "http://from-os-env:8081" {
		t.Errorf("OS environment variable should take precedence over .env file; got '%s'", cfg.SensoryURL)
	}
}

func TestConfig_FlagOverrides(t *testing.T) {
	os.Setenv("CLUSTER_SENSORY_URL", "http://env-sensory:8081")
	defer os.Unsetenv("CLUSTER_SENSORY_URL")

	flags := FlagOverrides{
		SensoryURL:   "http://flag-sensory:8081",
		WorkingURL:   "http://flag-working:8083",
		KnowledgeURL: "http://flag-knowledge:8084",
		Timeout:      2500 * time.Millisecond,
	}

	cfg, err := Load(flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SensoryURL != "http://flag-sensory:8081" {
		t.Errorf("flag should override env var; got '%s'", cfg.SensoryURL)
	}
	if cfg.WorkingURL != "http://flag-working:8083" {
		t.Errorf("expected working url from flag; got '%s'", cfg.WorkingURL)
	}
	if cfg.DefaultTimeout != 2500*time.Millisecond {
		t.Errorf("expected 2500ms timeout; got %v", cfg.DefaultTimeout)
	}
	if err := cfg.ValidateAll(); err != nil {
		t.Errorf("expected ValidateAll to pass with flags set, got: %v", err)
	}
}

func TestConfig_BuildTimeInjectedVariables(t *testing.T) {
	// Clean environment
	os.Unsetenv("CLUSTER_SENSORY_URL")
	os.Unsetenv("CLUSTER_WORKING_URL")
	os.Unsetenv("CLUSTER_KNOWLEDGE_URL")

	BuildSensoryURL = "http://build-sensory:8081"
	BuildWorkingURL = "http://build-working:8083"
	BuildKnowledgeURL = "http://build-knowledge:8084"
	defer func() {
		BuildSensoryURL = ""
		BuildWorkingURL = ""
		BuildKnowledgeURL = ""
	}()

	// Blank flags, no .env file
	cfg, err := Load(FlagOverrides{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.SensoryURL != "http://build-sensory:8081" {
		t.Errorf("expected build sensory URL, got '%s'", cfg.SensoryURL)
	}
	if cfg.WorkingURL != "http://build-working:8083" {
		t.Errorf("expected build working URL, got '%s'", cfg.WorkingURL)
	}
	if cfg.KnowledgeURL != "http://build-knowledge:8084" {
		t.Errorf("expected build knowledge URL, got '%s'", cfg.KnowledgeURL)
	}

	// Flag override must take precedence over build-time variables
	flags := FlagOverrides{
		SensoryURL: "http://override-sensory:8081",
	}
	cfgOverride, err := Load(flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgOverride.SensoryURL != "http://override-sensory:8081" {
		t.Errorf("flag should override build-time variable; got '%s'", cfgOverride.SensoryURL)
	}
}

func TestConfig_OrchestrationURLOverride(t *testing.T) {
	flags := FlagOverrides{
		OrchestrationURL: "http://orchestrator-working:8083",
	}
	cfg, err := Load(flags)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.WorkingURL != "http://orchestrator-working:8083" {
		t.Errorf("expected OrchestrationURL to set WorkingURL when WorkingURL is empty; got '%s'", cfg.WorkingURL)
	}
}
