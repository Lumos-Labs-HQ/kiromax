package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Guild struct {
	Name     string
	Dir      string
	Sessions []Session
	Active   bool // contains the currently active session
	Ended    bool // all sessions ended
}

// listGuilds returns all guild subdirectories under kiroDataDir.
func listGuilds() ([]Guild, error) {
	entries, err := os.ReadDir(kiroDataDir)
	if err != nil {
		return nil, err
	}
	activeUUID := liveActiveUUID()
	var guilds []Guild
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		g, err := loadGuild(e.Name(), activeUUID)
		if err != nil {
			continue
		}
		guilds = append(guilds, g)
	}
	sort.Slice(guilds, func(i, j int) bool {
		return guilds[i].Name < guilds[j].Name
	})
	return guilds, nil
}

func loadGuild(name, activeUUID string) (Guild, error) {
	dir := filepath.Join(kiroDataDir, name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Guild{}, err
	}
	var sessions []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sqlite3") {
			continue
		}
		fname := strings.TrimSuffix(e.Name(), ".sqlite3")
		path := filepath.Join(dir, e.Name())
		s, err := loadSession(path, fname, name)
		if err != nil {
			continue
		}
		s.Active = activeUUID != "" && s.UUID == activeUUID
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

	active := false
	allEnded := len(sessions) > 0
	for _, s := range sessions {
		if s.Active {
			active = true
		}
		if !s.Ended {
			allEnded = false
		}
	}
	return Guild{Name: name, Dir: dir, Sessions: sessions, Active: active, Ended: allEnded}, nil
}

// activeGuild returns the guild that contains the currently active session.
func activeGuild() (*Guild, error) {
	guilds, err := listGuilds()
	if err != nil {
		return nil, err
	}
	for i := range guilds {
		if guilds[i].Active {
			return &guilds[i], nil
		}
	}
	return nil, nil
}

// guildAutoSwap swaps to the next non-ended session within the given guild.
// Returns true if a swap happened, false if guild is exhausted.
func guildAutoSwap(g *Guild) (bool, error) {
	// mark current active as ended
	for _, s := range g.Sessions {
		if s.Active {
			db, err := openDB(s.File)
			if err != nil {
				return false, err
			}
			setMeta(db, "ended", "true")
			db.Close()
			fmt.Printf("→ Marked [%s/%s] as ended\n", g.Name, s.FileName)
			break
		}
	}

	// reload
	updated, err := loadGuild(g.Name, liveActiveUUID())
	if err != nil {
		return false, err
	}

	for _, s := range updated.Sessions {
		if s.Ended {
			continue
		}
		if err := swapTo(s); err != nil {
			return false, err
		}
		fmt.Printf("✓ Swapped to [%s/%s] — restart kiro-cli to apply\n", g.Name, s.FileName)
		return true, nil
	}
	return false, nil
}

// cmdGuildList prints all guilds with session counts and credit summary.
func cmdGuildList() error {
	guilds, err := listGuilds()
	if err != nil {
		return err
	}
	if len(guilds) == 0 {
		fmt.Println("No guilds found. Create one with: kiromax guild create <name>")
		return nil
	}
	fmt.Printf("%-16s %-8s %-8s %-8s\n", "GUILD", "SESSIONS", "STATUS", "ACTIVE")
	fmt.Println(strings.Repeat("-", 44))
	for _, g := range guilds {
		status := "ok"
		if g.Ended {
			status = "EXHAUSTED"
		}
		active := ""
		if g.Active {
			active = "◀ ACTIVE"
		}
		fmt.Printf("%-16s %-8d %-8s %s\n", g.Name, len(g.Sessions), status, active)
		for _, s := range g.Sessions {
			marker := "  "
			if s.Active {
				marker = "▶ "
			}
			st := "idle"
			if s.Ended {
				st = "ended"
			} else if s.Active {
				st = "ACTIVE"
			}
			used := "never"
			if !s.UsedAt.IsZero() {
				used = s.UsedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("  %s%-6s %-8s used: %s\n", marker, s.FileName, st, used)
		}
	}
	return nil
}

// cmdGuildCreate creates a new guild directory.
func cmdGuildCreate(name string) error {
	dir := filepath.Join(kiroDataDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	fmt.Printf("✓ Guild %q created at %s\n", name, dir)
	fmt.Printf("  Add sessions with: kiromax guild add %s <session.sqlite3>\n", name)
	return nil
}

// cmdGuildAdd copies or moves a sqlite3 file into the guild directory.
func cmdGuildAdd(guildName, srcPath string) error {
	dir := filepath.Join(kiroDataDir, guildName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("guild %q does not exist; create it first", guildName)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	dest := filepath.Join(dir, filepath.Base(srcPath))
	if err := os.WriteFile(dest, data, 0600); err != nil {
		return err
	}
	fmt.Printf("✓ Added %s to guild %q\n", filepath.Base(srcPath), guildName)
	return nil
}

// cmdGuildSwap manually advances to the next guild.
func cmdGuildSwap() error {
	guilds, err := listGuilds()
	if err != nil {
		return err
	}
	if len(guilds) == 0 {
		return fmt.Errorf("no guilds found")
	}

	// find active guild index
	activeIdx := -1
	for i, g := range guilds {
		if g.Active {
			activeIdx = i
			break
		}
	}

	// mark all sessions in active guild as ended
	if activeIdx >= 0 {
		ag := guilds[activeIdx]
		for _, s := range ag.Sessions {
			db, err := openDB(s.File)
			if err != nil {
				continue
			}
			setMeta(db, "ended", "true")
			db.Close()
		}
		fmt.Printf("→ Guild %q marked as exhausted\n", ag.Name)
	}

	// find next guild with available sessions
	start := activeIdx + 1
	for i := 0; i < len(guilds); i++ {
		g := guilds[(start+i)%len(guilds)]
		for _, s := range g.Sessions {
			if !s.Ended {
				if err := swapTo(s); err != nil {
					return err
				}
				fmt.Printf("✓ Switched to guild %q, session %s — restart kiro-cli to apply\n", g.Name, s.FileName)
				return nil
			}
		}
	}

	fmt.Println("✗ All guilds are exhausted.")
	fmt.Println("  Run: kiromax guild reset   to unend all guilds")
	return nil
}

// cmdCapture snapshots the current data.sqlite3 into a guild, auto-numbering the file.
func cmdCapture(guildName string) error {
	dir := filepath.Join(kiroDataDir, guildName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// find next available number
	n := 1
	for {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%d.sqlite3", n))); os.IsNotExist(err) {
			break
		}
		n++
	}
	dest := filepath.Join(dir, fmt.Sprintf("%d.sqlite3", n))
	data, err := os.ReadFile(dataDB)
	if err != nil {
		return fmt.Errorf("cannot read data.sqlite3: %w", err)
	}
	if err := os.WriteFile(dest, data, 0600); err != nil {
		return err
	}
	fmt.Printf("✓ Captured account %d into guild %q (%s)\n", n, guildName, dest)
	return nil
}
func cmdGuildReset(guildName string) error {
	guilds, err := listGuilds()
	if err != nil {
		return err
	}
	for _, g := range guilds {
		if guildName != "" && g.Name != guildName {
			continue
		}
		for _, s := range g.Sessions {
			db, err := openDB(s.File)
			if err != nil {
				continue
			}
			setMeta(db, "ended", "false")
			db.Close()
		}
		fmt.Printf("✓ Guild %q unended\n", g.Name)
	}
	return nil
}

// cmdGuildCredits shows total credit usage across all sessions in the active guild.
func cmdGuildCredits(guildName string) error {
	var g *Guild
	if guildName == "" {
		ag, err := activeGuild()
		if err != nil {
			return err
		}
		if ag == nil {
			return fmt.Errorf("no active guild; specify: kiromax guild credits <name>")
		}
		g = ag
	} else {
		loaded, err := loadGuild(guildName, liveActiveUUID())
		if err != nil {
			return err
		}
		g = &loaded
	}

	fmt.Printf("Guild: %s (%d sessions)\n", g.Name, len(g.Sessions))
	fmt.Println(strings.Repeat("-", 44))

	var guildTotal float64
	for _, s := range g.Sessions {
		s = liveToken(s)
		if s.Token == "" {
			fmt.Printf("  %-8s no token\n", s.FileName)
			continue
		}
		if !s.TokenExpiresAt.IsZero() && time.Now().After(s.TokenExpiresAt) {
			fmt.Printf("  %-8s token expired\n", s.FileName)
			continue
		}
		result, err := callUsageLimits(s.Token)
		if err != nil {
			fmt.Printf("  %-8s offline\n", s.FileName)
			continue
		}
		used := totalUsed(result)
		guildTotal += used
		marker := " "
		if s.Active {
			marker = "▶"
		}
		fmt.Printf(" %s %-8s %.0f/50\n", marker, s.FileName, used)
	}
	fmt.Println(strings.Repeat("-", 44))
	fmt.Printf("  Total: %.0f / %d\n", guildTotal, len(g.Sessions)*50)
	return nil
}
