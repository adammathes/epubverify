package validate

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
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

	// OPF-027: package unique-identifier attribute present
	checkPackageUniqueIdentifierAttr(pkg, r)

	// OPF-028: dcterms:modified must occur exactly once
	checkDCTermsModifiedExactlyOnce(pkg, r)

	// OPF-029: manifest property values must be valid
	checkManifestPropertyValid(pkg, r)

	// OPF-030: manifest href must not be empty
	checkManifestHrefNotEmpty(pkg, r)

	// OPF-031: dc:identifier must not be empty
	checkDCIdentifierNotEmpty(pkg, r)

	// OPF-032: dc:title must not be empty
	checkDCTitleNotEmpty(pkg, r)

	// OPF-033: manifest href must not contain fragment
	checkManifestHrefNoFragment(pkg, r)

	// OPF-034: package dir attribute must be valid
	checkPackageDirValid(pkg, r)

	// OPF-035: page-progression-direction must be valid
	checkPageProgressionDirection(pkg, r)

	// OPF-036: dc:date format
	checkDCDateFormat(pkg, r)

	// OPF-037: meta refines target must exist
	checkMetaRefinesTarget(ep, r)

	// OPF-038: spine linear attribute must be valid
	checkSpineLinearValid(pkg, r)

	// OPF-039: guide element deprecated in EPUB 3
	checkEPUB3GuideDeprecated(pkg, r)

	// OPF-040: UUID format validation
	checkUUIDFormat(pkg, r)

	// OPF-041: spine must contain at least one linear item
	checkSpineHasLinear(pkg, r)

	// OPF-042: rendition:flow must be valid
	checkRenditionFlowValid(pkg, r)

	// OPF-043: prefix declaration syntax
	checkPrefixDeclaration(pkg, r)

	// OPF-044: media-overlay references
	checkMediaOverlayRef(pkg, r)

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

	// Track all items already identified as part of any cycle
	inCycle := make(map[string]bool)
	for id := range fallbackMap {
		if inCycle[id] {
			continue
		}
		visited := make(map[string]bool)
		var chain []string
		current := id
		for {
			if visited[current] {
				// Mark all chain items to avoid duplicate reports
				for _, c := range chain {
					inCycle[c] = true
				}
				r.Add(report.Error, "OPF-022",
					fmt.Sprintf("Manifest fallback chain contains a circular reference starting at '%s'", id))
				break
			}
			visited[current] = true
			chain = append(chain, current)
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
			// Skip image-to-image mismatches - handled by MED-001
			if strings.HasPrefix(item.MediaType, "image/") && strings.HasPrefix(expectedType, "image/") {
				continue
			}
			// Skip SVG mismatch - handled by MED-004
			if expectedType == "image/svg+xml" {
				continue
			}
			// Skip equivalent MIME types for fonts, JavaScript, and audio/video
			if mediaTypesEquivalent(item.MediaType, expectedType) {
				continue
			}
			r.Add(report.Error, "OPF-024",
				fmt.Sprintf("The file '%s' does not appear to match the media type '%s'", item.Href, item.MediaType))
		}
	}
}

// mediaTypesEquivalent returns true if two MIME types are functionally
// equivalent for EPUB purposes (e.g., different font MIME types for the
// same font format, or text/javascript vs application/javascript).
func mediaTypesEquivalent(declared, expected string) bool {
	// Font MIME type equivalences: older EPUBs use application/vnd.ms-opentype
	// or application/x-font-* types for fonts that newer specs call font/otf etc.
	fontTypes := map[string]bool{
		"font/otf":                        true,
		"font/ttf":                        true,
		"font/woff":                       true,
		"font/woff2":                      true,
		"application/font-woff":           true,
		"application/font-woff2":          true,
		"application/vnd.ms-opentype":     true,
		"application/font-sfnt":           true,
		"application/x-font-ttf":          true,
		"application/x-font-opentype":     true,
		"application/x-font-truetype":     true,
	}
	if fontTypes[declared] && fontTypes[expected] {
		return true
	}
	// JavaScript MIME type equivalences
	jsTypes := map[string]bool{
		"application/javascript":   true,
		"text/javascript":          true,
		"application/ecmascript":   true,
		"application/x-javascript": true,
	}
	if jsTypes[declared] && jsTypes[expected] {
		return true
	}
	// MP4 container can be audio or video
	mp4Types := map[string]bool{
		"audio/mp4": true,
		"video/mp4": true,
	}
	if mp4Types[declared] && mp4Types[expected] {
		return true
	}
	return false
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

// Valid manifest item properties (EPUB 3)
var validManifestProperties = map[string]bool{
	"cover-image":      true,
	"data-nav":         true, // EPUB Region-Based Navigation
	"mathml":           true,
	"nav":              true,
	"remote-resources": true,
	"scripted":         true,
	"svg":              true,
	"switch":           true,
}

// Valid spine itemref properties
var validSpineProperties = map[string]bool{
	"page-spread-left":        true,
	"page-spread-right":       true,
	"rendition:layout-pre-paginated": true,
	"rendition:layout-reflowable":    true,
	"rendition:orientation-auto":     true,
	"rendition:orientation-landscape": true,
	"rendition:orientation-portrait":  true,
	"rendition:spread-auto":          true,
	"rendition:spread-landscape":     true,
	"rendition:spread-both":          true,
	"rendition:spread-none":          true,
}

// OPF-027: package element must have unique-identifier attribute
func checkPackageUniqueIdentifierAttr(pkg *epub.Package, r *report.Report) {
	if pkg.UniqueIdentifier == "" {
		r.Add(report.Error, "OPF-027", "Package element is missing unique-identifier attribute")
	}
}

// OPF-028: dcterms:modified must occur exactly once (EPUB 3)
func checkDCTermsModifiedExactlyOnce(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	if pkg.ModifiedCount > 1 {
		r.Add(report.Error, "OPF-028",
			fmt.Sprintf("Element dcterms:modified must occur exactly once, but found %d", pkg.ModifiedCount))
	}
}

// OPF-029: manifest item properties must be valid
func checkManifestPropertyValid(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	for _, item := range pkg.Manifest {
		if item.Properties == "" {
			continue
		}
		for _, prop := range strings.Fields(item.Properties) {
			if !validManifestProperties[prop] {
				r.Add(report.Error, "OPF-029",
					fmt.Sprintf("Undefined property '%s' on manifest item '%s'", prop, item.ID))
			}
		}
	}
}

// OPF-030: manifest href must not be empty
func checkManifestHrefNotEmpty(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "" {
			r.Add(report.Error, "OPF-030",
				fmt.Sprintf("Manifest must not list the package document: item '%s' has empty href", item.ID))
		}
	}
}

// OPF-031: dc:identifier must not be empty
func checkDCIdentifierNotEmpty(pkg *epub.Package, r *report.Report) {
	for _, id := range pkg.Metadata.Identifiers {
		if strings.TrimSpace(id.Value) == "" {
			r.Add(report.Error, "OPF-031",
				"Element dc:identifier has invalid value: must not be empty")
		}
	}
}

// OPF-032: dc:title must not be empty
func checkDCTitleNotEmpty(pkg *epub.Package, r *report.Report) {
	for _, t := range pkg.Metadata.Titles {
		if strings.TrimSpace(t.Value) == "" {
			r.Add(report.Error, "OPF-032",
				"Element dc:title has invalid value: must not be empty")
		}
	}
}

// OPF-033: manifest href must not contain a fragment identifier
func checkManifestHrefNoFragment(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if strings.Contains(item.Href, "#") {
			r.Add(report.Error, "OPF-033",
				fmt.Sprintf("Manifest item href must not have a fragment identifier: '%s'", item.Href))
		}
	}
}

// OPF-034: package dir attribute must be valid
func checkPackageDirValid(pkg *epub.Package, r *report.Report) {
	if pkg.Dir == "" {
		return
	}
	if pkg.Dir != "ltr" && pkg.Dir != "rtl" {
		r.Add(report.Error, "OPF-034",
			fmt.Sprintf("Package element dir attribute value '%s' is invalid: must be equal to 'ltr' or 'rtl'", pkg.Dir))
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

// OPF-035: page-progression-direction must be ltr, rtl, or default
func checkPageProgressionDirection(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" || pkg.PageProgressionDirection == "" {
		return
	}
	valid := map[string]bool{"ltr": true, "rtl": true, "default": true}
	if !valid[pkg.PageProgressionDirection] {
		r.Add(report.Error, "OPF-035",
			fmt.Sprintf("The spine page-progression-direction value '%s' must be equal to 'ltr', 'rtl', or 'default'", pkg.PageProgressionDirection))
	}
}

// OPF-036: dc:date should follow W3CDTF format
var w3cdtfRe = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})?)?)?)?$`)

func checkDCDateFormat(pkg *epub.Package, r *report.Report) {
	for _, date := range pkg.Metadata.Dates {
		if !w3cdtfRe.MatchString(date) {
			r.Add(report.Warning, "OPF-036",
				fmt.Sprintf("Date value '%s' does not follow recommended syntax of W3CDTF", date))
		}
	}
}

// OPF-037: meta refines target must exist
func checkMetaRefinesTarget(ep *epub.EPUB, r *report.Report) {
	pkg := ep.Package
	if pkg.Version < "3.0" {
		return
	}

	// Collect all valid IDs in the package document
	validIDs := make(map[string]bool)
	for _, title := range pkg.Metadata.Titles {
		if title.ID != "" {
			validIDs[title.ID] = true
		}
	}
	for _, id := range pkg.Metadata.Identifiers {
		if id.ID != "" {
			validIDs[id.ID] = true
		}
	}
	for _, creator := range pkg.Metadata.Creators {
		if creator.ID != "" {
			validIDs[creator.ID] = true
		}
	}
	for _, contrib := range pkg.Metadata.Contributors {
		if contrib.ID != "" {
			validIDs[contrib.ID] = true
		}
	}
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			validIDs[item.ID] = true
		}
	}

	// Also collect IDs from meta elements (a meta can refine another meta,
	// and non-refining metas like belongs-to-collection can have IDs)
	for _, metaID := range pkg.MetaIDs {
		validIDs[metaID] = true
	}

	for _, mr := range pkg.MetaRefines {
		target := strings.TrimPrefix(mr.Refines, "#")
		if target == "" {
			continue
		}
		if !validIDs[target] {
			r.Add(report.Error, "OPF-037",
				fmt.Sprintf("Element '%s' refines missing target id '%s'", mr.Property, target))
		}
	}
}

// OPF-038: spine itemref linear must be "yes" or "no"
func checkSpineLinearValid(pkg *epub.Package, r *report.Report) {
	for _, ref := range pkg.Spine {
		if ref.Linear == "" {
			continue
		}
		if ref.Linear != "yes" && ref.Linear != "no" {
			r.Add(report.Error, "OPF-038",
				fmt.Sprintf("The spine itemref linear attribute value '%s' must be equal to 'yes' or 'no'", ref.Linear))
		}
	}
}

// OPF-039: guide element is deprecated in EPUB 3
func checkEPUB3GuideDeprecated(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	if pkg.HasGuide {
		r.Add(report.Warning, "OPF-039",
			"The guide element is deprecated in EPUB 3 and should not be used")
	}
}

// OPF-040: UUID format validation
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func checkUUIDFormat(pkg *epub.Package, r *report.Report) {
	for _, id := range pkg.Metadata.Identifiers {
		if strings.HasPrefix(id.Value, "urn:uuid:") {
			uuid := strings.TrimPrefix(id.Value, "urn:uuid:")
			if !uuidRe.MatchString(uuid) {
				r.Add(report.Warning, "OPF-040",
					fmt.Sprintf("UUID value '%s' is invalid", uuid))
			}
		}
	}
}

// OPF-041: spine must contain at least one linear resource
func checkSpineHasLinear(pkg *epub.Package, r *report.Report) {
	if len(pkg.Spine) == 0 {
		return // OPF-010 already covers empty spine
	}
	hasLinear := false
	allExplicitlyNonlinear := true
	for _, ref := range pkg.Spine {
		if ref.Linear != "no" {
			hasLinear = true
		}
		if ref.Linear == "" {
			allExplicitlyNonlinear = false
		}
	}
	if !hasLinear && allExplicitlyNonlinear {
		r.Add(report.Error, "OPF-041",
			"The spine contains no linear resources")
	}
}

// OPF-042: rendition:flow must be valid
func checkRenditionFlowValid(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" || pkg.RenditionFlow == "" {
		return
	}
	valid := map[string]bool{
		"paginated": true, "scrolled-doc": true,
		"scrolled-continuous": true, "auto": true,
	}
	if !valid[pkg.RenditionFlow] {
		r.Add(report.Error, "OPF-042",
			fmt.Sprintf("The value of property rendition:flow must be either 'paginated', 'scrolled-doc', 'scrolled-continuous', or 'auto', but was '%s'", pkg.RenditionFlow))
	}
}

// OPF-043: prefix declaration must be well-formed
func checkPrefixDeclaration(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" || pkg.Prefix == "" {
		return
	}
	// Prefix syntax: "prefix: URI" pairs separated by whitespace
	// Each prefix must be followed by a colon and a URI
	parts := strings.Fields(pkg.Prefix)
	i := 0
	for i < len(parts) {
		prefix := parts[i]
		if !strings.HasSuffix(prefix, ":") {
			r.Add(report.Error, "OPF-043",
				fmt.Sprintf("Invalid prefix declaration: '%s' must end with ':'", prefix))
			i++
			continue
		}
		i++
		if i >= len(parts) {
			r.Add(report.Error, "OPF-043",
				fmt.Sprintf("Invalid prefix declaration: prefix '%s' has no URI mapping", prefix))
			break
		}
		uri := parts[i]
		if !strings.Contains(uri, ":") && !strings.Contains(uri, "/") {
			r.Add(report.Error, "OPF-043",
				fmt.Sprintf("Invalid prefix declaration: '%s' is not a valid URI", uri))
		}
		i++
	}
}

// OPF-044: media-overlay must reference existing SMIL manifest item
func checkMediaOverlayRef(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	manifestByID := make(map[string]epub.ManifestItem)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestByID[item.ID] = item
		}
	}

	for _, item := range pkg.Manifest {
		if item.MediaOverlay == "" {
			continue
		}
		target, ok := manifestByID[item.MediaOverlay]
		if !ok {
			r.Add(report.Error, "OPF-044",
				fmt.Sprintf("Media Overlay Document referenced by '%s' could not be found: '%s'", item.Href, item.MediaOverlay))
			continue
		}
		if target.MediaType != "application/smil+xml" {
			r.Add(report.Error, "OPF-044",
				fmt.Sprintf("Media Overlay Document referenced by '%s' has wrong type '%s': expected application/smil+xml", item.Href, target.MediaType))
		}
	}
}
