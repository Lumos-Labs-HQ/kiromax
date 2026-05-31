package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Lumos-Labs-HQ/kiromax/internal/config"
	"github.com/Lumos-Labs-HQ/kiromax/internal/ui"
)

func Login() {
	fmt.Print("Session name (e.g. company-1, work): ")
	var name string
	fmt.Scanln(&name)
	name = strings.TrimSpace(name)
	if name == "" {
		config.Die("session name cannot be empty")
	}

	config.EnsureDataDir()

	dest := fmt.Sprintf("%s/%s.sqlite3", config.KiroDataDir, name)
	if _, err := os.Stat(dest); err == nil {
		config.Die(fmt.Sprintf("session %q already exists", name))
	}

	tmp := config.DataDB + ".kiromax_tmp"
	if err := config.CopyFile(config.DataDB, tmp); err != nil {
		config.Die("failed to back up data.sqlite3:", err)
	}

	if err := os.Remove(config.DataDB); err != nil {
		config.Die("failed to clear data.sqlite3:", err)
	}

	fmt.Println()
	ui.Info("Starting kiro-cli login (Builder ID)...")
	fmt.Println()

	cmd := exec.Command("kiro-cli", "login", "--license", "free")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = config.CopyFile(tmp, config.DataDB)
		_ = os.Remove(tmp)
		config.Die("login failed:", err)
	}

	if err := config.CopyFile(config.DataDB, dest); err != nil {
		config.Die("error:", err)
	}
	if err := config.CopyFile(tmp, config.DataDB); err != nil {
		config.Die("error:", err)
	}
	_ = os.Remove(tmp)

	fmt.Println()
	ui.Success(fmt.Sprintf("Session %s saved — run 'kiromax use %s' to switch to it", ui.Bold(name), name))
}
