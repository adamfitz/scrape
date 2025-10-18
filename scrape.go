package main

import (
	"github.com/adamfitz/scrape/commands"
	"log"
	"os"

	"github.com/adamfitz/scrape/cf"
)

func init() {
	logFile, err := os.OpenFile("/var/log/scrape/scrape.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)

	chromeLocation, browserErr := cf.FindChromeExec()
	if browserErr != nil {
		log.Printf("chrome browser not found, %v", browserErr)
	}
	log.Printf("Found chrome binary at: %s", chromeLocation)
}

func main() {
	commands.Execute()
}
