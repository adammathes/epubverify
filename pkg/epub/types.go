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
	IsLegacyOEBPS12 bool // true if package uses OEBPS 1.2 namespace

	// Raw OPF parse info (set during ParseOPF)
	OPFParseError error
	HasMetadata   bool
	HasManifest   bool
	HasSpine      bool
	PackageXMLLang string // xml:lang attribute on <package> element
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
	SpinePageMap     string // EPUB 2 spine page-map attribute (Adobe extension)
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
	HasMediaActiveClass bool        // true if media:active-class or media:playback-active-class is defined
	MetadataLinks    []MetadataLink // <link> elements in the metadata section
	MetaSchemes      []MetaScheme   // scheme attributes on meta elements
	AllXMLLangs      []string       // all xml:lang attribute values found in the OPF
	PrimaryMetas     []MetaPrimary  // meta elements without refines (primary metadata)
	MetaEmptyProps   int            // count of meta elements with empty property attribute
	MetaListProps    []string       // meta property attributes that contain spaces (multiple values)
	MetaEmptyValues  int            // count of meta elements with empty text content
	HasBindings      bool           // whether a <bindings> element was present
	BindingsTypes    map[string]bool // media types with bindings handlers
	Collections      []Collection   // <collection> elements in the package document
	UnknownElements  []string       // unknown child elements of <package>
	XMLIDCounts      map[string]int // counts of all id attributes in the OPF
	PackageNamespace string         // namespace of the <package> element
}

// MetaPrimary represents a non-refining meta element (primary metadata).
type MetaPrimary struct {
	Property string
	Value    string
}

// MetadataLink represents a <link> element in the OPF metadata section.
type MetadataLink struct {
	Href       string
	Rel        string
	MediaType  string
	Hreflang   string
	Refines    string // refines attribute
	Properties string // properties attribute
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
	DCElementIDs []string            // id attributes from all dc:* elements (publisher, subject, description, etc.)
	IDToElement  map[string]string   // maps element id → element local name (e.g., "creator", "title", "subject")
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

// DCIdentifier is a dc:identifier element with optional id and scheme attributes.
type DCIdentifier struct {
	ID     string
	Value  string
	Scheme string // opf:scheme attribute (EPUB 2)
}

// MetaScheme represents a scheme attribute on a meta element.
type MetaScheme struct {
	Scheme   string // the scheme attribute value
	Property string // the property attribute value
}

// ManifestItem represents a single item in the OPF manifest.
type ManifestItem struct {
	ID            string
	Href          string
	MediaType     string
	Properties    string
	Fallback      string
	FallbackStyle string // fallback-style attribute (EPUB 2, deprecated in EPUB 3)
	HasID         bool   // false when id attribute is missing
	MediaOverlay  string // media-overlay attribute
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

// Collection represents a <collection> element in the OPF package document.
type Collection struct {
	Role     string
	TopLevel bool     // true if this is a direct child of <package>
	Links    []string // href attributes from child <link> elements
}
