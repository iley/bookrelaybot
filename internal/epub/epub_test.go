package epub

import (
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
