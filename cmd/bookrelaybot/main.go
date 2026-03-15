package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/iley/bookrelaybot/internal/epub"
	"github.com/iley/bookrelaybot/internal/mailer"
	"github.com/iley/bookrelaybot/internal/settings"
)

type pendingSetup struct {
	chatID   int64
	document *tgbotapi.Document
}

type pendingConfirmation struct {
	chatID     int64
	filePath   string
	title      string
	author     string
	messageID  int
	waitingFor string // "", "title", or "author"
}

func confirmationKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Looks good", "confirm"),
			tgbotapi.NewInlineKeyboardButtonData("Edit title", "edit_title"),
			tgbotapi.NewInlineKeyboardButtonData("Edit author", "edit_author"),
		),
	)
}

func confirmationText(title, author string) string {
	return fmt.Sprintf("Title: %s\nAuthor: %s", title, author)
}

func downloadFile(bot *tgbotapi.BotAPI, doc *tgbotapi.Document, dir string) (string, error) {
	fileURL, err := bot.GetFileDirectURL(doc.FileID)
	if err != nil {
		return "", fmt.Errorf("get file URL: %w", err)
	}

	resp, err := http.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download file: unexpected status %d", resp.StatusCode)
	}

	savePath := filepath.Join(dir, filepath.Base(doc.FileName))
	out, err := os.Create(savePath)
	if err != nil {
		return "", fmt.Errorf("create file %s: %w", savePath, err)
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return "", fmt.Errorf("save file %s: %w", savePath, err)
	}

	log.Printf("Saved file to %s", savePath)
	return savePath, nil
}

func processDocument(bot *tgbotapi.BotAPI, chatID int64, doc *tgbotapi.Document, dir string, confirmations map[int64]*pendingConfirmation, userID int64) {
	if !strings.HasSuffix(strings.ToLower(doc.FileName), ".epub") {
		reply := tgbotapi.NewMessage(chatID, "Only EPUB files are supported.")
		bot.Send(reply)
		return
	}

	savePath, err := downloadFile(bot, doc, dir)
	if err != nil {
		log.Printf("Failed to process document: %v", err)
		reply := tgbotapi.NewMessage(chatID, "Failed to download the file. Please try again.")
		bot.Send(reply)
		return
	}

	meta, err := epub.ReadMetadata(savePath)
	if err != nil {
		log.Printf("Failed to read EPUB metadata from %s: %v", savePath, err)
		reply := tgbotapi.NewMessage(chatID, "Failed to read EPUB metadata. Is the file a valid EPUB?")
		bot.Send(reply)
		return
	}

	msg := tgbotapi.NewMessage(chatID, confirmationText(meta.Title, meta.Author))
	keyboard := confirmationKeyboard()
	msg.ReplyMarkup = keyboard
	sent, err := bot.Send(msg)
	if err != nil {
		log.Printf("Failed to send confirmation message: %v", err)
		return
	}

	confirmations[userID] = &pendingConfirmation{
		chatID:    chatID,
		filePath:  savePath,
		title:     meta.Title,
		author:    meta.Author,
		messageID: sent.MessageID,
	}
}

func isValidEmail(s string) bool {
	parts := strings.Split(s, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".")
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20, // control characters including \n, \r, \t
			r == '/', r == '\\', r == ':', r == '"',
			r == '<', r == '>', r == '|', r == '?', r == '*',
			r == 0x7f, r == 0:
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func main() {
	dir := flag.String("dir", ".", "directory to save received files")
	allowlist := flag.String("allowlist", "", "comma-separated list of allowed Telegram usernames")
	smtpHost := flag.String("smtp-host", "", "SMTP server hostname")
	smtpPort := flag.Int("smtp-port", 587, "SMTP server port")
	smtpFrom := flag.String("smtp-from", "", "sender email address")
	flag.Parse()

	allowed := make(map[string]bool)
	if *allowlist != "" {
		for _, u := range strings.Split(*allowlist, ",") {
			u = strings.TrimSpace(u)
			u = strings.TrimPrefix(u, "@")
			if u != "" {
				allowed[strings.ToLower(u)] = true
			}
		}
	}

	token := os.Getenv("BOOKRELAYBOT_TOKEN")
	if token == "" {
		log.Fatal("BOOKRELAYBOT_TOKEN environment variable is required")
	}

	smtpUser := os.Getenv("BOOKRELAY_SMTP_USER")
	smtpPassword := os.Getenv("BOOKRELAY_SMTP_PASSWORD")

	if *smtpHost == "" || *smtpFrom == "" || smtpUser == "" || smtpPassword == "" {
		log.Fatal("SMTP configuration is required: --smtp-host, --smtp-from, BOOKRELAY_SMTP_USER, BOOKRELAY_SMTP_PASSWORD")
	}

	m := mailer.New(mailer.Config{
		Host:     *smtpHost,
		Port:     *smtpPort,
		From:     *smtpFrom,
		Username: smtpUser,
		Password: smtpPassword,
	})

	if err := os.MkdirAll(*dir, 0755); err != nil {
		log.Fatalf("Failed to create directory %s: %v", *dir, err)
	}

	store, err := settings.NewStore(filepath.Join(*dir, "settings.json"))
	if err != nil {
		log.Fatalf("Failed to load settings: %v", err)
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Printf("Authorized as @%s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	pending := make(map[int64]*pendingSetup)
	confirmations := make(map[int64]*pendingConfirmation)

	for update := range updates {
		// Handle callback queries (inline button presses).
		if update.CallbackQuery != nil {
			from := update.CallbackQuery.From
			if from == nil {
				continue
			}
			userID := from.ID
			pc, ok := confirmations[userID]
			if !ok {
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "No pending file.")
				bot.Request(callback)
				continue
			}

			switch update.CallbackQuery.Data {
			case "confirm":
				us, _ := store.GetSettings(strconv.FormatInt(userID, 10))

				if err := epub.WriteMetadata(pc.filePath, epub.Metadata{Title: pc.title, Author: pc.author}); err != nil {
					log.Printf("Failed to write metadata: %v", err)
					reply := tgbotapi.NewMessage(pc.chatID, "Failed to update book metadata. Please try again.")
					bot.Send(reply)
					break
				}

				tmpDir, err := os.MkdirTemp("", "bookrelaybot-*")
				if err != nil {
					log.Printf("Failed to create temp directory: %v", err)
					reply := tgbotapi.NewMessage(pc.chatID, "Failed to prepare the book for sending. Please try again.")
					bot.Send(reply)
					break
				}

				newName := sanitizeFilename(pc.title+" - "+pc.author) + ".epub"
				newPath := filepath.Join(tmpDir, newName)
				if err := os.Rename(pc.filePath, newPath); err != nil {
					log.Printf("Failed to rename file: %v", err)
					os.RemoveAll(tmpDir)
					reply := tgbotapi.NewMessage(pc.chatID, "Failed to rename file. Please try again.")
					bot.Send(reply)
					break
				}
				pc.filePath = newPath

				if err := m.SendBook(us.KindleEmail, newPath); err != nil {
					log.Printf("Failed to send book: %v", err)
					reply := tgbotapi.NewMessage(pc.chatID, "Failed to email the book. Please try again.")
					bot.Send(reply)
					break
				}

				os.RemoveAll(tmpDir)
				reply := tgbotapi.NewMessage(pc.chatID, fmt.Sprintf("Sent %q to %s!", pc.title, us.KindleEmail))
				bot.Send(reply)
				delete(confirmations, userID)

			case "edit_title":
				pc.waitingFor = "title"
				reply := tgbotapi.NewMessage(pc.chatID, "Send me the new title.")
				bot.Send(reply)

			case "edit_author":
				pc.waitingFor = "author"
				reply := tgbotapi.NewMessage(pc.chatID, "Send me the new author.")
				bot.Send(reply)
			}

			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
			bot.Request(callback)
			continue
		}

		if update.Message == nil {
			continue
		}

		from := update.Message.From
		if from == nil {
			continue
		}

		if len(allowed) > 0 && !allowed[strings.ToLower(from.UserName)] {
			log.Printf("Ignoring message from unauthorized user %q (ID %d)", from.UserName, from.ID)
			continue
		}

		chatID := update.Message.Chat.ID
		userID := from.ID
		userKey := strconv.FormatInt(userID, 10)

		// Handle text input for editing title/author.
		if pc, ok := confirmations[userID]; ok && pc.waitingFor != "" && update.Message.Text != "" {
			text := strings.TrimSpace(update.Message.Text)
			switch pc.waitingFor {
			case "title":
				pc.title = text
			case "author":
				pc.author = text
			}
			pc.waitingFor = ""

			// Update the confirmation message with new values.
			edit := tgbotapi.NewEditMessageText(pc.chatID, pc.messageID, confirmationText(pc.title, pc.author))
			keyboard := confirmationKeyboard()
			edit.ReplyMarkup = &keyboard
			bot.Send(edit)
			continue
		}

		_, hasSettings := store.GetSettings(userKey)
		if !hasSettings {
			ps, isPending := pending[userID]
			if isPending {
				// User is in setup: expecting an email reply.
				text := strings.TrimSpace(update.Message.Text)
				if text != "" && isValidEmail(text) {
					us := settings.UserSettings{KindleEmail: text}
					if err := store.SetSettings(userKey, us); err != nil {
						log.Printf("Failed to save settings for user %d: %v", userID, err)
						reply := tgbotapi.NewMessage(chatID, "Failed to save settings. Please try again.")
						bot.Send(reply)
						continue
					}
					reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("Saved! Kindle email set to %s", text))
					bot.Send(reply)
					if ps.document != nil {
						processDocument(bot, chatID, ps.document, *dir, confirmations, userID)
					}
					delete(pending, userID)
				} else if update.Message.Document != nil {
					ps.document = update.Message.Document
					log.Printf("Received file %q from user %d (pending setup)", update.Message.Document.FileName, userID)
					reply := tgbotapi.NewMessage(chatID, "I still need your Send-to-Kindle email address first. Please send it as a text message.")
					bot.Send(reply)
				} else {
					reply := tgbotapi.NewMessage(chatID, "That doesn't look like a valid email address. Please try again.")
					bot.Send(reply)
				}
			} else {
				// First contact.
				ps := &pendingSetup{chatID: chatID}
				if update.Message.Document != nil {
					ps.document = update.Message.Document
					log.Printf("Received file %q from user %d (pending setup)", update.Message.Document.FileName, userID)
				}
				pending[userID] = ps
				reply := tgbotapi.NewMessage(chatID, "Welcome! Please send me your Send-to-Kindle email address.")
				bot.Send(reply)
			}
			continue
		}

		// Normal path: user has settings.
		if update.Message.Document == nil {
			continue
		}

		doc := update.Message.Document
		log.Printf("Received file %q from user %d", doc.FileName, userID)
		processDocument(bot, chatID, doc, *dir, confirmations, userID)
	}
}
