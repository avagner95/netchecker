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
