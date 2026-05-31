package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Lumos-Labs-HQ/kiromax/internal/credits"
	"github.com/Lumos-Labs-HQ/kiromax/internal/db"
	"github.com/Lumos-Labs-HQ/kiromax/internal/session"
	"github.com/Lumos-Labs-HQ/kiromax/internal/ui"
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
			ui.Info(fmt.Sprintf("Saved & ended session %s %s", ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
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
		ui.Success(fmt.Sprintf("Swapped to session %s %s — restart kiro-cli to apply",
			ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
		return nil
	}

	ui.Fail("All sessions are ended or already used this month.")
	fmt.Println(ui.Dim("  Run: kiromax reset   to unend all sessions"))
	return nil
}

func printHelp() {
	fmt.Println()
	fmt.Println(ui.Bold(ui.Cyan("kiromax")) + ui.Dim(" — Kiro session manager"))
	fmt.Println()
	fmt.Println(ui.Bold("COMMANDS"))
	rows := [][2]string{
		{"  list          ", "List all sessions with status"},
		{"  swap          ", "Auto-swap to next available session"},
		{"  use <id>      ", "Force swap to a specific session"},
		{"  end <id>      ", "Mark a session as ended"},
		{"  reset [<id>]  ", "Unend all sessions (or one), clearing used_at"},
		{"  credits [<id>]", "Show live credit usage (defaults to active)"},
		{"  continue, c   ", "Pick & resume a previous conversation (resume-picker)"},
	}
	for _, r := range rows {
		fmt.Println(ui.Cyan(r[0]) + ui.Dim(r[1]))
	}
	fmt.Println()
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
			fmt.Println(ui.Dim("No sessions found in " + kiroDataDir))
			return
		}

		const (
			wID     = 4
			wName   = 12
			wUUID   = 8
			wStatus = 8
		)

		fmt.Println()
		// header: plain text padded, then bolded as a whole line
		header := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s",
			wID, "ID", wName, "NAME", wUUID, "UUID", wStatus, "STATUS", "LAST USED")
		fmt.Println(ui.Bold(header))
		fmt.Println("  " + ui.Dim(strings.Repeat("─", 56)))

		for _, s := range sessions {
			status := "idle"
			if s.Active {
				status = "ACTIVE"
			} else if s.Ended {
				status = "ended"
			}

			used := "never"
			if !s.UsedAt.IsZero() {
				used = s.UsedAt.Format("2006-01-02 15:04")
			}

			// pad to fixed width before applying color
			namePad   := fmt.Sprintf("%-*s", wName, s.FileName)
			uuidPad   := fmt.Sprintf("%-*s", wUUID, s.UUID[:8])
			statusPad := fmt.Sprintf("%-*s", wStatus, status)
			usedPad   := used

			switch {
			case s.Active:
				namePad   = ui.Bold(ui.Green(namePad))
				uuidPad   = ui.Green(uuidPad)
				statusPad = ui.Green(statusPad)
				usedPad   = ui.Green(used)
			case s.Ended:
				namePad   = ui.Dim(namePad)
				uuidPad   = ui.Dim(uuidPad)
				statusPad = ui.Dim(statusPad)
				usedPad   = ui.Dim(used)
			}

			idStr := fmt.Sprintf("%-*d", wID, s.ID)
			if s.Active {
				idStr = ui.Green(idStr)
			} else if s.Ended {
				idStr = ui.Dim(idStr)
			}

			fmt.Printf("  %s  %s  %s  %s  %s\n",
				idStr, namePad, uuidPad, statusPad, usedPad)
		}
		fmt.Println()

	case "swap":
		fmt.Println()
		if err := autoSwap(); err != nil {
			die("error:", err)
		}
		fmt.Println()

	case "use":
		if len(os.Args) < 3 {
			die("usage: kiromax use <id>")
		}
		s, err := session.Resolve(os.Args[2], kiroDataDir, dataDB)
		if err != nil {
			die("error:", err)
		}
		fmt.Println()
		if err := session.SwapTo(s, dataDB, kiroDataDir); err != nil {
			die("error:", err)
		}
		ui.Success(fmt.Sprintf("Swapped to session %s %s — restart kiro-cli to apply",
			ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
		fmt.Println()

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
		fmt.Println()
		ui.Success(fmt.Sprintf("Session %s marked as ended", ui.Bold(s.FileName)))
		fmt.Println()

	case "reset":
		fmt.Println()
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
			ui.Success(fmt.Sprintf("Session %s is available again", ui.Bold(s.FileName)))
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
			ui.Success(fmt.Sprintf("All %d sessions unended — available for swap again", len(sessions)))
		}
		fmt.Println()

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
		fmt.Println()
		fmt.Printf("  %s %s\n\n", ui.Bold("Credits for session"), ui.Cyan(s.FileName))
		if err := credits.Print(s, dataDB); err != nil {
			die("error:", err)
		}
		fmt.Println()

	case "continue", "c":
		bin, err := exec.LookPath("kiro-cli-chat")
		if err != nil {
			die("kiro-cli-chat not found in PATH")
		}
		syscall.Exec(bin, []string{"kiro-cli-chat", "chat", "--resume-picker"}, os.Environ())

	default:
		printHelp()
	}
}
