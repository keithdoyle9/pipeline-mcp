package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
)

type Store interface {
	Append(context.Context, domain.AuditEvent) error
}

type JSONLStore struct {
	path       string
	signingKey []byte
	mu         sync.Mutex
}

func NewJSONLStore(path, signingKey string) *JSONLStore {
	return &JSONLStore{
		path:       path,
		signingKey: []byte(signingKey),
	}
}

func (s *JSONLStore) Append(_ context.Context, event domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.Signature == "" {
		event.Signature = s.signEvent(event)
	}

	dir := filepath.Dir(s.path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create audit dir: %w", err)
		}
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod audit log: %w", err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func (s *JSONLStore) signEvent(event domain.AuditEvent) string {
	if len(s.signingKey) == 0 {
		return ""
	}

	h := hmac.New(sha256.New, s.signingKey)
	_, _ = h.Write([]byte(event.EventID))
	_, _ = h.Write([]byte(event.Tool))
	_, _ = h.Write([]byte(event.Actor))
	_, _ = h.Write([]byte(event.Repository))
	_, _ = h.Write([]byte(fmt.Sprintf("%d", event.RunID)))
	_, _ = h.Write([]byte(event.Reason))
	_, _ = h.Write([]byte(event.Scope))
	_, _ = h.Write([]byte(event.Timestamp))
	_, _ = h.Write([]byte(event.Outcome))
	return hex.EncodeToString(h.Sum(nil))
}
