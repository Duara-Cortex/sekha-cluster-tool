package config

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateCACert verifies that the specified TLS CA certificate file can be read and contains valid PEM certificates.
// If the path is relative, it is resolved against the current working directory, and the resolved absolute path is included in errors.
// source describes where the CA path came from (e.g. "--tls-ca-cert flag", "environment variable CLUSTER_TLS_CA_CERT", ".env file (...)").
func ValidateCACert(path, source string) error {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		absPath = cleanPath
	}

	if strings.HasPrefix(cleanPath, "~/") {
		if home, hErr := os.UserHomeDir(); hErr == nil {
			absPath = filepath.Join(home, cleanPath[2:])
		}
	}

	var pathDisplay string
	if !filepath.IsAbs(cleanPath) && cleanPath != absPath {
		pathDisplay = fmt.Sprintf("'%s' (resolved to '%s')", cleanPath, absPath)
	} else {
		pathDisplay = fmt.Sprintf("'%s'", absPath)
	}

	var sourceDisplay string
	if strings.TrimSpace(source) != "" {
		sourceDisplay = fmt.Sprintf(" (source: %s)", strings.TrimSpace(source))
	}

	fi, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to read TLS CA certificate %s%s: %w", pathDisplay, sourceDisplay, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("failed to read TLS CA certificate %s%s: read %s: is a directory", pathDisplay, sourceDisplay, absPath)
	}

	caBytes, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read TLS CA certificate %s%s: %w", pathDisplay, sourceDisplay, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return fmt.Errorf("TLS CA certificate file %s%s contains no valid PEM certificates; check it with: openssl x509 -in %s -noout -subject", pathDisplay, sourceDisplay, cleanPath)
	}

	return nil
}

// ValidateTLS verifies the configured TLS CA certificate if one is specified.
// Returns an actionable error if the CA certificate file cannot be read or contains no valid PEM certificates.
func (c *Config) ValidateTLS() error {
	if strings.TrimSpace(c.TLSCACert) == "" {
		return nil
	}
	return ValidateCACert(c.TLSCACert, c.TLSCACertSource)
}

// LoadCACertPool loads the system certificate pool and appends the certificates from the given path.
// It fails loudly if the CA file cannot be read or contains no valid PEM certificates.
func LoadCACertPool(path, source string) (*x509.CertPool, error) {
	if err := ValidateCACert(path, source); err != nil {
		return nil, err
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}

	absPath, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		absPath = strings.TrimSpace(path)
	}
	if strings.HasPrefix(strings.TrimSpace(path), "~/") {
		if home, hErr := os.UserHomeDir(); hErr == nil {
			absPath = filepath.Join(home, strings.TrimSpace(path)[2:])
		}
	}

	caBytes, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	pool.AppendCertsFromPEM(caBytes)
	return pool, nil
}
