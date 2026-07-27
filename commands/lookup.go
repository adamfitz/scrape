package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/adamfitz/scrape/internal/config"
	"github.com/adamfitz/scrape/internal/database"
	"github.com/adamfitz/scrape/internal/pipeline"
	"github.com/spf13/cobra"
)

var lookupCmd = &cobra.Command{
	Use:   "lookup <title>",
	Short: "Look up a manga by name on MangaDex",
	Long: `Look up a manga title on MangaDex. Checks the local database first,
then queries the MangaDex API if not found locally. Successful lookups are
automatically stored in the local database. Output defaults to the English
title; use -v for full details.`,
	Args: cobra.ExactArgs(1),
	RunE: runLookup,
}

var lookupJSON bool
var lookupLimit int
var lookupVerbose bool
var lookupLocal bool

func init() {
	lookupCmd.Flags().BoolVar(&lookupJSON, "json", false, "Output results in JSON format")
	lookupCmd.Flags().BoolVarP(&lookupVerbose, "verbose", "v", false, "Show full details (alt titles, author, cover, description)")
	lookupCmd.Flags().BoolVarP(&lookupLocal, "local", "l", false, "Only check the local database, skip the API")
	lookupCmd.Flags().IntVar(&lookupLimit, "limit", 5, "Max MangaDex results to consider")
}

type lookupOutput struct {
	Query       string                `json:"query"`
	Source      string                `json:"source"`
	SourceID    string                `json:"source_id"`
	Title       string                `json:"title"`
	AltTitles   pipeline.AltTitleData `json:"alt_titles,omitempty"`
	Author      string                `json:"author,omitempty"`
	Description string                `json:"description,omitempty"`
	CoverURL    string                `json:"cover_url,omitempty"`
	URL         string                `json:"url"`
	Status      string                `json:"status"`
	SourceLang  string                `json:"original_language"`
	Error       string                `json:"error,omitempty"`
}

func runLookup(cmd *cobra.Command, args []string) error {
	query := args[0]

	p, err := openPipeline()
	if err != nil {
		return err
	}

	result, err := p.LookupAPI(database.MediaTypeManga, query, pipeline.LookupOptions{
		LocalOnly: lookupLocal,
		Limit:     lookupLimit,
	})
	if err != nil {
		return err
	}

	if len(result.Candidates) > 0 {
		result, err = handleDisambiguation(p, result, query)
		if err != nil {
			return err
		}
	}

	out := toLookupOutput(result, query)
	return printLookupOutput(out, lookupJSON, lookupVerbose)
}

func handleDisambiguation(p *pipeline.Pipeline, result *pipeline.LookupResult, query string) (*pipeline.LookupResult, error) {
	if lookupJSON {
		c := result.Candidates[0]
		return &pipeline.LookupResult{
			Source: result.Source,
			Query:  query,
			Media: &database.Media{
				Type:        database.MediaTypeManga,
				Title:       c.Title,
				SourceID:    c.SourceID,
				URL:         c.URL,
				Status:      c.Status,
				Description: c.DescEn,
			},
		}, nil
	}

	if result.Source == pipeline.SourceFuzzy {
		fmt.Printf("\nMultiple local matches (fuzzy):\n\n")
	} else {
		fmt.Printf("\nMultiple matches found on MangaDex:\n\n")
	}
	for i, c := range result.Candidates {
		fmt.Printf("  [%d] %s (%s) — %s\n", i+1, c.Title, c.Language, c.SourceID)
	}
	if result.Source == pipeline.SourceFuzzy {
		fmt.Printf("  [0] Skip\n\n")
	} else {
		fmt.Printf("  [0] Skip (don't add)\n\n")
	}
	fmt.Print("Pick a number: ")

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	choice, err := strconv.Atoi(line)
	if err != nil || choice < 0 || choice > len(result.Candidates) {
		return &pipeline.LookupResult{
			Source: result.Source,
			Query:  query,
			Error:  "invalid selection",
		}, nil
	}
	if choice == 0 {
		return &pipeline.LookupResult{
			Source: result.Source,
			Query:  query,
			Error:  "skipped by user",
		}, nil
	}

	c := result.Candidates[choice-1]

	if result.Source == pipeline.SourceFuzzy {
		return &pipeline.LookupResult{
			Media: &database.Media{
				Type:        database.MediaTypeManga,
				Title:       c.Title,
				SourceID:    c.SourceID,
				URL:         c.URL,
				Status:      c.Status,
				Description: c.DescEn,
			},
			Source: result.Source,
			Query:  query,
		}, nil
	}

	media, err := p.IngestCandidate(database.MediaTypeManga, c)
	if err != nil {
		return nil, fmt.Errorf("ingest: %w", err)
	}

	return &pipeline.LookupResult{
		Media:  media,
		Source: result.Source,
		Query:  query,
	}, nil
}

func toLookupOutput(result *pipeline.LookupResult, query string) lookupOutput {
	out := lookupOutput{
		Query:  query,
		Source: string(result.Source),
		Error:  result.Error,
	}

	if result.Media != nil {
		out.SourceID = result.Media.SourceID
		out.Title = result.Media.Title
		out.AltTitles = pipeline.ParseAltTitles(result.Media.AltTitle)
		out.Author = result.Media.Author
		out.Description = result.Media.Description
		out.CoverURL = result.Media.CoverURL
		out.URL = result.Media.URL
		out.SourceLang = result.Media.Language
		out.Status = result.Media.Status
	}

	return out
}

func printLookupOutput(out lookupOutput, jsonOutput bool, verbose bool) error {
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	if out.Error != "" {
		fmt.Printf("[%s] %s — %s\n", out.Source, out.Query, out.Error)
		return nil
	}

	if verbose {
		return printVerbose(out)
	}

	fmt.Printf("[%s] %s\n", out.Source, out.Title)
	return nil
}

func printVerbose(out lookupOutput) error {
	fmt.Printf("Source:     %s\n", out.Source)
	fmt.Printf("Title:      %s\n", out.Title)
	if len(out.AltTitles.Primary) > 0 || len(out.AltTitles.Alts) > 0 {
		fmt.Println("Alt Titles:")
		for lang, t := range out.AltTitles.Primary {
			fmt.Printf("  %-8s %s\n", lang+":", t)
		}
		for _, alt := range out.AltTitles.Alts {
			for lang, t := range alt {
				fmt.Printf("  %-8s %s\n", lang+":", t)
			}
		}
	}
	if out.Author != "" {
		fmt.Printf("Author:     %s\n", out.Author)
	}
	if out.Status != "" {
		fmt.Printf("Status:     %s\n", out.Status)
	}
	if out.SourceLang != "" {
		fmt.Printf("Language:   %s\n", out.SourceLang)
	}
	if out.URL != "" {
		fmt.Printf("MangaDex:   %s\n", out.URL)
	}
	if out.CoverURL != "" {
		fmt.Printf("Cover:      %s\n", out.CoverURL)
	}
	if out.Description != "" {
		fmt.Printf("Description: %s\n", out.Description)
	}
	return nil
}

// openDB opens the SQLite database at the configured path.
func openDB() (*database.DB, error) {
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, fmt.Errorf("database path: %w", err)
	}
	db, err := database.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}

// openPipeline creates a Pipeline backed by the configured SQLite database.
func openPipeline() (*pipeline.Pipeline, error) {
	db, err := openDB()
	if err != nil {
		return nil, err
	}
	return pipeline.New(db, pipeline.Options{}), nil
}
