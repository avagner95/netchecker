package storage

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

func OpenSQLite(path string) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=1", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA temp_store=MEMORY;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	s := &SQLiteStore{db: db}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS ping_results (
  ts_ms   INTEGER NOT NULL,
  name    TEXT    NOT NULL,
  address TEXT    NOT NULL,
  ttl     INTEGER DEFAULT 0,
  rtt_ms  INTEGER DEFAULT 0,
  error   TEXT
);

CREATE INDEX IF NOT EXISTS idx_ping_results_ts
  ON ping_results(ts_ms);

CREATE INDEX IF NOT EXISTS idx_ping_results_name_ts
  ON ping_results(name, ts_ms);

CREATE INDEX IF NOT EXISTS idx_ping_results_addr_ts
  ON ping_results(address, ts_ms);

CREATE TABLE IF NOT EXISTS trace_results (
  ts_ms   INTEGER NOT NULL,
  name    TEXT    NOT NULL,
  address TEXT    NOT NULL,
  reason  TEXT    NOT NULL,  -- start|loss|high_rtt
  ok      INTEGER NOT NULL,  -- 0/1
  output  TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trace_results_ts
  ON trace_results(ts_ms);

CREATE INDEX IF NOT EXISTS idx_trace_results_name_ts
  ON trace_results(name, ts_ms);

CREATE INDEX IF NOT EXISTS idx_trace_results_addr_ts
  ON trace_results(address, ts_ms);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}
