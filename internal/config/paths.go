package config

import (
	"os"
	"path/filepath"
	"strings"
)

// StoreDir returns the default store path (~/.discocli).
// Can be overridden with --store flag.
func StoreDir(override string) string {
	if override != "" {
		return override
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".discocli")
}

func TokenPath(store string) string {
	return filepath.Join(store, "token")
}

// DefaultGuild returns the guild name or ID saved in <store>/guild, or "" if unset.
// Used when sync is run without --guild.
func DefaultGuild(store string) string {
	b, err := os.ReadFile(filepath.Join(store, "guild"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func DBPath(store string) string {
	return filepath.Join(store, "data.db")
}

func EnsureStore(store string) error {
	return os.MkdirAll(store, 0700)
}
