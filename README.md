# BookRelayBot

A Telegram bot that forwards EPUB ebooks to Amazon's Send-to-Kindle email service.

Users send an EPUB file via Telegram DM. The bot extracts metadata (title, author), lets the user review and edit it, then emails the file to their Kindle address via SMTP.

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
| `--dir` | `.` | Directory for state and temporary book storage |
| `--smtp-host` | *(required)* | SMTP server hostname |
| `--smtp-port` | `587` | SMTP server port |
| `--smtp-from` | *(required)* | Sender email address |
| `--allowlist` | *(empty)* | Comma-separated list of allowed Telegram usernames |

## State

All persistent state lives in the `--dir` directory:

- `settings.json` — per-user settings (Kindle email addresses)
- `book_*` subdirectories — temporary storage for books being processed (cleaned up on startup)

## Running with Docker

```bash
docker run -d \
  -e BOOKRELAYBOT_TOKEN=123456:ABC-DEF \
  -e BOOKRELAY_SMTP_USER=user@example.com \
  -e BOOKRELAY_SMTP_PASSWORD=secret \
  -v /path/to/data:/data \
  bookrelaybot \
  --dir /data \
  --smtp-host smtp.example.com \
  --smtp-from user@example.com \
  --allowlist alice,bob
```

The mounted `/path/to/data` directory will contain `settings.json` and any in-progress book files.
