# cli-commands Specification

## Purpose

Define the CLI command structure, flags, and behavior for the scrape tool using cobra.

## Command Tree

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
│   └── normalize      Unicode-fold all titles (idempotent migration)
├── khinsider          Download video game OST
│   --url <url>        Album URL
│   --mp3              Download as MP3
│   --flac             Download as FLAC
└── version            Print version
```

## Global Flags

- `--help` — Display help for any command

## Output Format

### Default (minimal)

```
[db] One Piece | Ongoing | en
[api] Naruto | Ongoing | ja
[db] Unknown Manga — not found locally
```

Format: `[source] Title | Status | Language`

### Verbose (`-v`)

```
Source:     api
Title:      One Piece
Alt Titles:
  en:       One Piece
  ja:       ワンピース
Author:     Eiichiro Oda
Status:     Ongoing
Language:   en
MangaDex:   https://mangadex.org/title/...
```

### JSON (`--json`)

```json
{
  "query": "One Piece",
  "source": "api",
  "mangadex_id": "...",
  "title": "One Piece",
  "alt_titles": {"en": "One Piece", "ja": "ワンピース"},
  "status": "Ongoing",
  "original_language": "en",
  "url": "https://mangadex.org/title/..."
}
```

## Source Indicator

Every output line indicates where the result came from:
- `[db]` — local database
- `api` — MangaDex API (JSON field)

## English Title Resolution

The default output always displays the English title (`en` field) when available, regardless of the input language.

## Log file location

Logging writes to `~/.config/scrape/scrape.log`.
