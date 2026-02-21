package validate

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// checkOPF parses the OPF and runs all package document checks.
// Returns true if a fatal error prevents further processing.
func checkOPF(ep *epub.EPUB, r *report.Report) bool {
	if err := ep.ParseOPF(); err != nil {
		// OPF-011: malformed XML in OPF
		r.Add(report.Fatal, "OPF-011", "Could not parse package document: XML document structures must start and end within the same entity")
		return true
	}

	pkg := ep.Package

	// OPF-012: metadata element present
	if !ep.HasMetadata {
		r.Add(report.Error, "OPF-012", "Package document is missing required element: metadata")
	}

	// OPF-013: manifest element present
	if !ep.HasManifest {
		r.Add(report.Error, "OPF-013", "Package document is missing required element: manifest")
	}

	// OPF-014: spine element present
	if !ep.HasSpine {
		r.Add(report.Error, "OPF-014", "Package document is missing required element: spine")
	}

	// OPF-015: version must be valid (2.0 or 3.0)
	checkPackageVersion(pkg, r)

	// OPF-001: dc:title must be present
	checkDCTitle(pkg, r)

	// OPF-002: dc:identifier must be present
	checkDCIdentifier(pkg, r)

	// OPF-003: dc:language must be present
	checkDCLanguage(pkg, r)

	// OPF-004: dcterms:modified must be present (EPUB 3)
	checkDCTermsModified(pkg, r)

	// OPF-019: dcterms:modified must be valid format
	checkDCTermsModifiedFormat(pkg, r)

	// OPF-020: dc:language must be valid BCP 47
	checkDCLanguageValid(pkg, r)

	// OPF-005: manifest item IDs must be unique
	checkManifestUniqueIDs(pkg, r)

	// OPF-016: manifest item hrefs must be unique
	checkManifestUniqueHrefs(pkg, r)

	// OPF-018: manifest items must have id
	checkManifestIDRequired(pkg, r)

	// OPF-006: manifest items must have href
	checkManifestHrefRequired(pkg, r)

	// OPF-007: manifest items must have media-type
	checkManifestMediaTypeRequired(pkg, r)

	// OPF-008: unique-identifier must resolve
	checkUniqueIdentifierResolves(pkg, r)

	// OPF-009: spine itemrefs must reference valid manifest items
	checkSpineIdrefResolves(pkg, r)

	// OPF-010: spine must not be empty
	checkSpineNotEmpty(pkg, r)

	// OPF-017: spine idrefs should be unique
	checkSpineUniqueIdrefs(pkg, r)

	// OPF-021: fallback attribute must reference existing manifest item
	checkFallbackExists(pkg, r)

	// OPF-022: fallback chains must not be circular
	checkFallbackNoCycle(pkg, r)

	// OPF-023: spine items must be content documents (or have fallback)
	checkSpineContentDocs(pkg, r)

	// OPF-024: media-type must match actual content
	checkMediaTypeMatches(ep, r)

	// OPF-025: cover-image must be on image media type
	checkCoverImageIsImage(pkg, r)

	return false
}

// OPF-001
func checkDCTitle(pkg *epub.Package, r *report.Report) {
	if len(pkg.Metadata.Titles) == 0 {
		r.Add(report.Error, "OPF-001", "Package metadata is missing required element dc:title")
	}
}

// OPF-002
func checkDCIdentifier(pkg *epub.Package, r *report.Report) {
	if len(pkg.Metadata.Identifiers) == 0 {
		r.Add(report.Error, "OPF-002", "Package metadata is missing required element dc:identifier")
	}
}

// OPF-003
func checkDCLanguage(pkg *epub.Package, r *report.Report) {
	if len(pkg.Metadata.Languages) == 0 {
		r.Add(report.Error, "OPF-003", "Package metadata is missing required element dc:language")
	}
}

// OPF-004
func checkDCTermsModified(pkg *epub.Package, r *report.Report) {
	if pkg.Version >= "3.0" && pkg.Metadata.Modified == "" {
		r.Add(report.Error, "OPF-004", "Package metadata is missing required element dcterms:modified")
	}
}

// OPF-005
func checkManifestUniqueIDs(pkg *epub.Package, r *report.Report) {
	seen := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID == "" {
			continue
		}
		if seen[item.ID] {
			r.Add(report.Error, "OPF-005",
				fmt.Sprintf("Duplicate manifest item id '%s'", item.ID))
		}
		seen[item.ID] = true
	}
}

// OPF-006
func checkManifestHrefRequired(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" {
			r.Add(report.Error, "OPF-006",
				fmt.Sprintf("Manifest item '%s' is missing required attribute 'href'", item.ID))
		}
	}
}

// OPF-007
func checkManifestMediaTypeRequired(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.MediaType == "\x00MISSING" {
			r.Add(report.Error, "OPF-007",
				fmt.Sprintf("Manifest item '%s' is missing required attribute 'media-type'", item.ID))
		}
	}
}

// OPF-008
func checkUniqueIdentifierResolves(pkg *epub.Package, r *report.Report) {
	if pkg.UniqueIdentifier == "" {
		r.Add(report.Error, "OPF-008", "Package element is missing unique-identifier attribute")
		return
	}
	for _, id := range pkg.Metadata.Identifiers {
		if id.ID == pkg.UniqueIdentifier {
			return
		}
	}
	r.Add(report.Error, "OPF-008",
		fmt.Sprintf("The unique-identifier '%s' was not found among dc:identifier elements", pkg.UniqueIdentifier))
}

// OPF-009
func checkSpineIdrefResolves(pkg *epub.Package, r *report.Report) {
	manifestIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestIDs[item.ID] = true
		}
	}
	for _, ref := range pkg.Spine {
		if ref.IDRef == "" {
			continue
		}
		if !manifestIDs[ref.IDRef] {
			r.Add(report.Error, "OPF-009",
				fmt.Sprintf("Spine itemref '%s' not found in manifest", ref.IDRef))
		}
	}
}

// OPF-010
func checkSpineNotEmpty(pkg *epub.Package, r *report.Report) {
	if len(pkg.Spine) == 0 {
		r.Add(report.Error, "OPF-010", "The spine is incomplete: it must contain at least one itemref element")
	}
}

// OPF-015: package version must be valid
func checkPackageVersion(pkg *epub.Package, r *report.Report) {
	if pkg.Version != "" && pkg.Version != "2.0" && pkg.Version != "3.0" {
		r.Add(report.Error, "OPF-015",
			fmt.Sprintf("Unsupported package version '%s'", pkg.Version))
	}
}

// OPF-016: manifest hrefs must be unique
func checkManifestUniqueHrefs(pkg *epub.Package, r *report.Report) {
	seen := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if seen[item.Href] {
			r.Add(report.Error, "OPF-016",
				fmt.Sprintf("Resource '%s' is declared in several manifest items", item.Href))
		}
		seen[item.Href] = true
	}
}

// OPF-017: spine idrefs should be unique
func checkSpineUniqueIdrefs(pkg *epub.Package, r *report.Report) {
	seen := make(map[string]bool)
	for _, ref := range pkg.Spine {
		if ref.IDRef == "" {
			continue
		}
		if seen[ref.IDRef] {
			r.Add(report.Error, "OPF-017",
				fmt.Sprintf("Spine itemref references same manifest entry as a previous itemref: '%s'", ref.IDRef))
		}
		seen[ref.IDRef] = true
	}
}

// OPF-018: manifest items must have id
func checkManifestIDRequired(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if !item.HasID {
			r.Add(report.Error, "OPF-018",
				"Manifest item is missing required attribute 'id'")
		}
	}
}

// OPF-019: dcterms:modified format validation
var modifiedDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

func checkDCTermsModifiedFormat(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" || pkg.Metadata.Modified == "" {
		return
	}
	if !modifiedDateRe.MatchString(pkg.Metadata.Modified) {
		r.Add(report.Error, "OPF-019",
			fmt.Sprintf("Invalid dcterms:modified value '%s': must be CCYY-MM-DDThh:mm:ssZ format", pkg.Metadata.Modified))
	}
}

// OPF-020: dc:language must be a well-formed BCP 47 tag
var bcp47Re = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{1,8})*$`)

func checkDCLanguageValid(pkg *epub.Package, r *report.Report) {
	for _, lang := range pkg.Metadata.Languages {
		if !bcp47Re.MatchString(lang) {
			r.Add(report.Error, "OPF-020",
				fmt.Sprintf("Language tag '%s' is not well-formed according to BCP 47", lang))
		}
	}
}

// OPF-021: fallback must reference existing manifest item
func checkFallbackExists(pkg *epub.Package, r *report.Report) {
	manifestIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestIDs[item.ID] = true
		}
	}

	for _, item := range pkg.Manifest {
		if item.Fallback == "" {
			continue
		}
		if !manifestIDs[item.Fallback] {
			r.Add(report.Error, "OPF-021",
				fmt.Sprintf("Manifest item '%s' fallback '%s' could not be found", item.ID, item.Fallback))
		}
	}
}

// OPF-022: fallback chains must not be circular
func checkFallbackNoCycle(pkg *epub.Package, r *report.Report) {
	fallbackMap := make(map[string]string)
	for _, item := range pkg.Manifest {
		if item.Fallback != "" && item.ID != "" {
			fallbackMap[item.ID] = item.Fallback
		}
	}

	reported := make(map[string]bool)
	for id := range fallbackMap {
		visited := make(map[string]bool)
		current := id
		for {
			if visited[current] {
				if !reported[id] {
					r.Add(report.Error, "OPF-022",
						fmt.Sprintf("Manifest fallback chain contains a circular reference starting at '%s'", id))
					reported[id] = true
				}
				break
			}
			visited[current] = true
			next, ok := fallbackMap[current]
			if !ok {
				break
			}
			current = next
		}
	}
}

// OPF-023: spine items with non-standard media types must have a fallback
var contentDocTypes = map[string]bool{
	"application/xhtml+xml": true,
	"image/svg+xml":         true,
}

func checkSpineContentDocs(pkg *epub.Package, r *report.Report) {
	manifestByID := make(map[string]epub.ManifestItem)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestByID[item.ID] = item
		}
	}

	for _, ref := range pkg.Spine {
		item, ok := manifestByID[ref.IDRef]
		if !ok {
			continue
		}
		if item.MediaType == "\x00MISSING" {
			continue
		}
		if contentDocTypes[item.MediaType] {
			continue
		}
		if !hasFallbackToContentDoc(item.ID, manifestByID) {
			r.Add(report.Error, "OPF-023",
				fmt.Sprintf("Spine item '%s' has non-standard media-type '%s' with no fallback to a content document", item.ID, item.MediaType))
		}
	}
}

func hasFallbackToContentDoc(startID string, manifest map[string]epub.ManifestItem) bool {
	visited := make(map[string]bool)
	current := startID
	for {
		if visited[current] {
			return false
		}
		visited[current] = true
		item, ok := manifest[current]
		if !ok {
			return false
		}
		if item.Fallback == "" {
			return false
		}
		fb, ok := manifest[item.Fallback]
		if !ok {
			return false
		}
		if contentDocTypes[fb.MediaType] {
			return true
		}
		current = item.Fallback
	}
}

// OPF-024: media-type should match actual file content
func checkMediaTypeMatches(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.MediaType == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		if _, exists := ep.Files[fullPath]; !exists {
			continue
		}

		ext := strings.ToLower(path.Ext(item.Href))
		expectedType := extensionToMediaType(ext)
		if expectedType == "" {
			continue
		}

		if item.MediaType != expectedType {
			r.Add(report.Error, "OPF-024",
				fmt.Sprintf("The file '%s' does not appear to match the media type '%s'", item.Href, item.MediaType))
		}
	}
}

func extensionToMediaType(ext string) string {
	switch ext {
	case ".xhtml", ".html", ".htm":
		return "application/xhtml+xml"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".ncx":
		return "application/x-dtbncx+xml"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".smil":
		return "application/smil+xml"
	default:
		return ""
	}
}

// OPF-025: cover-image property must be on image media type
func checkCoverImageIsImage(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	for _, item := range pkg.Manifest {
		if hasProperty(item.Properties, "cover-image") {
			if !strings.HasPrefix(item.MediaType, "image/") {
				r.Add(report.Error, "OPF-025",
					fmt.Sprintf("The cover-image property is not defined for media type '%s'", item.MediaType))
			}
		}
	}
}
