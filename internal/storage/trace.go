package storage

import (
	"context"
	"fmt"
)

func (s *SQLiteStore) InsertTrace(ctx context.Context, tsMs int64, name, addr, reason string, ok bool, output string) error {
	oi := 0
	if ok {
		oi = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trace_results(ts_ms,name,address,reason,ok,output) VALUES(?,?,?,?,?,?)`,
		tsMs, name, addr, reason, oi, output,
	)
	if err != nil {
		return fmt.Errorf("insert trace: %w", err)
	}
	return nil
}
