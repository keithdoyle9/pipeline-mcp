package gitlabapi

import "testing"

func TestParseRunURLForBase(t *testing.T) {
	locator, err := ParseRunURLForBase("https://gitlab.example.com/root/team/app/-/pipelines/123?foo=bar", "https://gitlab.example.com/api/v4")
	if err != nil {
		t.Fatalf("ParseRunURLForBase() error = %v", err)
	}
	if locator.ProjectPath != "root/team/app" || locator.RunID != 123 {
		t.Fatalf("unexpected locator: %+v", locator)
	}
}

func TestParseRunURLForBaseSupportsSubpathInstance(t *testing.T) {
	locator, err := ParseRunURLForBase("https://gitlab.example.com/gitlab/root/team/app/-/pipelines/456", "https://gitlab.example.com/gitlab/api/v4")
	if err != nil {
		t.Fatalf("ParseRunURLForBase() error = %v", err)
	}
	if locator.ProjectPath != "root/team/app" || locator.RunID != 456 {
		t.Fatalf("unexpected locator: %+v", locator)
	}
}

func TestParseProjectPath(t *testing.T) {
	project, err := ParseProjectPath("group/subgroup/app")
	if err != nil {
		t.Fatalf("ParseProjectPath() error = %v", err)
	}
	if project != "group/subgroup/app" {
		t.Fatalf("unexpected project path %q", project)
	}
}

func TestParseProjectPathRejectsInvalid(t *testing.T) {
	if _, err := ParseProjectPath("app"); err == nil {
		t.Fatal("expected invalid project path error")
	}
}
