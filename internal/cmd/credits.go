package cmd

import (
	"fmt"

	"github.com/Lumos-Labs-HQ/kmax/internal/config"
	"github.com/Lumos-Labs-HQ/kmax/internal/credits"
	"github.com/Lumos-Labs-HQ/kmax/internal/session"
	"github.com/Lumos-Labs-HQ/kmax/internal/ui"
)

func Credits(arg string) {
	config.EnsureDataDir()
	var s session.Session
	if arg == "" {
		sessions, err := session.List(config.KiroDataDir, config.DataDB)
		if err != nil {
			config.Die("error:", err)
		}
		for _, ss := range sessions {
			if ss.Active {
				s = ss
				break
			}
		}
		if s.FileName == "" {
			config.Die("no active session; specify: kmax credits <id|name>")
		}
	} else {
		var err error
		s, err = session.Resolve(arg, config.KiroDataDir, config.DataDB)
		if err != nil {
			config.Die(err)
		}
	}
	fmt.Printf("\n  %s %s\n\n", ui.Bold("Credits for session"), ui.Cyan(s.FileName))
	if err := credits.Print(s, config.DataDB); err != nil {
		config.Die("error:", err)
	}
	fmt.Println()
}
