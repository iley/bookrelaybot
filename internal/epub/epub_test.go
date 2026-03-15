package epub

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReadMetadata(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "moby_dick.epub")

	meta, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if meta.Title != "Moby Dick; Or, The Whale" {
		t.Errorf("title = %q, want %q", meta.Title, "Moby Dick; Or, The Whale")
	}
	if meta.Author != "Herman Melville" {
		t.Errorf("author = %q, want %q", meta.Author, "Herman Melville")
	}
}

func TestReadMetadataNotAZip(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "moby_dick.epub")
	// Use the test file itself (Go source) as a non-ZIP input.
	_, err := ReadMetadata("epub_test.go")
	if err == nil {
		t.Fatal("expected error for non-zip file")
	}
	// Sanity: the real file should still work.
	_, err = ReadMetadata(path)
	if err != nil {
		t.Fatalf("real file failed: %v", err)
	}
}

func TestReadMetadataFileNotFound(t *testing.T) {
	_, err := ReadMetadata("/nonexistent/path.epub")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestWriteMetadataRoundtrip(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "moby_dick.epub")

	// Copy to a temp file.
	tmp, err := os.CreateTemp(t.TempDir(), "*.epub")
	if err != nil {
		t.Fatal(err)
	}
	srcFile, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(tmp, srcFile); err != nil {
		t.Fatal(err)
	}
	srcFile.Close()
	tmp.Close()

	newMeta := Metadata{
		Title:  "New Title",
		Author: "New Author",
	}
	if err := WriteMetadata(tmp.Name(), newMeta); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}

	got, err := ReadMetadata(tmp.Name())
	if err != nil {
		t.Fatalf("ReadMetadata after write: %v", err)
	}
	if got.Title != newMeta.Title {
		t.Errorf("title = %q, want %q", got.Title, newMeta.Title)
	}
	if got.Author != newMeta.Author {
		t.Errorf("author = %q, want %q", got.Author, newMeta.Author)
	}
}
