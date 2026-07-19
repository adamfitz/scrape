package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "scrape",
	Short: "A manga and media lookup tool",
	Long: `A command-line tool for looking up manga, manhwa, light novels,
and other media via the MangaDex API. Results are cached in a local
SQLite database for fast repeated lookups.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(lookupCmd)
	rootCmd.AddCommand(batchCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(maintenanceCmd)
	rootCmd.AddCommand(khinsiderCmd)
	rootCmd.AddCommand(versionNumber)
}
