package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/beevik/etree"
)

// Metadata holds the title and author extracted from an EPUB file.
type Metadata struct {
	Title  string
	Author string
}

// container represents META-INF/container.xml.
type container struct {
	Rootfiles []rootfile `xml:"rootfiles>rootfile"`
}

type rootfile struct {
	FullPath string `xml:"full-path,attr"`
}

// opfPackage represents the OPF package document.
type opfPackage struct {
	Metadata opfMetadata `xml:"metadata"`
}

type opfMetadata struct {
	Title   string `xml:"title"`
	Creator string `xml:"creator"`
}

// ReadMetadata extracts title and author from an EPUB file.
func ReadMetadata(filepath string) (Metadata, error) {
	r, err := zip.OpenReader(filepath)
	if err != nil {
		return Metadata{}, fmt.Errorf("open epub: %w", err)
	}
	defer r.Close()

	// Find the OPF file path from container.xml.
	opfPath, err := findOPFPath(&r.Reader)
	if err != nil {
		return Metadata{}, err
	}

	// Parse the OPF file.
	for _, f := range r.File {
		if f.Name == opfPath {
			rc, err := f.Open()
			if err != nil {
				return Metadata{}, fmt.Errorf("open opf: %w", err)
			}
			defer rc.Close()

			var pkg opfPackage
			if err := xml.NewDecoder(rc).Decode(&pkg); err != nil {
				return Metadata{}, fmt.Errorf("parse opf: %w", err)
			}
			return Metadata{
				Title:  pkg.Metadata.Title,
				Author: pkg.Metadata.Creator,
			}, nil
		}
	}

	return Metadata{}, fmt.Errorf("opf file %q not found in archive", opfPath)
}

func findOPFPath(r *zip.Reader) (string, error) {
	for _, f := range r.File {
		if path.Clean(f.Name) == "META-INF/container.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("open container.xml: %w", err)
			}
			defer rc.Close()

			var c container
			if err := xml.NewDecoder(rc).Decode(&c); err != nil {
				return "", fmt.Errorf("parse container.xml: %w", err)
			}
			if len(c.Rootfiles) == 0 {
				return "", fmt.Errorf("no rootfile in container.xml")
			}
			return c.Rootfiles[0].FullPath, nil
		}
	}
	return "", fmt.Errorf("META-INF/container.xml not found")
}

// WriteMetadata updates the title and author in an EPUB file's OPF metadata.
func WriteMetadata(filepath string, meta Metadata) error {
	r, err := zip.OpenReader(filepath)
	if err != nil {
		return fmt.Errorf("open epub: %w", err)
	}
	defer r.Close()

	opfPath, err := findOPFPath(&r.Reader)
	if err != nil {
		return err
	}

	tmpPath := filepath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath) // clean up on error; no-op after successful rename
	}()

	w := zip.NewWriter(tmpFile)

	for _, f := range r.File {
		if f.Name == opfPath {
			// Parse and modify OPF with etree.
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open opf: %w", err)
			}
			doc := etree.NewDocument()
			if _, err := doc.ReadFrom(rc); err != nil {
				rc.Close()
				return fmt.Errorf("parse opf: %w", err)
			}
			rc.Close()

			metaEl := doc.FindElement("//metadata")
			if metaEl == nil {
				return fmt.Errorf("no <metadata> element found in OPF")
			}

			if el := doc.FindElement("//title"); el != nil {
				el.SetText(meta.Title)
			} else {
				titleEl := metaEl.CreateElement("dc:title")
				titleEl.SetText(meta.Title)
			}
			if el := doc.FindElement("//creator"); el != nil {
				el.SetText(meta.Author)
			} else {
				creatorEl := metaEl.CreateElement("dc:creator")
				creatorEl.SetText(meta.Author)
			}

			header := f.FileHeader
			fw, err := w.CreateHeader(&header)
			if err != nil {
				return fmt.Errorf("create opf entry: %w", err)
			}
			doc.Indent(2)
			if _, err := doc.WriteTo(fw); err != nil {
				return fmt.Errorf("write opf: %w", err)
			}
		} else if f.FileInfo().IsDir() {
			// Directory entries are created implicitly; skip them.
			continue
		} else {
			// Copy entry verbatim.
			raw, err := f.OpenRaw()
			if err != nil {
				return fmt.Errorf("open raw %s: %w", f.Name, err)
			}
			fw, err := w.CreateRaw(&f.FileHeader)
			if err != nil {
				return fmt.Errorf("create raw %s: %w", f.Name, err)
			}
			if _, err := io.Copy(fw, raw); err != nil {
				return fmt.Errorf("copy raw %s: %w", f.Name, err)
			}
		}
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	return os.Rename(tmpPath, filepath)
}
