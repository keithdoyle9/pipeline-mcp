package githubapi

import "testing"

func TestParseRunURL(t *testing.T) {
	locator, err := ParseRunURL("https://github.com/octo/repo/actions/runs/123456789?check_suite_focus=true")
	if err != nil {
		t.Fatalf("ParseRunURL() error = %v", err)
	}

	if locator.Owner != "octo" || locator.Repo != "repo" || locator.RunID != 123456789 {
		t.Fatalf("unexpected locator: %+v", locator)
	}
}

func TestParseRepository(t *testing.T) {
	owner, repo, err := ParseRepository("acme/service")
	if err != nil {
		t.Fatalf("ParseRepository() error = %v", err)
	}
	if owner != "acme" || repo != "service" {
		t.Fatalf("unexpected owner/repo: %s/%s", owner, repo)
	}
}

func TestParseRepositoryInvalid(t *testing.T) {
	if _, _, err := ParseRepository("acme"); err == nil {
		t.Fatal("expected error for invalid repository")
	}
}

func TestParseCheckRunURL(t *testing.T) {
	checkRunID, err := ParseCheckRunURL("https://api.github.com/repos/acme/app/check-runs/987654321")
	if err != nil {
		t.Fatalf("ParseCheckRunURL() error = %v", err)
	}
	if checkRunID != 987654321 {
		t.Fatalf("expected check run id 987654321, got %d", checkRunID)
	}
}
