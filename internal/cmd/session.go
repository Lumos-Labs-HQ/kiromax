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

	for _, s := range sessions {
		if s.Active {
			d, err := db.Open(s.File)
			if err != nil {
				config.Die("error:", err)
			}
			db.SetMeta(d, "ended", "true")
			d.Close()
			ui.Info(fmt.Sprintf("Saved & ended session %s %s", ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
			break
		}
	}

	sessions, err = session.List(config.KiroDataDir, config.DataDB)
	if err != nil {
		config.Die("error:", err)
	}

	for _, s := range sessions {
		if s.Ended || session.UsedThisMonth(s) {
			continue
		}
		if err := session.SwapTo(s, config.DataDB, config.KiroDataDir); err != nil {
			config.Die("error:", err)
		}
		ui.Success(fmt.Sprintf("Swapped to session %s %s — restart kiro-cli to apply",
			ui.Bold(s.FileName), ui.Dim("["+s.UUID[:8]+"]")))
		return
	}

	ui.Fail("All sessions are ended or already used this month.")
	fmt.Println(ui.Dim("  Run: kmax reset   to unend all sessions"))
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
