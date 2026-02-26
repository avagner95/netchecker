package storage

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type Row struct {
	TsMs  int64
	Name  string
	Addr  string
	TTL   sql.NullInt64
	RTTms sql.NullInt64
	Error sql.NullString
}

type SQLiteStore struct {
	db *sql.DB
}

type DashboardSummaryRow struct {
	Name       string  `json:"name"`
	Address    string  `json:"address"`
	LastTsMs   int64   `json:"last_ts_ms"`
	LastOK     bool    `json:"last_ok"`
	Total      int64   `json:"total"`
	Errors     int64   `json:"errors"`
	LossPct    float64 `json:"loss_pct"`
	AvgRttMs   float64 `json:"avg_rtt_ms"`
	MaxRttMs   int64   `json:"max_rtt_ms"`
	LastOKTsMs int64   `json:"last_ok_ts_ms"`
}

type DashboardSeriesPoint struct {
	BucketMs int64  `json:"bucket_ms"` // e.g. 1700000000000 aligned to 1s
	Name     string `json:"name"`
	AvgRttMs int64  `json:"avg_rtt_ms"` // avg of rtt within bucket, errors excluded
	Errors   int64  `json:"errors"`     // count errors in bucket
	Total    int64  `json:"total"`      // total samples in bucket
}

type DashboardResponse struct {
	NowMs        int64                  `json:"now_ms"`
	FromMs       int64                  `json:"from_ms"`   // now-30m
	BucketMs     int64                  `json:"bucket_ms"` // 1000
	Summary      []DashboardSummaryRow  `json:"summary"`
	Series       []DashboardSeriesPoint `json:"series"`
	LastBucketMs int64                  `json:"last_bucket_ms"` // max bucket in response
}
