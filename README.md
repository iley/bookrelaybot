# BookRelayBot

A Telegram bot that forwards ebooks to Amazon's Send-to-Kindle email service.

Users send an ebook file (EPUB, FB2, MOBI, AZW3) via Telegram DM. The bot converts non-EPUB formats to EPUB using Calibre, extracts metadata (title, author), lets the user review and edit it, then emails the file to their Kindle address via SMTP.

## Configuration

### Environment variables

| Variable | Required | Description |
|---|---|---|
| `BOOKRELAYBOT_TOKEN` | Yes | Telegram bot token |
| `BOOKRELAY_SMTP_USER` | Yes | SMTP username |
| `BOOKRELAY_SMTP_PASSWORD` | Yes | SMTP password |

### Command-line flags

| Flag | Default | Description |
|---|---|---|
| `--dir` | *(temporary directory)* | Directory for book file storage (removed on shutdown if not set) |
| `--settings` | `settings.json` | Path to the settings file |
| `--smtp-host` | *(required)* | SMTP server hostname |
| `--smtp-port` | `587` | SMTP server port |
| `--smtp-from` | *(required)* | Sender email address |
| `--allowlist` | *(empty)* | Comma-separated list of allowed Telegram usernames |

## State

- `settings.json` (or the path given by `--settings`) — per-user settings (Kindle email addresses)
- Subdirectories under `--dir` — temporary storage for books being processed

## Running with Docker

```bash
docker run -d \
  -e BOOKRELAYBOT_TOKEN=123456:ABC-DEF \
  -e BOOKRELAY_SMTP_USER=user@example.com \
  -e BOOKRELAY_SMTP_PASSWORD=secret \
  -v /path/to/data:/data \
  bookrelaybot \
  --settings /data/settings.json \
  --dir /data/books \
  --smtp-host smtp.example.com \
  --smtp-from user@example.com \
  --allowlist alice,bob
```

The mounted `/path/to/data` directory will contain `settings.json` and the `books/` subdirectory for in-progress book files.
