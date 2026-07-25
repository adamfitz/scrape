package config

import (
	"os"
	"path/filepath"
)

const appName = "scrape"

// ConfigDir returns the path to the application config directory (~/.config/scrape/).
// It creates the directory if it does not exist.
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// DBPath returns the full path to the SQLite database file.
func DBPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName+".db"), nil
}

// LogPath returns the full path to the log file.
func LogPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName+".log"), nil
}

// EnsureConfigDir creates the config directory if it does not exist.
func EnsureConfigDir() error {
	_, err := ConfigDir()
	return err
}
