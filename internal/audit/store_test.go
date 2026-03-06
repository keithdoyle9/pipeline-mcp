package audit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
)

func TestAppendOmitsSignatureWithoutSigningKeyAndLocksFilePermissions(t *testing.T) {
	store := NewJSONLStore(filepath.Join(t.TempDir(), "audit.jsonl"), "")
	event := domain.AuditEvent{
		EventID:    "evt_1",
		Tool:       "pipeline.rerun",
		Actor:      "pipeline-mcp",
		Repository: "acme/app",
		RunID:      42,
		Reason:     "retry",
		Scope:      "failed_jobs",
		Timestamp:  "2026-03-06T12:00:00Z",
		Outcome:    "accepted",
	}

	if err := store.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	stored := readAuditEvent(t, store.path)
	if stored.Signature != "" {
		t.Fatalf("expected signature to be omitted without signing key, got %q", stored.Signature)
	}

	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perms := info.Mode().Perm(); perms != 0o600 {
		t.Fatalf("expected audit log permissions 0600, got %04o", perms)
	}
}

func TestAppendSignsEventWhenSigningKeyConfigured(t *testing.T) {
	store := NewJSONLStore(filepath.Join(t.TempDir(), "audit.jsonl"), "audit-secret")
	event := domain.AuditEvent{
		EventID:    "evt_2",
		Tool:       "pipeline.rerun",
		Actor:      "pipeline-mcp",
		Repository: "acme/app",
		RunID:      77,
		Reason:     "retry",
		Scope:      "all_jobs",
		Timestamp:  "2026-03-06T12:00:00Z",
		Outcome:    "accepted",
	}

	if err := store.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	stored := readAuditEvent(t, store.path)
	if stored.Signature == "" {
		t.Fatal("expected signature to be present when signing key is configured")
	}
	if stored.Signature != store.signEvent(event) {
		t.Fatalf("expected HMAC signature %q, got %q", store.signEvent(event), stored.Signature)
	}
}

func TestAppendTightensExistingAuditLogPermissionsBeforeWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewJSONLStore(path, "")
	event := domain.AuditEvent{
		EventID:    "evt_3",
		Tool:       "pipeline.rerun",
		Actor:      "pipeline-mcp",
		Repository: "acme/app",
		RunID:      99,
		Reason:     "retry",
		Scope:      "failed_jobs",
		Timestamp:  "2026-03-06T12:00:00Z",
		Outcome:    "accepted",
	}

	if err := store.Append(context.Background(), event); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perms := info.Mode().Perm(); perms != 0o600 {
		t.Fatalf("expected audit log permissions 0600, got %04o", perms)
	}
}

func readAuditEvent(t *testing.T, path string) domain.AuditEvent {
	t.Helper()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var event domain.AuditEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return event
}
