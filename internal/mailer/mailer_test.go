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

func TestBuildMessageFilenameWithQuotes(t *testing.T) {
	msg := buildMessage(
		"sender@example.com",
		"reader@kindle.com",
		`Book "Title" - Author.epub`,
		[]byte("data"),
	)
	s := string(msg)

	// The raw unescaped quote must not appear in Content-Disposition.
	for _, line := range strings.Split(s, "\r\n") {
		if strings.HasPrefix(line, "Content-Disposition:") {
			if strings.Contains(line, `"Title"`) {
				t.Errorf("Content-Disposition contains unescaped quotes: %s", line)
			}
			break
		}
	}
}

func TestBuildMessageFilenameWithNewline(t *testing.T) {
	msg := buildMessage(
		"sender@example.com",
		"reader@kindle.com",
		"Book\r\nX-Injected: evil",
		[]byte("data"),
	)
	s := string(msg)

	// The CRLF must not survive into the output as an actual line break
	// that could be parsed as a separate header.
	for _, line := range strings.Split(s, "\r\n") {
		if strings.HasPrefix(line, "X-Injected:") {
			t.Error("newline in filename allowed header injection")
		}
	}
}

func TestBuildMessageFilenameNonASCII(t *testing.T) {
	msg := buildMessage(
		"sender@example.com",
		"reader@kindle.com",
		"Книга - Автор.epub",
		[]byte("data"),
	)
	s := string(msg)

	// mime.FormatMediaType should RFC 2231-encode the non-ASCII filename.
	for _, line := range strings.Split(s, "\r\n") {
		if strings.HasPrefix(line, "Content-Disposition:") {
			if strings.Contains(line, "Книга") {
				t.Errorf("Content-Disposition contains raw non-ASCII: %s", line)
			}
			if !strings.Contains(line, "utf-8") {
				t.Errorf("Content-Disposition missing utf-8 charset tag: %s", line)
			}
			break
		}
	}
}
