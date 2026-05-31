package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Lumos-Labs-HQ/kiromax/internal/credits"
	"github.com/Lumos-Labs-HQ/kiromax/internal/db"
	"github.com/Lumos-Labs-HQ/kiromax/internal/session"
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
	kiroDataDir = filepath.Join(kiroBase(), "kiro_data")
)

func autoSwap() error {
	sessions, err := session.List(kiroDataDir, dataDB)
	if err != nil {
		return err
	}

	for _, s := range sessions {
		if s.Active {
			d, err := db.Open(s.File)
			if err != nil {
				return err
			}
			db.SetMeta(d, "ended", "true")
			d.Close()
			fmt.Printf("→ Marked session %d [%s] (%s) as ended\n", s.ID, s.UUID[:8], s.FileName)
			break
		}
	}

	sessions, err = session.List(kiroDataDir, dataDB)
	if err != nil {
		return err
	}

	for _, s := range sessions {
		if s.Ended || session.UsedThisMonth(s) {
			continue
		}
		if err := session.SwapTo(s, dataDB, kiroDataDir); err != nil {
			return err
		}
		fmt.Printf("✓ Swapped to session %d [%s] (%s) — restart kiro-cli to apply\n", s.ID, s.UUID[:8], s.FileName)
		return nil
	}

	fmt.Println("✗ All sessions are ended or already used this month.")
	fmt.Println("  Run: kiromax reset   to unend all sessions")
	return nil
}

func printHelp() {
	fmt.Print(`kiromax - Kiro session manager

Commands:
  list           List all sessions
  swap           Auto-swap to next available session (marks current as ended)
  use <id>       Force swap to specific session ID
  end <id>       Mark session as ended
  reset [<id>]   Unend all sessions (or specific one), clearing used_at
  credits [<id>] Show live credit usage (defaults to active session)
`)
}

func die(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	switch os.Args[1] {
	case "list":
		sessions, err := session.List(kiroDataDir, dataDB)
		if err != nil {
			die("error:", err)
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
			die("error:", err)
		}

	case "use":
		if len(os.Args) < 3 {
			die("usage: kiromax use <id>")
		}
		s, err := session.Resolve(os.Args[2], kiroDataDir, dataDB)
		if err != nil {
			die("error:", err)
		}
		if err := session.SwapTo(s, dataDB, kiroDataDir); err != nil {
			die("error:", err)
		}
		fmt.Printf("✓ Swapped to session %d [%s] (%s) — restart kiro-cli to apply\n", s.ID, s.UUID[:8], s.FileName)

	case "end":
		if len(os.Args) < 3 {
			die("usage: kiromax end <id|name>")
		}
		s, err := session.Resolve(os.Args[2], kiroDataDir, dataDB)
		if err != nil {
			die(err)
		}
		d, err := db.Open(s.File)
		if err != nil {
			die("error:", err)
		}
		db.SetMeta(d, "ended", "true")
		d.Close()
		fmt.Printf("✓ Session %d (%s) marked as ended\n", s.ID, s.FileName)

	case "reset":
		if len(os.Args) >= 3 {
			s, err := session.Resolve(os.Args[2], kiroDataDir, dataDB)
			if err != nil {
				die(err)
			}
			d, err := db.Open(s.File)
			if err != nil {
				die("error:", err)
			}
			db.SetMeta(d, "ended", "false")
			db.SetMeta(d, "used_at", "")
			d.Close()
			fmt.Printf("✓ Session %d (%s) unended\n", s.ID, s.FileName)
		} else {
			sessions, err := session.List(kiroDataDir, dataDB)
			if err != nil {
				die("error:", err)
			}
			for _, s := range sessions {
				d, err := db.Open(s.File)
				if err != nil {
					continue
				}
				db.SetMeta(d, "ended", "false")
				db.SetMeta(d, "used_at", "")
				d.Close()
			}
			fmt.Println("✓ All sessions unended — available for swap again")
		}

	case "credits":
		var s session.Session
		if len(os.Args) < 3 {
			sessions, err := session.List(kiroDataDir, dataDB)
			if err != nil {
				die("error:", err)
			}
			for _, ss := range sessions {
				if ss.Active {
					s = ss
					break
				}
			}
			if s.FileName == "" {
				die("no active session; specify: kiromax credits <id|name>")
			}
		} else {
			var err error
			s, err = session.Resolve(os.Args[2], kiroDataDir, dataDB)
			if err != nil {
				die(err)
			}
		}
		if err := credits.Print(s, dataDB); err != nil {
			die("error:", err)
		}

	default:
		printHelp()
	}
}
