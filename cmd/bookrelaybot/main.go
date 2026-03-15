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

	"github.com/iley/bookrelaybot/internal/settings"
)

type pendingSetup struct {
	chatID   int64
	document *tgbotapi.Document
}

func processDocument(bot *tgbotapi.BotAPI, chatID int64, doc *tgbotapi.Document, dir string) {
	fileURL, err := bot.GetFileDirectURL(doc.FileID)
	if err != nil {
		log.Printf("Failed to get file URL: %v", err)
		return
	}

	resp, err := http.Get(fileURL)
	if err != nil {
		log.Printf("Failed to download file: %v", err)
		return
	}

	savePath := filepath.Join(dir, doc.FileName)
	out, err := os.Create(savePath)
	if err != nil {
		resp.Body.Close()
		log.Printf("Failed to create file %s: %v", savePath, err)
		return
	}

	_, err = io.Copy(out, resp.Body)
	resp.Body.Close()
	out.Close()
	if err != nil {
		log.Printf("Failed to save file %s: %v", savePath, err)
		return
	}

	log.Printf("Saved file to %s", savePath)
	reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("File %s received", doc.FileName))
	bot.Send(reply)
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

	for update := range updates {
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
						processDocument(bot, chatID, ps.document, *dir)
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
		processDocument(bot, chatID, doc, *dir)
	}
}
