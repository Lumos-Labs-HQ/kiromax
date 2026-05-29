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

	_ "modernc.org/sqlite"
)

const (
	kiroDir    = "/home/rana/.local/share/kiro-cli"
	dataDB     = "/home/rana/.local/share/kiro-cli/data.sqlite3"
	kiroDataDir = "/home/rana/.local/share/kiro-cli/kiro_data"
)

type Session struct {
	ID      string
	File    string
	Active  bool
	Ended   bool
	Token   string
	Expires time.Time
}

type SocialToken struct {
	AccessToken  string `json:"access_token"`
	ExpiresAt    string `json:"expires_at"`
	RefreshToken string `json:"refresh_token"`
	Provider     string `json:"provider"`
	ProfileARN   string `json:"profile_arn"`
}

func openDB(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path)
}

func listSessions() ([]Session, error) {
	entries, err := os.ReadDir(kiroDataDir)
	if err != nil {
		return nil, err
	}

	// Get active session token from main db
	activeToken := getActiveToken(dataDB)

	var sessions []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sqlite3") {
			continue
		}
		path := filepath.Join(kiroDataDir, e.Name())
		id := strings.TrimSuffix(e.Name(), ".sqlite3")

		db, err := openDB(path)
		if err != nil {
			continue
		}
		token, expires := getTokenInfo(db)
		db.Close()

		ended := isEnded(path)
		active := !ended && activeToken != "" && token != "" && token == activeToken

		sessions = append(sessions, Session{
			ID:      id,
			File:    path,
			Active:  active,
			Ended:   ended,
			Token:   token,
			Expires: expires,
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		a, _ := strconv.Atoi(sessions[i].ID)
		b, _ := strconv.Atoi(sessions[j].ID)
		return a < b
	})
	return sessions, nil
}

func getActiveToken(dbPath string) string {
	db, err := openDB(dbPath)
	if err != nil {
		return ""
	}
	defer db.Close()
	token, _ := getTokenInfo(db)
	return token
}

func getTokenInfo(db *sql.DB) (string, time.Time) {
	var raw string
	err := db.QueryRow(`SELECT value FROM auth_kv WHERE key='kirocli:social:token'`).Scan(&raw)
	if err != nil {
		return "", time.Time{}
	}
	var t SocialToken
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return "", time.Time{}
	}
	exp, _ := time.Parse(time.RFC3339Nano, t.ExpiresAt)
	return t.AccessToken[:min(20, len(t.AccessToken))], exp
}

func isEnded(dbPath string) bool {
	marker := dbPath + ".ended"
	_, err := os.Stat(marker)
	return err == nil
}

func markEnded(dbPath string) error {
	marker := dbPath + ".ended"
	f, err := os.Create(marker)
	if err != nil {
		return err
	}
	f.Close()
	return nil
}

func unmarkEnded(dbPath string) error {
	marker := dbPath + ".ended"
	return os.Remove(marker)
}

func swapSession(id string) error {
	path := filepath.Join(kiroDataDir, id+".sqlite3")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("session %s not found", id)
	}
	if isEnded(path) {
		return fmt.Errorf("session %s is marked as ended", id)
	}

	// Copy session db over main data.sqlite3
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(dataDB, src, 0600)
}

func markAllEnded() error {
	sessions, err := listSessions()
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if !s.Ended {
			if err := markEnded(s.File); err != nil {
				return err
			}
		}
	}
	return nil
}

type UsageBreakdown struct {
	DisplayName              string  `json:"displayName"`
	CurrentUsage             float64 `json:"currentUsageWithPrecision"`
	UsageLimit               float64 `json:"usageLimitWithPrecision"`
	ResourceType             string  `json:"resourceType"`
}

type UsageLimitsResponse struct {
	DaysUntilReset    int              `json:"daysUntilReset"`
	UsageBreakdownList []UsageBreakdown `json:"usageBreakdownList"`
	SubscriptionInfo  struct {
		SubscriptionTitle string `json:"subscriptionTitle"`
	} `json:"subscriptionInfo"`
}

func fetchCredits(dbPath string) string {
	db, err := openDB(dbPath)
	if err != nil {
		return "err"
	}
	defer db.Close()

	var raw string
	if err := db.QueryRow(`SELECT value FROM auth_kv WHERE key='kirocli:social:token'`).Scan(&raw); err != nil {
		return "no token"
	}
	var t SocialToken
	if err := json.Unmarshal([]byte(raw), &t); err != nil {
		return "err"
	}
	if time.Now().After(func() time.Time { e, _ := time.Parse(time.RFC3339Nano, t.ExpiresAt); return e }()) {
		return "expired"
	}

	req, _ := http.NewRequest("GET", "https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits", nil)
	req.Header.Set("Authorization", "Bearer "+t.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "offline"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var usage UsageLimitsResponse
	if err := json.Unmarshal(body, &usage); err != nil || len(usage.UsageBreakdownList) == 0 {
		return "N/A"
	}

	var parts []string
	for _, u := range usage.UsageBreakdownList {
		parts = append(parts, fmt.Sprintf("%s %.0f/%.0f", u.DisplayName, u.CurrentUsage, u.UsageLimit))
	}
	return strings.Join(parts, " | ")
}

func printHelp() {
	fmt.Println(`kiromax - Kiro session manager

Commands:
  list              List all sessions with status
  swap <id>         Swap to session by ID (replaces active data.sqlite3)
  end <id>          Mark session as ended
  unend <id>        Remove ended mark from session
  reset             Mark ALL sessions as ended
  help              Show this help`)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	cmd := os.Args[1]

	switch cmd {
	case "list":
		sessions, err := listSessions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions found in", kiroDataDir)
			return
		}
		fmt.Printf("%-6s %-8s %-8s %-22s %s\n", "ID", "STATUS", "ENDED", "EXPIRES", "CREDITS")
		fmt.Println(strings.Repeat("-", 75))
		for _, s := range sessions {
			status := "idle"
			if s.Active {
				status = "ACTIVE"
			}
			ended := "no"
			if s.Ended {
				ended = "YES"
			}
			exp := "N/A"
			if !s.Expires.IsZero() {
				if time.Now().After(s.Expires) {
					exp = "EXPIRED"
				} else {
					exp = s.Expires.Format("2006-01-02 15:04:05")
				}
			}
			credits := fetchCredits(s.File)
			fmt.Printf("%-6s %-8s %-8s %-22s %s\n", s.ID, status, ended, exp, credits)
		}

	case "swap":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax swap <id>")
			os.Exit(1)
		}
		id := os.Args[2]
		if err := swapSession(id); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Swapped to session %s (restart kiro-cli to apply)\n", id)

	case "end":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax end <id>")
			os.Exit(1)
		}
		id := os.Args[2]
		path := filepath.Join(kiroDataDir, id+".sqlite3")
		if _, err := os.Stat(path); err != nil {
			fmt.Fprintf(os.Stderr, "session %s not found\n", id)
			os.Exit(1)
		}
		if err := markEnded(path); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Session %s marked as ended\n", id)

	case "unend":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax unend <id>")
			os.Exit(1)
		}
		id := os.Args[2]
		path := filepath.Join(kiroDataDir, id+".sqlite3")
		if err := unmarkEnded(path); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Session %s unended\n", id)

	case "reset":
		if err := markAllEnded(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ All sessions marked as ended")

	default:
		printHelp()
	}
}


