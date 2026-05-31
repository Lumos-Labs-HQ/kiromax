package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/Lumos-Labs-HQ/kiromax/internal/cmd"
	"github.com/Lumos-Labs-HQ/kiromax/internal/ui"
)

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
		{"  login         ", "Log in to a new account and save it as a session"},
		{"  continue, c   ", "Pick & resume a previous conversation"},
	}
	for _, r := range rows {
		fmt.Println(ui.Cyan(r[0]) + ui.Dim(r[1]))
	}
	fmt.Println()
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	switch os.Args[1] {
	case "list":
		cmd.List()
	case "swap":
		fmt.Println()
		cmd.Swap()
		fmt.Println()
	case "use":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax use <id>")
			os.Exit(1)
		}
		fmt.Println()
		cmd.Use(os.Args[2])
		fmt.Println()
	case "end":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: kiromax end <id|name>")
			os.Exit(1)
		}
		fmt.Println()
		cmd.End(os.Args[2])
		fmt.Println()
	case "reset":
		arg := ""
		if len(os.Args) >= 3 {
			arg = os.Args[2]
		}
		fmt.Println()
		cmd.Reset(arg)
		fmt.Println()
	case "credits":
		arg := ""
		if len(os.Args) >= 3 {
			arg = os.Args[2]
		}
		cmd.Credits(arg)
	case "login":
		fmt.Println()
		cmd.Login()
		fmt.Println()
	case "continue", "c":
		bin, _ := exec.LookPath("kiro-cli-chat")
		syscall.Exec(bin, []string{"kiro-cli-chat", "chat", "--resume-picker"}, os.Environ())
	default:
		printHelp()
	}
}
