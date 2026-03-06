package audit

import (
	"context"
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
	path string
	mu   sync.Mutex
}

func NewJSONLStore(path string) *JSONLStore {
	return &JSONLStore{path: path}
}

func (s *JSONLStore) Append(_ context.Context, event domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if event.Signature == "" {
		event.Signature = signEvent(event)
	}

	dir := filepath.Dir(s.path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create audit dir: %w", err)
		}
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	if _, err := file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func signEvent(event domain.AuditEvent) string {
	h := sha256.New()
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
