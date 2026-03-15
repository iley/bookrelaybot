# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

BookRelayBot is a Telegram bot that forwards ebooks to Amazon's Send-to-Kindle email service. Users send ebook files (EPUB, FB2, MOBI, AZW3) via Telegram DM; the bot converts non-EPUB formats to EPUB using Calibre, extracts/edits metadata, renames the file, and emails it to the user's Kindle address via SMTP.

## Build & Run

```bash
# Build
go build ./cmd/bookrelaybot

# Run (requires BOOKRELAYBOT_TOKEN env var)
BOOKRELAYBOT_TOKEN=<token> ./bookrelaybot --dir /tmp/books
```

No tests or linter configured yet.

After making changes, always run:
```bash
go vet ./...
go fmt ./...
```

## Architecture

Single-binary Go application. Entry point: `cmd/bookrelaybot/main.go`.

The bot uses long-polling (`GetUpdatesChan`) to receive Telegram updates and processes document messages by downloading and saving files locally. The `internal/` directory is reserved for future internal packages.

Key domain behavior (partially implemented): Kindle uses the **file name** as the displayed title and **EPUB metadata** (`<dc:creator>`) for the author. The bot must handle both renaming and metadata editing.

### Supported formats

- **EPUB** — passed through directly
- **FB2, MOBI, AZW3** — converted to EPUB via Calibre's `ebook-convert` CLI (`internal/converter` package)

### Dependencies

- **Calibre** — required at runtime for `ebook-convert` (installed in Docker image; for local dev: `brew install calibre` or system package manager)

## Configuration

- `BOOKRELAYBOT_TOKEN` — Telegram bot token (required)
- `BOOKRELAY_SMTP_USER` — SMTP username (required)
- `BOOKRELAY_SMTP_PASSWORD` — SMTP password (required)
- `--dir` flag — directory to save received book files (uses a temporary directory if not set)
- `--settings` flag — path to the settings file (defaults to `settings.json`)
- `--smtp-host` flag — SMTP server hostname (required)
- `--smtp-port` flag — SMTP server port (defaults to `587`)
- `--smtp-from` flag — sender email address (required)
