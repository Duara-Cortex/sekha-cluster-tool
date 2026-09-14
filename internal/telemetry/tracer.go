package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// HeaderTraceID is the canonical HTTP header name for distributed request tracing.
const HeaderTraceID = "X-Trace-ID"

var (
	verboseMu sync.Mutex
	verbose   bool
)

// SetVerbose toggles diagnostic logging to stderr.
func SetVerbose(enabled bool) {
	verboseMu.Lock()
	defer verboseMu.Unlock()
	verbose = enabled
}

// IsVerbose returns whether diagnostic logging is enabled.
func IsVerbose() bool {
	verboseMu.Lock()
	defer verboseMu.Unlock()
	return verbose
}

// GenerateTraceID creates a unique, cryptographically random trace identifier.
func GenerateTraceID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("trc-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("trc-%x", hex.EncodeToString(bytes))
}

// InjectTraceID adds the trace identifier to outgoing HTTP request headers.
func InjectTraceID(req *http.Request, traceID string) {
	if traceID == "" {
		traceID = GenerateTraceID()
	}
	req.Header.Set(HeaderTraceID, traceID)
}

// LogStep logs a structured diagnostic execution step to stderr.
func LogStep(traceID, stage, message string) {
	if !IsVerbose() {
		return
	}
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	fmt.Fprintf(os.Stderr, "[%s] [Trace: %s] [%s] %s\n", timestamp, traceID, stage, message)
}
