package storage

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExportMergedCSVGZ делает один CSV.gz со строками ping/trace по времени.
// startMs/endMs = 0 -> без фильтра по времени.
func (s *SQLiteStore) ExportMergedCSVGZ(ctx context.Context, outPath string, startMs, endMs int64) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("sqlite store is nil")
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	bw := bufio.NewWriterSize(f, 1024*1024)
	defer bw.Flush()

	gz, err := gzip.NewWriterLevel(bw, gzip.BestCompression)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	cw := csv.NewWriter(gz)
	defer cw.Flush()

	// Единый формат строк
	if err := cw.Write([]string{
		"ts_ms", "ts_iso", "kind",
		"name", "address",
		"ttl", "rtt_ms", "error",
		"reason", "ok", "output",
	}); err != nil {
		return "", fmt.Errorf("header: %w", err)
	}

	where := ""
	args := []any{}
	if startMs > 0 {
		where += " AND ts_ms >= ?"
		args = append(args, startMs)
	}
	if endMs > 0 {
		where += " AND ts_ms <= ?"
		args = append(args, endMs)
	}

	// kind_order: ping(0) раньше trace(1) при одинаковом ts_ms
	q := fmt.Sprintf(`
SELECT ts_ms, kind, name, address, ttl, rtt_ms, error, reason, ok, output
FROM (
  SELECT
    ts_ms, 0 AS kind_order, 'ping' AS kind,
    name, address,
    ttl, rtt_ms,
    COALESCE(error,'') AS error,
    '' AS reason, NULL AS ok, '' AS output
  FROM ping_results
  WHERE 1=1 %s

  UNION ALL

  SELECT
    ts_ms, 1 AS kind_order, 'trace' AS kind,
    name, address,
    NULL AS ttl, NULL AS rtt_ms,
    '' AS error,
    reason, ok, output
  FROM trace_results
  WHERE 1=1 %s
)
ORDER BY ts_ms ASC, kind_order ASC;
`, where, where)

	rows, err := s.db.QueryContext(ctx, q, append(args, args...)...)
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tsMs   int64
			kind   string
			name   string
			addr   string
			ttl    sql.NullInt64
			rtt    sql.NullInt64
			errTxt string
			reason string
			ok     sql.NullInt64
			out    string
		)

		if err := rows.Scan(&tsMs, &kind, &name, &addr, &ttl, &rtt, &errTxt, &reason, &ok, &out); err != nil {
			return "", fmt.Errorf("scan: %w", err)
		}

		tsISO := time.UnixMilli(tsMs).UTC().Format(time.RFC3339Nano)

		ttlStr := ""
		if ttl.Valid {
			ttlStr = fmt.Sprintf("%d", ttl.Int64)
		}
		rttStr := ""
		if rtt.Valid {
			rttStr = fmt.Sprintf("%d", rtt.Int64)
		}
		okStr := ""
		if ok.Valid {
			okStr = fmt.Sprintf("%d", ok.Int64)
		}

		if err := cw.Write([]string{
			fmt.Sprintf("%d", tsMs),
			tsISO,
			kind,
			name,
			addr,
			ttlStr,
			rttStr,
			errTxt,
			reason,
			okStr,
			out,
		}); err != nil {
			return "", fmt.Errorf("write: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("rows: %w", err)
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		return "", fmt.Errorf("csv flush: %w", err)
	}

	return outPath, nil
}
