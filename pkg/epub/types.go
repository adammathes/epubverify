package epub

import "archive/zip"

// EPUB represents a parsed EPUB file.
type EPUB struct {
	Path    string
	ZipFile *zip.ReadCloser
	Files   map[string]*zip.File // path -> zip.File

	// Parsed from container.xml
	RootfilePath  string
	AllRootfiles  []Rootfile // all rootfile elements from container.xml
	ContainerData []byte     // raw container.xml bytes

	// Parsed from OPF
	Package *Package

	// Raw OPF parse info (set during ParseOPF)
	OPFParseError error
	HasMetadata   bool
	HasManifest   bool
	HasSpine      bool
}

// Rootfile represents a rootfile element from container.xml.
type Rootfile struct {
	FullPath  string
	MediaType string
}

// Package represents the OPF package document.
type Package struct {
	UniqueIdentifier string
	Version          string
	Dir              string // dir attribute on package element
	Metadata         Metadata
	Manifest         []ManifestItem
	Spine            []SpineItemref
	SpineToc         string // EPUB 2 spine toc attribute
	RenditionLayout  string // "pre-paginated" or "reflowable"
	Guide            []GuideReference
	ModifiedCount    int // number of dcterms:modified meta elements
	RenditionOrientation string
	RenditionSpread      string
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
	IDRef      string
	Properties string
}

// GuideReference represents a guide reference element in EPUB 2.
type GuideReference struct {
	Type  string
	Title string
	Href  string
}
