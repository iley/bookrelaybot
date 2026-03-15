# Book Relay Bot

A Telegram bot that forwards ebooks to Amazon's Send-to-Kindle email service.

## How It Works

1. The user sends an EPUB file to the bot via Telegram DM.
2. The bot extracts title and author from the EPUB metadata.
3. The bot prompts the user to confirm or edit the title and author.
4. If the user chooses to edit, the bot asks for the corrected title and author in separate messages.
5. The bot updates the file name and/or EPUB metadata as needed.
6. The bot sends the file to the user's configured Send-to-Kindle email address via SMTP.

## User Settings

- On first use, the bot prompts the user for their Kindle email address.
- Per-user settings are stored in a single JSON file.
- The bot is designed for direct message use only.

## Author and Title Handling

The bot ensures that the title and author metadata in the EPUB file are set correctly before sending to Kindle. This is important because Send-to-Kindle uses EPUB metadata to determine how the book appears on the device.

## Tech Stack

- **Language:** Go
- **Dependencies:** Telegram bot API, EPUB parsing, SMTP for email delivery
