package main

import (
	"fmt"
	"time"
)

const creditLimitPerAccount = 50.0

// Watch polls the active session's credits every interval.
// Auto-swaps accounts within the guild when one hits 50.
// Guild swap is always manual: kiromax guild swap
func Watch(intervalMin int) {
	fmt.Printf("👁  Watching credits every %d min (%.0f/account)...\n", intervalMin, creditLimitPerAccount)
	ticker := time.NewTicker(time.Duration(intervalMin) * time.Minute)
	defer ticker.Stop()
	checkAndSwap()
	for range ticker.C {
		checkAndSwap()
	}
}

func checkAndSwap() {
	activeUUID := liveActiveUUID()
	if activeUUID == "" {
		fmt.Println("[watch] no active session")
		return
	}

	guilds, _ := listGuilds()
	for i := range guilds {
		for _, s := range guilds[i].Sessions {
			if s.UUID != activeUUID {
				continue
			}
			g := &guilds[i]
			s = liveToken(s)
			if s.Token == "" {
				fmt.Printf("[watch] %s/%s: no token\n", g.Name, s.FileName)
				return
			}
			result, err := callUsageLimits(s.Token)
			if err != nil {
				fmt.Printf("[watch] %s/%s: offline (%v)\n", g.Name, s.FileName, err)
				return
			}
			accountUsed := totalUsed(result)

			// guild total: ended accounts = 50 each, active = live value, unused = 0
			guildTotal := accountUsed
			for _, gs := range g.Sessions {
				if gs.UUID != s.UUID && gs.Ended {
					guildTotal += creditLimitPerAccount
				}
			}
			guildLimit := float64(len(g.Sessions)) * creditLimitPerAccount

			fmt.Printf("[watch] %s/%s: %.1f/%.0f | guild: %.0f/%.0f\n",
				g.Name, s.FileName, accountUsed, creditLimitPerAccount, guildTotal, guildLimit)

			if accountUsed >= creditLimitPerAccount {
				fmt.Printf("[watch] ⚡ Account limit reached — swapping within guild %q\n", g.Name)
				swapped, err := guildAutoSwap(g)
				if err != nil {
					fmt.Printf("[watch] swap error: %v\n", err)
					return
				}
				if !swapped {
					fmt.Printf("[watch] ✗ Guild %q exhausted (%.0f/%.0f) — run: kiromax guild swap\n", g.Name, guildTotal, guildLimit)
				}
			}
			return
		}
	}

	// fallback: flat session
	sessions, _ := listSessions()
	for _, s := range sessions {
		if s.UUID != activeUUID {
			continue
		}
		s = liveToken(s)
		if s.Token == "" {
			fmt.Printf("[watch] %s: no token\n", s.FileName)
			return
		}
		result, err := callUsageLimits(s.Token)
		if err != nil {
			fmt.Printf("[watch] %s: offline\n", s.FileName)
			return
		}
		used := totalUsed(result)
		fmt.Printf("[watch] %s: %.1f/%.0f credits used\n", s.FileName, used, creditLimitPerAccount)
		if used >= creditLimitPerAccount {
			fmt.Println("[watch] ⚡ Credit limit reached — run: kiromax swap")
		}
		return
	}

	fmt.Println("[watch] active session not found in any guild or flat list")
}
