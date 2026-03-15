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
)

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

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Printf("Authorized as @%s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

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

		if update.Message.Document == nil {
			continue
		}

		doc := update.Message.Document
		log.Printf("Received file %q from %s", doc.FileName, from.UserName)

		fileURL, err := bot.GetFileDirectURL(doc.FileID)
		if err != nil {
			log.Printf("Failed to get file URL: %v", err)
			continue
		}

		resp, err := http.Get(fileURL)
		if err != nil {
			log.Printf("Failed to download file: %v", err)
			continue
		}

		savePath := filepath.Join(*dir, doc.FileName)
		out, err := os.Create(savePath)
		if err != nil {
			resp.Body.Close()
			log.Printf("Failed to create file %s: %v", savePath, err)
			continue
		}

		_, err = io.Copy(out, resp.Body)
		resp.Body.Close()
		out.Close()
		if err != nil {
			log.Printf("Failed to save file %s: %v", savePath, err)
			continue
		}

		log.Printf("Saved file to %s", savePath)
		reply := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("File %s received", doc.FileName))
		bot.Send(reply)
	}
}
