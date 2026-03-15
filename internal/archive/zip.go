package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/iley/bookrelaybot/internal/converter"
)

const maxExtractedSize = 200 << 20 // 200 MB

// ExtractBook opens a ZIP archive and extracts the single ebook file it contains.
// It returns the path to the extracted file, which is placed in outputDir.
// Returns an error if the ZIP contains no supported ebook files or more than one.
func ExtractBook(zipPath string, outputDir string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	var found *zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Skip macOS resource fork entries.
		if strings.HasPrefix(f.Name, "__MACOSX") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext == ".epub" || converter.SupportedFormats[ext] {
			if found != nil {
				return "", fmt.Errorf("zip contains multiple ebook files: %s and %s", found.Name, f.Name)
			}
			found = f
		}
	}

	if found == nil {
		return "", fmt.Errorf("no supported ebook file found in zip")
	}

	if found.UncompressedSize64 > maxExtractedSize {
		return "", fmt.Errorf("ebook file too large (%d bytes, limit %d)", found.UncompressedSize64, maxExtractedSize)
	}

	ext := strings.ToLower(filepath.Ext(found.Name))
	outPath := filepath.Join(outputDir, "extracted"+ext)

	rc, err := found.Open()
	if err != nil {
		return "", fmt.Errorf("open file in zip: %w", err)
	}
	defer rc.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create extracted file: %w", err)
	}

	n, err := io.Copy(out, io.LimitReader(rc, maxExtractedSize+1))
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(outPath)
		return "", fmt.Errorf("extract file: %w", err)
	}
	if n > maxExtractedSize {
		os.Remove(outPath)
		return "", fmt.Errorf("ebook file too large (exceeds %d byte limit)", maxExtractedSize)
	}

	return outPath, nil
}
