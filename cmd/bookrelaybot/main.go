package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/iley/bookrelaybot/internal/epub"
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

	savePath := filepath.Join(dir, doc.FileName)
	out, err := os.Create(savePath)
	if err != nil {
		resp.Body.Close()
		return "", fmt.Errorf("create file %s: %w", savePath, err)
	}

	_, err = io.Copy(out, resp.Body)
	resp.Body.Close()
	out.Close()
	if err != nil {
		return "", fmt.Errorf("save file %s: %w", savePath, err)
	}

	log.Printf("Saved file to %s", savePath)
	return savePath, nil
}

func processDocument(bot *tgbotapi.BotAPI, chatID int64, doc *tgbotapi.Document, dir string, confirmations map[string]*pendingConfirmation, username string) {
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

	confirmations[username] = &pendingConfirmation{
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

func main() {
	dir := flag.String("dir", ".", "directory to save received files")
	allowlist := flag.String("allowlist", "", "comma-separated list of allowed Telegram usernames")
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

	pending := make(map[string]*pendingSetup)
	confirmations := make(map[string]*pendingConfirmation)

	for update := range updates {
		// Handle callback queries (inline button presses).
		if update.CallbackQuery != nil {
			from := update.CallbackQuery.From
			if from == nil {
				continue
			}
			username := strings.ToLower(from.UserName)
			pc, ok := confirmations[username]
			if !ok {
				callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "No pending file.")
				bot.Request(callback)
				continue
			}

			switch update.CallbackQuery.Data {
			case "confirm":
				reply := tgbotapi.NewMessage(pc.chatID, fmt.Sprintf("Sending file with title %q author %q", pc.title, pc.author))
				bot.Send(reply)
				delete(confirmations, username)

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
		if len(allowed) > 0 && (from == nil || !allowed[strings.ToLower(from.UserName)]) {
			username := ""
			if from != nil {
				username = from.UserName
			}
			log.Printf("Ignoring message from unauthorized user %q", username)
			continue
		}

		chatID := update.Message.Chat.ID
		username := strings.ToLower(from.UserName)

		// Handle text input for editing title/author.
		if pc, ok := confirmations[username]; ok && pc.waitingFor != "" && update.Message.Text != "" {
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

		_, hasSettings := store.GetSettings(username)
		if !hasSettings {
			ps, isPending := pending[username]
			if isPending {
				// User is in setup: expecting an email reply.
				text := strings.TrimSpace(update.Message.Text)
				if text != "" && isValidEmail(text) {
					us := settings.UserSettings{KindleEmail: text}
					if err := store.SetSettings(username, us); err != nil {
						log.Printf("Failed to save settings for %s: %v", username, err)
						reply := tgbotapi.NewMessage(chatID, "Failed to save settings. Please try again.")
						bot.Send(reply)
						continue
					}
					reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("Saved! Kindle email set to %s", text))
					bot.Send(reply)
					if ps.document != nil {
						processDocument(bot, chatID, ps.document, *dir, confirmations, username)
					}
					delete(pending, username)
				} else if update.Message.Document != nil {
					ps.document = update.Message.Document
					log.Printf("Received file %q from %s (pending setup)", update.Message.Document.FileName, from.UserName)
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
					log.Printf("Received file %q from %s (pending setup)", update.Message.Document.FileName, from.UserName)
				}
				pending[username] = ps
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
		log.Printf("Received file %q from %s", doc.FileName, from.UserName)
		processDocument(bot, chatID, doc, *dir, confirmations, username)
	}
}
