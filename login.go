package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// cmdLogin runs kiro-cli login, saves the new session, then restores the original data.sqlite3.
func cmdLogin() error {
	fmt.Print("Guild name to capture into (leave blank to skip capture): ")
	var guildName string
	fmt.Scanln(&guildName)
	guildName = strings.TrimSpace(guildName)

	tmp := dataDB + ".kiromax_tmp"
	orig, err := os.ReadFile(dataDB)
	if err != nil {
		return fmt.Errorf("cannot read data.sqlite3: %w", err)
	}
	if err := os.WriteFile(tmp, orig, 0600); err != nil {
		return err
	}
	defer os.Remove(tmp)

	// clear data.sqlite3 so kiro-cli login starts fresh
	if err := os.Remove(dataDB); err != nil {
		return err
	}

	fmt.Println("\nStarting kiro-cli login...")
	cmd := exec.Command("kiro-cli", "login", "--license", "free")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if runErr := cmd.Run(); runErr != nil {
		// restore original on failure
		os.WriteFile(dataDB, orig, 0600)
		return fmt.Errorf("login failed: %w", runErr)
	}

	if guildName != "" {
		if err := cmdCapture(guildName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: capture failed: %v\n", err)
		}
	}

	// restore original session
	if err := os.WriteFile(dataDB, orig, 0600); err != nil {
		return err
	}
	fmt.Println("✓ Original session restored — run 'kiromax guild swap' or 'kiromax use' to switch")
	return nil
}
