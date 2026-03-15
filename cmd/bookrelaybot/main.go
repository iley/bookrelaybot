package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/iley/bookrelaybot/internal/converter"
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
	bookDir    string
	epubPath   string
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
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
		),
	)
}

func sendMsg(bot *tgbotapi.BotAPI, msg tgbotapi.Chattable) {
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

func removeAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		log.Printf("Failed to remove %s: %v", path, err)
	}
}

func confirmationText(title, author string) string {
	return fmt.Sprintf("Title: %s\nAuthor: %s", title, author)
}

func newBookDir(parentDir string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	name := hex.EncodeToString(b[:])
	bookDir := filepath.Join(parentDir, name)
	if err := os.MkdirAll(bookDir, 0755); err != nil {
		return "", fmt.Errorf("create book directory %s: %w", bookDir, err)
	}
	return bookDir, nil
}

// downloadFile downloads a document into a new subdirectory, preserving its extension.
// Returns the book directory and the path to the saved file.
func downloadFile(bot *tgbotapi.BotAPI, doc *tgbotapi.Document, dir string, ext string) (bookDir string, savedPath string, err error) {
	fileURL, err := bot.GetFileDirectURL(doc.FileID)
	if err != nil {
		return "", "", fmt.Errorf("get file URL: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(fileURL)
	if err != nil {
		return "", "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download file: unexpected status %d", resp.StatusCode)
	}

	bookDir, err = newBookDir(dir)
	if err != nil {
		return "", "", err
	}

	savePath := filepath.Join(bookDir, "original"+ext)
	out, err := os.Create(savePath)
	if err != nil {
		removeAll(bookDir)
		return "", "", fmt.Errorf("create file %s: %w", savePath, err)
	}

	_, err = io.Copy(out, resp.Body)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		removeAll(bookDir)
		return "", "", fmt.Errorf("save file %s: %w", savePath, err)
	}

	log.Printf("Saved file to %s", savePath)
	return bookDir, savePath, nil
}

func cancelPendingConfirmation(bot *tgbotapi.BotAPI, confirmations map[int64]*pendingConfirmation, userID int64) {
	if pc, ok := confirmations[userID]; ok {
		removeAll(pc.bookDir)
		edit := tgbotapi.NewEditMessageText(pc.chatID, pc.messageID, "Cancelled (new file received).")
		sendMsg(bot, edit)
		delete(confirmations, userID)
	}
}

func processDocument(bot *tgbotapi.BotAPI, chatID int64, doc *tgbotapi.Document, dir string, confirmations map[int64]*pendingConfirmation, userID int64) {
	ext := strings.ToLower(filepath.Ext(doc.FileName))
	if !converter.IsSupportedFormat(ext) {
		reply := tgbotapi.NewMessage(chatID,
			"Unsupported file format. Supported formats: EPUB, FB2, MOBI.")
		sendMsg(bot, reply)
		return
	}

	cancelPendingConfirmation(bot, confirmations, userID)

	bookDir, downloadedPath, err := downloadFile(bot, doc, dir, ext)
	if err != nil {
		log.Printf("Failed to process document: %v", err)
		reply := tgbotapi.NewMessage(chatID, "Failed to download the file. Please try again.")
		sendMsg(bot, reply)
		return
	}

	epubPath := downloadedPath

	if converter.NeedsConversion(ext) {
		statusMsg := tgbotapi.NewMessage(chatID, "Converting to EPUB...")
		sendMsg(bot, statusMsg)

		epubPath, err = converter.ConvertToEPUB(downloadedPath, bookDir)
		if err != nil {
			log.Printf("Conversion failed for %s: %v", doc.FileName, err)
			removeAll(bookDir)
			userMsg := err.Error()
			if ce, ok := err.(*converter.ConvertError); ok {
				userMsg = ce.UserMessage()
			}
			reply := tgbotapi.NewMessage(chatID,
				fmt.Sprintf("Failed to convert the file to EPUB: %s", userMsg))
			sendMsg(bot, reply)
			return
		}
	}

	meta, err := epub.ReadMetadata(epubPath)
	if err != nil {
		log.Printf("Failed to read EPUB metadata from %s: %v", epubPath, err)
		removeAll(bookDir)
		reply := tgbotapi.NewMessage(chatID, "Failed to read EPUB metadata. Is the file a valid EPUB?")
		sendMsg(bot, reply)
		return
	}

	msg := tgbotapi.NewMessage(chatID, confirmationText(meta.Title, meta.Author))
	keyboard := confirmationKeyboard()
	msg.ReplyMarkup = keyboard
	sent, err := bot.Send(msg)
	if err != nil {
		log.Printf("Failed to send confirmation message: %v", err)
		removeAll(bookDir)
		return
	}

	confirmations[userID] = &pendingConfirmation{
		chatID:    chatID,
		bookDir:   bookDir,
		epubPath:  epubPath,
		title:     meta.Title,
		author:    meta.Author,
		messageID: sent.MessageID,
	}
}

func isValidEmail(s string) bool {
	parts := strings.Split(s, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
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
	dir := flag.String("dir", "", "directory to save received book files (temporary directory if not set)")
	settingsPath := flag.String("settings", "settings.json", "path to the settings file")
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

	bookDir := *dir
	var removeBooksOnShutdown bool
	if bookDir == "" {
		tmp, err := os.MkdirTemp("", "bookrelaybot-*")
		if err != nil {
			log.Fatalf("Failed to create temporary directory: %v", err)
		}
		bookDir = tmp
		removeBooksOnShutdown = true
		log.Printf("Using temporary book directory: %s", bookDir)
	} else {
		if err := os.MkdirAll(bookDir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", bookDir, err)
		}
	}

	if err := converter.CheckAvailable(); err != nil {
		log.Fatalf("Preflight check failed: %v", err)
	}

	if removeBooksOnShutdown {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			log.Printf("Shutting down, removing temporary directory %s", bookDir)
			os.RemoveAll(bookDir)
			os.Exit(0)
		}()
	}

	if dir := filepath.Dir(*settingsPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create settings directory %s: %v", dir, err)
		}
	}

	store, err := settings.NewStore(*settingsPath)
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
				if _, err := bot.Request(callback); err != nil {
					log.Printf("Failed to answer callback query: %v", err)
				}
				continue
			}

			switch update.CallbackQuery.Data {
			case "confirm":
				us, ok := store.GetSettings(strconv.FormatInt(userID, 10))
				if !ok {
					log.Printf("Settings missing for user %d at confirm time", userID)
					reply := tgbotapi.NewMessage(pc.chatID, "Your Kindle email is no longer configured. Please send your email address to set it up again.")
					sendMsg(bot, reply)
					removeAll(pc.bookDir)
					delete(confirmations, userID)
					pending[userID] = &pendingSetup{chatID: pc.chatID}
					break
				}

				newName := sanitizeFilename(pc.title+" - "+pc.author) + ".epub"
				sendPath := filepath.Join(pc.bookDir, newName)

				if err := copyFile(pc.epubPath, sendPath); err != nil {
					log.Printf("Failed to copy file: %v", err)
					sendMsg(bot, tgbotapi.NewMessage(pc.chatID, "Failed to prepare the book for sending. Please try again."))
					break
				}

				if err := epub.WriteMetadata(sendPath, epub.Metadata{Title: pc.title, Author: pc.author}); err != nil {
					log.Printf("Failed to write metadata: %v", err)
					sendMsg(bot, tgbotapi.NewMessage(pc.chatID, "Failed to update book metadata. Please try again."))
					break
				}

				log.Printf("Sending book %q (file: %s) to %s", pc.title, sendPath, us.KindleEmail)
				if err := m.SendBook(us.KindleEmail, sendPath); err != nil {
					log.Printf("Failed to send book %q to %s: %v", pc.title, us.KindleEmail, err)
					sendMsg(bot, tgbotapi.NewMessage(pc.chatID, "Failed to email the book. Please try again."))
					break
				}

				log.Printf("Successfully sent book %q (file: %s) to %s", pc.title, sendPath, us.KindleEmail)
				removeAll(pc.bookDir)
				sendMsg(bot, tgbotapi.NewMessage(pc.chatID, fmt.Sprintf("Sent %q to %s!", pc.title, us.KindleEmail)))
				delete(confirmations, userID)

			case "edit_title":
				pc.waitingFor = "title"
				sendMsg(bot, tgbotapi.NewMessage(pc.chatID, "Send me the new title."))

			case "edit_author":
				pc.waitingFor = "author"
				sendMsg(bot, tgbotapi.NewMessage(pc.chatID, "Send me the new author."))

			case "cancel":
				removeAll(pc.bookDir)
				sendMsg(bot, tgbotapi.NewEditMessageText(pc.chatID, pc.messageID, "Cancelled."))
				delete(confirmations, userID)
			}

			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
			if _, err := bot.Request(callback); err != nil {
				log.Printf("Failed to answer callback query: %v", err)
			}
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

			if text == "/cancel" {
				removeAll(pc.bookDir)
				sendMsg(bot, tgbotapi.NewEditMessageText(pc.chatID, pc.messageID, "Cancelled."))
				delete(confirmations, userID)
				continue
			}

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
			sendMsg(bot, edit)
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
						sendMsg(bot, tgbotapi.NewMessage(chatID, "Failed to save settings. Please try again."))
						continue
					}
					sendMsg(bot, tgbotapi.NewMessage(chatID, fmt.Sprintf("Saved! Kindle email set to %s", text)))
					if ps.document != nil {
						processDocument(bot, chatID, ps.document, bookDir, confirmations, userID)
					}
					delete(pending, userID)
				} else if update.Message.Document != nil {
					ps.document = update.Message.Document
					log.Printf("Received file %q from user %d (pending setup)", update.Message.Document.FileName, userID)
					sendMsg(bot, tgbotapi.NewMessage(chatID, "I still need your Send-to-Kindle email address first. Please send it as a text message."))
				} else {
					sendMsg(bot, tgbotapi.NewMessage(chatID, "That doesn't look like a valid email address. Please try again."))
				}
			} else {
				// First contact.
				ps := &pendingSetup{chatID: chatID}
				if update.Message.Document != nil {
					ps.document = update.Message.Document
					log.Printf("Received file %q from user %d (pending setup)", update.Message.Document.FileName, userID)
				}
				pending[userID] = ps
				sendMsg(bot, tgbotapi.NewMessage(chatID, "Welcome! Please send me your Send-to-Kindle email address."))
			}
			continue
		}

		// Normal path: user has settings.
		if update.Message.Document == nil {
			continue
		}

		doc := update.Message.Document
		log.Printf("Received file %q from user %d", doc.FileName, userID)
		processDocument(bot, chatID, doc, bookDir, confirmations, userID)
	}
}
