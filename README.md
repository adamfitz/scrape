# scrape

A CLI tool for looking up manga, manhwa, light novels, and other media via the [MangaDex](https://mangadex.org) API. Results are cached in a local SQLite database for fast repeated lookups.

## Features

- **Local-first** — checks the local database before making any API calls
- **Fuzzy matching** — built-in similarity scoring (Levenshtein + token Jaccard) with 36 grammar expansion rules
- **Title normalization** — Unicode folding, grammar expansion, separator handling, noise removal
- **Batch processing** — process hundreds of titles from text or CSV files with progress display
- **Rate limiting** — respects MangaDex's 5 req/s limit (default 4 req/s with headroom)
- **Pure Go** — no CGO, no system SQLite dependency

## Installation

```bash
go install github.com/adamfitz/scrape@latest
```

Or build from source:

```bash
git clone https://github.com/adamfitz/scrape.git
cd scrape
go build -o scrape .
```

## Usage

### Single title lookup

```bash
scrape lookup "One Piece"
scrape lookup "鬼滅の刃"           # Japanese
scrape lookup "외모지상주의"       # Korean
scrape lookup "One Piece" --json  # JSON output
```

### Batch lookup

```bash
scrape batch titles.txt           # text file, one title per line
scrape batch titles.csv           # CSV file, first column is title
scrape batch titles.txt --json    # JSON output
scrape batch titles.txt --local   # skip API, local DB only
scrape batch titles.txt --diff    # show titles missing from DB
```

Example `titles.txt`:

```
One Piece
Naruto
# This is a comment
Bleach
鬼滅の刃
```

### Database maintenance

```bash
scrape maintenance vacuum         # compact the database
scrape maintenance check          # integrity check
scrape maintenance stats          # show record counts
scrape maintenance normalize      # normalize all titles and rebuild index
```

### Backup

```bash
scrape backup ~/backups/          # auto-timestamped filename
scrape backup ~/backups/scrape.db.gz
```

### Video game music (khinsider)

```bash
scrape khinsider --url https://downloads.khinsider.com/game-soundtracks/album/... --mp3
scrape khinsider --url https://downloads.khinsider.com/game-soundtracks/album/... --flac
```

## CLI Reference

```
scrape
├── lookup <title>     Look up a manga on MangaDex
│   --json             Output in JSON format
│   -v, --verbose      Show full details
│   -l, --local        Only check local database
│   --limit <n>        Max results to consider (default 5)
├── batch <file>       Look up multiple titles from a file
│   --json             Output in JSON format
│   -v, --verbose      Show full details
│   -l, --local        Only check local database
│   --limit <n>        Max results per title (default 5)
│   --show-unmatched   Show titles not added to the DB
│   --diff             Show input entries missing from DB (implies --local)
├── backup <dest>      Backup the database
├── maintenance        Database maintenance
│   ├── vacuum         Compact the database
│   ├── check          Integrity check
│   ├── stats          Show table record counts
│   └── normalize      Normalize all titles and rebuild index
├── khinsider          Download video game OST
│   --url <url>        Album URL
│   --mp3              Download as MP3
│   --flac             Download as FLAC
└── version            Print version
```

## Database

SQLite database stored at `~/.config/scrape/scrape.db`, created automatically on first use.

### Tables

| Table | Purpose |
|-------|---------|
| `manga` | Manga metadata (title, alt titles, author, status, cover) |
| `title_index` | Pre-computed normalized titles for O(log N) lookups |
| `observation` | Reading progress tracking |
| `bookmarks` | User bookmarks across all media types |
| `anime` | Anime metadata (future) |
| `lightnovel` | Light novel metadata (future) |
| `webnovel` | Web novel metadata (future) |
| `webtoons` | Webtoon metadata (future) |

### Title normalization

Titles are normalized for consistent matching:

1. **Unicode folding** — confusable characters → ASCII equivalents
2. **Grammar expansion** — "Dont" → "Do not", "Wont" → "Will not" (36 rules)
3. **Separator folding** — periods, underscores, dashes → spaces
4. **Noise removal** — edition markers, volume/chapter numbers, trailing years

This ensures "Akame.ga.KILL!" and "Akame ga KILL!" match the same record.

### Fuzzy matching

When exact matching fails, the system uses a combined score:

- **Token Jaccard** — word-level overlap (0.4 weight)
- **Levenshtein** — character-level edit distance (0.6 weight)

Threshold: 0.85 (single match auto-linked, multiple matches prompt for disambiguation).

## Development

```bash
go test ./...          # run all tests
go vet ./...           # static analysis
go build .             # build binary
```

### Project structure

```
scrape/
├── commands/          CLI commands (cobra)
├── internal/
│   ├── config/        Application paths
│   ├── database/      SQLite schema and queries
│   ├── mangadex/      MangaDex API client
│   ├── normalize/     Title normalization and fuzzy matching
│   ├── pipeline/      Orchestration layer (lookup, batch, ingest)
│   └── ratelimit/     Token bucket rate limiter
├── khinsider/         Video game music scraper
├── documentation/     Specs and architecture docs
└── version/           Build version
```

## License

See [LICENSE](LICENSE) for details.
