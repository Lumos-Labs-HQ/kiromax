package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID             int
	UUID           string
	File           string
	FileName       string
	GuildName      string // empty if not in a guild
	Active         bool
	Ended          bool
	Token          string
	TokenExpiresAt time.Time
	UsedAt         time.Time
}

type SocialToken struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
}

func loadSession(path, fileName, guildName string) (Session, error) {
	db, err := openDB(path)
	if err != nil {
		return Session{}, err
	}
	defer db.Close()

	uid := getMeta(db, "uuid", uuid.New().String())
	ended := getMeta(db, "ended", "false") == "true"
	usedAtStr := getMeta(db, "used_at", "")
	var usedAt time.Time
	if usedAtStr != "" {
		usedAt, _ = time.Parse(time.RFC3339, usedAtStr)
	}

	var token string
	var tokenExpiresAt time.Time
	var raw string
	if err := db.QueryRow(`SELECT value FROM auth_kv WHERE key='kirocli:social:token'`).Scan(&raw); err == nil {
		var t SocialToken
		if json.Unmarshal([]byte(raw), &t) == nil {
			token = t.AccessToken
			tokenExpiresAt, _ = time.Parse(time.RFC3339Nano, t.ExpiresAt)
		}
	}

	return Session{
		UUID: uid, File: path, FileName: fileName, GuildName: guildName,
		Ended: ended, Token: token, TokenExpiresAt: tokenExpiresAt, UsedAt: usedAt,
	}, nil
}

// listSessions lists flat sessions (kiro_data/*.sqlite3, not in guild subdirs).
func listSessions() ([]Session, error) {
	entries, err := os.ReadDir(kiroDataDir)
	if err != nil {
		return nil, err
	}
	activeUUID := liveActiveUUID()
	var sessions []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sqlite3") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".sqlite3")
		path := filepath.Join(kiroDataDir, e.Name())
		s, err := loadSession(path, name, "")
		if err != nil {
			continue
		}
		s.Active = activeUUID != "" && s.UUID == activeUUID
		sessions = append(sessions, s)
	}
	sortSessions(sessions)
	for i := range sessions {
		sessions[i].ID = i + 1
	}
	return sessions, nil
}

func sortSessions(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		a, aerr := strconv.Atoi(sessions[i].FileName)
		b, berr := strconv.Atoi(sessions[j].FileName)
		if aerr == nil && berr == nil {
			return a < b
		}
		return sessions[i].FileName < sessions[j].FileName
	})
}

func liveActiveUUID() string {
	db, err := openDB(dataDB)
	if err != nil {
		return ""
	}
	defer db.Close()
	var v string
	db.QueryRow(`SELECT value FROM kiromax_meta WHERE key='active_uuid'`).Scan(&v)
	return v
}

func usedThisMonth(s Session) bool {
	if s.UsedAt.IsZero() {
		return false
	}
	now := time.Now()
	return s.UsedAt.Year() == now.Year() && s.UsedAt.Month() == now.Month()
}

func swapTo(s Session) error {
	data, err := os.ReadFile(s.File)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dataDB, data, 0600); err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339)
	for _, path := range []string{s.File, dataDB} {
		db, err := openDB(path)
		if err != nil {
			continue
		}
		setMeta(db, "active_uuid", s.UUID)
		setMeta(db, "used_at", now)
		db.Close()
	}
	return nil
}

// autoSwap swaps to next flat session (original behaviour).
func autoSwap() error {
	sessions, err := listSessions()
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.Active {
			db, _ := openDB(s.File)
			setMeta(db, "ended", "true")
			db.Close()
			fmt.Printf("→ Marked session %d [%s] (%s) as ended\n", s.ID, s.UUID[:8], s.FileName)
			break
		}
	}
	sessions, err = listSessions()
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.Ended || usedThisMonth(s) {
			continue
		}
		if err := swapTo(s); err != nil {
			return err
		}
		fmt.Printf("✓ Swapped to session %d [%s] (%s) — restart kiro-cli to apply\n", s.ID, s.UUID[:8], s.FileName)
		return nil
	}
	fmt.Println("✗ All sessions are ended or already used this month.")
	fmt.Println("  Run: kiromax reset   to unend all sessions")
	return nil
}

func resolveSession(arg string) (Session, error) {
	sessions, err := listSessions()
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

// liveToken reads the refreshed token from data.sqlite3 for the active session.
func liveToken(s Session) Session {
	if !s.Active {
		return s
	}
	db, err := openDB(dataDB)
	if err != nil {
		return s
	}
	defer db.Close()
	var raw string
	if db.QueryRow(`SELECT value FROM auth_kv WHERE key='kirocli:social:token'`).Scan(&raw) == nil {
		var t SocialToken
		if json.Unmarshal([]byte(raw), &t) == nil {
			s.Token = t.AccessToken
			s.TokenExpiresAt, _ = time.Parse(time.RFC3339Nano, t.ExpiresAt)
		}
	}
	return s
}
