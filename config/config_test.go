package config

import "testing"

func TestFirstEnv(t *testing.T) {
	t.Setenv("GITHUB_READ_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "shared-token")
	t.Setenv("GH_TOKEN", "gh-token")

	got := firstEnv("GITHUB_READ_TOKEN", "GITHUB_TOKEN", "GH_TOKEN")
	if got != "shared-token" {
		t.Fatalf("expected shared-token, got %q", got)
	}
}
