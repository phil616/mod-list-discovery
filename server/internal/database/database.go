package database

import (
	"context"
	"database/sql"
	"embed"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sort"
)

//go:embed *.sql
var migrations embed.FS

func Open(path string) (*sql.DB, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "CREATE TABLE IF NOT EXISTS schema_migrations(version TEXT PRIMARY KEY)"} {
		if _, e = db.Exec(q); e != nil {
			db.Close()
			return nil, e
		}
	}
	entries, e := migrations.ReadDir(".")
	if e != nil {
		db.Close()
		return nil, e
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, f := range entries {
		var n int
		if e = db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=?", f.Name()).Scan(&n); e != nil {
			db.Close()
			return nil, e
		}
		if n > 0 {
			continue
		}
		b, e := migrations.ReadFile(f.Name())
		if e != nil {
			db.Close()
			return nil, e
		}
		tx, e := db.BeginTx(context.Background(), nil)
		if e != nil {
			db.Close()
			return nil, e
		}
		if _, e = tx.Exec(string(b)); e == nil {
			_, e = tx.Exec("INSERT INTO schema_migrations VALUES(?)", f.Name())
		}
		if e != nil {
			tx.Rollback()
			db.Close()
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			db.Close()
			return nil, e
		}
	}
	return db, nil
}
