package commands

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/adamfitz/scrape/internal/config"
	"github.com/adamfitz/scrape/internal/database"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup <destination>",
	Short: "Backup the local database",
	Long: `Create a compressed backup of the local SQLite database.
If the destination is a directory, a timestamped filename is generated.
The backup uses VACUUM INTO for a consistent, corruption-free copy.`,
	Args: cobra.ExactArgs(1),
	RunE: runBackup,
}

func runBackup(cmd *cobra.Command, args []string) error {
	dest := args[0]

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("database path: %w", err)
	}

	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// If dest is a directory, generate timestamped filename
	info, err := os.Stat(dest)
	if err == nil && info.IsDir() {
		ts := time.Now().Format("2006-01-02-150405")
		dest = filepath.Join(dest, fmt.Sprintf("scrape-%s.db", ts))
	}

	// Create temp file for the raw backup
	tmpFile := dest + ".tmp"
	if err := db.Backup(tmpFile); err != nil {
		return fmt.Errorf("backup database: %w", err)
	}

	// Compress with gzip
	gzDest := dest + ".gz"
	if err := gzipFile(tmpFile, gzDest); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("compress backup: %w", err)
	}

	os.Remove(tmpFile)

	// Show result
	info, _ = os.Stat(gzDest)
	fmt.Printf("Backup created: %s", gzDest)
	if info != nil {
		fmt.Printf(" (%s)", formatSize(info.Size()))
	}
	fmt.Println()

	return nil
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	_, err = io.Copy(gz, in)
	return err
}

func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
