package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "scrape",
	Short: "A manga scraping tool",
	Long: `A command-line tool for scraping manga chapters from various websites.
Supports multiple manga sites with different download options.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Add all site-specific commands
	rootCmd.AddCommand(manhuausCmd)
	rootCmd.AddCommand(kunmangaCmd)
	rootCmd.AddCommand(xbatoCmd)
	rootCmd.AddCommand(iluimCmd)
	rootCmd.AddCommand(orvCmd)
	rootCmd.AddCommand(rizzfablesCmd)
	rootCmd.AddCommand(hlsCmd)
	rootCmd.AddCommand(mgekoCmd)
	rootCmd.AddCommand(cfotzCmd)
	rootCmd.AddCommand(stonescapeCmd)
	rootCmd.AddCommand(asuraCmd)
	rootCmd.AddCommand(ravenscansCmd)
}
