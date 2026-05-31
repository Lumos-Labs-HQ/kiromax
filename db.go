package main

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS kiromax_meta (key TEXT PRIMARY KEY, value TEXT)`)
	return db, nil
}

func getMeta(db *sql.DB, key, def string) string {
	var v string
	if err := db.QueryRow(`SELECT value FROM kiromax_meta WHERE key=?`, key).Scan(&v); err == nil {
		return v
	}
	db.Exec(`INSERT INTO kiromax_meta(key,value) VALUES(?,?)`, key, def)
	return def
}

func setMeta(db *sql.DB, key, val string) {
	db.Exec(`INSERT OR REPLACE INTO kiromax_meta(key,value) VALUES(?,?)`, key, val)
}
