package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const (
	dataDB      = "/home/rana/.local/share/kiro-cli/data.sqlite3"
	kiroDataDir = "/home/rana/.local/share/kiro-cli/kiro_data"
)

type Session struct {
	ID     string
	UUID   string
	File   string
	Active bool
	Ended  bool
	Token  string
	UsedAt time.Time
}

type SocialToken struct {
	AccessToken string `json:"access_token"`
}

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

func loadSession(name string) (Session, error) {
	path := filepath.Join(kiroDataDir, name+".sqlite3")
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
	var raw string
	if err := db.QueryRow(`SELECT value FROM auth_kv WHERE key='kirocli:social:token'`).Scan(&raw); err == nil {
		var t SocialToken
		if json.Unmarshal([]byte(raw), &t) == nil {
			token = t.AccessToken
		}
	}

	return Session{ID: name, UUID: uid, File: path, Ended: ended, Token: token, UsedAt: usedAt}, nil
}

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
		s, err := loadSession(name)
		if err != nil {
			continue
		}
		s.Active = !s.Ended && activeUUID != "" && s.UUID == activeUUID
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		a, _ := strconv.Atoi(sessions[i].ID)
		b, _ := strconv.Atoi(sessions[j].ID)
		return a < b
	})
	return sessions, nil
}

// liveActiveUUID reads the UUID kiromax stored in data.sqlite3 on last swap.
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

// usedThisMonth: session was swapped in during the current calendar month.
func usedThisMonth(s Session) bool {
	if s.UsedAt.IsZero() {
		return false
	}
	now := time.Now()
	return s.UsedAt.Year() == now.Year() && s.UsedAt.Month() == now.Month()
}

// swapTo copies src session file to data.sqlite3 and records active_uuid + used_at.
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

func autoSwap() error {
	sessions, err := listSessions()
	if err != nil {
		return err
	}

	// mark current active as ended
	for _, s := range sessions {
		if s.Active {
			db, err := openDB(s.File)
			if err != nil {
				return err
			}
			setMeta(db, "ended", "true")
			db.Close()
			fmt.Printf("→ Marked session %s [%s] as ended\n", s.ID, s.UUID[:8])
			break
		}
	}

	// reload fresh state
	sessions, err = listSessions()
	if err != nil {
		return err
	}

	// pick next: not ended AND not used this month
	for _, s := range sessions {
		if s.Ended {
			continue
		}
		if usedThisMonth(s) {
			continue
		}
		if err := swapTo(s); err != nil {
			return err
		}
		fmt.Printf("✓ Swapped to session %s [%s] — restart kiro-cli to apply\n", s.ID, s.UUID[:8])
		return nil
	}

	fmt.Println("✗ All sessions are ended or already used this month.")
	fmt.Println("  Run: kiromax reset   to unend all sessions")
	return nil
}

func useSession(id string) error {
	s, err := loadSession(id)
	if err != nil {
		return fmt.Errorf("session %s not found", id)
	}
	sessions, _ := listSessions()
	for _, cur := range sessions {
		if cur.Active && cur.UUID != s.UUID {
			db, _ := openDB(cur.File)
			setMeta(db, "ended", "true")
			db.Close()
			fmt.Printf("→ Marked session %s [%s] as ended\n", cur.ID, cur.UUID[:8])
			break
		}
	}
	// reload to get fresh state (ended may have changed)
	s, _ = loadSession(id)
	if err := swapTo(s); err != nil {
		return err
	}
	fmt.Printf("✓ Swapped to session %s [%s] — restart kiro-cli to apply\n", s.ID, s.UUID[:8])
	return nil
}

type UsageBreakdown struct {
	DisplayName               string  `json:"displayName"`
	CurrentUsageWithPrecision float64 `json:"currentUsageWithPrecision"`
	UsageLimitWithPrecision   float64 `json:"usageLimitWithPrecision"`
}

type UsageLimitsResp struct {
	UsageBreakdownList []UsageBreakdown `json:"usageBreakdownList"`
}

func callUsageLimits(token string) (*UsageLimitsResp, error) {
	req, _ := http.NewRequest("GET", "https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result UsageLimitsResp
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func fetchCredits(token string) string {
	result, err := callUsageLimits(token)
	if err != nil {
		return "offline"
	}
	if len(result.UsageBreakdownList) == 0 {
		return "N/A"
	}
	var parts []string
	for _, u := range result.UsageBreakdownList {
		parts = append(parts, fmt.Sprintf("%s %.0f/%.0f", u.DisplayName, u.CurrentUsageWithPrecision, u.UsageLimitWithPrecision))
	}
	return strings.Join(parts, " | ")
}

func printHelp() {
	fmt.Print(`kiromax - Kiro session manager

Commands:
  list           List all sessions
  swap           Auto-swap to next available session (marks current as ended)
  use <id>       Force swap to specific session ID
  end <id>       Mark session as ended
  reset          Unend ALL sessions (make available again)
  credits <id>   Show live credit usage for a session
`)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	switch os.Args[1] {
	case "list":
		sessions, err := listSessions()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions found in", kiroDataDir)
			return
		}
		fmt.Printf("%-4s %-10s %-8s %-6s %-16s\n", "ID", "UUID", "STATUS", "ENDED", "USED")
		fmt.Println(strings.Repeat("-", 48))
		for _, s := range sessions {
			status := "idle"
			if s.Active {
				status = "ACTIVE"
			}
			ended := "no"
			if s.Ended {
				ended = "YES"
			}
			used := "never"
			if !s.UsedAt.IsZero() {
				used = s.UsedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("%-4s %-10s %-8s %-6s %-16s\n", s.ID, s.UUID[:8], status, ended, used)
		}

	case "swap":
		if err := autoSwap(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "use":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax use <id>")
			os.Exit(1)
		}
		if err := useSession(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}

	case "end":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax end <id>")
			os.Exit(1)
		}
		db, err := openDB(filepath.Join(kiroDataDir, os.Args[2]+".sqlite3"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "session not found")
			os.Exit(1)
		}
		setMeta(db, "ended", "true")
		db.Close()
		fmt.Printf("✓ Session %s marked as ended\n", os.Args[2])

	case "reset":
		sessions, err := listSessions()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		for _, s := range sessions {
			db, err := openDB(s.File)
			if err != nil {
				continue
			}
			setMeta(db, "ended", "false")
			db.Close()
		}
		fmt.Println("✓ All sessions unended — available for swap again")

	case "credits":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax credits <id>")
			os.Exit(1)
		}
		s, err := loadSession(os.Args[2])
		if err != nil || s.Token == "" {
			fmt.Fprintln(os.Stderr, "no token for session", os.Args[2])
			os.Exit(1)
		}
		result, err := callUsageLimits(s.Token)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if len(result.UsageBreakdownList) == 0 {
			fmt.Println("N/A")
			return
		}
		for _, u := range result.UsageBreakdownList {
			fmt.Printf("%s: %.0f / %.0f\n", u.DisplayName, u.CurrentUsageWithPrecision, u.UsageLimitWithPrecision)
		}

	default:
		printHelp()
	}
}
