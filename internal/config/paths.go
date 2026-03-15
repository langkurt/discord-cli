package config

import (
	"os"
	"path/filepath"
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

func DBPath(store string) string {
	return filepath.Join(store, "data.db")
}

func EnsureStore(store string) error {
	return os.MkdirAll(store, 0700)
}
