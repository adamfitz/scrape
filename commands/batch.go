package commands

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adamfitz/scrape/internal/database"
	"github.com/adamfitz/scrape/internal/pipeline"
	"github.com/spf13/cobra"
)

var batchCmd = &cobra.Command{
	Use:   "batch <file>",
	Short: "Look up multiple manga titles from a file",
	Long: `Process multiple manga titles from a text or CSV file.
Text files: one title per line (empty lines and # comments ignored).
CSV files: first column treated as title (header row skipped).
All titles are checked in the local database before any API calls are made.
Output defaults to English titles; use -v for full details.`,
	Args: cobra.ExactArgs(1),
	RunE: runBatch,
}

var batchJSON bool
var batchLimit int
var batchVerbose bool
var batchLocal bool
var batchShowUnmatched bool
var batchDiff bool

func init() {
	batchCmd.Flags().BoolVar(&batchJSON, "json", false, "Output results in JSON format")
	batchCmd.Flags().BoolVarP(&batchVerbose, "verbose", "v", false, "Show full details (alt titles, author, cover, description)")
	batchCmd.Flags().BoolVarP(&batchLocal, "local", "l", false, "Only check the local database, skip the API")
	batchCmd.Flags().IntVar(&batchLimit, "limit", 5, "Max MangaDex results per title")
	batchCmd.Flags().BoolVar(&batchShowUnmatched, "show-unmatched", false, "Show titles not added to the DB (API rejected or not found)")
	batchCmd.Flags().BoolVar(&batchDiff, "diff", false, "Show input file entries missing from the database (implies --local)")
}

func runBatch(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	titles, err := parseInputFile(filePath)
	if err != nil {
		return fmt.Errorf("parse input: %w", err)
	}

	if len(titles) == 0 {
		fmt.Println("No titles found in input file.")
		return nil
	}

	unique := pipeline.Deduplicate(titles)
	fmt.Printf("Processing %d unique titles...\n\n", len(unique))

	p, err := openPipeline()
	if err != nil {
		return err
	}

	// --diff mode: local-only, show missing
	if batchDiff {
		found, _ := p.BatchLookup(database.MediaTypeManga, unique)
		count := 0
		for _, title := range unique {
			if _, ok := found[title]; !ok {
				fmt.Println(title)
				count++
			}
		}
		fmt.Printf("\n%d titles missing from database\n", count)
		return nil
	}

	// Stream results from pipeline
	results := p.BatchLookupStream(database.MediaTypeManga, unique, pipeline.BatchLookupOptions{
		LocalOnly: batchLocal,
		Limit:     batchLimit,
	})

	var allResults []lookupOutput
	cachedCount := 0
	fuzzyFound := 0
	fuzzyMultiple := 0
	apiFound := 0
	apiNotFound := 0
	i := 0

	for result := range results {
		i++
		if !batchJSON {
			if result.Error != "" {
				fmt.Printf("[%d/%d] [%s] %s — %s\n", i, len(unique), result.Source, result.Query, result.Error)
			} else if result.Media != nil {
				fmt.Printf("[%d/%d] [%s] %s\n", i, len(unique), result.Source, result.Media.Title)
			} else {
				fmt.Printf("[%d/%d] [%s] [fuzzy multiple] %s\n", i, len(unique), result.Source, result.Query)
			}
		}

		out := toLookupOutput(&result, result.Query)
		allResults = append(allResults, out)

		if result.Source == pipeline.SourceDB {
			cachedCount++
		} else if result.Source == pipeline.SourceFuzzy {
			if result.Media != nil {
				fuzzyFound++
			} else {
				fuzzyMultiple++
			}
		} else if result.Source == pipeline.SourceAPI {
			if result.Error != "" {
				apiNotFound++
			} else {
				apiFound++
			}
		}
	}

	// JSON output
	if batchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(allResults)
	}

	// Summary
	notFound := apiNotFound
	if batchLocal {
		notFound = len(unique) - cachedCount - fuzzyFound - fuzzyMultiple
	}

	totalFound := cachedCount + fuzzyFound + apiFound

	fmt.Printf("\n---\n%d processed — %d found (DB: %d, Fuzzy: %d, API: %d), %d ambiguous, %d not found\n",
		len(unique), totalFound, cachedCount, fuzzyFound, apiFound, fuzzyMultiple, notFound)

	if fuzzyMultiple > 0 {
		fmt.Printf("  → Ambiguous: run `scrape lookup \"<title>\"` for each to disambiguate\n")
	}
	if notFound > 0 {
		fmt.Printf("  → Not found: not on MangaDex — check manually or skip\n")
	}

	if batchShowUnmatched {
		type unmatchedGroup struct {
			count  int
			titles []string
		}
		groups := make(map[string]*unmatchedGroup)

		addUnmatched := func(title, reason string) {
			g, ok := groups[reason]
			if !ok {
				g = &unmatchedGroup{}
				groups[reason] = g
			}
			g.count++
			g.titles = append(g.titles, title)
		}

		for _, r := range allResults {
			if r.Error != "" {
				addUnmatched(r.Query, r.Error)
			}
		}

		if len(groups) > 0 {
			total := 0
			for _, g := range groups {
				total += g.count
			}
			fmt.Printf("\n--- Not added to database (%d):\n", total)
			for reason, g := range groups {
				for _, title := range g.titles {
					fmt.Printf("  %s\n", title)
				}
				fmt.Printf("  (%d %s)\n\n", g.count, reason)
			}
			fmt.Println("Use `scrape lookup \"<title>\"` to find and pick the correct match.")
		}
	}

	return nil
}

// parseInputFile reads a .txt or .csv file and returns a list of titles.
func parseInputFile(path string) ([]string, error) {
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".txt":
		return parseTextFile(path)
	case ".csv":
		return parseCSVFile(path)
	default:
		return nil, fmt.Errorf("unsupported file format %q (use .txt or .csv)", ext)
	}
}

// parseTextFile reads a line-per-title text file, skipping blanks and # comments.
func parseTextFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var titles []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		titles = append(titles, line)
	}
	return titles, scanner.Err()
}

// parseCSVFile reads the first column of a CSV file as titles, skipping the header row.
func parseCSVFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}

	var titles []string
	for i, record := range records {
		if i == 0 {
			continue
		}
		if len(record) > 0 {
			title := strings.TrimSpace(record[0])
			if title != "" {
				titles = append(titles, title)
			}
		}
	}
	return titles, nil
}
