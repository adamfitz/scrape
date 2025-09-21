package main

import (
	"log"
	"os"
	"scrape/commands"
)

func init() {
	logFile, err := os.OpenFile("/var/log/scrape/scrape.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
}

func main() {
	commands.Execute()
}
