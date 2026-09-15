package main

import (
	"testing"
	"time"
)

func TestParseArgs_FlagPositions(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		expectedSubcommand string
		expectedSensory    string
		expectedWorking    string
		expectedKnowledge  string
		expectedVerbose    bool
		expectedTimeout    time.Duration
		expectedArgs       []string
	}{
		{
			name:               "subcommand only",
			args:               []string{"status"},
			expectedSubcommand: "status",
			expectedArgs:       []string{},
		},
		{
			name:               "flag before subcommand",
			args:               []string{"--sensory-url", "http://192.168.8.183:8081", "status"},
			expectedSubcommand: "status",
			expectedSensory:    "http://192.168.8.183:8081",
			expectedArgs:       []string{},
		},
		{
			name:               "flag after subcommand",
			args:               []string{"status", "--sensory-url", "http://192.168.8.183:8081"},
			expectedSubcommand: "status",
			expectedSensory:    "http://192.168.8.183:8081",
			expectedArgs:       []string{},
		},
		{
			name:               "inline flags both before and after",
			args:               []string{"--sensory-url=http://sensory:8081", "status", "--working-url=http://working:8083"},
			expectedSubcommand: "status",
			expectedSensory:    "http://sensory:8081",
			expectedWorking:    "http://working:8083",
			expectedArgs:       []string{},
		},
		{
			name:               "legacy aliases",
			args:               []string{"--node3-url", "http://node3:8081", "status", "--node2-url", "http://node2:8083", "--node1-url", "http://node1:8084"},
			expectedSubcommand: "status",
			expectedSensory:    "http://node3:8081",
			expectedWorking:    "http://node2:8083",
			expectedKnowledge:  "http://node1:8084",
			expectedArgs:       []string{},
		},
		{
			name:               "orchestration url alias for working url",
			args:               []string{"--orchestration-url", "http://orch:8083", "status"},
			expectedSubcommand: "status",
			expectedWorking:    "http://orch:8083",
			expectedArgs:       []string{},
		},
		{
			name:               "subcommand with subcommand-specific flags interleaved with global flags",
			args:               []string{"--sensory-url", "http://sensory:8081", "filter", "--text", "hello world", "--verbose", "--timeout", "500ms"},
			expectedSubcommand: "filter",
			expectedSensory:    "http://sensory:8081",
			expectedVerbose:    true,
			expectedTimeout:    500 * time.Millisecond,
			expectedArgs:       []string{"--text", "hello world"},
		},
		{
			name:               "subcommand args before global flag",
			args:               []string{"filter", "--text", "hello world", "--sensory-url", "http://sensory:8081"},
			expectedSubcommand: "filter",
			expectedSensory:    "http://sensory:8081",
			expectedArgs:       []string{"--text", "hello world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global, subcmd, remaining := parseArgs(tt.args)

			if subcmd != tt.expectedSubcommand {
				t.Errorf("expected subcommand '%s', got '%s'", tt.expectedSubcommand, subcmd)
			}
			if global.SensoryURL != tt.expectedSensory {
				t.Errorf("expected SensoryURL '%s', got '%s'", tt.expectedSensory, global.SensoryURL)
			}
			if global.WorkingURL != tt.expectedWorking {
				t.Errorf("expected WorkingURL '%s', got '%s'", tt.expectedWorking, global.WorkingURL)
			}
			if global.KnowledgeURL != tt.expectedKnowledge {
				t.Errorf("expected KnowledgeURL '%s', got '%s'", tt.expectedKnowledge, global.KnowledgeURL)
			}
			if global.Verbose != tt.expectedVerbose {
				t.Errorf("expected Verbose %v, got %v", tt.expectedVerbose, global.Verbose)
			}
			if global.Timeout != tt.expectedTimeout {
				t.Errorf("expected Timeout %v, got %v", tt.expectedTimeout, global.Timeout)
			}
			if len(remaining) != len(tt.expectedArgs) {
				t.Fatalf("expected remaining args length %d, got %d (%v)", len(tt.expectedArgs), len(remaining), remaining)
			}
			for i := range remaining {
				if remaining[i] != tt.expectedArgs[i] {
					t.Errorf("arg[%d]: expected '%s', got '%s'", i, tt.expectedArgs[i], remaining[i])
				}
			}
		})
	}
}
