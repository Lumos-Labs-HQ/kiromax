package db

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
)

type SocialToken struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
}

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS kiromax_meta (key TEXT PRIMARY KEY, value TEXT)`)
	return db, nil
}

func GetMeta(db *sql.DB, key string) string {
	var v string
	db.QueryRow(`SELECT value FROM kiromax_meta WHERE key=?`, key).Scan(&v)
	return v
}

func SetMeta(db *sql.DB, key, val string) {
	db.Exec(`INSERT OR REPLACE INTO kiromax_meta(key,value) VALUES(?,?)`, key, val)
}

func ReadToken(db *sql.DB) (token string, expiresAt time.Time) {
	var raw string
	if db.QueryRow(`SELECT value FROM auth_kv WHERE key='kirocli:social:token'`).Scan(&raw) == nil {
		var t SocialToken
		if json.Unmarshal([]byte(raw), &t) == nil {
			token = t.AccessToken
			expiresAt, _ = time.Parse(time.RFC3339Nano, t.ExpiresAt)
		}
	}
	return
}
