package converter

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

const ebookConvert = "ebook-convert"

// SupportedFormats lists non-EPUB extensions that can be converted.
var SupportedFormats = map[string]bool{
	".fb2":  true,
	".mobi": true,
}

// NeedsConversion returns true if the extension requires conversion to EPUB.
func NeedsConversion(ext string) bool {
	return SupportedFormats[ext]
}

// IsSupportedFormat returns true if the extension is EPUB, a convertible format, or ZIP.
func IsSupportedFormat(ext string) bool {
	return ext == ".epub" || ext == ".zip" || SupportedFormats[ext]
}

// CheckAvailable verifies that ebook-convert is on PATH.
func CheckAvailable() error {
	_, err := exec.LookPath(ebookConvert)
	if err != nil {
		return fmt.Errorf("ebook-convert not found in PATH (is Calibre installed?): %w", err)
	}
	return nil
}

// ConvertToEPUB shells out to Calibre's ebook-convert to produce an EPUB.
// Returns the path to the output EPUB file.
// On failure, the returned error contains only the last line of Calibre's output;
// the full output is included via wrapping for logging.
func ConvertToEPUB(inputPath, outputDir string) (string, error) {
	epubPath := filepath.Join(outputDir, "converted.epub")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, ebookConvert, inputPath, epubPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", &ConvertError{Err: err, Output: string(output)}
	}

	return epubPath, nil
}

// ConvertError wraps a conversion failure with the full Calibre output.
type ConvertError struct {
	Err    error
	Output string
}

func (e *ConvertError) Error() string {
	return fmt.Sprintf("%v: %s", e.Err, e.Output)
}

func (e *ConvertError) Unwrap() error {
	return e.Err
}

// UserMessage returns a short summary suitable for sending to the user.
func (e *ConvertError) UserMessage() string {
	// Use the last non-empty line of Calibre output as it's typically the most informative.
	last := lastLine(e.Output)
	if last == "" {
		return e.Err.Error()
	}
	const maxLen = 200
	if len(last) > maxLen {
		last = last[:maxLen] + "..."
	}
	return last
}

func lastLine(s string) string {
	end := len(s)
	// Trim trailing whitespace/newlines.
	for end > 0 && (s[end-1] == '\n' || s[end-1] == '\r' || s[end-1] == ' ') {
		end--
	}
	if end == 0 {
		return ""
	}
	start := end
	for start > 0 && s[start-1] != '\n' {
		start--
	}
	return s[start:end]
}
