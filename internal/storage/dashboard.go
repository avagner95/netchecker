package storage

import (
	"context"
	"fmt"
	"time"
)

const (
	dashboardWindow = 30 * time.Minute
	dashboardBucket = int64(1000) // 1s
)

func (s *SQLiteStore) IsReady() bool {
	if s == nil || s.db == nil {
		return false
	}
	return true
}
func (s *SQLiteStore) DashboardPoll(ctx context.Context, lastBucketMs int64) (*DashboardResponse, error) {
	nowMs := time.Now().UnixMilli()
	fromMs := nowMs - int64(dashboardWindow/time.Millisecond)

	summary, err := s.queryDashboardSummary(ctx, fromMs)
	if err != nil {
		return nil, err
	}

	series, lastOutBucket, err := s.queryDashboardSeriesBuckets(ctx, fromMs, lastBucketMs)
	if err != nil {
		return nil, err
	}

	return &DashboardResponse{
		NowMs:        nowMs,
		FromMs:       fromMs,
		BucketMs:     dashboardBucket,
		Summary:      summary,
		Series:       series,
		LastBucketMs: lastOutBucket,
	}, nil
}

func (s *SQLiteStore) queryDashboardSummary(ctx context.Context, fromMs int64) ([]DashboardSummaryRow, error) {
	// 1) aggregates per name,address over window
	// 2) last row per name,address in window to compute UP/DOWN
	// 3) last ok ts per name,address
	q := `
WITH agg AS (
  SELECT
    name,
    address,
    COUNT(*) AS total,
    SUM(CASE WHEN error IS NOT NULL THEN 1 ELSE 0 END) AS errors,
    AVG(CASE WHEN error IS NULL THEN rtt_ms END) AS avg_rtt_ms,
    MAX(CASE WHEN error IS NULL THEN rtt_ms END) AS max_rtt_ms,
    MAX(CASE WHEN error IS NULL THEN ts_ms ELSE NULL END) AS last_ok_ts_ms
  FROM ping_results
  WHERE ts_ms >= ?
  GROUP BY name, address
),
last_ts AS (
  SELECT name, address, MAX(ts_ms) AS last_ts_ms
  FROM ping_results
  WHERE ts_ms >= ?
  GROUP BY name, address
),
last_row AS (
  SELECT p.name, p.address, p.ts_ms AS last_ts_ms, p.error
  FROM ping_results p
  JOIN last_ts lt
    ON lt.name = p.name AND lt.address = p.address AND lt.last_ts_ms = p.ts_ms
)
SELECT
  a.name,
  a.address,
  lr.last_ts_ms,
  CASE WHEN lr.error IS NULL THEN 1 ELSE 0 END AS last_ok,
  a.total,
  a.errors,
  CASE WHEN a.total = 0 THEN 0.0 ELSE (a.errors * 100.0 / a.total) END AS loss_pct,
  COALESCE(a.avg_rtt_ms, 0) AS avg_rtt_ms,
  COALESCE(a.max_rtt_ms, 0) AS max_rtt_ms,
  COALESCE(a.last_ok_ts_ms, 0) AS last_ok_ts_ms
FROM agg a
JOIN last_row lr
  ON lr.name = a.name AND lr.address = a.address
ORDER BY a.name;
`

	rows, err := s.db.QueryContext(ctx, q, fromMs, fromMs)
	if err != nil {
		return nil, fmt.Errorf("query dashboard summary: %w", err)
	}
	defer rows.Close()

	out := make([]DashboardSummaryRow, 0, 32)
	for rows.Next() {
		var r DashboardSummaryRow
		var lastOK int64
		if err := rows.Scan(
			&r.Name,
			&r.Address,
			&r.LastTsMs,
			&lastOK,
			&r.Total,
			&r.Errors,
			&r.LossPct,
			&r.AvgRttMs,
			&r.MaxRttMs,
			&r.LastOKTsMs,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard summary: %w", err)
		}
		r.LastOK = lastOK == 1
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows dashboard summary: %w", err)
	}
	return out, nil
}
func (s *SQLiteStore) queryDashboardSeriesBuckets(ctx context.Context, fromMs, lastBucketMs int64) ([]DashboardSeriesPoint, int64, error) {
	// If lastBucketMs==0 -> full window
	// else -> only buckets newer than lastBucketMs
	q := `
SELECT
  (ts_ms / ?) * ? AS bucket_ms,
  name,
  CAST(COALESCE(AVG(CASE WHEN error IS NULL THEN rtt_ms END), 0) AS INTEGER) AS avg_rtt_ms,
  SUM(CASE WHEN error IS NOT NULL THEN 1 ELSE 0 END) AS errors,
  COUNT(*) AS total
FROM ping_results
WHERE ts_ms >= ?
  AND ((ts_ms / ?) * ?) > ?
GROUP BY bucket_ms, name
ORDER BY bucket_ms ASC, name ASC;
`
	rows, err := s.db.QueryContext(ctx, q,
		dashboardBucket, dashboardBucket,
		fromMs,
		dashboardBucket, dashboardBucket, lastBucketMs,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query dashboard series: %w", err)
	}
	defer rows.Close()

	out := make([]DashboardSeriesPoint, 0, 4096)
	var maxBucket int64 = lastBucketMs
	for rows.Next() {
		var p DashboardSeriesPoint
		if err := rows.Scan(&p.BucketMs, &p.Name, &p.AvgRttMs, &p.Errors, &p.Total); err != nil {
			return nil, 0, fmt.Errorf("scan dashboard series: %w", err)
		}
		if p.BucketMs > maxBucket {
			maxBucket = p.BucketMs
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows dashboard series: %w", err)
	}
	return out, maxBucket, nil
}
