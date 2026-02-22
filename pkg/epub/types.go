package epub

import "archive/zip"

// EPUB represents a parsed EPUB file.
type EPUB struct {
	Path    string
	ZipFile *zip.ReadCloser
	Files   map[string]*zip.File // path -> zip.File

	// Parsed from container.xml
	RootfilePath   string
	AllRootfiles   []Rootfile // all rootfile elements from container.xml
	ContainerLinks []string   // hrefs from <links> in container.xml
	ContainerData  []byte     // raw container.xml bytes

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
	Prefix           string // prefix attribute on package element
	Metadata         Metadata
	Manifest         []ManifestItem
	Spine            []SpineItemref
	SpineToc         string // EPUB 2 spine toc attribute
	RenditionLayout  string // "pre-paginated" or "reflowable"
	RenditionFlow    string // rendition:flow property
	Guide            []GuideReference
	HasGuide         bool   // whether a <guide> element was present
	ModifiedCount    int    // number of dcterms:modified meta elements
	RenditionOrientation string
	RenditionSpread      string
	PageProgressionDirection string // spine page-progression-direction attribute
	MetaRefines      []MetaRefines  // meta elements with refines attribute
	MetaIDs          []string       // id attributes from all meta elements in metadata
	ElementOrder     []string       // order of top-level OPF elements (metadata, manifest, spine, guide)
}

// Metadata holds the OPF metadata section.
type Metadata struct {
	Titles       []DCTitle
	Identifiers  []DCIdentifier
	Languages    []string
	Modified     string // dcterms:modified value
	Dates        []string
	Sources      []string
	Creators     []DCCreator
	Contributors []DCCreator  // dc:contributor elements (same structure as dc:creator)
	DCElementIDs []string     // id attributes from all dc:* elements (publisher, subject, description, etc.)
}

// DCTitle represents a dc:title element with optional id attribute.
type DCTitle struct {
	ID    string // id attribute (used as refines target in EPUB 3)
	Value string
}

// DCCreator represents a dc:creator element with optional opf:role.
type DCCreator struct {
	ID    string // id attribute (used as refines target in EPUB 3)
	Value string
	Role  string // opf:role attribute (EPUB 2)
}

// MetaRefines represents a meta element with a refines attribute.
type MetaRefines struct {
	ID       string // id attribute on the meta element itself
	Refines  string // the refines attribute value (e.g., "#id")
	Property string
	Value    string
}

// DCIdentifier is a dc:identifier element with optional id attribute.
type DCIdentifier struct {
	ID    string
	Value string
}

// ManifestItem represents a single item in the OPF manifest.
type ManifestItem struct {
	ID           string
	Href         string
	MediaType    string
	Properties   string
	Fallback     string
	HasID        bool   // false when id attribute is missing
	MediaOverlay string // media-overlay attribute
}

// SpineItemref represents a single itemref in the OPF spine.
type SpineItemref struct {
	IDRef      string
	Properties string
	Linear     string // linear attribute value ("yes", "no", or empty)
}

// GuideReference represents a guide reference element in EPUB 2.
type GuideReference struct {
	Type  string
	Title string
	Href  string
}
