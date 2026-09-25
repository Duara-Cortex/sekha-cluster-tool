package model

import (
	"strings"
	"testing"
)

func TestBuildPositionalConsolidation(t *testing.T) {
	t.Run("basic positional shortcut with defaults", func(t *testing.T) {
		req := BuildPositionalConsolidation("Project Kestrel", "PORT: 51742", "", "", "", []string{"#project:kestrel"}, true, "trc-test-1")

		if req.TraceID != "trc-test-1" {
			t.Errorf("expected TraceID 'trc-test-1', got '%s'", req.TraceID)
		}
		if !strings.HasPrefix(req.SessionID, "sess-memorise-") {
			t.Errorf("expected SessionID prefix 'sess-memorise-', got '%s'", req.SessionID)
		}
		if req.TaskGoal != "Store Project Kestrel configuration" {
			t.Errorf("expected TaskGoal 'Store Project Kestrel configuration', got '%s'", req.TaskGoal)
		}
		if req.Outcome != "success" {
			t.Errorf("expected Outcome 'success', got '%s'", req.Outcome)
		}
		if req.Status != "completed" {
			t.Errorf("expected Status 'completed', got '%s'", req.Status)
		}
		if !req.Synchronous {
			t.Errorf("expected Synchronous true, got false")
		}

		if len(req.SensoryContext) != 1 {
			t.Fatalf("expected 1 sensory context item, got %d", len(req.SensoryContext))
		}
		sc := req.SensoryContext[0]
		if sc.ID != "fact-01" {
			t.Errorf("expected sensory ID 'fact-01', got '%s'", sc.ID)
		}
		if sc.Text != "Project Kestrel config: PORT: 51742" {
			t.Errorf("expected text 'Project Kestrel config: PORT: 51742', got '%s'", sc.Text)
		}
		if sc.Salience != 1.0 {
			t.Errorf("expected salience 1.0, got %f", sc.Salience)
		}
		if sc.Source != "user" {
			t.Errorf("expected source 'user', got '%s'", sc.Source)
		}
		if sc.Timestamp.IsZero() {
			t.Errorf("expected non-zero sensory timestamp")
		}

		if len(req.Trajectory) != 1 {
			t.Fatalf("expected 1 trajectory step, got %d", len(req.Trajectory))
		}
		tr := req.Trajectory[0]
		if tr.StepIndex != 0 {
			t.Errorf("expected step index 0, got %d", tr.StepIndex)
		}
		if tr.Thought != "Committed Project Kestrel configuration to long-term memory" {
			t.Errorf("expected thought 'Committed Project Kestrel configuration to long-term memory', got '%s'", tr.Thought)
		}
		if tr.Status != "completed" {
			t.Errorf("expected trajectory status 'completed', got '%s'", tr.Status)
		}
		if tr.Timestamp.IsZero() {
			t.Errorf("expected non-zero trajectory timestamp")
		}

		if len(req.Anchors) != 1 || req.Anchors[0] != "#project:kestrel" {
			t.Errorf("expected anchor '#project:kestrel', got %v", req.Anchors)
		}
	})

	t.Run("summary already starting with label", func(t *testing.T) {
		req := BuildPositionalConsolidation("Project Kestrel", "Project Kestrel: INGEST_PORT: 51742", "sess-123", "Custom Goal", "partial", []string{"kestrel"}, false, "")

		if req.TaskGoal != "Custom Goal" {
			t.Errorf("expected TaskGoal 'Custom Goal', got '%s'", req.TaskGoal)
		}
		if req.SessionID != "sess-123" {
			t.Errorf("expected SessionID 'sess-123', got '%s'", req.SessionID)
		}
		if req.Outcome != "partial" {
			t.Errorf("expected Outcome 'partial', got '%s'", req.Outcome)
		}
		if req.Synchronous {
			t.Errorf("expected Synchronous false, got true")
		}

		if len(req.SensoryContext) != 1 {
			t.Fatalf("expected 1 sensory item, got %d", len(req.SensoryContext))
		}
		if req.SensoryContext[0].Text != "Project Kestrel: INGEST_PORT: 51742" {
			t.Errorf("expected summary as-is, got '%s'", req.SensoryContext[0].Text)
		}
		if len(req.Anchors) != 1 || req.Anchors[0] != "#kestrel" {
			t.Errorf("expected anchor normalized to '#kestrel', got %v", req.Anchors)
		}
	})
}

func TestDetectRawSecrets(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		shouldDetect  bool
		expectedType  string
	}{
		{
			name:         "clean text",
			payload:      "Project Kestrel operates on port 8080 with logging level INFO",
			shouldDetect: false,
		},
		{
			name:         "external vault reference should not be detected",
			payload:      "Database password stored at vault://cluster/db-pass",
			shouldDetect: false,
		},
		{
			name:         "external env reference should not be detected",
			payload:      "Service token retrieved via env://SEKHA_API_KEY",
			shouldDetect: false,
		},
		{
			name:         "RSA private key block",
			payload:      "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA0...\n-----END RSA PRIVATE KEY-----",
			shouldDetect: true,
			expectedType: "Private Key",
		},
		{
			name:         "EC private key block",
			payload:      "-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEII...\n-----END EC PRIVATE KEY-----",
			shouldDetect: true,
			expectedType: "Private Key",
		},
		{
			name:         "OPENSSH private key block",
			payload:      "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNza...\n-----END OPENSSH PRIVATE KEY-----",
			shouldDetect: true,
			expectedType: "Private Key",
		},
		{
			name:         "Generic private key block",
			payload:      "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC...\n-----END PRIVATE KEY-----",
			shouldDetect: true,
			expectedType: "Private Key",
		},
		{
			name:         "OpenAI API key",
			payload:      "Configuration: openai_key=sk-abc123456789012345678901234567890",
			shouldDetect: true,
			expectedType: "OpenAI API Key",
		},
		{
			name:         "GitHub personal access token",
			payload:      "Clone token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
			shouldDetect: true,
			expectedType: "GitHub Token",
		},
		{
			name:         "AWS access key ID",
			payload:      "Backup storage: AKIAIOSFODNN7EXAMPLE",
			shouldDetect: true,
			expectedType: "AWS Access Key",
		},
		{
			name:         "Bearer token",
			payload:      "Authorization header: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.t-IDcSemACt8x4iTMCda8Yhe3iZaWbvV5XKSTbuAn0M",
			shouldDetect: true,
			expectedType: "Bearer Token",
		},
		{
			name:         "Slack token",
			payload:      "Notification hook: xoxb-1234567890-abcdefghij",
			shouldDetect: true,
			expectedType: "Slack Token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected, typ := DetectRawSecrets(tt.payload)
			if detected != tt.shouldDetect {
				t.Errorf("DetectRawSecrets() detected = %v, expected %v (type: %s)", detected, tt.shouldDetect, typ)
			}
			if tt.shouldDetect && typ != tt.expectedType {
				t.Errorf("expected secret type '%s', got '%s'", tt.expectedType, typ)
			}
		})
	}
}

func TestConsolidateRequest_DetectSecrets(t *testing.T) {
	req := ConsolidateRequest{
		SessionID: "sess-1",
		TaskGoal:  "Safe task goal",
		SensoryContext: []SensoryItem{
			{
				Text: "api_key: sk-proj-123456789012345678901234567890",
			},
		},
	}

	detected, typ := req.DetectSecrets()
	if !detected {
		t.Errorf("expected secret detected in SensoryContext, got false")
	}
	if typ != "OpenAI API Key" {
		t.Errorf("expected 'OpenAI API Key', got '%s'", typ)
	}
}

