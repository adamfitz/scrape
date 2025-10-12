package commands

import (
	"fmt"

	"github.com/adamfitz/scrape/version"

	"github.com/spf13/cobra"
)

var versionNum = version.Version

// RavensPick me up infinite gatchaans command
var versionNumber = &cobra.Command{
	Use:   "version",
	Short: "scrape version",
	Long:  `Display scrpae version number`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\n", versionNum)
	},
}
