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

func kiroBase() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("cannot determine home directory: " + err.Error())
	}
	return filepath.Join(home, ".local", "share", "kiro-cli")
}

var (
	dataDB      = filepath.Join(kiroBase(), "data.sqlite3")
	kiroDataDir = filepath.Join(kiroBase(), "kiro_data")  // need to create this directory manually and put session sqlite3 files there, named like "work.sqlite3", "personal.sqlite3", etc.
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

type SocialToken struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
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

	uid := getMeta(db, "uuid", "")
	if uid == "" {
		uid = uuid.New().String()
		setMeta(db, "uuid", uid)
	}
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

	return Session{UUID: uid, File: path, FileName: name, Ended: ended, Token: token, TokenExpiresAt: tokenExpiresAt, UsedAt: usedAt}, nil
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
			fmt.Printf("→ Marked session %d [%s] (%s) as ended\n", s.ID, s.UUID[:8], s.FileName)
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
		fmt.Printf("✓ Swapped to session %d [%s] (%s) — restart kiro-cli to apply\n", s.ID, s.UUID[:8], s.FileName)
		return nil
	}

	fmt.Println("✗ All sessions are ended or already used this month.")
	fmt.Println("  Run: kiromax reset   to unend all sessions")
	return nil
}

// resolveSession accepts a numeric ID (e.g. "2") or a filename (e.g. "work").
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

func useSession(arg string) error {
	s, err := resolveSession(arg)
	if err != nil {
		return err
	}
	if err := swapTo(s); err != nil {
		return err
	}
	fmt.Printf("✓ Swapped to session %d [%s] (%s) — restart kiro-cli to apply\n", s.ID, s.UUID[:8], s.FileName)
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
		fmt.Printf("%-4s %-20s %-10s %-8s %-6s %-16s\n", "ID", "FILE", "UUID", "STATUS", "ENDED", "USED")
		fmt.Println(strings.Repeat("-", 68))
		for _, s := range sessions {
			status := "idle"
			if s.Active {
				status = "ACTIVE"
			} else if s.Ended {
				status = "ended"
			}
			ended := "no"
			if s.Ended {
				ended = "YES"
			}
			used := "never"
			if !s.UsedAt.IsZero() {
				used = s.UsedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("%-4d %-20s %-10s %-8s %-6s %-16s\n", s.ID, s.FileName, s.UUID[:8], status, ended, used)
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
			fmt.Fprintln(os.Stderr, "usage: kiromax end <id|name>")
			os.Exit(1)
		}
		s, err := resolveSession(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		db, err := openDB(s.File)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		setMeta(db, "ended", "true")
		db.Close()
		fmt.Printf("✓ Session %d (%s) marked as ended\n", s.ID, s.FileName)

	case "reset":
		if len(os.Args) >= 3 {
			s, err := resolveSession(os.Args[2])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			db, err := openDB(s.File)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			setMeta(db, "ended", "false")
			setMeta(db, "used_at", "")
			db.Close()
			fmt.Printf("✓ Session %d (%s) unended\n", s.ID, s.FileName)
		} else {
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
				setMeta(db, "used_at", "")
				db.Close()
			}
			fmt.Println("✓ All sessions unended — available for swap again")
		}

	case "credits":
		var s Session
		if len(os.Args) < 3 {
			// default to active session
			sessions, err := listSessions()
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(1)
			}
			for _, ss := range sessions {
				if ss.Active {
					s = ss
					break
				}
			}
			if s.FileName == "" {
				fmt.Fprintln(os.Stderr, "no active session; specify: kiromax credits <id|name>")
				os.Exit(1)
			}
		} else {
			var err error
			s, err = resolveSession(os.Args[2])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		if s.Active {
			// use live data.sqlite3 token — kiro-cli refreshes it there
			if liveDB, err := openDB(dataDB); err == nil {
				var raw string
				if liveDB.QueryRow(`SELECT value FROM auth_kv WHERE key='kirocli:social:token'`).Scan(&raw) == nil {
					var t SocialToken
					if json.Unmarshal([]byte(raw), &t) == nil {
						s.Token = t.AccessToken
						s.TokenExpiresAt, _ = time.Parse(time.RFC3339Nano, t.ExpiresAt)
					}
				}
				liveDB.Close()
			}
		}
		if s.Token == "" {
			fmt.Fprintln(os.Stderr, "no token for session", s.FileName)
			os.Exit(1)
		}
		if !s.TokenExpiresAt.IsZero() && time.Now().After(s.TokenExpiresAt) {
			fmt.Fprintf(os.Stderr, "warning: token expired at %s — swap to this session to refresh\n", s.TokenExpiresAt.Local().Format("2006-01-02 15:04"))
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
			fmt.Printf("%s: %.2f / %.0f\n", u.DisplayName, u.CurrentUsageWithPrecision, u.UsageLimitWithPrecision)
		}

	default:
		printHelp()
	}
}
