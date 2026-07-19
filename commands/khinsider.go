package commands

import (
	"fmt"
	"log"
	"os"

	"github.com/adamfitz/scrape/khinsider"
	"github.com/spf13/cobra"
)

var khinsiderCmd = &cobra.Command{
	Use:   "khinsider",
	Short: "Scrape albums from khinsider",
	Long:  `Download video game OST from khinsider website`,
	Run: func(cmd *cobra.Command, args []string) {
		url, _ := cmd.Flags().GetString("url")
		wantMp3, _ := cmd.Flags().GetBool("mp3")
		wantFlac, _ := cmd.Flags().GetBool("flac")

		if url == "" {
			fmt.Println("Error: --url flag is required")
			cmd.Usage()
			os.Exit(1)
		}
		if !wantMp3 && !wantFlac {
			fmt.Println("Error: --mp3 or --flac flag is required")
			cmd.Usage()
			os.Exit(1)
		}
		if wantMp3 && wantFlac {
			fmt.Println("Error: choose either --mp3 or --flac (not both)")
			cmd.Usage()
			os.Exit(1)
		}

		if wantMp3 {
			log.Printf("Starting MP3 scrape from khinsider: %s", url)
			khinsider.DownloadAlbum(url, "mp3")
		}
		if wantFlac {
			log.Printf("Starting FLAC scrape from khinsider: %s", url)
			khinsider.DownloadAlbum(url, "flac")
		}
	},
}

func init() {
	khinsiderCmd.Flags().String("url", "", "Album URL to scrape (required)")
	khinsiderCmd.Flags().Bool("mp3", false, "Download MP3 tracks")
	khinsiderCmd.Flags().Bool("flac", false, "Download FLAC tracks")
}
