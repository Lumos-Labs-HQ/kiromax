package cmd

import (
	"fmt"

	"github.com/Lumos-Labs-HQ/kmax/internal/config"
	"github.com/Lumos-Labs-HQ/kmax/internal/db"
	"github.com/Lumos-Labs-HQ/kmax/internal/session"
	"github.com/Lumos-Labs-HQ/kmax/internal/ui"
)

func Swap() {
	config.EnsureDataDir()
	sessions, err := session.List(config.KiroDataDir, config.DataDB)
	if err != nil {
		config.Die("error:", err)
	}

	// Mark the current active session as ended in data.sqlite3 BEFORE SyncActiveBack
	// so the ended flag is preserved when the live file is copied back to s.File.
	for _, s := range sessions {
		if s.Active {
			d, err := db.Open(config.DataDB)
			if err != nil {
				config.Die("error:", err)
			}
			db.SetMeta(d, "ended", "true")
			d.Close()
			ui.Info(fmt.Sprintf("Saved & ended session %s %s", ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
			break
		}
	}

	// Re-list so ended flags are reflected.
	sessions, err = session.List(config.KiroDataDir, config.DataDB)
	if err != nil {
		config.Die("error:", err)
	}

	var available []session.Session
	for _, s := range sessions {
		if !s.Ended && !session.UsedThisMonth(s) {
			available = append(available, s)
		}
	}

	if len(available) == 0 {
		ui.Fail("All sessions are ended or already used this month.")
		fmt.Println(ui.Dim("  Run: kmax reset   to unend all sessions"))
		return
	}

	for _, s := range available {
		if err := session.SwapTo(s, config.DataDB, config.KiroDataDir); err != nil {
			config.Die("error:", err)
		}
		ui.Success(fmt.Sprintf("Swapped to session %s %s", ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
	}
	fmt.Println(ui.Dim("  Restart kiro-cli to apply"))
}

func Use(arg string) {
	config.EnsureDataDir()
	s, err := session.Resolve(arg, config.KiroDataDir, config.DataDB)
	if err != nil {
		config.Die("error:", err)
	}
	if err := session.SwapTo(s, config.DataDB, config.KiroDataDir); err != nil {
		config.Die("error:", err)
	}
	ui.Success(fmt.Sprintf("Swapped to session %s %s — restart kiro-cli to apply",
		ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
}

func End(arg string) {
	config.EnsureDataDir()
	s, err := session.Resolve(arg, config.KiroDataDir, config.DataDB)
	if err != nil {
		config.Die(err)
	}
	d, err := db.Open(s.File)
	if err != nil {
		config.Die("error:", err)
	}
	db.SetMeta(d, "ended", "true")
	d.Close()
	ui.Success(fmt.Sprintf("Session %s marked as ended", ui.Bold(s.FileName)))
}

func Reset(arg string) {
	config.EnsureDataDir()
	if arg != "" {
		s, err := session.Resolve(arg, config.KiroDataDir, config.DataDB)
		if err != nil {
			config.Die(err)
		}
		d, err := db.Open(s.File)
		if err != nil {
			config.Die("error:", err)
		}
		db.SetMeta(d, "ended", "false")
		db.SetMeta(d, "used_at", "")
		d.Close()
		ui.Success(fmt.Sprintf("Session %s is available again", ui.Bold(s.FileName)))
		return
	}
	sessions, err := session.List(config.KiroDataDir, config.DataDB)
	if err != nil {
		config.Die("error:", err)
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
