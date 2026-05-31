package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func KiroBase() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("cannot determine home directory: " + err.Error())
	}
	return filepath.Join(home, ".local", "share", "kiro-cli")
}

var (
	DataDB      = filepath.Join(KiroBase(), "data.sqlite3")
	KiroDataDir = filepath.Join(KiroBase(), "kiro_data")
)

// EnsureDataDir creates kiro_data if it doesn't exist.
func EnsureDataDir() {
	if err := os.MkdirAll(KiroDataDir, 0700); err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot create kiro_data dir:", err)
		os.Exit(1)
	}
}

func Die(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(1)
}

func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}
