package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Lumos-Labs-HQ/kiromax/internal/db"
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

func LiveActiveUUID(dataDB string) string {
	d, err := db.Open(dataDB)
	if err != nil {
		return ""
	}
	defer d.Close()
	return db.GetMeta(d, "active_uuid")
}

func UsedThisMonth(s Session) bool {
	if s.UsedAt.IsZero() {
		return false
	}
	now := time.Now()
	return s.UsedAt.Year() == now.Year() && s.UsedAt.Month() == now.Month()
}

// SyncActiveBack saves the live data.sqlite3 back into the active session file
// before a swap, preserving refreshed tokens and chat history.
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

// SwapTo syncs current session back then copies target session to data.sqlite3.
func SwapTo(s Session, dataDB, kiroDataDir string) error {
	if err := SyncActiveBack(dataDB, kiroDataDir); err != nil {
		return fmt.Errorf("failed to sync active session back: %w", err)
	}
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
