package storage

import "database/sql"

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
