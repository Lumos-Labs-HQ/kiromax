package cmd

import (
	"fmt"
	"strings"

	"github.com/Lumos-Labs-HQ/kiromax/internal/config"
	"github.com/Lumos-Labs-HQ/kiromax/internal/session"
	"github.com/Lumos-Labs-HQ/kiromax/internal/ui"
)

func List() {
	config.EnsureDataDir()
	sessions, err := session.List(config.KiroDataDir, config.DataDB)
	if err != nil {
		config.Die("error:", err)
	}
	if len(sessions) == 0 {
		fmt.Println(ui.Dim("No sessions found in " + config.KiroDataDir))
		return
	}

	const (
		wID     = 4
		wName   = 12
		wUUID   = 8
		wStatus = 8
	)

	fmt.Println()
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

		namePad   := fmt.Sprintf("%-*s", wName, s.FileName)
		uuidPad   := fmt.Sprintf("%-*s", wUUID, s.UUID[:8])
		statusPad := fmt.Sprintf("%-*s", wStatus, status)
		usedPad   := used
		idStr     := fmt.Sprintf("%-*d", wID, s.ID)

		switch {
		case s.Active:
			idStr     = ui.Green(idStr)
			namePad   = ui.Bold(ui.Green(namePad))
			uuidPad   = ui.Green(uuidPad)
			statusPad = ui.Green(statusPad)
			usedPad   = ui.Green(used)
		case s.Ended:
			idStr     = ui.Dim(idStr)
			namePad   = ui.Dim(namePad)
			uuidPad   = ui.Dim(uuidPad)
			statusPad = ui.Dim(statusPad)
			usedPad   = ui.Dim(used)
		}

		fmt.Printf("  %s  %s  %s  %s  %s\n", idStr, namePad, uuidPad, statusPad, usedPad)
	}
	fmt.Println()
}
