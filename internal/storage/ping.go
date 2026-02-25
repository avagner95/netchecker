package storage

import (
	"context"
	"fmt"
)

func (s *SQLiteStore) InsertBatch(ctx context.Context, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO ping_results(ts_ms,name,address,ttl,rtt_ms,error) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		_, err := stmt.ExecContext(ctx, r.TsMs, r.Name, r.Addr, r.TTL, r.RTTms, r.Error)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
