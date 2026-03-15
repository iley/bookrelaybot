package mailer

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
)

// Config holds SMTP connection settings.
type Config struct {
	Host     string
	Port     int
	From     string
	Username string
	Password string
}

// Mailer sends emails via SMTP.
type Mailer struct {
	cfg Config
}

// New creates a Mailer with the given config.
func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// SendBook emails an EPUB file as an attachment.
func (m *Mailer) SendBook(to, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	msg := buildMessage(m.cfg.From, to, filepath.Base(filePath), data)

	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	if err := smtp.SendMail(addr, auth, m.cfg.From, []string{to}, msg); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}

func buildMessage(from, to, filename string, fileData []byte) []byte {
	boundary := "==BookRelayBotBoundary=="

	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", filename) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n")
	b.WriteString("\r\n")

	// Text part.
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString("Book delivery from BookRelayBot.\r\n")
	b.WriteString("\r\n")

	// Attachment part.
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: application/epub+zip\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + filename + "\"\r\n")
	b.WriteString("\r\n")

	encoded := base64.StdEncoding.EncodeToString(fileData)
	// Wrap at 76 characters per RFC 2045.
	for i := 0; i < len(encoded); i += 76 {
		end := i + 76
		if end > len(encoded) {
			end = len(encoded)
		}
		b.WriteString(encoded[i:end] + "\r\n")
	}

	b.WriteString("--" + boundary + "--\r\n")

	return []byte(b.String())
}
