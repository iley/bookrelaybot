package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"path"
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
	opfPath, err := findOPFPath(r)
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

func findOPFPath(r *zip.ReadCloser) (string, error) {
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
