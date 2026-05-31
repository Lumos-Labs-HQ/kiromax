package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
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

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	switch os.Args[1] {

	case "list":
		sessions, err := listSessions()
		if err != nil {
			fatal(err)
		}
		if len(sessions) == 0 {
			fmt.Println("No flat sessions found in", kiroDataDir)
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
		// --guild flag: advance to next guild manually
		if len(os.Args) >= 3 && os.Args[2] == "--guild" {
			if err := cmdGuildSwap(); err != nil {
				fatal(err)
			}
			return
		}
		if err := autoSwap(); err != nil {
			fatal(err)
		}

	case "use":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax use <id|name>")
			os.Exit(1)
		}
		s, err := resolveSession(os.Args[2])
		if err != nil {
			fatal(err)
		}
		if err := swapTo(s); err != nil {
			fatal(err)
		}
		fmt.Printf("✓ Swapped to session %d [%s] (%s) — restart kiro-cli to apply\n", s.ID, s.UUID[:8], s.FileName)

	case "end":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax end <id|name>")
			os.Exit(1)
		}
		s, err := resolveSession(os.Args[2])
		if err != nil {
			fatal(err)
		}
		db, _ := openDB(s.File)
		setMeta(db, "ended", "true")
		db.Close()
		fmt.Printf("✓ Session %d (%s) marked as ended\n", s.ID, s.FileName)

	case "reset":
		if len(os.Args) >= 3 {
			s, err := resolveSession(os.Args[2])
			if err != nil {
				fatal(err)
			}
			db, _ := openDB(s.File)
			setMeta(db, "ended", "false")
			db.Close()
			fmt.Printf("✓ Session %d (%s) unended\n", s.ID, s.FileName)
		} else {
			sessions, err := listSessions()
			if err != nil {
				fatal(err)
			}
			for _, s := range sessions {
				db, err := openDB(s.File)
				if err != nil {
					continue
				}
				setMeta(db, "ended", "false")
				db.Close()
			}
			fmt.Println("✓ All flat sessions unended")
		}

	case "credits":
		var s Session
		if len(os.Args) < 3 {
			sessions, err := listSessions()
			if err != nil {
				fatal(err)
			}
			for _, ss := range sessions {
				if ss.Active {
					s = ss
					break
				}
			}
			if s.FileName == "" {
				fmt.Fprintln(os.Stderr, "no active flat session; specify: kiromax credits <id|name>")
				os.Exit(1)
			}
		} else {
			var err error
			s, err = resolveSession(os.Args[2])
			if err != nil {
				fatal(err)
			}
		}
		s = liveToken(s)
		if s.Token == "" {
			fmt.Fprintln(os.Stderr, "no token for session", s.FileName)
			os.Exit(1)
		}
		if !s.TokenExpiresAt.IsZero() && time.Now().After(s.TokenExpiresAt) {
			fmt.Fprintf(os.Stderr, "warning: token expired at %s\n", s.TokenExpiresAt.Local().Format("2006-01-02 15:04"))
		}
		result, err := callUsageLimits(s.Token)
		if err != nil {
			fatal(err)
		}
		for _, u := range result.UsageBreakdownList {
			fmt.Printf("%s: %.2f / %.0f\n", u.DisplayName, u.CurrentUsageWithPrecision, u.UsageLimitWithPrecision)
		}

	case "capture":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax capture <guild>")
			os.Exit(1)
		}
		if err := cmdCapture(os.Args[2]); err != nil {
			fatal(err)
		}

	case "login":
		if err := cmdLogin(); err != nil {
			fatal(err)
		}

	case "continue", "c":
		bin, err := exec.LookPath("kiro-cli-chat")
		if err != nil {
			fatal(fmt.Errorf("kiro-cli-chat not found on PATH"))
		}
		syscall.Exec(bin, []string{"kiro-cli-chat", "chat", "--resume-picker"}, os.Environ())

	case "guild":
		if len(os.Args) < 3 {
			printGuildHelp()
			return
		}
		switch os.Args[2] {
		case "list":
			if err := cmdGuildList(); err != nil {
				fatal(err)
			}
		case "create":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "usage: kiromax guild create <name>")
				os.Exit(1)
			}
			if err := cmdGuildCreate(os.Args[3]); err != nil {
				fatal(err)
			}
		case "add":
			if len(os.Args) < 5 {
				fmt.Fprintln(os.Stderr, "usage: kiromax guild add <guild> <session.sqlite3>")
				os.Exit(1)
			}
			if err := cmdGuildAdd(os.Args[3], os.Args[4]); err != nil {
				fatal(err)
			}
		case "swap":
			if err := cmdGuildSwap(); err != nil {
				fatal(err)
			}
		case "reset":
			name := ""
			if len(os.Args) >= 4 {
				name = os.Args[3]
			}
			if err := cmdGuildReset(name); err != nil {
				fatal(err)
			}
		case "credits":
			name := ""
			if len(os.Args) >= 4 {
				name = os.Args[3]
			}
			if err := cmdGuildCredits(name); err != nil {
				fatal(err)
			}
		default:
			printGuildHelp()
		}

	case "watch":
		interval := 5
		for i, arg := range os.Args[2:] {
			if arg == "--interval" && i+1 < len(os.Args[2:]) {
				if n, err := strconv.Atoi(os.Args[3+i]); err == nil {
					interval = n
				}
			}
		}
		Watch(interval)

	default:
		printHelp()
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func printHelp() {
	fmt.Print(`kiromax - Kiro session manager

Commands:
  list                    List flat sessions
  swap                    Auto-swap to next flat session
  swap --guild            Manually advance to next guild
  use <id>                Force swap to specific session
  end <id>                Mark session as ended
  reset [<id>]            Unend flat sessions
  credits [<id>]          Show credit usage for a session
  login                   Log in to a new account and capture into a guild
  continue, c             Pick & resume a previous conversation

  capture <guild>         Snapshot current session into a guild (auto-numbered)
  guild list              List all guilds
  guild create <name>     Create a new guild
  guild add <g> <file>    Add a session file to a guild
  guild swap              Advance to next guild
  guild reset [<name>]    Unend sessions in guild(s)
  guild credits [<name>]  Show total credits for active/named guild

  watch [--interval N]    Poll credits every N min (default 5), auto-swap within guild
`)
}

func printGuildHelp() {
	fmt.Print(`kiromax guild - Guild management

  guild list              List all guilds with session status
  guild create <name>     Create a new guild directory
  guild add <g> <file>    Copy a session sqlite3 into a guild
  guild swap              Manually advance to next guild
  guild reset [<name>]    Unend all sessions in guild(s)
  guild credits [<name>]  Show credit totals for active/named guild
`)
}
