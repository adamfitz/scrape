package commands

import (
	"fmt"

	"github.com/adamfitz/scrape/internal/config"
	"github.com/adamfitz/scrape/internal/database"
	"github.com/spf13/cobra"
)

var maintenanceCmd = &cobra.Command{
	Use:   "maintenance",
	Short: "Database maintenance commands",
	Long:  `Perform database maintenance: vacuum, integrity check, and statistics.`,
}

var vacuumCmd = &cobra.Command{
	Use:   "vacuum",
	Short: "Compact the database",
	Long:  `Run VACUUM to compact the database and reduce file size.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := config.DBPath()
		if err != nil {
			return fmt.Errorf("database path: %w", err)
		}

		db, err := database.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		fmt.Println("Compacting database...")
		if err := db.Vacuum(); err != nil {
			return fmt.Errorf("vacuum: %w", err)
		}
		fmt.Println("Database compacted successfully.")
		return nil
	},
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Run integrity check",
	Long:  `Run PRAGMA integrity_check to verify database integrity.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := config.DBPath()
		if err != nil {
			return fmt.Errorf("database path: %w", err)
		}

		db, err := database.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		result, err := db.IntegrityCheck()
		if err != nil {
			return fmt.Errorf("integrity check: %w", err)
		}

		if result == "ok" {
			fmt.Println("Database integrity: OK")
		} else {
			fmt.Printf("Database integrity: %s\n", result)
		}
		return nil
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Display database statistics",
	Long:  `Show record counts for all tables and database file size.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := config.DBPath()
		if err != nil {
			return fmt.Errorf("database path: %w", err)
		}

		db, err := database.Open(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		stats, err := db.Stats()
		if err != nil {
			return fmt.Errorf("stats: %w", err)
		}

		fmt.Printf("Database: %s\n\n", db.Path())
		fmt.Println("Table             Records")
		fmt.Println("───────           ───────")
		for _, table := range []string{"manga", "observation", "bookmarks", "anime", "lightnovel", "webnovel", "webtoons"} {
			fmt.Printf("%-18s %d\n", table, stats[table])
		}

		return nil
	},
}

var normalizeCmd = &cobra.Command{
	Use:   "normalize",
	Short: "Normalize all titles in the database",
	Long:  `Re-normalise every title and alt_title across all media tables, folding confusable Unicode characters to ASCII. Safe to run repeatedly.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := openPipeline()
		if err != nil {
			return err
		}

		fmt.Println("Normalising titles...")
		stats, err := p.NormalizeAllTitles()
		if err != nil {
			return fmt.Errorf("normalize: %w", err)
		}
		fmt.Printf("Indexed %d record(s) (%d updated)\n", stats.Indexed, stats.Updated)
		return nil
	},
}

func init() {
	maintenanceCmd.AddCommand(vacuumCmd)
	maintenanceCmd.AddCommand(checkCmd)
	maintenanceCmd.AddCommand(statsCmd)
	maintenanceCmd.AddCommand(normalizeCmd)
}
