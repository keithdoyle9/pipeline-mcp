package service

import "testing"

func TestParseDateTime(t *testing.T) {
	if _, err := ParseDateTime("2026-03-06T12:00:00Z"); err != nil {
		t.Fatalf("expected RFC3339 parse success: %v", err)
	}
	if _, err := ParseDateTime("2026-03-06"); err != nil {
		t.Fatalf("expected date parse success: %v", err)
	}
	if _, err := ParseDateTime("invalid"); err == nil {
		t.Fatal("expected parse error for invalid timestamp")
	}
}
