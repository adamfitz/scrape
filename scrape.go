package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/adamfitz/scrape/commands"
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.SetOutput(os.Stderr)
		return
	}

	logDir := filepath.Join(home, ".config", "scrape")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.SetOutput(os.Stderr)
		return
	}

	logFile, err := os.OpenFile(
		filepath.Join(logDir, "scrape.log"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		log.SetOutput(os.Stderr)
		return
	}
	log.SetOutput(logFile)
}

func main() {
	commands.Execute()
}
