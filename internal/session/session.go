package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Lumos-Labs-HQ/kmax/internal/db"
	"github.com/google/uuid"
)

type Session struct {
	ID             int
	UUID           string
	File           string
	FileName       string
	Active         bool
	Ended          bool
	Token          string
	TokenExpiresAt time.Time
	UsedAt         time.Time
}

// Load reads session metadata from a session file in kiroDataDir.
func Load(name, kiroDataDir string) (Session, error) {
	path := filepath.Join(kiroDataDir, name+".sqlite3")
	d, err := db.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer d.Close()

	uid := db.GetMeta(d, "uuid")
	if uid == "" {
		uid = uuid.New().String()
		db.SetMeta(d, "uuid", uid)
	}
	ended := db.GetMeta(d, "ended") == "true"
	var usedAt time.Time
	if s := db.GetMeta(d, "used_at"); s != "" {
		usedAt, _ = time.Parse(time.RFC3339, s)
	}
	token, tokenExpiresAt := db.ReadToken(d)

	return Session{
		UUID: uid, File: path, FileName: name,
		Ended: ended, Token: token, TokenExpiresAt: tokenExpiresAt, UsedAt: usedAt,
	}, nil
}

// List returns all sessions in kiroDataDir, sorted numerically then alphabetically.
func List(kiroDataDir, dataDB string) ([]Session, error) {
	entries, err := os.ReadDir(kiroDataDir)
	if err != nil {
		return nil, err
	}
	activeUUID := LiveActiveUUID(dataDB)
	var sessions []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sqlite3") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".sqlite3")
		s, err := Load(name, kiroDataDir)
		if err != nil {
			continue
		}
		s.Active = !s.Ended && activeUUID != "" && s.UUID == activeUUID
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		a, aerr := strconv.Atoi(sessions[i].FileName)
		b, berr := strconv.Atoi(sessions[j].FileName)
		if aerr == nil && berr == nil {
			return a < b
		}
		return sessions[i].FileName < sessions[j].FileName
	})
	for i := range sessions {
		sessions[i].ID = i + 1
	}
	return sessions, nil
}

// LiveActiveUUID returns the UUID of the session currently loaded in data.sqlite3.
func LiveActiveUUID(dataDB string) string {
	d, err := db.Open(dataDB)
	if err != nil {
		return ""
	}
	defer d.Close()
	return db.GetMeta(d, "active_uuid")
}

// UsedThisMonth reports whether the session was swapped in during the current calendar month.
func UsedThisMonth(s Session) bool {
	if s.UsedAt.IsZero() {
		return false
	}
	now := time.Now()
	return s.UsedAt.Year() == now.Year() && s.UsedAt.Month() == now.Month()
}

// mergeConversations copies conversations_v2 rows from src into dst.
// Only runs if both files already have the table (kiro-cli ran its migrations).
func mergeConversations(srcPath, dstPath string) {
	src, err := db.Open(srcPath)
	if err != nil {
		return
	}
	defer src.Close()

	dst, err := db.Open(dstPath)
	if err != nil {
		return
	}
	defer dst.Close()

	// Only merge if both sides already have the table — never create it ourselves.
	var srcHas, dstHas int
	src.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='conversations_v2'`).Scan(&srcHas)
	dst.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='conversations_v2'`).Scan(&dstHas)
	if srcHas == 0 || dstHas == 0 {
		return
	}

	rows, err := src.Query(`SELECT key, conversation_id, value, created_at, updated_at FROM conversations_v2`)
	if err != nil {
		return
	}
	defer rows.Close()

	tx, _ := dst.Begin()
	for rows.Next() {
		var key, convID, value string
		var createdAt, updatedAt int64
		if rows.Scan(&key, &convID, &value, &createdAt, &updatedAt) != nil {
			continue
		}
		tx.Exec(`INSERT INTO conversations_v2(key,conversation_id,value,created_at,updated_at)
			VALUES(?,?,?,?,?)
			ON CONFLICT(key,conversation_id) DO UPDATE SET
				value=excluded.value, updated_at=excluded.updated_at
			WHERE excluded.updated_at > conversations_v2.updated_at`,
			key, convID, value, createdAt, updatedAt)
	}
	tx.Commit()
}

// normalizeStateToText ensures all rows in the state table are stored as TEXT.
// Some sessions end up with BLOB values which kiro-cli's driver cannot read.
func normalizeStateToText(dbPath string) {
	d, err := db.Open(dbPath)
	if err != nil {
		return
	}
	defer d.Close()
	d.Exec(`UPDATE state SET value = CAST(value AS TEXT) WHERE typeof(value) = 'blob'`)
}

// SyncActiveBack saves the live data.sqlite3 back to the active session file.
func SyncActiveBack(dataDB, kiroDataDir string) error {
	activeUUID := LiveActiveUUID(dataDB)
	if activeUUID == "" {
		return nil
	}
	sessions, err := List(kiroDataDir, dataDB)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.UUID == activeUUID {
			data, err := os.ReadFile(dataDB)
			if err != nil {
				return err
			}
			return os.WriteFile(s.File, data, 0600)
		}
	}
	return nil
}

// SwapTo switches the active session:
//  1. Saves the current data.sqlite3 back to the active session file.
//  2. Copies the target session file to data.sqlite3.
func SwapTo(s Session, dataDB, kiroDataDir string) error {
	if err := SyncActiveBack(dataDB, kiroDataDir); err != nil {
		return fmt.Errorf("failed to sync active session back: %w", err)
	}

	// Merge conversation history from all sessions into target (only if tables exist).
	entries, _ := os.ReadDir(kiroDataDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sqlite3") {
			continue
		}
		srcPath := filepath.Join(kiroDataDir, e.Name())
		if srcPath == s.File {
			continue
		}
		mergeConversations(srcPath, s.File)
	}

	normalizeStateToText(s.File)

	data, err := os.ReadFile(s.File)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dataDB, data, 0600); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339)
	for _, path := range []string{s.File, dataDB} {
		d, err := db.Open(path)
		if err != nil {
			continue
		}
		db.SetMeta(d, "active_uuid", s.UUID)
		db.SetMeta(d, "used_at", now)
		d.Close()
	}
	return nil
}

// Resolve finds a session by numeric ID or filename.
func Resolve(arg, kiroDataDir, dataDB string) (Session, error) {
	sessions, err := List(kiroDataDir, dataDB)
	if err != nil {
		return Session{}, err
	}
	if n, err2 := strconv.Atoi(arg); err2 == nil {
		for _, s := range sessions {
			if s.ID == n {
				return s, nil
			}
		}
		return Session{}, fmt.Errorf("no session with id %s", arg)
	}
	for _, s := range sessions {
		if s.FileName == arg {
			return s, nil
		}
	}
	return Session{}, fmt.Errorf("session %q not found", arg)
}
