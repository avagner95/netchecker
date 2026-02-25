package storage

import (
	"context"
	"fmt"
	"time"
)

func (s *SQLiteStore) CleanupOlderThan(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention).UnixMilli()

	if _, err := s.db.ExecContext(ctx, `DELETE FROM ping_results WHERE ts_ms < ?`, cutoff); err != nil {
		return fmt.Errorf("cleanup ping_results: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM trace_results WHERE ts_ms < ?`, cutoff); err != nil {
		return fmt.Errorf("cleanup trace_results: %w", err)
	}

	_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE);`)
	return nil
}
