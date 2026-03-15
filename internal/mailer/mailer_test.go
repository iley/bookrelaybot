package mailer

import (
	"strings"
	"testing"
)

func TestBuildMessage(t *testing.T) {
	msg := buildMessage(
		"sender@example.com",
		"reader@kindle.com",
		"Test Book.epub",
		[]byte("fake epub content"),
	)
	s := string(msg)

	checks := []struct {
		name    string
		contain string
	}{
		{"from header", "From: sender@example.com"},
		{"to header", "To: reader@kindle.com"},
		{"subject header", "Subject:"},
		{"mime version", "MIME-Version: 1.0"},
		{"multipart boundary", "Content-Type: multipart/mixed"},
		{"text part", "text/plain"},
		{"attachment type", "application/epub+zip"},
		{"base64 encoding", "Content-Transfer-Encoding: base64"},
		{"filename", "Test Book.epub"},
		{"closing boundary", "--==BookRelayBotBoundary==--"},
	}

	for _, c := range checks {
		if !strings.Contains(s, c.contain) {
			t.Errorf("%s: message does not contain %q", c.name, c.contain)
		}
	}
}
