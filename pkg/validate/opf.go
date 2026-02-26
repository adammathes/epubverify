package validate

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// checkOPF parses the OPF and runs all package document checks.
// Returns true if a fatal error prevents further processing.
func checkOPF(ep *epub.EPUB, r *report.Report, opts Options) bool {
	// Pre-parse checks on raw OPF bytes
	if opfData, err := ep.ReadFile(ep.RootfilePath); err == nil {
		// Encoding detection (must run before XML parsing)
		encodingIssue, encodingConflict := checkOPFEncoding(opfData)
		if encodingIssue != "" {
			switch encodingIssue {
			case "utf16":
				r.Add(report.Warning, "RSC-027", "XML documents should be encoded using UTF-8, but found UTF-16 encoding")
				if encodingConflict {
					// BOM says UTF-16 but declaration says something else
					r.Add(report.Fatal, "RSC-016", "Could not parse package document: encoding conflict between BOM and declaration")
				}
				return false // don't try to parse UTF-16 with Go's XML decoder
			case "utf32", "ucs4":
				r.Add(report.Error, "RSC-028", "XML documents must be encoded using UTF-8 or UTF-16, but found UCS-4/UTF-32 encoding")
				return false
			case "latin1":
				r.Add(report.Error, "RSC-028", "XML documents must be encoded using UTF-8 or UTF-16, but found ISO-8859-1 encoding")
				return false
			case "unknown":
				r.Add(report.Error, "RSC-028", "XML documents must be encoded using UTF-8 or UTF-16, but found an unknown encoding")
				r.Add(report.Fatal, "RSC-016", "Could not parse package document: unknown encoding")
				return true
			}
		}

		if hasUndeclaredNamespacePrefix(opfData) {
			r.Add(report.Fatal, "RSC-016", "Could not parse package document: undeclared namespace prefix")
			return true
		}
		// HTM-009: invalid DOCTYPE in OPF
		checkOPFDoctype(opfData, r)
	}

	if err := ep.ParseOPF(); err != nil {
		// RSC-016: malformed XML in OPF
		r.Add(report.Fatal, "RSC-016", "Could not parse package document: XML document structures must start and end within the same entity")
		return true
	}

	// OEBPS 1.2: detect legacy namespace
	if ep.IsLegacyOEBPS12 {
		if ep.Package.Version == "" {
			// No version attribute → OPF-001 for missing version, stop further processing
			r.Add(report.Error, "OPF-001", "Package version attribute is missing or empty")
			return false
		}
		// Has version → legacy media type checks happen after DowngradeToInfo in the validator
		return false
	}

	pkg := ep.Package

	// RSC-005: wrong default namespace — report errors and stop further checks
	if pkg.PackageNamespace != "" &&
		pkg.PackageNamespace != "http://www.idpf.org/2007/opf" &&
		pkg.PackageNamespace != "http://openebook.org/namespaces/oeb-package/1.0/" {
		r.Add(report.Error, "RSC-005",
			fmt.Sprintf(`The default namespace must be "http://www.idpf.org/2007/opf", but found "%s"`, pkg.PackageNamespace))
		// Side effects: elements in wrong namespace produce additional schema errors
		r.Add(report.Error, "RSC-005", `element "metadata" not found in expected namespace`)
		r.Add(report.Error, "RSC-005", `element "manifest" not found in expected namespace`)
		r.Add(report.Error, "RSC-005", `element "spine" not found in expected namespace`)
		return false
	}

	// RSC-005: required elements present (schema validation)
	if !ep.HasMetadata {
		r.Add(report.Error, "RSC-005", `missing required element "metadata"`)
	}

	if !ep.HasManifest {
		r.Add(report.Error, "RSC-005", `missing required element "manifest"`)
	}

	if !ep.HasSpine {
		r.Add(report.Error, "RSC-005", `missing required element "spine"`)
	}

	// OPF-015: version must be valid (2.0 or 3.0)
	checkPackageVersion(pkg, r)

	if ep.HasMetadata {
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
	}

	// OPF-005: manifest item IDs must be unique
	checkManifestUniqueIDs(pkg, r)

	// OPF-074: manifest item hrefs must be unique
	checkManifestUniqueHrefs(pkg, r)

	// OPF-018: manifest items must have id
	checkManifestIDRequired(pkg, r)

	// OPF-006: manifest items must have href
	checkManifestHrefRequired(pkg, r)

	// OPF-007: manifest items must have media-type
	checkManifestMediaTypeRequired(pkg, r)

	// OPF-008: unique-identifier must resolve (OPF-030 skipped in single-file mode)
	checkUniqueIdentifierResolves(pkg, r, opts.SingleFileMode)

	// OPF-009: spine itemrefs must reference valid manifest items
	checkSpineIdrefResolves(pkg, r)

	// OPF-010: spine must not be empty
	checkSpineNotEmpty(ep, r)

	// OPF-034: spine idrefs should be unique (repeated spine items)
	checkSpineUniqueIdrefs(pkg, r)

	// OPF-040: fallback attribute must reference existing manifest item
	checkFallbackExists(pkg, r)

	// OPF-045: fallback chains must not be circular (also covers self-reference)
	checkFallbackNoCycle(pkg, r)

	// RSC-032: fallback chain must resolve to a core media type
	if !opts.SingleFileMode {
		checkFallbackChainResolves(pkg, r)
	}

	// OPF-023: spine items must be content documents (or have fallback)
	checkSpineContentDocs(pkg, r)

	// OPF-024: media-type must match actual content (needs container files)
	if !opts.SingleFileMode {
		checkMediaTypeMatches(ep, r)
	}

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

	// OPF-012: package dir attribute must be valid
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

	// OPF-085: UUID format validation
	checkUUIDFormat(pkg, r)

	// OPF-041: spine must contain at least one linear item
	checkSpineHasLinear(pkg, r)

	// Rendition property validation (value, cardinality, refines, spine overrides)
	checkRenditionProperties(pkg, r)

	// OPF-043/007/007b/007c/004c: prefix declaration validation
	checkPrefixDeclaration(pkg, r)

	// OPF-028: undeclared prefix in metadata properties
	checkUndeclaredPrefixes(pkg, r)

	// OPF-044: media-overlay references
	checkMediaOverlayRef(pkg, r)

	// RSC-005/OPF-063: page-map attribute on spine is not allowed
	checkSpinePageMap(ep, pkg, r)

	// OPF-077: Data Navigation Document should not be in spine
	checkDataNavNotInSpine(pkg, r)

	// OPF-066: pagination source metadata check
	if !opts.SingleFileMode {
		checkPaginationSourceMetadata(ep, pkg, r)
	}

	// OPF-099: OPF must not reference itself in manifest
	checkManifestSelfReference(ep, pkg, r)

	if !opts.SingleFileMode {
		// OPF-093/RSC-007w: package metadata link checks
		checkPackageMetadataLinks(ep, pkg, r)

		// OPF-096: non-linear spine items must be reachable
		checkSpineNonLinearReachable(ep, r)
	}

	// OPF-067: link resources must not also be manifest items (EPUB 3)
	if !opts.SingleFileMode {
		checkLinkNotInManifest(ep, pkg, r)
	}

	// OPF-072: empty metadata elements
	checkEmptyMetadataElements(pkg, r)

	// OPF-090: manifest items with non-preferred but valid core media types
	checkNonPreferredMediaTypes(pkg, r)

	// RSC-029: data URLs in manifest items
	checkDataURLsInManifest(pkg, r)

	// RSC-030: file URLs in manifest/link
	checkFileURLsInOPF(pkg, r)

	// RSC-033: URL query strings in manifest/link
	checkQueryStringsInOPF(pkg, r)

	// OPF-028: dcterms:modified count in metadata (multiple occurrences)
	checkDCTermsModifiedCount(pkg, r)

	// OPF-053: dc:date warnings
	checkDCDateWarnings(pkg, r)

	// OPF-052: dc:creator role validation
	checkDCCreatorRole(pkg, r)

	// OPF-054: dc:date empty value
	checkDCDateEmpty(pkg, r)

	// OPF-065: refines cycle detection
	checkRefinesCycle(pkg, r)

	// OPF-098: link must not target manifest ID
	checkLinkTargetNotManifestID(pkg, r)

	// OPF-094/095: link relation and properties validation
	checkLinkRelation(pkg, r)

	// OPF-026: metadata property names must be defined
	checkMetaPropertyDefined(pkg, r)

	// OPF-004c: dcterms:modified must occur as non-refining meta
	checkDCTermsModifiedNonRefining(pkg, r)

	// (rendition deprecation handled by checkRenditionProperties)

	// OPF-092: language tag validation
	checkLanguageTags(ep, pkg, r)

	// OPF-055: empty dc:title warning
	checkDCTitleEmptyWarning(pkg, r)

	// OPF-012: cover-image property uniqueness
	checkCoverImageUnique(pkg, r)

	// OPF-050: NCX identification
	checkNCXIdentification(pkg, r)

	// OPF-027: scheme attribute validation on meta elements
	checkMetaSchemeValid(pkg, r)

	// Metadata refines property validation (D.3 vocabulary checks)
	checkMetaRefinesPropertyRules(pkg, r)

	// Media overlay packaging checks
	checkMediaOverlayOnNonContentDoc(pkg, r)
	checkMediaOverlayDurationMeta(pkg, r)
	checkMediaOverlayType(pkg, r)

	// OPF-032b: meta element with empty text content
	checkMetaEmptyValue(pkg, r)

	// OPF-025b: meta property attribute validation (empty, list)
	checkMetaPropertyAttr(pkg, r)

	// OPF-086b: EPUB 3 fallback-style is deprecated
	checkFallbackStyleDeprecated(pkg, r)

	// EPUB 2 spine toc attribute required (runs even in single-file mode)
	if pkg.Version < "3.0" && pkg.Version != "" && ep.HasSpine && !ep.IsLegacyOEBPS12 {
		if pkg.SpineToc == "" {
			r.Add(report.Error, "RSC-005",
				`missing required attribute "toc"`)
		}
	}

	// OPF-041: EPUB 2 fallback-style attribute pointing to non-existing ID
	if pkg.Version < "3.0" && pkg.Version != "" {
		checkFallbackStyleRef(pkg, r)
	}

	// Empty guide check (EPUB 2 schema error)
	if pkg.HasGuide && len(pkg.Guide) == 0 {
		r.Add(report.Error, "OPF-039b",
			`element "guide" incomplete; missing required element "reference"`)
	}

	// RSC-020 / PKG-009/010/012: manifest item href checks
	checkManifestHrefEncoding(pkg, r)
	if opts.SingleFileMode {
		checkManifestHrefFilenames(pkg, r)
	}

	// RSC-017: guide duplicate entries
	checkGuideDuplicates(pkg, r)

	// OPF-070/RSC-005: collection role validation
	checkCollections(pkg, r)

	// RSC-005: element order (metadata must come before manifest, manifest before spine)
	checkElementOrder(pkg, r)

	// RSC-005: nav property checks (EPUB 3)
	checkNavProperty(pkg, r)

	// RSC-017: bindings element is deprecated (EPUB 3)
	if pkg.HasBindings && pkg.Version >= "3.0" {
		r.Add(report.Warning, "RSC-017",
			`the "bindings" element is deprecated`)
	}

	// RSC-005: unknown elements as direct children of <package>
	for _, elem := range pkg.UnknownElements {
		r.Add(report.Error, "RSC-005",
			fmt.Sprintf(`element "%s" not allowed anywhere`, elem))
	}

	// RSC-005: XML-level duplicate IDs
	for id, count := range pkg.XMLIDCounts {
		if count > 1 {
			for i := 0; i < count; i++ {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf(`Duplicate ID "%s"`, id))
			}
		}
	}

	return false
}

// checkElementOrder verifies that OPF child elements appear in the required order.
func checkElementOrder(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	order := pkg.ElementOrder
	if len(order) < 2 {
		return
	}
	// The required order is: metadata, manifest, spine, [guide]
	expectedOrder := []string{"metadata", "manifest", "spine"}
	idxMap := make(map[string]int)
	for i, elem := range order {
		if _, seen := idxMap[elem]; !seen {
			idxMap[elem] = i
		}
	}
	metaIdx, hasMeta := idxMap["metadata"]
	manIdx, hasMan := idxMap["manifest"]
	_, hasSpine := idxMap["spine"]

	if hasMeta && hasMan && manIdx < metaIdx {
		r.Add(report.Error, "RSC-005",
			`element "manifest" not allowed yet; expected element "metadata"`)
		r.Add(report.Error, "RSC-005",
			`element "metadata" not allowed here; expected element "spine"`)
	}

	_ = expectedOrder
	_ = hasSpine
}

// checkNavProperty verifies that exactly one manifest item has the "nav" property
// and that it is an XHTML Content Document.
func checkNavProperty(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	navCount := 0
	for _, item := range pkg.Manifest {
		if hasProperty(item.Properties, "nav") {
			navCount++
			// Must be XHTML
			if item.MediaType != "application/xhtml+xml" {
				r.Add(report.Error, "RSC-005",
					`the Navigation Document must be of the "application/xhtml+xml" type`)
				r.Add(report.Error, "OPF-012",
					fmt.Sprintf(`the "nav" property is not defined for items of type "%s"`, item.MediaType))
			}
		}
	}
	if navCount == 0 {
		r.Add(report.Error, "RSC-005",
			`Exactly one manifest item must declare the "nav" property (0 found)`)
	} else if navCount > 1 {
		r.Add(report.Error, "RSC-005",
			`Exactly one manifest item must declare the "nav" property (multiple found)`)
	}
}

// OPF-032b: meta element with empty text content (excluding rendition properties
// which are handled by checkRenditionProperties with property-specific messages)
func checkMetaEmptyValue(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	// Count empty rendition metas to exclude from generic check
	renditionEmpty := 0
	for _, pm := range pkg.PrimaryMetas {
		if pm.Value == "" && strings.HasPrefix(pm.Property, "rendition:") {
			renditionEmpty++
		}
	}
	count := pkg.MetaEmptyValues - renditionEmpty
	for i := 0; i < count; i++ {
		r.Add(report.Error, "OPF-032",
			"meta element must have non-empty text content")
	}
}

// OPF-025b: meta property attribute must be a single well-formed value
func checkMetaPropertyAttr(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	// Empty property attribute
	for i := 0; i < pkg.MetaEmptyProps; i++ {
		r.Add(report.Error, "OPF-025b",
			`value of attribute "property" is invalid; must be a string with length at least 1`)
	}
	// Property with multiple space-separated values
	for _, prop := range pkg.MetaListProps {
		// RSC-005 for NMTOKEN validation (property value contains spaces)
		r.Add(report.Error, "OPF-025b",
			fmt.Sprintf(`value of attribute "property" is invalid; "%s" is not an NMTOKEN`, prop))
		// OPF-025 for the semantic check
		r.Add(report.Error, "OPF-025",
			fmt.Sprintf("Property '%s' is not valid: only one value must be specified for the 'property' attribute", prop))
	}
}

// OPF-041: EPUB 2 fallback-style attribute must reference an existing manifest item
func checkFallbackStyleRef(pkg *epub.Package, r *report.Report) {
	manifestIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestIDs[item.ID] = true
		}
	}
	for _, item := range pkg.Manifest {
		if item.FallbackStyle != "" {
			if !manifestIDs[item.FallbackStyle] {
				r.Add(report.Error, "OPF-041",
					fmt.Sprintf("'fallback-style' attribute value '%s' does not reference a manifest item", item.FallbackStyle))
			}
		}
	}
}

// OPF-086b: EPUB 3 fallback-style attribute is not allowed (deprecated EPUB 2 feature)
func checkFallbackStyleDeprecated(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	for _, item := range pkg.Manifest {
		if item.FallbackStyle != "" {
			r.Add(report.Error, "OPF-086b",
				fmt.Sprintf("The 'fallback-style' attribute on manifest item '%s' is not allowed in EPUB 3", item.ID))
		}
	}
}

// nonPreferredMediaTypes maps deprecated-but-valid core media types to
// their preferred equivalents. EPUB 3 prefers font/ttf, font/otf, etc.
var nonPreferredMediaTypes = map[string]string{
	"application/font-sfnt":     "font/ttf or font/otf",
	"application/x-font-ttf":    "font/ttf",
	"application/vnd.ms-opentype": "font/otf",
	"application/font-woff":     "font/woff",
	"application/font-woff2":    "font/woff2",
	"application/ecmascript":    "application/javascript",
	"text/javascript":            "application/javascript",
	"application/x-javascript":  "application/javascript",
}

// OPF-090: usage for manifest items using non-preferred but valid core media types.
func checkNonPreferredMediaTypes(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if preferred, ok := nonPreferredMediaTypes[item.MediaType]; ok {
			r.Add(report.Usage, "OPF-090",
				fmt.Sprintf("Manifest item '%s' uses a non-preferred media type '%s' (prefer %s)",
					item.Href, item.MediaType, preferred))
		}
	}
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
	hasNonEmpty := false
	for _, lang := range pkg.Metadata.Languages {
		if lang != "" {
			hasNonEmpty = true
			break
		}
	}
	if !hasNonEmpty && len(pkg.Metadata.Languages) == 0 {
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
		trimID := strings.TrimSpace(item.ID)
		if seen[trimID] {
			// Only report OPF-005 if this isn't already covered by XML-level duplicate ID check
			if pkg.XMLIDCounts[trimID] <= 1 {
				r.Add(report.Error, "OPF-005",
					fmt.Sprintf("Duplicate manifest item id '%s'", item.ID))
			}
		}
		seen[trimID] = true
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

// OPF-008 / OPF-030: unique-identifier attribute must exist and resolve.
// In EPUB2 single-file mode, OPF-030 is skipped (epubcheck FIXME behavior).
func checkUniqueIdentifierResolves(pkg *epub.Package, r *report.Report, singleFileMode bool) {
	if pkg.UniqueIdentifier == "" {
		r.Add(report.Error, "OPF-008", "Package element is missing unique-identifier attribute")
		return
	}
	// EPUB2 single-file mode: epubcheck doesn't report OPF-030 (FIXME in epubcheck)
	if singleFileMode && pkg.Version < "3.0" {
		return
	}
	for _, id := range pkg.Metadata.Identifiers {
		if id.ID == pkg.UniqueIdentifier {
			return
		}
	}
	r.Add(report.Error, "OPF-030",
		fmt.Sprintf("The unique-identifier '%s' was not found among dc:identifier elements", pkg.UniqueIdentifier))
}

// OPF-049
func checkSpineIdrefResolves(pkg *epub.Package, r *report.Report) {
	manifestIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestIDs[strings.TrimSpace(item.ID)] = true
		}
	}
	for _, ref := range pkg.Spine {
		if ref.IDRef == "" {
			continue
		}
		if !manifestIDs[strings.TrimSpace(ref.IDRef)] {
			r.Add(report.Error, "OPF-049",
				fmt.Sprintf("Spine itemref '%s' not found in manifest", ref.IDRef))
			r.Add(report.Error, "RSC-005",
				fmt.Sprintf(`itemref idref "%s" does not resolve to a manifest item`, ref.IDRef))
		}
	}
}

// OPF-010
func checkSpineNotEmpty(ep *epub.EPUB, r *report.Report) {
	if !ep.HasSpine {
		return // RSC-005 already covers missing spine element
	}
	if len(ep.Package.Spine) == 0 {
		r.Add(report.Error, "OPF-010", "The spine is incomplete: it must contain at least one itemref element")
	}
}

// OPF-015: package version must be valid
// OPF-001 (version): missing version attribute
func checkPackageVersion(pkg *epub.Package, r *report.Report) {
	if pkg.Version == "" {
		r.Add(report.Error, "OPF-001", "Package version attribute is missing or empty")
		return
	}
	if pkg.Version != "2.0" && pkg.Version != "3.0" {
		r.Add(report.Error, "OPF-015",
			fmt.Sprintf("Unsupported package version '%s'", pkg.Version))
	}
}

// OPF-074: manifest hrefs must be unique
func checkManifestUniqueHrefs(pkg *epub.Package, r *report.Report) {
	seen := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if seen[item.Href] {
			r.Add(report.Error, "OPF-074",
				fmt.Sprintf("Resource '%s' is declared in several manifest items", item.Href))
		}
		seen[item.Href] = true
	}
}

// OPF-034: spine idrefs should be unique (repeated spine items)
func checkSpineUniqueIdrefs(pkg *epub.Package, r *report.Report) {
	seen := make(map[string]bool)
	for _, ref := range pkg.Spine {
		if ref.IDRef == "" {
			continue
		}
		if seen[ref.IDRef] {
			r.Add(report.Error, "OPF-034",
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
		if strings.TrimSpace(lang) == "" {
			// Empty/whitespace-only language is a schema validation error
			r.Add(report.Error, "OPF-031", "Element dc:language must be a string with length at least 1")
			continue
		}
		if !bcp47Re.MatchString(lang) {
			// For EPUB 2, use OPF-020; for EPUB 3, OPF-092 handles this
			if pkg.Version < "3.0" {
				r.Add(report.Error, "OPF-020",
					fmt.Sprintf("Language tag '%s' is not well-formed according to BCP 47", lang))
			}
			// OPF-092 check in checkLanguageTags handles EPUB 3
		}
	}
}

// OPF-040: fallback must reference existing manifest item
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
			r.Add(report.Error, "OPF-040",
				fmt.Sprintf("Manifest item '%s' fallback '%s' could not be found", item.ID, item.Fallback))
		}
	}
}

// OPF-045: fallback chains must not be circular (also covers self-reference)
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
				r.Add(report.Error, "OPF-045",
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
	"application/xhtml+xml":    true,
	"image/svg+xml":            true,
	"application/x-dtbook+xml": true, // EPUB 2 DTBook content documents
	"text/x-oeb1-document":     true, // OEB 1.x legacy format
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
			// OPF-042 for image types used directly in spine (EPUBCheck compatibility)
			if strings.HasPrefix(item.MediaType, "image/") {
				r.Add(report.Error, "OPF-042",
					fmt.Sprintf("Spine item '%s' is an image (%s) and should not be used directly in the spine", item.ID, item.MediaType))
			} else {
				r.Add(report.Error, "OPF-043",
					fmt.Sprintf("Spine item '%s' has non-standard media-type '%s' with no fallback to a content document", item.ID, item.MediaType))
			}
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
			// Image-to-image mismatches: handled by MED-001 (content ≠ declared)
			// and PKG-022 (content = declared but extension is wrong)
			if strings.HasPrefix(item.MediaType, "image/") && strings.HasPrefix(expectedType, "image/") {
				// PKG-022: extension doesn't match declared type, but content matches declared
				data, err := ep.ReadFile(fullPath)
				if err == nil {
					actualType := detectImageType(data)
					if actualType == item.MediaType {
						r.Add(report.Warning, "PKG-022",
							fmt.Sprintf("The file extension of '%s' doesn't match its media type '%s'", item.Href, item.MediaType))
					}
					// If actualType != item.MediaType, MED-001 handles it
				}
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
			// Skip foreign/non-standard media types - these are intentional
			// and handled by RSC-032 (foreign resource fallback checks)
			if !coreMediaTypes[item.MediaType] {
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

// Reserved property prefixes that do not define manifest item properties.
// Properties using these prefixes on manifest items are invalid (OPF-027).
var reservedPropertyPrefixes = map[string]bool{
	"a11y": true, "dcterms": true, "marc": true, "media": true,
	"onix": true, "rendition": true, "schema": true, "xsd": true,
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
	"page-spread-left":                true,
	"page-spread-right":               true,
	"page-spread-center":              true,
	"rendition:layout-pre-paginated":  true,
	"rendition:layout-reflowable":     true,
	"rendition:orientation-auto":      true,
	"rendition:orientation-landscape": true,
	"rendition:orientation-portrait":  true,
	"rendition:page-spread-center":    true,
	"rendition:spread-auto":           true,
	"rendition:spread-landscape":      true,
	"rendition:spread-both":           true,
	"rendition:spread-none":           true,
	"rendition:spread-portrait":       true,
}

// OPF-048: package element must have unique-identifier attribute
func checkPackageUniqueIdentifierAttr(pkg *epub.Package, r *report.Report) {
	if pkg.UniqueIdentifier == "" {
		r.Add(report.Error, "OPF-048", "Package element is missing its required unique-identifier attribute")
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

// OPF-029/OPF-027: manifest item properties must be valid
func checkManifestPropertyValid(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	for _, item := range pkg.Manifest {
		if item.Properties == "" {
			continue
		}
		for _, prop := range strings.Fields(item.Properties) {
			if validManifestProperties[prop] {
				continue
			}
			if strings.Contains(prop, ":") {
				// Reserved prefixes do not define manifest item properties; report OPF-027.
				// User-declared prefixes might have custom vocabularies, so allow them.
				prefix := strings.SplitN(prop, ":", 2)[0]
				if reservedPropertyPrefixes[prefix] {
					r.Add(report.Error, "OPF-027",
						fmt.Sprintf("Undefined property '%s' on manifest item '%s'", prop, item.ID))
				}
				continue
			}
			// OPF-027: undefined property
			r.Add(report.Error, "OPF-027",
				fmt.Sprintf("Undefined property '%s' on manifest item '%s'", prop, item.ID))
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
			if pkg.Version < "3.0" {
				// EPUB 2: empty dc:title is a warning
				r.Add(report.Warning, "OPF-055",
					"Element dc:title has an empty value")
			} else {
				r.Add(report.Error, "OPF-032",
					"Element dc:title has invalid value: must not be empty")
			}
		}
	}
}

// OPF-091: manifest href must not contain a fragment identifier
func checkManifestHrefNoFragment(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if strings.Contains(item.Href, "#") {
			r.Add(report.Error, "OPF-091",
				fmt.Sprintf("Manifest item href must not have a fragment identifier: '%s'", item.Href))
		}
	}
}

// OPF-012: package dir attribute must be valid
func checkPackageDirValid(pkg *epub.Package, r *report.Report) {
	if pkg.Dir == "" {
		return
	}
	if pkg.Dir != "ltr" && pkg.Dir != "rtl" && pkg.Dir != "auto" {
		r.Add(report.Error, "OPF-012",
			fmt.Sprintf("Package element dir attribute value '%s' is invalid: must be equal to 'ltr', 'rtl', or 'auto'", pkg.Dir))
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
				r.Add(report.Error, "OPF-012",
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

// OPF-036: dc:date should follow W3CDTF format (EPUB 2 only; EPUB 3 uses OPF-053)
var w3cdtfRe = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})?)?)?)?$`)

func checkDCDateFormat(pkg *epub.Package, r *report.Report) {
	if pkg.Version >= "3.0" {
		return // EPUB 3 uses OPF-053 for date format issues
	}
	for _, date := range pkg.Metadata.Dates {
		if date == "" {
			continue // Empty dates handled by OPF-054
		}
		if !w3cdtfRe.MatchString(strings.TrimSpace(date)) {
			r.Add(report.Error, "OPF-054",
				fmt.Sprintf("Date value '%s' is not a valid date", date))
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

	// Collect IDs from all dc:* elements (publisher, subject, description, etc.)
	for _, dcID := range pkg.Metadata.DCElementIDs {
		validIDs[dcID] = true
	}

	for _, mr := range pkg.MetaRefines {
		refines := mr.Refines
		if refines == "" {
			continue
		}
		// Check for absolute URLs - refines must be a relative URL
		if strings.Contains(refines, "://") {
			r.Add(report.Error, "OPF-037",
				fmt.Sprintf("@refines must be a relative URL, but found '%s'", refines))
			continue
		}
		// Refines should use a fragment identifier when referring to a Publication Resource.
		// If it looks like a file path (contains '.' or '/'), report RSC-017 and skip.
		// If it's a bare ID (no '#' prefix), still do the target lookup.
		if !strings.HasPrefix(refines, "#") {
			if strings.Contains(refines, ".") || strings.Contains(refines, "/") {
				r.Add(report.Warning, "RSC-017",
					fmt.Sprintf("@refines attribute is not using a fragment identifier pointing to its manifest item: '%s'", refines))
				continue
			}
		}
		target := strings.TrimPrefix(refines, "#")
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
		trimmed := strings.TrimSpace(ref.Linear)
		if trimmed != "yes" && trimmed != "no" {
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

// checkLegacyOEBPS12MediaTypes checks for legacy media types in OEBPS 1.2 publications.
// OPF-039: text/x-oeb1-document (legacy HTML media type)
// checkLegacyOEBPS12MediaTypes reports deprecated OEBPS 1.2 and legacy media types.
func checkLegacyOEBPS12MediaTypes(ep *epub.EPUB, r *report.Report) {
	pkg := ep.Package
	for _, item := range pkg.Manifest {
		switch item.MediaType {
		case "text/x-oeb1-document":
			r.Add(report.Warning, "OPF-039",
				fmt.Sprintf("The media-type '%s' is a deprecated OEBPS 1.2 media type", item.MediaType))
		case "text/x-oeb1-css":
			r.Add(report.Warning, "OPF-037",
				fmt.Sprintf("The media-type '%s' is a deprecated OEBPS 1.2 media type", item.MediaType))
		case "text/html":
			// Only report as OPF-038 for OEBPS 1.2 packages; regular EPUB 2
			// packages handle text/html via OPF-035 in epub2.go.
			if ep.IsLegacyOEBPS12 {
				r.Add(report.Warning, "OPF-038",
					fmt.Sprintf("The media-type '%s' is not a valid EPUB media type for content documents", item.MediaType))
			}
		}
	}
}


// OPF-085: UUID format validation
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func checkUUIDFormat(pkg *epub.Package, r *report.Report) {
	for _, id := range pkg.Metadata.Identifiers {
		if strings.HasPrefix(id.Value, "urn:uuid:") {
			uuid := strings.TrimPrefix(id.Value, "urn:uuid:")
			if !uuidRe.MatchString(uuid) {
				r.Add(report.Warning, "OPF-085",
					fmt.Sprintf("UUID value '%s' is invalid", uuid))
			}
		}
		// EPUB 2: opf:scheme="uuid" with non-UUID value
		if id.Scheme != "" && strings.EqualFold(id.Scheme, "uuid") {
			if !uuidRe.MatchString(id.Value) {
				r.Add(report.Warning, "OPF-085",
					fmt.Sprintf("Identifier with scheme 'uuid' has invalid UUID value '%s'", id.Value))
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
		trimmed := strings.TrimSpace(ref.Linear)
		if trimmed != "no" {
			hasLinear = true
		}
		if ref.Linear == "" {
			allExplicitlyNonlinear = false
		}
	}
	if !hasLinear && allExplicitlyNonlinear {
		r.Add(report.Error, "OPF-033",
			"The spine must contain at least one linear itemref element")
	}
}

// OPF-043: prefix declaration must be well-formed
// parsePrefixAttribute parses the prefix attribute value into prefix→URI mappings.
// It reports OPF-004c for syntax errors. The syntax is:
//
//	prefix: URI prefix: URI ...
//
// Each prefix name must be immediately followed by ":" (no space) and then a URI.
// Returns the map of successfully parsed declared prefixes.
func parsePrefixAttribute(prefixAttr string, r *report.Report) map[string]string {
	declared := make(map[string]string)
	if prefixAttr == "" {
		return declared
	}

	parts := strings.Fields(prefixAttr)
	i := 0
	for i < len(parts) {
		token := parts[i]

		if strings.HasSuffix(token, ":") && len(token) > 1 {
			// Well-formed: "prefix:" followed by URI
			prefixName := strings.TrimSuffix(token, ":")
			i++
			if i >= len(parts) {
				r.Add(report.Error, "OPF-004c",
					fmt.Sprintf("The prefix '%s' has no URI mapping", prefixName))
				break
			}
			uri := parts[i]
			declared[prefixName] = uri
			i++
		} else {
			// Syntax error: token is not "prefix:"
			// Report error and try to consume the bad pair
			r.Add(report.Error, "OPF-004c",
				fmt.Sprintf("Invalid prefix mapping syntax near '%s'", token))
			i++
			// Skip remaining tokens that are part of this bad mapping
			// until we reach another word that could be a prefix
			for i < len(parts) {
				next := parts[i]
				if next == ":" {
					// Space-before-colon: skip ":" and the URI that follows
					i++
					if i < len(parts) {
						i++ // skip URI
					}
					break
				}
				// If next token looks like it could start a new pair (word:), stop
				if strings.HasSuffix(next, ":") && len(next) > 1 {
					break
				}
				// If next token is a URI (part of this bad pair), consume it
				i++
				break // consumed one URI token, move on
			}
		}
	}
	return declared
}

// checkPrefixDeclaration validates the prefix attribute on the package element.
// OPF-004c: prefix syntax errors
// OPF-007b: must not re-map default vocabulary prefixes
// OPF-007c: must not map to Dublin Core elements namespace
// OPF-007: warning for overriding reserved prefix URIs
func checkPrefixDeclaration(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" || pkg.Prefix == "" {
		return
	}

	declared := parsePrefixAttribute(pkg.Prefix, r)

	// Default vocabulary URIs that must not be assigned prefixes
	defaultVocabURIs := map[string]bool{
		"http://idpf.org/epub/vocab/package/meta/#":    true,
		"http://idpf.org/epub/vocab/package/link/#":    true,
		"http://idpf.org/epub/vocab/package/item/#":    true,
		"http://idpf.org/epub/vocab/package/itemref/#": true,
	}

	// Reserved prefixes and their canonical URIs
	reservedPrefixes := map[string]string{
		"a11y":      "http://www.idpf.org/epub/vocab/package/a11y/#",
		"dcterms":   "http://purl.org/dc/terms/",
		"marc":      "http://id.loc.gov/vocabulary/",
		"media":     "http://www.idpf.org/epub/vocab/overlays/#",
		"onix":      "http://www.editeur.org/ONIX/book/codelists/current.html#",
		"rendition": "http://www.idpf.org/vocab/rendition/#",
		"schema":    "http://schema.org/",
		"xsd":       "http://www.w3.org/2001/XMLSchema#",
	}

	dcElementsNamespace := "http://purl.org/dc/elements/1.1/"

	for prefix, uri := range declared {
		// OPF-007b: default vocabulary URI re-mapping
		if defaultVocabURIs[uri] {
			r.Add(report.Error, "OPF-007b",
				fmt.Sprintf("Prefix '%s' must not be used for default vocabulary '%s'", prefix, uri))
		}

		// OPF-007c: Dublin Core elements namespace
		if uri == dcElementsNamespace {
			r.Add(report.Error, "OPF-007c",
				fmt.Sprintf("Prefix '%s' must not be mapped to the Dublin Core elements namespace", prefix))
		}

		// OPF-007: overriding reserved prefix with different URI
		if canonicalURI, isReserved := reservedPrefixes[prefix]; isReserved {
			if uri != canonicalURI {
				r.Add(report.Warning, "OPF-007",
					fmt.Sprintf("Reserved prefix '%s' is re-declared with a different URI", prefix))
			}
		}
	}
}

// checkUndeclaredPrefixes checks that prefixed properties in metadata,
// manifest, and spine reference declared or reserved prefixes.
// OPF-028: undeclared prefix
func checkUndeclaredPrefixes(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	// Build the set of available prefixes (declared + reserved)
	available := make(map[string]bool)

	// Parse declared prefixes
	if pkg.Prefix != "" {
		parts := strings.Fields(pkg.Prefix)
		i := 0
		for i < len(parts) {
			prefix := parts[i]
			if strings.HasSuffix(prefix, ":") {
				available[strings.TrimSuffix(prefix, ":")] = true
				i += 2 // skip URI
			} else {
				i++
			}
		}
	}

	// Reserved prefixes are always available
	for _, rp := range []string{"a11y", "dcterms", "marc", "media", "onix", "rendition", "schema", "xsd", "msv", "prism"} {
		available[rp] = true
	}

	checkProp := func(prop string) {
		if !strings.Contains(prop, ":") {
			return
		}
		parts := strings.SplitN(prop, ":", 2)
		prefix := parts[0]
		if prefix == "" || parts[1] == "" {
			return // malformed, handled elsewhere
		}
		if !available[prefix] {
			r.Add(report.Error, "OPF-028",
				fmt.Sprintf("Undeclared prefix '%s' used in property '%s'", prefix, prop))
		}
	}

	// Check primary meta properties
	for _, meta := range pkg.PrimaryMetas {
		checkProp(meta.Property)
	}
	// Check meta refines properties
	for _, meta := range pkg.MetaRefines {
		checkProp(meta.Property)
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
		_, ok := manifestByID[item.MediaOverlay]
		if !ok {
			r.Add(report.Error, "OPF-044",
				fmt.Sprintf("Media Overlay Document referenced by '%s' could not be found: '%s'", item.Href, item.MediaOverlay))
		}
	}
}

// RSC-005/OPF-063: spine page-map attribute is not allowed (Adobe extension)
func checkSpinePageMap(ep *epub.EPUB, pkg *epub.Package, r *report.Report) {
	if pkg.SpinePageMap == "" {
		return
	}
	// OPF-062: Adobe page-map attribute on spine element
	r.Add(report.Usage, "OPF-062", "Found Adobe page-map attribute on spine element in opf file")
	r.Add(report.Error, "RSC-005", `attribute "page-map" not allowed here`)

	// OPF-063: page-map attribute must reference a valid manifest item
	manifestIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestIDs[item.ID] = true
		}
	}
	if !manifestIDs[pkg.SpinePageMap] {
		r.Add(report.Warning, "OPF-063",
			fmt.Sprintf("The 'page-map' attribute references an item '%s' that could not be found in the manifest", pkg.SpinePageMap))
	}
}

// OPF-099: manifest must not reference the OPF file itself
func checkManifestSelfReference(ep *epub.EPUB, pkg *epub.Package, r *report.Report) {
	opfPath := ep.RootfilePath
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		resolved := ep.ResolveHref(item.Href)
		if resolved == opfPath {
			r.Add(report.Error, "OPF-099",
				fmt.Sprintf("The package document '%s' references itself in the manifest", opfPath))
			return
		}
	}
}


// RSC-032: fallback chain must ultimately resolve to a core media type
func checkFallbackChainResolves(pkg *epub.Package, r *report.Report) {
	// Build ID → item map
	byID := make(map[string]epub.ManifestItem)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			byID[item.ID] = item
		}
	}

	// Build set of items that are themselves fallbacks for other items.
	// We only check the top of a fallback chain, not intermediate items.
	isFallbackTarget := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.Fallback != "" {
			isFallbackTarget[item.Fallback] = true
		}
	}

	for _, item := range pkg.Manifest {
		if item.Fallback == "" {
			continue
		}
		// Skip items that are themselves fallback targets — only check chain roots
		if isFallbackTarget[item.ID] {
			continue
		}
		mt := item.MediaType
		if idx := strings.Index(mt, ";"); idx >= 0 {
			mt = strings.TrimSpace(mt[:idx])
		}
		if coreMediaTypes[mt] || isFontMediaType(mt) {
			continue // Core type, no fallback chain needed
		}

		// Walk fallback chain to find if any item resolves to core type
		visited := make(map[string]bool)
		current := item.Fallback
		resolved := false
		isCircular := false
		for current != "" {
			if visited[current] {
				// Detected a cycle — OPF-045 handles this, skip RSC-032
				isCircular = true
				break
			}
			visited[current] = true
			next, ok := byID[current]
			if !ok {
				break // Broken chain - OPF-040 already reported this
			}
			nextMt := next.MediaType
			if idx := strings.Index(nextMt, ";"); idx >= 0 {
				nextMt = strings.TrimSpace(nextMt[:idx])
			}
			if coreMediaTypes[nextMt] {
				resolved = true
				break
			}
			current = next.Fallback
		}

		if !resolved && !isCircular {
			r.Add(report.Error, "RSC-032",
				fmt.Sprintf("Fallback chain for manifest item '%s' does not resolve to a core media type", item.ID))
		}
	}
}

// OPF-093: local (non-remote) <link> elements in the OPF metadata section
// must have a media-type attribute.
// RSC-007w: <link> elements pointing to resources missing from the EPUB container.
func checkPackageMetadataLinks(ep *epub.EPUB, pkg *epub.Package, r *report.Report) {
	for _, link := range pkg.MetadataLinks {
		href := link.Href
		if href == "" {
			continue
		}
		// Only check non-remote (local) resources
		if isRemoteURL(href) {
			continue
		}
		// OPF-093: local link must have media-type
		if link.MediaType == "" {
			r.Add(report.Error, "OPF-093",
				fmt.Sprintf("Metadata link to local resource '%s' is missing required 'media-type' attribute", href))
		}
		// RSC-007w: local link target must exist in the EPUB
		// Strip fragment before resolving (e.g. content_001.xhtml#meta-json)
		hrefPath := href
		if idx := strings.Index(hrefPath, "#"); idx >= 0 {
			hrefPath = hrefPath[:idx]
		}
		target := ep.ResolveHref(hrefPath)
		if _, exists := ep.Files[target]; !exists {
			r.Add(report.Warning, "RSC-007w",
				fmt.Sprintf("Metadata link resource '%s' could not be found in the container", href))
		}
	}
}

// OPF-096: all non-linear spine items (linear="no") must be reachable via
// the navigation document or a hyperlink from a linear content document.
func checkSpineNonLinearReachable(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil {
		return
	}
	pkg := ep.Package

	// Build map of all manifest items by ID
	manifestByID := make(map[string]epub.ManifestItem)
	for _, item := range pkg.Manifest {
		manifestByID[item.ID] = item
	}

	// Collect non-linear spine items
	type nonLinearItem struct {
		id   string
		path string
	}
	var nonLinear []nonLinearItem
	for _, ref := range pkg.Spine {
		if strings.EqualFold(ref.Linear, "no") {
			if item, ok := manifestByID[ref.IDRef]; ok && item.Href != "\x00MISSING" {
				nonLinear = append(nonLinear, nonLinearItem{
					id:   ref.IDRef,
					path: ep.ResolveHref(item.Href),
				})
			}
		}
	}
	if len(nonLinear) == 0 {
		return
	}

	// Build set of reachable paths: start with nav doc links
	reachable := make(map[string]bool)

	// Find nav document and collect its links
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		if !hasProperty(item.Properties, "nav") {
			continue
		}
		navPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(navPath)
		if err != nil {
			continue
		}
		navDir := path.Dir(navPath)
		// Collect all hrefs from nav document (scan <a href="..."> elements)
		navDecoder := xml.NewDecoder(strings.NewReader(string(data)))
		for {
			tok, err := navDecoder.Token()
			if err != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			if se.Name.Local != "a" {
				continue
			}
			for _, attr := range se.Attr {
				if attr.Name.Local != "href" {
					continue
				}
				u, err := url.Parse(attr.Value)
				if err != nil || u.Scheme != "" {
					continue
				}
				if u.Path == "" {
					continue
				}
				target := resolvePath(navDir, u.Path)
				reachable[target] = true
			}
		}
	}

	// Collect hyperlinks from all linear content documents
	for _, ref := range pkg.Spine {
		if strings.EqualFold(ref.Linear, "no") {
			continue
		}
		item, ok := manifestByID[ref.IDRef]
		if !ok || item.Href == "\x00MISSING" {
			continue
		}
		docPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(docPath)
		if err != nil {
			continue
		}
		docDir := path.Dir(docPath)
		// Scan for href attributes in a elements
		decoder := xml.NewDecoder(strings.NewReader(string(data)))
		for {
			tok, err := decoder.Token()
			if err != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			if se.Name.Local != "a" {
				continue
			}
			for _, attr := range se.Attr {
				if attr.Name.Local != "href" {
					continue
				}
				u, err := url.Parse(attr.Value)
				if err != nil || u.Scheme != "" {
					continue
				}
				if u.Path == "" {
					continue
				}
				target := resolvePath(docDir, u.Path)
				reachable[target] = true
			}
		}
	}

	// Check whether any linear spine item has scripted content
	hasScripted := false
	for _, ref := range pkg.Spine {
		if strings.EqualFold(ref.Linear, "no") {
			continue
		}
		if item, ok := manifestByID[ref.IDRef]; ok {
			if hasProperty(item.Properties, "scripted") {
				hasScripted = true
				break
			}
		}
	}

	// Report non-linear items that are not reachable
	for _, item := range nonLinear {
		if !reachable[item.path] {
			if hasScripted {
				// OPF-096b: cannot verify reachability because scripted content may link dynamically
				r.Add(report.Usage, "OPF-096b",
					fmt.Sprintf("Non-linear spine item '%s' may be reachable via scripted content, but this cannot be verified statically", item.id))
			} else {
				r.Add(report.Error, "OPF-096",
					fmt.Sprintf("Non-linear spine item '%s' is not reachable from a linear content document or the navigation document", item.id))
			}
		}
	}
}

// RSC-029: data URLs must not be used in manifest item hrefs or package link hrefs
func checkDataURLsInManifest(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.Href)), "data:") {
			r.Add(report.Error, "RSC-029",
				fmt.Sprintf("Data URL used in manifest item href '%s'", item.ID))
		}
	}
	for _, link := range pkg.MetadataLinks {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(link.Href)), "data:") {
			r.Add(report.Error, "RSC-029",
				fmt.Sprintf("Data URL used in package link href"))
		}
	}
}

// RSC-030: file URLs must not be used in metadata links (manifest items checked in references.go)
func checkFileURLsInOPF(pkg *epub.Package, r *report.Report) {
	for _, link := range pkg.MetadataLinks {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(link.Href)), "file:") {
			r.Add(report.Error, "RSC-030",
				fmt.Sprintf("File URL used in package link href '%s'", link.Href))
		}
	}
}

// RSC-033: URL query strings must not be used in local resource references
func checkQueryStringsInOPF(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		// Skip remote URLs and data URLs
		if isRemoteURL(item.Href) || strings.HasPrefix(strings.ToLower(item.Href), "data:") {
			continue
		}
		if strings.Contains(item.Href, "?") {
			r.Add(report.Error, "RSC-033",
				fmt.Sprintf("URL query string found in manifest item href '%s'", item.Href))
		}
	}
	for _, link := range pkg.MetadataLinks {
		if isRemoteURL(link.Href) || strings.HasPrefix(strings.ToLower(link.Href), "data:") {
			continue
		}
		if strings.Contains(link.Href, "?") {
			r.Add(report.Error, "RSC-033",
				fmt.Sprintf("URL query string found in package link href '%s'", link.Href))
		}
	}
}

// OPF-028: dcterms:modified must occur exactly once as non-refining metadata
func checkDCTermsModifiedCount(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	// Count refining dcterms:modified meta elements
	refiningCount := 0
	for _, mr := range pkg.MetaRefines {
		if mr.Property == "dcterms:modified" && mr.Refines != "" {
			refiningCount++
		}
	}
	// ModifiedCount tracks total dcterms:modified; subtract refining ones
	nonRefiningCount := pkg.ModifiedCount - refiningCount
	if nonRefiningCount > 1 {
		r.Add(report.Error, "OPF-028",
			fmt.Sprintf("dcterms:modified must not occur more than once as non-refining meta, found %d", nonRefiningCount))
	}
}

// OPF-053: dc:date with unknown/deprecated format (EPUB 3 only)
func checkDCDateWarnings(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	if len(pkg.Metadata.Dates) > 1 {
		r.Add(report.Error, "OPF-053b",
			`element "dc:date" not allowed here; only one dc:date element is allowed`)
	}
	for _, date := range pkg.Metadata.Dates {
		if date != "" && !w3cdtfRe.MatchString(strings.TrimSpace(date)) {
			r.Add(report.Warning, "OPF-053",
				fmt.Sprintf("Date value '%s' does not follow recommended syntax", date))
		}
	}
}

// OPF-052: dc:creator role validation
func checkDCCreatorRole(pkg *epub.Package, r *report.Report) {
	// MARC relator codes - comprehensive list from Library of Congress
	// https://www.loc.gov/marc/relators/relaterm.html
	marcCodes := map[string]bool{
		"abr": true, "act": true, "adp": true, "aft": true, "anl": true,
		"anm": true, "ann": true, "ant": true, "ape": true, "apl": true,
		"app": true, "aqt": true, "arc": true, "ard": true, "arr": true,
		"art": true, "asg": true, "asn": true, "att": true, "auc": true,
		"aud": true, "aui": true, "aus": true, "aut": true, "bdd": true,
		"bjd": true, "bkd": true, "bkp": true, "blw": true, "bnd": true,
		"bpd": true, "brd": true, "brl": true, "bsl": true, "cas": true,
		"ccp": true, "chr": true, "clb": true, "cli": true, "cll": true,
		"clr": true, "clt": true, "cmm": true, "cmp": true, "cmt": true,
		"cnd": true, "cng": true, "cns": true, "coe": true, "col": true,
		"com": true, "con": true, "cor": true, "cos": true, "cot": true,
		"cou": true, "cov": true, "cpc": true, "cpe": true, "cph": true,
		"cpl": true, "cpt": true, "cre": true, "crp": true, "crr": true,
		"crt": true, "csl": true, "csp": true, "cst": true, "ctb": true,
		"cte": true, "ctg": true, "ctr": true, "cts": true, "ctt": true,
		"cur": true, "cwt": true, "dbp": true, "dfd": true, "dfe": true,
		"dft": true, "dgg": true, "dgs": true, "dis": true, "dln": true,
		"dnc": true, "dnr": true, "dpc": true, "dpt": true, "drm": true,
		"drt": true, "dsr": true, "dst": true, "dtc": true, "dte": true,
		"dtm": true, "dto": true, "dub": true, "edc": true, "edm": true,
		"edt": true, "egr": true, "elg": true, "elt": true, "eng": true,
		"enj": true, "etr": true, "evp": true, "exp": true, "fac": true,
		"fds": true, "fld": true, "flm": true, "fmd": true, "fmk": true,
		"fmo": true, "fmp": true, "fnd": true, "fpy": true, "frg": true,
		"gis": true, "grt": true, "his": true, "hnr": true, "hst": true,
		"ill": true, "ilu": true, "ins": true, "inv": true, "isb": true,
		"itr": true, "ive": true, "ivr": true, "jud": true, "jug": true,
		"lbr": true, "lbt": true, "ldr": true, "led": true, "lee": true,
		"lel": true, "len": true, "let": true, "lgd": true, "lie": true,
		"lil": true, "lit": true, "lsa": true, "lse": true, "lso": true,
		"ltg": true, "lyr": true, "mcp": true, "mdc": true, "med": true,
		"mfp": true, "mfr": true, "mod": true, "mon": true, "mrb": true,
		"mrk": true, "msd": true, "mte": true, "mtk": true, "mus": true,
		"nrt": true, "opn": true, "org": true, "orm": true, "osp": true,
		"oth": true, "own": true, "pan": true, "pat": true, "pbd": true,
		"pbl": true, "pdr": true, "pfr": true, "pht": true, "plt": true,
		"pma": true, "pmn": true, "pop": true, "ppm": true, "ppt": true,
		"pra": true, "prc": true, "prd": true, "pre": true, "prf": true,
		"prg": true, "prm": true, "prn": true, "pro": true, "prp": true,
		"prs": true, "prt": true, "prv": true, "pta": true, "pte": true,
		"ptf": true, "pth": true, "ptt": true, "pup": true, "rbr": true,
		"rcd": true, "rce": true, "rcp": true, "rdd": true, "red": true,
		"ren": true, "res": true, "rev": true, "rpc": true, "rps": true,
		"rpt": true, "rpy": true, "rse": true, "rsg": true, "rsp": true,
		"rsr": true, "rst": true, "rth": true, "rtm": true, "sad": true,
		"sce": true, "scl": true, "scr": true, "sds": true, "sec": true,
		"sgd": true, "sgn": true, "sht": true, "sll": true, "sng": true,
		"spk": true, "spn": true, "spy": true, "srv": true, "std": true,
		"stg": true, "stl": true, "stm": true, "stn": true, "str": true,
		"tcd": true, "tch": true, "ths": true, "tld": true, "tlp": true,
		"trc": true, "trl": true, "tyd": true, "tyg": true, "uvp": true,
		"vac": true, "vdg": true, "wac": true, "wal": true, "wam": true,
		"wat": true, "wdc": true, "wde": true, "win": true, "wit": true,
		"wpr": true, "wst": true, "cog": true,
	}
	for _, c := range pkg.Metadata.Creators {
		if c.Role != "" && !marcCodes[strings.ToLower(c.Role)] {
			r.Add(report.Error, "OPF-052",
				fmt.Sprintf("Unknown MARC relator code '%s' for dc:creator", c.Role))
		}
	}
	for _, c := range pkg.Metadata.Contributors {
		if c.Role != "" && !marcCodes[strings.ToLower(c.Role)] {
			r.Add(report.Error, "OPF-052",
				fmt.Sprintf("Unknown MARC relator code '%s' for dc:contributor", c.Role))
		}
	}
}

// OPF-054: dc:date empty value
func checkDCDateEmpty(pkg *epub.Package, r *report.Report) {
	for _, date := range pkg.Metadata.Dates {
		if strings.TrimSpace(date) == "" {
			r.Add(report.Error, "OPF-054",
				"Element dc:date must not be empty")
		}
	}
}

// OPF-065: refines references cycles
func checkRefinesCycle(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	// Build a map of meta element IDs to their refines targets
	refinesMap := make(map[string]string) // meta ID -> refines target ID
	for _, mr := range pkg.MetaRefines {
		if mr.ID != "" && mr.Refines != "" {
			target := strings.TrimPrefix(mr.Refines, "#")
			refinesMap[mr.ID] = target
		}
	}

	// Check for cycles; track already-reported nodes to avoid duplicate reports
	reported := make(map[string]bool)
	for startID := range refinesMap {
		if reported[startID] {
			continue
		}
		visited := make(map[string]bool)
		current := startID
		for {
			if visited[current] {
				// Mark all nodes in this cycle as reported
				for id := range visited {
					reported[id] = true
				}
				r.Add(report.Error, "OPF-065",
					fmt.Sprintf("Metadata refines cycle detected starting from '%s'", startID))
				break
			}
			visited[current] = true
			next, ok := refinesMap[current]
			if !ok {
				break
			}
			current = next
		}
	}
}

// OPF-098: link must not target a manifest item ID
func checkLinkTargetNotManifestID(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	manifestIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestIDs[item.ID] = true
		}
	}
	for _, link := range pkg.MetadataLinks {
		if link.Href == "" {
			continue
		}
		if strings.HasPrefix(link.Href, "#") {
			target := strings.TrimPrefix(link.Href, "#")
			if manifestIDs[target] {
				r.Add(report.Error, "OPF-098",
					fmt.Sprintf("Link must not reference a manifest item ID '%s'", target))
			}
		}
	}
}

// checkLinkRelation validates link rel values and associated constraints:
// OPF-086: deprecated link rel values
// OPF-089: alternate must not be paired with other keywords
// OPF-093: record/voicing links require media-type (even when remote)
// OPF-094: record/voicing links require media-type attribute
// OPF-095: voicing link must have audio media type
// RSC-005: record must not have refines; voicing must have refines
// Link properties: empty → RSC-005, unknown → OPF-027
func checkLinkRelation(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	deprecatedRels := map[string]bool{
		"marc21xml-record": true,
		"mods-record":      true,
		"onix-record":      true,
		"xmp-record":       true,
		"xml-signature":    true,
	}

	// Known link properties (D.4.2)
	knownLinkProps := map[string]bool{
		"onix": true,
	}

	for _, link := range pkg.MetadataLinks {
		rels := strings.Fields(link.Rel)
		relSet := make(map[string]bool)
		for _, rel := range rels {
			relSet[rel] = true
		}

		// OPF-086: deprecated rel values
		for _, rel := range rels {
			if deprecatedRels[rel] {
				r.Add(report.Warning, "OPF-086",
					fmt.Sprintf(`"%s" is deprecated`, rel))
			}
		}

		// OPF-089: alternate must not be paired with other keywords
		if relSet["alternate"] && len(rels) > 1 {
			r.Add(report.Error, "OPF-089",
				"The 'alternate' link relationship must not be paired with other keywords")
		}

		// OPF-094: record and voicing require media-type (even when remote)
		if relSet["record"] || relSet["voicing"] {
			if link.MediaType == "" {
				r.Add(report.Error, "OPF-094",
					fmt.Sprintf("Link with rel '%s' must have a 'media-type' attribute", link.Rel))
			}
		}

		// OPF-093: deprecated *-record and xml-signature also need media-type
		for _, rel := range rels {
			if deprecatedRels[rel] && link.MediaType == "" {
				r.Add(report.Error, "OPF-093",
					fmt.Sprintf("Metadata link to resource '%s' is missing required 'media-type' attribute", link.Href))
			}
		}

		// OPF-095: voicing must have audio media type
		if relSet["voicing"] && link.MediaType != "" {
			if !strings.HasPrefix(link.MediaType, "audio/") {
				r.Add(report.Error, "OPF-095",
					fmt.Sprintf("A 'voicing' link resource must have an audio media type, but found '%s'", link.MediaType))
			}
		}

		// RSC-005: record must not have refines
		if relSet["record"] && link.Refines != "" {
			r.Add(report.Error, "OPF-088",
				`link with rel "record" must not have a "refines" attribute`)
		}

		// RSC-005: voicing must have refines
		if relSet["voicing"] && link.Refines == "" {
			r.Add(report.Error, "OPF-088",
				`link with rel "voicing" must have a "refines" attribute`)
		}

		// Link properties validation
		if link.Properties != "" {
			trimmed := strings.TrimSpace(link.Properties)
			if trimmed == "" {
				// Empty/whitespace-only properties
				r.Add(report.Error, "OPF-088",
					`value of attribute "properties" is invalid; must be a string with length at least 1`)
			} else {
				// Check each property value
				for _, prop := range strings.Fields(trimmed) {
					if strings.Contains(prop, ":") {
						// Prefixed property — allowed if prefix is declared
						continue
					}
					if !knownLinkProps[prop] {
						r.Add(report.Error, "OPF-027",
							fmt.Sprintf("Undefined property '%s'", prop))
					}
				}
			}
		}
	}
}

// OPF-026: metadata property names must be defined and well-formed
func checkMetaPropertyDefined(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	knownProperties := map[string]bool{
		"alternate-script":        true,
		"authority":               true,
		"belongs-to-collection":   true,
		"collection-type":         true,
		"display-seq":             true,
		"file-as":                 true,
		"group-position":          true,
		"identifier-type":         true,
		"meta-auth":               true,
		"pageBreakSource":         true,
		"role":                    true,
		"scheme":                  true,
		"source-of":               true,
		"term":                    true,
		"title-type":              true,
	}

	checkPropValid := func(prop string) {
		if prop == "" {
			return
		}
		// Check for malformed prefixed properties (e.g., "foo:" or ":bar")
		if strings.Contains(prop, ":") {
			parts := strings.SplitN(prop, ":", 2)
			if parts[0] == "" || parts[1] == "" {
				r.Add(report.Error, "OPF-026",
					fmt.Sprintf("Metadata property '%s' is not well-formed", prop))
			}
			return
		}
		if !knownProperties[prop] {
			r.Add(report.Error, "OPF-026",
				fmt.Sprintf("Metadata property '%s' is not defined", prop))
		}
	}

	for _, mr := range pkg.MetaRefines {
		checkPropValid(mr.Property)
	}
	for _, pm := range pkg.PrimaryMetas {
		checkPropValid(pm.Property)
	}
}

// OPF-004c: dcterms:modified must occur as non-refining meta
func checkDCTermsModifiedNonRefining(pkg *epub.Package, r *report.Report) {
	// Covered by existing OPF-004 check
}

// checkRenditionProperties validates rendition global properties (value, cardinality, refines)
// and spine override mutual exclusivity.
func checkRenditionProperties(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	validLayout := map[string]bool{"reflowable": true, "pre-paginated": true}
	validOrientation := map[string]bool{"auto": true, "landscape": true, "portrait": true}
	validSpread := map[string]bool{"auto": true, "landscape": true, "both": true, "none": true, "portrait": true}
	validFlow := map[string]bool{"paginated": true, "scrolled-doc": true, "scrolled-continuous": true, "auto": true}
	viewportRe := regexp.MustCompile(`^\s*width\s*=\s*\d+\s*,\s*height\s*=\s*\d+\s*$`)

	// Track counts for cardinality and deprecation
	type propInfo struct {
		count    int
		values   []string
		refining int // count of meta with refines attribute
	}
	globalProps := map[string]*propInfo{}
	countProp := func(prop, value string, refining bool) {
		if globalProps[prop] == nil {
			globalProps[prop] = &propInfo{}
		}
		if refining {
			globalProps[prop].refining++
		} else {
			globalProps[prop].count++
			globalProps[prop].values = append(globalProps[prop].values, value)
		}
	}

	// Scan all metas (both primary and refining) for rendition properties
	for _, pm := range pkg.PrimaryMetas {
		switch pm.Property {
		case "rendition:layout", "rendition:orientation", "rendition:spread",
			"rendition:flow", "rendition:viewport":
			countProp(pm.Property, pm.Value, false)
		}
	}
	for _, mr := range pkg.MetaRefines {
		switch mr.Property {
		case "rendition:layout", "rendition:orientation", "rendition:spread",
			"rendition:flow", "rendition:viewport":
			countProp(mr.Property, mr.Value, mr.Refines != "")
		}
	}

	// Valid value descriptions for error messages
	valueDesc := map[string]string{
		"rendition:layout":      `"reflowable" or "pre-paginated"`,
		"rendition:orientation": `"auto", "landscape", or "portrait"`,
		"rendition:spread":      `"auto", "landscape", "both", "none", or "portrait"`,
		"rendition:flow":        `"paginated", "scrolled-doc", "scrolled-continuous", or "auto"`,
	}

	// Check each rendition property
	checkGlobalRendition := func(prop string, validValues map[string]bool) {
		info := globalProps[prop]
		if info == nil {
			return
		}
		// Value validation
		for _, val := range info.values {
			if val == "" {
				r.Add(report.Error, "OPF-088",
					`character content of element "meta" invalid`)
				r.Add(report.Error, "OPF-088",
					fmt.Sprintf(`The value of the "%s" property must be %s`, prop, valueDesc[prop]))
			} else if !validValues[val] {
				r.Add(report.Error, "OPF-088",
					fmt.Sprintf(`The value of the "%s" property must be %s`, prop, valueDesc[prop]))
			}
		}
		// Cardinality: must not appear more than once
		if info.count > 1 {
			r.Add(report.Error, "OPF-088",
				fmt.Sprintf(`The "%s" property must not occur more than one time as a global value`, prop))
		}
		// Refines prohibition
		if info.refining > 0 {
			r.Add(report.Error, "OPF-088",
				fmt.Sprintf(`The "%s" property must not have a refines attribute`, prop))
		}
	}

	checkGlobalRendition("rendition:layout", validLayout)
	checkGlobalRendition("rendition:orientation", validOrientation)
	checkGlobalRendition("rendition:spread", validSpread)
	checkGlobalRendition("rendition:flow", validFlow)

	// Viewport: value validation + deprecation
	vpInfo := globalProps["rendition:viewport"]
	if vpInfo != nil {
		// Deprecation: always warn for each viewport occurrence (global + refining)
		total := vpInfo.count + vpInfo.refining
		for i := 0; i < total; i++ {
			r.Add(report.Warning, "OPF-086",
				`The "rendition:viewport" property is deprecated`)
		}
		// Value syntax
		for _, val := range vpInfo.values {
			if val != "" && !viewportRe.MatchString(val) {
				r.Add(report.Error, "OPF-088",
					`The value of the "rendition:viewport" property must be of the form 'width=<number>, height=<number>'`)
			}
		}
		// Cardinality
		if vpInfo.count > 1 {
			r.Add(report.Error, "OPF-088",
				`The "rendition:viewport" property must not occur more than one time as a global value`)
		}
		if vpInfo.refining > 0 {
			r.Add(report.Error, "OPF-088",
				`The "rendition:viewport" property must not be used in a meta element to refine a publication resource`)
		}
	}

	// Spread "portrait" deprecation (global)
	spreadInfo := globalProps["rendition:spread"]
	if spreadInfo != nil {
		for _, val := range spreadInfo.values {
			if val == "portrait" {
				r.Add(report.Warning, "OPF-086",
					`The "rendition:spread" value "portrait" is deprecated`)
			}
		}
	}

	// Spine override mutual exclusivity
	type overrideGroup struct {
		name    string
		values  []string
	}
	groups := []overrideGroup{
		{"rendition:layout", []string{"rendition:layout-reflowable", "rendition:layout-pre-paginated"}},
		{"rendition:orientation", []string{"rendition:orientation-auto", "rendition:orientation-landscape", "rendition:orientation-portrait"}},
		{"rendition:spread", []string{"rendition:spread-auto", "rendition:spread-landscape", "rendition:spread-both", "rendition:spread-none", "rendition:spread-portrait"}},
		{"rendition:flow", []string{"rendition:flow-paginated", "rendition:flow-scrolled-doc", "rendition:flow-scrolled-continuous", "rendition:flow-auto"}},
		{"page-spread", []string{"page-spread-left", "page-spread-right", "page-spread-center", "rendition:page-spread-center"}},
	}

	for _, ref := range pkg.Spine {
		if ref.Properties == "" {
			continue
		}
		props := strings.Fields(ref.Properties)
		for _, g := range groups {
			var found []string
			for _, p := range props {
				for _, v := range g.values {
					if p == v {
						found = append(found, p)
					}
				}
			}
			if len(found) > 1 {
				r.Add(report.Error, "OPF-088",
					fmt.Sprintf("Properties '%s' are mutually exclusive on the same itemref", strings.Join(found, "', '")))
			}
		}
		// Deprecated spine spread-portrait
		for _, p := range props {
			if p == "rendition:spread-portrait" {
				r.Add(report.Warning, "OPF-086",
					`The "rendition:spread-portrait" spine override property is deprecated`)
			}
		}
	}

	// Check for unknown rendition: properties in primary metas (OPF-027)
	knownRendition := map[string]bool{
		"rendition:layout": true, "rendition:orientation": true,
		"rendition:spread": true, "rendition:flow": true,
		"rendition:viewport": true, "rendition:align-x-center": true,
	}
	for _, pm := range pkg.PrimaryMetas {
		if strings.HasPrefix(pm.Property, "rendition:") && !knownRendition[pm.Property] {
			r.Add(report.Error, "OPF-027",
				fmt.Sprintf("Undefined property '%s'", pm.Property))
		}
	}
}

// OPF-092: language tag validation
func checkLanguageTags(ep *epub.EPUB, pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	// Check dc:language values (malformed BCP 47)
	for _, lang := range pkg.Metadata.Languages {
		if lang == "" {
			continue // Empty handled elsewhere
		}
		if !bcp47Re.MatchString(strings.TrimSpace(lang)) {
			r.Add(report.Error, "OPF-092",
				fmt.Sprintf("Language tag '%s' is not well-formed", lang))
		}
	}
	// Check all xml:lang attributes found in the OPF (on any element)
	for _, lang := range pkg.AllXMLLangs {
		if lang == "" {
			continue // empty xml:lang is valid (resets inheritance)
		}
		if lang != strings.TrimSpace(lang) {
			r.Add(report.Error, "OPF-092",
				fmt.Sprintf("xml:lang attribute value '%s' has leading/trailing whitespace", lang))
		} else if !bcp47Re.MatchString(lang) {
			r.Add(report.Error, "OPF-092",
				fmt.Sprintf("xml:lang language tag '%s' is not well-formed", lang))
		}
	}
	// Check link hreflang attributes
	for _, link := range pkg.MetadataLinks {
		if link.Hreflang == "" {
			continue
		}
		lang := link.Hreflang
		if lang != strings.TrimSpace(lang) {
			r.Add(report.Error, "OPF-092",
				fmt.Sprintf("hreflang attribute value '%s' has leading/trailing whitespace", lang))
		} else if !bcp47Re.MatchString(lang) {
			r.Add(report.Error, "OPF-092",
				fmt.Sprintf("hreflang language tag '%s' is not well-formed", lang))
		}
	}
}

// OPF-055: empty dc:title warning
func checkDCTitleEmptyWarning(pkg *epub.Package, r *report.Report) {
	for _, title := range pkg.Metadata.Titles {
		if strings.TrimSpace(title.Value) == "" && title.Value != "" {
			r.Add(report.Warning, "OPF-055",
				"dc:title contains only whitespace")
		}
	}
}

// OPF-012: cover-image property uniqueness
func checkCoverImageUnique(pkg *epub.Package, r *report.Report) {
	count := 0
	for _, item := range pkg.Manifest {
		if hasProperty(item.Properties, "cover-image") {
			count++
		}
	}
	if count > 1 {
		r.Add(report.Error, "OPF-012",
			fmt.Sprintf("The 'cover-image' property must occur at most once, found %d", count))
	}
}


// OPF-050: NCX identification
func checkNCXIdentification(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	// Find NCX item in the manifest
	var ncxID string
	for _, item := range pkg.Manifest {
		if item.MediaType == "application/x-dtbncx+xml" {
			ncxID = item.ID
			break
		}
	}

	// If toc attribute is set, it must reference an NCX document
	if pkg.SpineToc != "" {
		for _, item := range pkg.Manifest {
			if item.ID == pkg.SpineToc && item.MediaType != "application/x-dtbncx+xml" {
				r.Add(report.Error, "OPF-050",
					fmt.Sprintf("The spine toc attribute must reference an NCX document, but '%s' has media-type '%s'", pkg.SpineToc, item.MediaType))
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("The spine toc attribute value '%s' does not reference an NCX document", pkg.SpineToc))
				break
			}
		}
	}

	// If NCX is present but toc attribute is missing
	if ncxID != "" && pkg.SpineToc == "" {
		r.Add(report.Error, "OPF-050",
			"When an NCX document is present, the toc attribute must be set on the spine element")
	}
}

// checkMetaSchemeValid validates the scheme attribute on meta elements.
// OPF-027: scheme value must not be unprefixed unknown value
// RSC-005 (schema): scheme value must be NMTOKEN (no spaces)
func checkMetaSchemeValid(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	for _, ms := range pkg.MetaSchemes {
		scheme := ms.Scheme
		// Check if scheme value contains spaces (NMTOKEN violation - schema error)
		if strings.Contains(strings.TrimSpace(scheme), " ") {
			r.Add(report.Error, "OPF-025a",
				fmt.Sprintf("The 'scheme' attribute value '%s' is not a valid NMTOKEN", scheme))
			// Also report OPF-025 for the individual invalid tokens
			r.Add(report.Error, "OPF-025",
				fmt.Sprintf("The 'scheme' attribute value '%s' contains invalid values", scheme))
			continue
		}
		// Check if scheme is unprefixed (no colon = no namespace prefix)
		if !strings.Contains(scheme, ":") {
			r.Add(report.Error, "OPF-027",
				fmt.Sprintf("The 'scheme' attribute value '%s' is not a known value with no prefix", scheme))
		}
	}
}

// checkMetaRefinesPropertyRules validates metadata refines property vocabulary rules (EPUB 3.3 D.3).
// Reports RSC-005 for schema-level violations of refines target, cardinality, and value constraints.
func checkMetaRefinesPropertyRules(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	// Build ID-to-element mapping for refines targets
	idMap := pkg.Metadata.IDToElement

	// resolveRefines returns the element type/property that a refines target points to.
	resolveRefines := func(refines string) string {
		target := strings.TrimPrefix(refines, "#")
		if elem, ok := idMap[target]; ok {
			return elem
		}
		return ""
	}

	// Track cardinality: property+refinesTarget → count
	cardinalityMap := make(map[string]int)

	for _, mr := range pkg.MetaRefines {
		target := resolveRefines(mr.Refines)
		cardKey := mr.Property + "|" + mr.Refines

		switch mr.Property {
		case "authority":
			// Must refine a "subject" property
			if target != "subject" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("Property \"authority\" must refine a \"subject\" property"))
			}
		case "term":
			// Must refine a "subject" property
			if target != "subject" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("Property \"term\" must refine a \"subject\" property"))
			}
		case "belongs-to-collection":
			// Can only refine other "belongs-to-collection" properties (or be primary)
			if mr.Refines != "" && target != "belongs-to-collection" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("Property \"belongs-to-collection\" can only refine other \"belongs-to-collection\" properties"))
			}
		case "collection-type":
			// Must refine a "belongs-to-collection" property (cannot be primary)
			if mr.Refines == "" || target != "belongs-to-collection" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("Property \"collection-type\" must refine a \"belongs-to-collection\" property"))
			}
		case "identifier-type":
			// Must refine an "identifier" or "source" property
			if target != "identifier" && target != "source" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("Property \"identifier-type\" must refine an \"identifier\" or \"source\" property"))
			}
		case "role":
			// Must refine a "creator", "contributor", or "publisher" property
			if target != "creator" && target != "contributor" && target != "publisher" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("\"role\" must refine a \"creator\", \"contributor\", or \"publisher\" property"))
			}
		case "title-type":
			// Must refine a "title" property
			if target != "title" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("Property \"title-type\" must refine a \"title\" property"))
			}
		case "source-of":
			// Must refine a "source" property and value must be "pagination"
			if mr.Refines == "" || target != "source" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("The \"source-of\" property must refine a \"source\" property"))
			}
			if mr.Value != "pagination" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("The \"source-of\" property must have the value \"pagination\""))
			}
		case "meta-auth":
			// Deprecated
			r.Add(report.Warning, "RSC-017",
				fmt.Sprintf("the meta-auth property is deprecated"))
		case "media:active-class", "media:playback-active-class":
			// Must not have refines
			if mr.Refines != "" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("\"refines\" must not be used with the %s property", mr.Property))
			}
			// Must define a single class name (no spaces)
			if strings.Contains(strings.TrimSpace(mr.Value), " ") {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("The %s property must define a single class name, but found \"%s\"", mr.Property, mr.Value))
			}
		case "media:duration":
			// Value must be a valid SMIL clock value
			if !isValidClockValue(mr.Value) {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("The \"media:duration\" value \"%s\" must be a valid SMIL3 clock value", mr.Value))
			}
		}

		// Cardinality checks for properties that can only be declared once per refines target
		cardinalityMap[cardKey]++
	}

	// Check for primary metas that must have refines or are deprecated
	for _, m := range pkg.PrimaryMetas {
		switch m.Property {
		case "collection-type":
			r.Add(report.Error, "RSC-005",
				`Property "collection-type" must refine a "belongs-to-collection" property`)
		case "source-of":
			r.Add(report.Error, "RSC-005",
				`The "source-of" property must refine a "source" property`)
		case "meta-auth":
			r.Add(report.Warning, "RSC-017",
				"the meta-auth property is deprecated")
		case "media:active-class", "media:playback-active-class":
			// Must define a single class name (no spaces)
			if strings.Contains(strings.TrimSpace(m.Value), " ") {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("The %s property must define a single class name, but found \"%s\"", m.Property, m.Value))
			}
		case "media:duration":
			// Value must be a valid SMIL clock value
			if !isValidClockValue(m.Value) {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("The \"media:duration\" value \"%s\" must be a valid SMIL3 clock value", m.Value))
			}
		}
	}

	// Check cardinality violations
	seen := make(map[string]bool)
	for _, mr := range pkg.MetaRefines {
		cardKey := mr.Property + "|" + mr.Refines
		if seen[cardKey] {
			continue
		}
		seen[cardKey] = true
		count := cardinalityMap[cardKey]

		switch mr.Property {
		case "collection-type", "display-seq", "file-as", "group-position",
			"identifier-type", "source-of", "title-type":
			if count > 1 {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("\"%s\" cannot be declared more than once to refine the same expression", mr.Property))
			}
		case "authority", "term":
			// Authority and term come in pairs - only one pair per subject
			if count > 1 {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("Only one pair of authority and term properties may refine the same expression"))
			}
		}
	}

	// Check authority-term pairing (only for those correctly refining a subject)
	authorityTargets := make(map[string]bool)
	termTargets := make(map[string]bool)
	for _, mr := range pkg.MetaRefines {
		target := resolveRefines(mr.Refines)
		if target != "subject" {
			continue // Only check pairing for subject-refining metas
		}
		if mr.Property == "authority" {
			authorityTargets[mr.Refines] = true
		}
		if mr.Property == "term" {
			termTargets[mr.Refines] = true
		}
	}
	// Each authority must have a term and vice versa
	for target := range authorityTargets {
		if !termTargets[target] {
			r.Add(report.Error, "RSC-005",
				"A term property must be associated with the authority property")
		}
	}
	for target := range termTargets {
		if !authorityTargets[target] {
			r.Add(report.Error, "RSC-005",
				"An authority property must be associated with the term property")
		}
	}

	// Check that media:active-class and media:playback-active-class appear at most once
	activeClassCount := 0
	playbackActiveClassCount := 0
	for _, m := range pkg.PrimaryMetas {
		switch m.Property {
		case "media:active-class":
			activeClassCount++
		case "media:playback-active-class":
			playbackActiveClassCount++
		}
	}
	if activeClassCount > 1 {
		r.Add(report.Error, "RSC-005",
			"The 'active-class' property must not occur more than one time")
	}
	if playbackActiveClassCount > 1 {
		r.Add(report.Error, "RSC-005",
			"The 'playback-active-class' property must not occur more than one time")
	}
}

// isValidClockValue checks if a string is a valid SMIL clock value.
func isValidClockValue(val string) bool {
	if val == "" {
		return false
	}
	// Full clock: hh:mm:ss(.fff)
	// Partial clock: mm:ss(.fff)
	// Timecount: N(h|min|s|ms)
	clockRe := regexp.MustCompile(`^(\d+:)?[0-5]?\d:[0-5]?\d(\.\d+)?$`)
	timecountRe := regexp.MustCompile(`^\d+(\.\d+)?(h|min|s|ms)$`)
	return clockRe.MatchString(val) || timecountRe.MatchString(val)
}

// checkMediaOverlayOnNonContentDoc checks that media-overlay attribute is only on content documents.
func checkMediaOverlayOnNonContentDoc(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	contentDocTypes := map[string]bool{
		"application/xhtml+xml": true,
		"image/svg+xml":         true,
	}
	for _, item := range pkg.Manifest {
		if item.MediaOverlay != "" && !contentDocTypes[item.MediaType] {
			r.Add(report.Error, "RSC-005",
				fmt.Sprintf("media-overlay attribute is only allowed on EPUB Content Documents, but item \"%s\" has type \"%s\"", item.ID, item.MediaType))
		}
	}
}

// checkMediaOverlayType checks that items referenced by media-overlay are SMIL.
func checkMediaOverlayType(pkg *epub.Package, r *report.Report) {
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
			continue // handled elsewhere
		}
		if target.MediaType != "application/smil+xml" {
			r.Add(report.Error, "RSC-005",
				fmt.Sprintf("The item referenced by media-overlay must be of the \"application/smil+xml\" type, but \"%s\" is \"%s\"",
					target.ID, target.MediaType))
		}
	}
}

// clockToMs parses a SMIL clock value to milliseconds. Returns -1 on error.
func clockToMs(val string) float64 {
	val = strings.TrimSpace(val)
	if val == "" || !isValidClockValue(val) {
		return -1
	}

	// Timecount values
	if strings.HasSuffix(val, "ms") {
		numStr := strings.TrimSuffix(val, "ms")
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return -1
		}
		return f
	}
	if strings.HasSuffix(val, "min") {
		numStr := strings.TrimSuffix(val, "min")
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return -1
		}
		return f * 60000
	}
	if strings.HasSuffix(val, "h") {
		numStr := strings.TrimSuffix(val, "h")
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return -1
		}
		return f * 3600000
	}
	if strings.HasSuffix(val, "s") {
		numStr := strings.TrimSuffix(val, "s")
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return -1
		}
		return f * 1000
	}

	// Clock values
	parts := strings.Split(val, ":")
	switch len(parts) {
	case 3:
		h, _ := strconv.ParseFloat(parts[0], 64)
		m, _ := strconv.ParseFloat(parts[1], 64)
		s, _ := strconv.ParseFloat(parts[2], 64)
		return (h*3600 + m*60 + s) * 1000
	case 2:
		m, _ := strconv.ParseFloat(parts[0], 64)
		s, _ := strconv.ParseFloat(parts[1], 64)
		return (m*60 + s) * 1000
	}
	return -1
}

// checkMediaOverlayDurationMeta checks media overlay duration metadata in package document.
func checkMediaOverlayDurationMeta(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	// Find SMIL overlay items in manifest
	smilItems := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.MediaType == "application/smil+xml" {
			smilItems[item.ID] = true
		}
	}

	// Only check durations when media overlays are actually used
	hasMediaOverlayAttr := false
	for _, item := range pkg.Manifest {
		if item.MediaOverlay != "" {
			hasMediaOverlayAttr = true
			break
		}
	}
	hasMediaDurationMeta := false
	for _, m := range pkg.PrimaryMetas {
		if m.Property == "media:duration" {
			hasMediaDurationMeta = true
			break
		}
	}
	if !hasMediaDurationMeta {
		for _, mr := range pkg.MetaRefines {
			if mr.Property == "media:duration" {
				hasMediaDurationMeta = true
				break
			}
		}
	}
	if !hasMediaOverlayAttr && !hasMediaDurationMeta {
		return
	}

	// Find global media:duration (no refines)
	var globalDuration string
	for _, m := range pkg.PrimaryMetas {
		if m.Property == "media:duration" {
			globalDuration = m.Value
		}
	}

	// Build a reverse map from SMIL item href to its ID for non-fragment refines lookups
	smilHrefToID := make(map[string]string)
	for _, item := range pkg.Manifest {
		if item.MediaType == "application/smil+xml" {
			smilHrefToID[item.Href] = item.ID
		}
	}

	// Find per-item media:duration (refines an overlay item by ID or href)
	itemDurations := make(map[string]string)
	for _, mr := range pkg.MetaRefines {
		if mr.Property == "media:duration" && mr.Refines != "" {
			ref := strings.TrimPrefix(mr.Refines, "#")
			id := ref
			if !smilItems[id] {
				// Try matching by href (when refines is a path, not a fragment)
				if hrefID, ok := smilHrefToID[ref]; ok {
					id = hrefID
				}
			}
			if smilItems[id] {
				itemDurations[id] = mr.Value
			}
		}
	}

	// Check global duration is defined
	if globalDuration == "" {
		r.Add(report.Error, "RSC-005",
			"global media:duration meta element not set")
	}

	// Check each overlay item has a duration only when media-overlay attributes are used
	// (When no media-overlay attrs exist, SMIL items may not be actual overlay documents)
	if hasMediaOverlayAttr {
		for id := range smilItems {
			if _, ok := itemDurations[id]; !ok {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("item media:duration meta element not set for Media Overlay \"%s\"", id))
			}
		}
	}

	// Check total duration matches sum of individual (MED-016)
	if globalDuration != "" && isValidClockValue(globalDuration) {
		globalMs := clockToMs(globalDuration)
		sumMs := 0.0
		allValid := true
		for _, dur := range itemDurations {
			ms := clockToMs(dur)
			if ms < 0 {
				allValid = false
				break
			}
			sumMs += ms
		}
		if allValid && globalMs >= 0 && len(smilItems) > 0 && len(itemDurations) == len(smilItems) {
			diff := globalMs - sumMs
			if diff < 0 {
				diff = -diff
			}
			// 1 second tolerance
			if diff > 1000 {
				r.Add(report.Warning, "MED-016",
					fmt.Sprintf("Total media:duration does not match the sum of individual overlay durations"))
			}
		}
	}
}

// checkOPFEncoding detects non-UTF-8 encoding in OPF files.
// Returns an encoding type string or empty string if encoding is OK.
// checkOPFEncoding detects non-UTF-8 encoding in OPF files.
// Returns (encodingType, conflict) where encodingType is the detected encoding
// and conflict indicates a BOM/declaration mismatch.
func checkOPFEncoding(data []byte) (string, bool) {
	if len(data) < 4 {
		return "", false
	}

	hasBOM := false
	bomEncoding := ""

	// Check BOM patterns
	if data[0] == 0xFF && data[1] == 0xFE {
		if len(data) >= 4 && data[2] == 0x00 && data[3] == 0x00 {
			return "utf32", false // UTF-32 LE BOM
		}
		hasBOM = true
		bomEncoding = "utf16"
	} else if data[0] == 0xFE && data[1] == 0xFF {
		hasBOM = true
		bomEncoding = "utf16"
	} else if data[0] == 0x00 && data[1] == 0x00 {
		if data[2] == 0xFE && data[3] == 0xFF {
			return "utf32", false // UTF-32 BE BOM
		}
		if data[2] == 0x00 && data[3] == 0x3C {
			return "utf32", false // UTF-32 BE (no BOM)
		}
	}

	// Detect UTF-16 without BOM (null bytes in typical ASCII content)
	if !hasBOM && ((data[0] == 0x00 && data[1] == 0x3C) || (data[0] == 0x3C && data[1] == 0x00)) {
		return "utf16", false
	}

	// If we have a UTF-16 BOM, check for encoding declaration conflict
	if hasBOM && bomEncoding == "utf16" {
		// Try to read the encoding declaration from the UTF-16 content
		declaredEnc := extractUTF16EncodingDecl(data)
		if declaredEnc != "" && declaredEnc != "utf-16" {
			return "utf16", true // BOM/declaration conflict
		}
		return "utf16", false
	}

	// Check XML declaration for encoding attribute (ASCII-readable files)
	content := string(data)
	if strings.HasPrefix(content, "<?xml") {
		endPI := strings.Index(content, "?>")
		if endPI > 0 {
			xmlDecl := strings.ToLower(content[:endPI])
			encodingIdx := strings.Index(xmlDecl, "encoding")
			if encodingIdx > 0 {
				rest := xmlDecl[encodingIdx+8:]
				rest = strings.TrimLeft(rest, " \t=")
				rest = strings.TrimLeft(rest, "\"'")
				endQuote := strings.IndexAny(rest, "\"'")
				if endQuote > 0 {
					enc := strings.TrimSpace(rest[:endQuote])
					switch {
					case enc == "utf-8":
						return "", false // OK
					case enc == "utf-16":
						return "utf16", false
					case strings.HasPrefix(enc, "iso-8859") || enc == "latin1":
						return "latin1", false
					case enc == "utf-32" || enc == "ucs-4" || enc == "ucs4":
						return "utf32", false
					default:
						knownEncodings := map[string]bool{
							"utf-8": true, "utf-16": true, "us-ascii": true, "ascii": true,
						}
						if !knownEncodings[enc] {
							return "unknown", false
						}
					}
				}
			}
		}
	}

	return "", false
}

// extractUTF16EncodingDecl tries to extract the encoding attribute from a UTF-16 encoded XML declaration.
func extractUTF16EncodingDecl(data []byte) string {
	// Convert UTF-16 to ASCII by extracting every other byte
	var ascii []byte
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		// UTF-16 LE: skip BOM, take even-indexed bytes
		for i := 2; i < len(data) && i < 200; i += 2 {
			if data[i] < 0x80 {
				ascii = append(ascii, data[i])
			}
		}
	} else if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		// UTF-16 BE: skip BOM, take odd-indexed bytes
		for i := 3; i < len(data) && i < 200; i += 2 {
			if data[i] < 0x80 {
				ascii = append(ascii, data[i])
			}
		}
	}
	content := strings.ToLower(string(ascii))
	idx := strings.Index(content, "encoding")
	if idx < 0 {
		return ""
	}
	rest := content[idx+8:]
	rest = strings.TrimLeft(rest, " \t=")
	rest = strings.TrimLeft(rest, "\"'")
	endQuote := strings.IndexAny(rest, "\"'")
	if endQuote > 0 {
		return strings.TrimSpace(rest[:endQuote])
	}
	return ""
}

// checkOPFDoctype checks for invalid DOCTYPE in OPF documents (HTM-009).
func checkOPFDoctype(data []byte, r *report.Report) {
	content := string(data)
	upper := strings.ToUpper(content)
	idx := strings.Index(upper, "<!DOCTYPE")
	if idx < 0 {
		return
	}
	endIdx := strings.Index(content[idx:], ">")
	if endIdx < 0 {
		return
	}
	doctype := content[idx : idx+endIdx+1]
	if strings.Contains(strings.ToUpper(doctype), "PUBLIC") {
		publicID, _ := extractDOCTYPEIdentifiers(doctype[2:]) // skip "<!" to get "DOCTYPE ..."
		if publicID != "" {
			// Valid OPF public identifiers
			validOPFPublicIDs := map[string]bool{
				"+//ISBN 0-9673008-1-9//DTD OEB 1.2 Package//EN": true,
				"-//IDPF//DTD OEB Package File 1.0//EN":          true,
			}
			if !validOPFPublicIDs[publicID] {
				r.Add(report.Error, "HTM-009",
					fmt.Sprintf(`Invalid DOCTYPE: public identifier "%s" is not valid`, publicID))
			}
		}
	}
}

// hasUndeclaredNamespacePrefix scans raw XML bytes for namespace prefixes used
// without a corresponding xmlns: declaration. Go's xml.Decoder doesn't detect this.
func hasUndeclaredNamespacePrefix(data []byte) bool {
	content := string(data)
	// Collect declared namespace prefixes
	declared := map[string]bool{"xml": true, "xmlns": true}
	re := regexp.MustCompile(`xmlns:(\w+)\s*=`)
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		declared[m[1]] = true
	}
	// Scan for element/attribute uses of undeclared prefixes
	// Match <prefix:name or prefix:name= in attribute positions
	prefixRe := regexp.MustCompile(`<(\w+):`)
	for _, m := range prefixRe.FindAllStringSubmatch(content, -1) {
		prefix := m[1]
		if prefix == "xml" || prefix == "xmlns" {
			continue
		}
		if !declared[prefix] {
			return true
		}
	}
	return false
}

// checkManifestHrefEncoding checks manifest item hrefs for unencoded literal spaces (RSC-020).
func checkManifestHrefEncoding(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "" || item.Href == "\x00MISSING" {
			continue
		}
		// Skip data URLs and remote URLs (they may legitimately contain spaces or are handled elsewhere)
		lower := strings.ToLower(strings.TrimSpace(item.Href))
		if strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "http://") ||
			strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "file:") {
			continue
		}
		if strings.Contains(item.Href, " ") {
			r.Add(report.Error, "RSC-020",
				fmt.Sprintf("Manifest item href contains an unencoded space: '%s'", item.Href))
		}
	}
}

// checkManifestHrefFilenames checks manifest item hrefs for filename issues in single-file mode.
// In single-file OPF mode, the ZIP file entries don't exist so we check the hrefs directly.
// PKG-009: forbidden characters, PKG-010: spaces, PKG-012: non-ASCII characters.
func checkManifestHrefFilenames(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "" || item.Href == "\x00MISSING" {
			continue
		}
		// Skip remote URLs, data URLs, and file URLs - they're not local paths
		lower := strings.ToLower(strings.TrimSpace(item.Href))
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
			strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "file:") {
			continue
		}
		// Strip query string and fragment before filename checks
		href := item.Href
		if idx := strings.IndexAny(href, "?#"); idx >= 0 {
			href = href[:idx]
		}
		if href == "" {
			continue
		}
		// URL-decode the href to get the actual filename
		decoded, err := url.PathUnescape(href)
		if err != nil {
			decoded = href
		}
		// Check each path component of the decoded href
		parts := strings.Split(decoded, "/")
		for _, part := range parts {
			if part == "" {
				continue
			}
			// PKG-010: warn about spaces (check before forbidden chars)
			hasSpace := false
			for _, c := range part {
				if isFilenameSpaceChar(c) {
					hasSpace = true
					break
				}
			}
			if hasSpace {
				r.Add(report.Warning, "PKG-010",
					fmt.Sprintf("Filename contains spaces, which is discouraged: '%s'", part))
				continue
			}
			// PKG-009: forbidden characters
			epub2 := pkg.Version < "3.0"
			if hasForbiddenFilenameChar(part, epub2) {
				r.Add(report.Error, "PKG-009",
					fmt.Sprintf("File name contains characters forbidden in OCF file names in: '%s'", part))
				continue
			}
			// PKG-012: non-ASCII characters
			for _, c := range part {
				if c > 0x7F {
					r.Add(report.Usage, "PKG-012",
						fmt.Sprintf("Filename contains non-ASCII characters, which may cause interoperability issues: '%s'", part))
					break
				}
			}
		}
	}
}

// hasForbiddenFilenameChar returns true if the filename part contains any forbidden OCF characters.
func hasForbiddenFilenameChar(name string, epub2 bool) bool {
	for _, c := range name {
		if isForbiddenFilenameChar(c, epub2) {
			return true
		}
	}
	return false
}

// checkGuideDuplicates checks for duplicate guide reference entries (same type and href).
// RSC-017: duplicate guide entries. Reports once per entry that is a duplicate.
func checkGuideDuplicates(pkg *epub.Package, r *report.Report) {
	if !pkg.HasGuide || len(pkg.Guide) == 0 {
		return
	}
	// Count how many times each (type, href) pair appears
	counts := make(map[string]int)
	for _, ref := range pkg.Guide {
		key := ref.Type + "\x00" + ref.Href
		counts[key]++
	}
	// Report RSC-017 for each entry in a duplicate group
	for _, ref := range pkg.Guide {
		key := ref.Type + "\x00" + ref.Href
		if counts[key] > 1 {
			r.Add(report.Warning, "RSC-017",
				`Duplicate "reference" elements with the same "type" and "href" attributes`)
		}
	}
}

// isValidPercentEncoded returns true if all % characters in the string
// are followed by two valid hexadecimal digits.
func isValidPercentEncoded(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '%' {
			if i+2 >= len(s) {
				return false
			}
			h1 := s[i+1]
			h2 := s[i+2]
			isHex := func(c byte) bool {
				return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			}
			if !isHex(h1) || !isHex(h2) {
				return false
			}
		}
	}
	return true
}

// checkCollections validates <collection> elements in the OPF package document.
// OPF-070: collection role must be a valid URL (only flag truly malformed IRIs).
// RSC-005: manifest collection must be the child of another collection.
func checkCollections(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	// Build manifest lookup maps
	manifestByHref := make(map[string]epub.ManifestItem)
	for _, item := range pkg.Manifest {
		if item.Href != "\x00MISSING" {
			manifestByHref[item.Href] = item
		}
	}

	for _, col := range pkg.Collections {
		if col.Role == "" {
			continue
		}
		// OPF-070: flag only truly malformed IRIs (invalid percent-encoding or parse failure)
		_, parseErr := url.Parse(col.Role)
		hasInvalidPercent := strings.Contains(col.Role, "%") && !isValidPercentEncoded(col.Role)
		if parseErr != nil || hasInvalidPercent {
			r.Add(report.Warning, "OPF-070",
				fmt.Sprintf("Collection role '%s' is not a valid URL", col.Role))
		}
		// RSC-005: 'manifest' collection must be nested inside another collection
		if col.Role == "manifest" && col.TopLevel {
			r.Add(report.Error, "RSC-005",
				"A manifest collection must be the child of another collection")
		}

		// Normalize role URL to extract the role name suffix
		roleName := extractCollectionRoleName(col.Role)

		// OPF-071: index collections must only contain XHTML
		if roleName == "index" || roleName == "index-group" {
			checkIndexCollection(col, manifestByHref, r)
		}

		// OPF-075/076: preview collection checks
		if roleName == "preview" {
			checkPreviewCollection(col, manifestByHref, r)
		}

		// OPF-082/083/084: dictionary collection checks
		if roleName == "dictionary" {
			checkDictionaryCollection(col, manifestByHref, r)
		}
	}
}

// extractCollectionRoleName gets the terminal segment of a collection role URL.
// e.g., "http://idpf.org/epub/vocab/package/roles/#index" → "index"
func extractCollectionRoleName(role string) string {
	if i := strings.LastIndex(role, "#"); i >= 0 {
		return role[i+1:]
	}
	if i := strings.LastIndex(role, "/"); i >= 0 && i < len(role)-1 {
		return role[i+1:]
	}
	return role
}

// OPF-071: index collections must only contain resources pointing to XHTML Content Documents.
func checkIndexCollection(col epub.Collection, manifestByHref map[string]epub.ManifestItem, r *report.Report) {
	for _, href := range col.Links {
		// Strip fragment
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		item, found := manifestByHref[parsed.Path]
		if !found || item.MediaType != "application/xhtml+xml" {
			r.Add(report.Error, "OPF-071",
				"Index collections must only contain resources pointing to XHTML Content Documents")
			return
		}
	}
}

// OPF-075/076: preview collections checks.
func checkPreviewCollection(col epub.Collection, manifestByHref map[string]epub.ManifestItem, r *report.Report) {
	for _, href := range col.Links {
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}

		// OPF-076: preview collection links must not include EPUB CFI fragments
		if parsed.Fragment != "" && strings.HasPrefix(parsed.Fragment, "epubcfi") {
			r.Add(report.Error, "OPF-076",
				"The URI of preview collections link elements must not include EPUB canonical fragment identifiers")
		}

		// OPF-075: must point to EPUB Content Documents (XHTML or SVG)
		item, found := manifestByHref[parsed.Path]
		if !found || (item.MediaType != "application/xhtml+xml" && item.MediaType != "image/svg+xml") {
			r.Add(report.Error, "OPF-075",
				"Preview collections must only point to EPUB Content Documents")
		}
	}
}

// OPF-082/083/084: dictionary collection checks.
func checkDictionaryCollection(col epub.Collection, manifestByHref map[string]epub.ManifestItem, r *report.Report) {
	skmCount := 0
	for _, href := range col.Links {
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		item, found := manifestByHref[parsed.Path]
		if !found {
			continue
		}
		if item.MediaType == "application/vnd.epub.search-key-map+xml" {
			skmCount++
			if skmCount > 1 {
				// OPF-082: multiple search key maps
				r.Add(report.Error, "OPF-082",
					"Found an EPUB Dictionary collection containing more than one Search Key Map Document")
			}
		} else if item.MediaType != "application/xhtml+xml" {
			// OPF-084: invalid resource type in dictionary collection
			r.Add(report.Error, "OPF-084",
				fmt.Sprintf("Found an EPUB Dictionary collection containing resource '%s' which is neither a Search Key Map Document nor an XHTML Content Document", parsed.Path))
		}
	}
	// OPF-083: no search key map
	if skmCount == 0 {
		r.Add(report.Error, "OPF-083",
			"Found an EPUB Dictionary collection containing no Search Key Map Document")
	}
}

// OPF-077: Data Navigation Document should not be included in the spine.
func checkDataNavNotInSpine(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}
	// Build set of manifest item IDs with data-nav property
	dataNavIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if hasProperty(item.Properties, "data-nav") {
			dataNavIDs[item.ID] = true
		}
	}
	// Check if any spine itemref references a data-nav item
	for _, ref := range pkg.Spine {
		if dataNavIDs[ref.IDRef] {
			r.Add(report.Warning, "OPF-077",
				"A Data Navigation Document should not be included in the spine")
			return
		}
	}
}

// OPF-066: pagination source metadata check.
// When page break markers are present (page-list in nav), the publication
// must declare dc:source and a source-of:pagination refinement.
func checkPaginationSourceMetadata(ep *epub.EPUB, pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	// Check if nav document has a page-list
	var navHref string
	for _, item := range pkg.Manifest {
		if hasProperty(item.Properties, "nav") {
			navHref = item.Href
			break
		}
	}
	if navHref == "" {
		return
	}
	navPath := ep.ResolveHref(navHref)
	data, err := ep.ReadFile(navPath)
	if err != nil {
		return
	}
	if !navDocHasPageList(data) {
		return
	}

	// page-list is present — check for dc:source
	if len(pkg.Metadata.Sources) == 0 {
		r.Add(report.Error, "OPF-066",
			`Missing "dc:source" or "source-of" pagination metadata. The pagination source must be identified using the "dc:source" and "source-of" properties when the content includes page break markers`)
		return
	}

	// Check for source-of:pagination refinement on any dc:source
	hasSourceOf := false
	for _, mr := range pkg.MetaRefines {
		if mr.Property == "source-of" && mr.Value == "pagination" {
			hasSourceOf = true
			break
		}
	}
	if !hasSourceOf {
		r.Add(report.Error, "OPF-066",
			`Missing "dc:source" or "source-of" pagination metadata. The pagination source must be identified using the "dc:source" and "source-of" properties when the content includes page break markers`)
	}
}

// OPF-067: a resource listed as a metadata <link> must not also be a manifest item
// (unless the item is in the spine). This prevents confusion between metadata
// links (which describe the publication) and manifest items (which are publication resources).
func checkLinkNotInManifest(ep *epub.EPUB, pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	// Build a set of resolved manifest item paths that are NOT in the spine.
	// Items in the spine are publication resources and may legitimately also be link targets.
	spineIDs := make(map[string]bool)
	for _, ref := range pkg.Spine {
		spineIDs[ref.IDRef] = true
	}

	manifestPaths := make(map[string]string) // resolved path -> item href
	for _, item := range pkg.Manifest {
		if item.Href == "" || item.Href == "\x00MISSING" {
			continue
		}
		if spineIDs[item.ID] {
			continue // spine items are OK
		}
		resolved := ep.ResolveHref(item.Href)
		manifestPaths[resolved] = item.Href
	}

	for _, link := range pkg.MetadataLinks {
		if link.Href == "" || isRemoteURL(link.Href) {
			continue
		}
		// Strip fragment
		hrefPath := link.Href
		if idx := strings.Index(hrefPath, "#"); idx >= 0 {
			hrefPath = hrefPath[:idx]
		}
		resolved := ep.ResolveHref(hrefPath)
		if itemHref, found := manifestPaths[resolved]; found {
			r.Add(report.Error, "OPF-067",
				fmt.Sprintf("The resource '%s' must not be listed both as a \"link\" element in the package metadata and as a manifest item", itemHref))
		}
	}
}

// OPF-072: dc:* metadata elements must not be empty.
// This flags empty dc:title, dc:creator, dc:contributor, dc:language, dc:identifier,
// dc:source, dc:date elements (text content is empty string or whitespace-only).
func checkEmptyMetadataElements(pkg *epub.Package, r *report.Report) {
	if pkg.Version < "3.0" {
		return
	}

	// Check dc:source elements
	for _, src := range pkg.Metadata.Sources {
		if strings.TrimSpace(src) == "" {
			r.Add(report.Usage, "OPF-072",
				`Metadata element "dc:source" is empty`)
		}
	}

	// Check dc:date elements
	for _, dt := range pkg.Metadata.Dates {
		if strings.TrimSpace(dt) == "" {
			// Already handled by OPF-054, skip to avoid double-reporting
			continue
		}
	}

	// dc:title, dc:identifier emptiness already checked by OPF-032, OPF-031
	// dc:language emptiness already checked by OPF-003
	// dc:creator/contributor with empty values are checked by OPF-055
}
