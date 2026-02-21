package epub

import "archive/zip"

// EPUB represents a parsed EPUB file.
type EPUB struct {
	Path    string
	ZipFile *zip.ReadCloser
	Files   map[string]*zip.File // path -> zip.File

	// Parsed from container.xml
	RootfilePath string

	// Parsed from OPF
	Package *Package

	// Raw OPF parse info (set during ParseOPF)
	OPFParseError    error
	HasMetadata      bool
	HasManifest      bool
	HasSpine         bool
}

// Package represents the OPF package document.
type Package struct {
	UniqueIdentifier string
	Version          string
	Metadata         Metadata
	Manifest         []ManifestItem
	Spine            []SpineItemref
	SpineToc         string // EPUB 2 spine toc attribute
	RenditionLayout  string // "pre-paginated" or "reflowable"
}

// Metadata holds the OPF metadata section.
type Metadata struct {
	Titles      []string
	Identifiers []DCIdentifier
	Languages   []string
	Modified    string // dcterms:modified value
}

// DCIdentifier is a dc:identifier element with optional id attribute.
type DCIdentifier struct {
	ID    string
	Value string
}

// ManifestItem represents a single item in the OPF manifest.
type ManifestItem struct {
	ID         string
	Href       string
	MediaType  string
	Properties string
	Fallback   string
	HasID      bool // false when id attribute is missing
}

// SpineItemref represents a single itemref in the OPF spine.
type SpineItemref struct {
	IDRef string
}
