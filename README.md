# scrape

A CLI tool for looking up manga, manhwa, light novels, and other media via the [MangaDex](https://mangadex.org) API. Results are cached in a local SQLite database for fast repeated lookups.

## Features

- **Single title lookup** — query MangaDex by name, get multi-language title variants (English, Japanese, Japanese Hepburn, Korean, Chinese)
- **Batch lookup** — process hundreds of titles from text or CSV files
- **Local-first** — checks the local database before making any API calls; only missing titles hit the API
- **Fuzzy matching** — optional Sorensen-Dice similarity for approximate title matching (typo tolerance)
- **Rate limiting** — respects MangaDex's 5 req/s limit (default 4 req/s with headroom)
- **Pure Go** — no CGO, no system SQLite dependency, cross-compiles to Linux, Windows, and macOS
- **Extensible schema** — manga, anime, light novels, web novels, webtoons, bookmarks, and reading observations

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

### Look up a single manga

```bash
scrape lookup "One Piece"
scrape lookup "鬼滅の刃"           # Japanese
scrape lookup "외모지상주의"       # Korean
scrape lookup "One Piece" --json  # JSON output
scrape lookup "Akame ga KILL" --fuzzy  # handle typos like "Akame ga KIL"
```

### Batch lookup from a file

Text file (one title per line, `#` comments and blank lines ignored):

```bash
scrape batch titles.txt
scrape batch titles.txt --fuzzy  # fuzzy match handles typos in input
```

CSV file (first column treated as title, header row skipped):

```bash
scrape batch titles.csv
```

With JSON output:

```bash
scrape batch titles.txt --json
```

Example `titles.txt`:

```
One Piece
Naruto
# This is a comment
Bleach
鬼滅の刃
외모지상주의
```

### Backup the database

```bash
scrape backup ~/backups/scrape.db.gz        # specific file
scrape backup ~/backups/                     # auto-timestamped filename
```

### Database maintenance

```bash
scrape maintenance vacuum   # compact the database
scrape maintenance check    # run integrity check
scrape maintenance stats    # show record counts per table
```

### Video game music (khinsider)

```bash
scrape khinsider --url https://downloads.khinsider.com/game-soundtracks/album/... --mp3
scrape khinsider --url https://downloads.khinsider.com/game-soundtracks/album/... --flac
```

### Version

```bash
scrape version
```

## Database

The SQLite database is stored at `~/.config/scrape/scrape.db` (or the platform-equivalent config directory). It is created automatically on first use.

### Tables

| Table | Purpose |
|-------|---------|
| `manga` | Manga metadata from MangaDex (title, alt titles, author, status, cover) |
| `observation` | Reading progress tracking (chapter, volume, episode) |
| `bookmarks` | User bookmarks across all media types |
| `anime` | Anime metadata (future) |
| `lightnovel` | Light novel metadata (future) |
| `webnovel` | Web novel metadata (future) |
| `webtoons` | Webtoon metadata (future) |

### Language Support

MangaDex returns titles in multiple languages. The tool extracts and stores:

| Code | Language |
|------|----------|
| `en` | English |
| `ja` | Japanese (kanji/kana) |
| `ja-ro` | Japanese (Hepburn/romaji) |
| `ko` | Korean |
| `zh` | Chinese (simplified) |
| `zh-ro` | Chinese (romanized) |

## Configuration

All configuration lives in `~/.config/scrape/`:

```
~/.config/scrape/
├── scrape.db      # SQLite database
└── scrape.log     # Application log
```

## CLI Reference

```
scrape
├── lookup <title>     Look up a manga on MangaDex
│   --json             Output in JSON format
│   -v, --verbose      Show full details
│   -l, --local        Only check local database
│   --limit <n>        Max results to consider (default 5)
│   --fuzzy [threshold] Enable fuzzy matching for typos (default 0.7)
├── batch <file>       Look up multiple titles from a file
│   --json             Output in JSON format
│   -v, --verbose      Show full details
│   -l, --local        Only check local database
│   --limit <n>        Max results per title (default 5)
│   --fuzzy [threshold] Enable fuzzy matching for typos (default 0.7)
├── backup <dest>      Backup the database
├── maintenance        Database maintenance
│   ├── vacuum         Compact the database
│   ├── check          Integrity check
│   └── stats          Show table record counts
├── khinsider          Download video game OST
│   --url <url>        Album URL
│   --mp3              Download as MP3
│   --flac             Download as FLAC
└── version            Print version
```

## Development

```bash
go test ./...          # run all tests
go vet ./...           # static analysis
go build .             # build binary
```

### Project Structure

```
scrape/
├── commands/          CLI commands (cobra)
├── internal/
│   ├── config/        Application paths and settings
│   ├── database/      SQLite schema and queries
│   ├── fuzzy/         Sorensen-Dice fuzzy string matching
│   ├── mangadex/      MangaDex API client
│   ├── normalize/     Title normalization
│   └── ratelimit/     Token bucket rate limiter
├── khinsider/         Video game music scraper
├── openspec/specs/    OpenSpec capability specifications
└── version/           Build version
```

### Shared Library Policy

All reusable functions live in `internal/` packages. No package may duplicate logic from a shared package. See `openspec/specs/shared-library/spec.md` for the full policy.

## Acceptable Use

This tool complies with the [MangaDex API acceptable use policy](https://api.mangadex.org/docs/2-limitations/):

- Rate limited to 4 req/s (below the 5 req/s limit)
- Sends a proper `User-Agent` header
- No proxy or `Via` headers
- No ads or paid services wrapping the API

## License

See [LICENSE](LICENSE) for details.
