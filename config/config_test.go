package config

import (
	"testing"

	"github.com/keithdoyle9/pipeline-mcp/internal/buildinfo"
)

func TestFirstEnv(t *testing.T) {
	t.Setenv("GITHUB_READ_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "shared-token")
	t.Setenv("GH_TOKEN", "gh-token")

	got := firstEnv("GITHUB_READ_TOKEN", "GITHUB_TOKEN", "GH_TOKEN")
	if got != "shared-token" {
		t.Fatalf("expected shared-token, got %q", got)
	}
}

func TestLoadDefaultsUseBuildVersion(t *testing.T) {
	t.Setenv("VERSION", "")
	t.Setenv("USER_AGENT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Version != buildinfo.Version {
		t.Fatalf("expected version %q, got %q", buildinfo.Version, cfg.Version)
	}

	wantUserAgent := "pipeline-mcp/" + buildinfo.Version
	if cfg.UserAgent != wantUserAgent {
		t.Fatalf("expected user agent %q, got %q", wantUserAgent, cfg.UserAgent)
	}
}

func TestLoadUserAgentDefaultsToConfiguredVersion(t *testing.T) {
	t.Setenv("VERSION", "v9.9.9")
	t.Setenv("USER_AGENT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Version != "v9.9.9" {
		t.Fatalf("expected version %q, got %q", "v9.9.9", cfg.Version)
	}

	if cfg.UserAgent != "pipeline-mcp/v9.9.9" {
		t.Fatalf("expected user agent %q, got %q", "pipeline-mcp/v9.9.9", cfg.UserAgent)
	}
}

func TestLoadIncludesAuditSigningKey(t *testing.T) {
	t.Setenv("AUDIT_SIGNING_KEY", "  audit-secret  ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AuditSigningKey != "audit-secret" {
		t.Fatalf("expected trimmed audit signing key, got %q", cfg.AuditSigningKey)
	}
}

func TestLoadDoesNotUseSharedFallbackForWriteToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "shared-token")
	t.Setenv("GH_TOKEN", "gh-token")
	t.Setenv("GITHUB_WRITE_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.GitHubReadToken != "shared-token" {
		t.Fatalf("expected read token fallback, got %q", cfg.GitHubReadToken)
	}
	if cfg.GitHubWriteToken != "" {
		t.Fatalf("expected write token to require explicit GITHUB_WRITE_TOKEN, got %q", cfg.GitHubWriteToken)
	}
}

func TestLoadRejectsInvalidRuntimeConfiguration(t *testing.T) {
	testCases := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "negative max log bytes",
			env:     map[string]string{"MAX_LOG_BYTES": "-1"},
			wantErr: "MAX_LOG_BYTES must be greater than zero",
		},
		{
			name:    "zero lookback days",
			env:     map[string]string{"DEFAULT_LOOKBACK_DAYS": "0"},
			wantErr: "DEFAULT_LOOKBACK_DAYS must be greater than zero",
		},
		{
			name:    "zero max historical runs",
			env:     map[string]string{"MAX_HISTORICAL_RUNS": "0"},
			wantErr: "MAX_HISTORICAL_RUNS must be greater than zero",
		},
		{
			name:    "zero http timeout",
			env:     map[string]string{"HTTP_TIMEOUT_SECONDS": "0"},
			wantErr: "HTTP_TIMEOUT_SECONDS must be greater than zero",
		},
		{
			name: "mutations enabled without write token",
			env: map[string]string{
				"DISABLE_MUTATIONS":  "false",
				"GITHUB_WRITE_TOKEN": "",
				"GITHUB_TOKEN":       "",
				"GH_TOKEN":           "",
			},
			wantErr: "GITHUB_WRITE_TOKEN is required when DISABLE_MUTATIONS=false",
		},
		{
			name:    "invalid api base url scheme",
			env:     map[string]string{"GITHUB_API_BASE_URL": "ssh://github.example.com"},
			wantErr: "GITHUB_API_BASE_URL must use http or https",
		},
		{
			name:    "missing api base url host",
			env:     map[string]string{"GITHUB_API_BASE_URL": "https:///"},
			wantErr: "GITHUB_API_BASE_URL must include a host",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			_, err := Load()
			if err == nil {
				t.Fatal("expected Load() to fail")
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("expected error %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}
