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
