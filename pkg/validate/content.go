package validate

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// checkContentWithSkips validates XHTML content documents, skipping files with known encoding issues.
func checkContentWithSkips(ep *epub.EPUB, r *report.Report, skipFiles map[string]bool) {
	if ep.Package == nil {
		return
	}

	// Build set of manifest-declared resources (resolved full paths).
	manifestPaths := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestPaths[ep.ResolveHref(item.Href)] = true
		}
	}

	isFXL := ep.Package.RenditionLayout == "pre-paginated"

	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		if item.MediaType != "application/xhtml+xml" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)

		// Skip files with encoding errors
		if skipFiles[fullPath] {
			continue
		}

		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue // Missing file reported by RSC-001
		}

		isNav := hasProperty(item.Properties, "nav")

		// HTM-001: XHTML must be well-formed XML
		// Skip nav docs - NAV-011 handles them
		if !isNav {
			if !checkXHTMLWellFormed(data, fullPath, r) {
				continue // Can't check further if not well-formed
			}
		}

		// HTM-002: content should have title (WARNING)
		checkContentHasTitle(data, fullPath, r)

		// HTM-003: empty href attributes
		checkEmptyHrefAttributes(data, fullPath, r)

		// HTM-004: no obsolete elements
		checkNoObsoleteElements(data, fullPath, r)

		// HTM-009: base element not allowed
		checkNoBaseElement(data, fullPath, r)

		// HTM-010/HTM-011/HTM-012: DOCTYPE and namespace checks (EPUB 3 only)
		if ep.Package.Version >= "3.0" {
			if !checkDoctypeHTML5(data, fullPath, r) {
				checkDoctype(data, fullPath, r)
			}
		}
		checkXHTMLNamespace(data, fullPath, r)

		// HTM-005/HTM-006/HTM-007: property declarations
		if ep.Package.Version >= "3.0" {
			checkPropertyDeclarations(ep, data, fullPath, item, r)
		}

		// HTM-015: epub:type values must be valid (EPUB 3 only)
		if ep.Package.Version >= "3.0" {
			checkEpubTypeValid(data, fullPath, r)
		}

		// HTM-020: no processing instructions
		checkNoProcessingInstructions(data, fullPath, r)

		// HTM-021: position:absolute warning
		checkNoPositionAbsolute(data, fullPath, r)

		// HTM-013/HTM-014: FXL viewport checks
		if isFXL && ep.Package.Version >= "3.0" {
			// Skip nav document from FXL viewport checks
			if !hasProperty(item.Properties, "nav") {
				checkFXLViewport(data, fullPath, r)
			}
		}

		// RSC-003: fragment identifiers must resolve (skip nav - handled by NAV checks)
		if !isNav {
			checkFragmentIdentifiers(ep, data, fullPath, r)
		}

		// RSC-004: no remote resources (img src with http://)
		// RSC-008: no remote stylesheets
		checkNoRemoteResources(ep, data, fullPath, item, r)

		// HTM-008 / RSC-007: check internal links and resource references
		// Skip nav document - its links are checked by NAV-003/006/007
		if !isNav {
			checkContentReferences(ep, data, fullPath, item.Href, manifestPaths, r)
		}

		// HTM-016: unique IDs within content document
		checkUniqueIDs(data, fullPath, r)

		// HTM-018: single body element
		checkSingleBody(data, fullPath, r)

		// HTM-019: html root element
		hasHTMLRoot := checkHTMLRootElement(data, fullPath, r)

		// HTM-022: object data references must resolve
		if !isNav {
			checkObjectReferences(ep, data, fullPath, r)
		}

		// HTM-023: no parent directory links that escape container
		if !isNav {
			checkNoParentDirLinks(ep, data, fullPath, r)
		}

		// HTM-024: content documents must have a head element (skip if no html root)
		if hasHTMLRoot {
			checkContentHasHead(data, fullPath, r)
		}

		// HTM-025: embed element references must exist
		if !isNav {
			checkEmbedReferences(ep, data, fullPath, r)
		}

		// HTM-026: lang and xml:lang must match
		checkLangXMLLangMatch(data, fullPath, r)

		// HTM-027: video poster must exist
		if ep.Package.Version >= "3.0" && !isNav {
			checkVideoPosterExists(ep, data, fullPath, r)
		}

		// HTM-028: audio src must exist
		if ep.Package.Version >= "3.0" && !isNav {
			checkAudioSrcExists(ep, data, fullPath, r)
		}

		// HTM-030: img src must not be empty
		checkImgSrcNotEmpty(data, fullPath, r)

		// HTM-031: SSML namespace check
		if ep.Package.Version >= "3.0" {
			checkSSMLNamespace(data, fullPath, r)
		}

		// HTM-032: style element CSS syntax
		checkStyleElementValid(data, fullPath, r)

		// HTM-033: no RDF elements in content
		checkNoRDFElements(data, fullPath, r)
	}
}

// HTM-001: check that XHTML is well-formed XML
func checkXHTMLWellFormed(data []byte, location string, r *report.Report) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			errMsg := err.Error()
			// HTM-017: HTML entity references not valid in XHTML
			if strings.Contains(errMsg, "invalid character entity") || strings.Contains(errMsg, "entity") {
				r.AddWithLocation(report.Fatal, "HTM-017",
					"Content document is not well-formed: entity was referenced but not declared",
					location)
			} else if strings.Contains(errMsg, "attribute") {
				// HTM-029: attribute-related XML errors (e.g., malformed SVG attributes)
				r.AddWithLocation(report.Fatal, "HTM-001",
					fmt.Sprintf("Content document is not well-formed XML: Attribute name is not associated with an element (%s)", errMsg),
					location)
			} else {
				r.AddWithLocation(report.Fatal, "HTM-001",
					fmt.Sprintf("Content document is not well-formed XML: element not terminated by the matching end-tag (%s)", errMsg),
					location)
			}
			return false
		}
	}
	return true
}

// HTM-002: content documents should have a title element
func checkContentHasTitle(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	inHead := false
	hasTitle := false

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "head" {
				inHead = true
			}
			if inHead && t.Name.Local == "title" {
				hasTitle = true
			}
		case xml.EndElement:
			if t.Name.Local == "head" {
				if !hasTitle {
					r.AddWithLocation(report.Warning, "HTM-002",
						"Missing title element in content document head",
						location)
				}
				return
			}
		}
	}
}

// HTM-004: no obsolete HTML elements
var obsoleteElements = map[string]bool{
	"center":    true,
	"font":      true,
	"basefont":  true,
	"big":       true,
	"blink":     true,
	"marquee":   true,
	"multicol":  true,
	"nobr":      true,
	"spacer":    true,
	"strike":    true,
	"tt":        true,
	"acronym":   true,
	"applet":    true,
	"dir":       true,
	"frame":     true,
	"frameset":  true,
	"noframes":  true,
	"isindex":   true,
	"listing":   true,
	"plaintext": true,
	"xmp":       true,
}

func checkNoObsoleteElements(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	reported := make(map[string]bool)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			elemName := se.Name.Local
			if obsoleteElements[elemName] && !reported[elemName] {
				r.AddWithLocation(report.Error, "HTM-004",
					fmt.Sprintf("Element '%s' is not allowed in EPUB content documents", elemName),
					location)
				reported[elemName] = true
			}
		}
	}
}

// HTM-011: DOCTYPE check for EPUB 3
func checkDoctype(data []byte, location string, r *report.Report) {
	content := string(data)
	// Look for DOCTYPE declaration
	idx := strings.Index(content, "<!DOCTYPE")
	if idx == -1 {
		return // No DOCTYPE is fine for EPUB 3
	}

	// Find the full DOCTYPE
	endIdx := strings.Index(content[idx:], ">")
	if endIdx == -1 {
		return
	}
	doctype := content[idx : idx+endIdx+1]

	// EPUB 3 should use HTML5 DOCTYPE: <!DOCTYPE html> (case insensitive)
	// It should NOT have PUBLIC or SYSTEM identifiers
	if strings.Contains(doctype, "PUBLIC") || strings.Contains(doctype, "SYSTEM") {
		r.AddWithLocation(report.Error, "HTM-011",
			"Irregular DOCTYPE: EPUB 3 content documents should use <!DOCTYPE html>",
			location)
	}
}

// HTM-012: XHTML namespace check
func checkXHTMLNamespace(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "html" {
				ns := se.Name.Space
				if ns != "" && ns != "http://www.w3.org/1999/xhtml" {
					r.AddWithLocation(report.Error, "HTM-012",
						fmt.Sprintf("The html element namespace is wrong: '%s'", ns),
						location)
				}
				return
			}
		}
	}
}

// HTM-005/HTM-006/HTM-007: check for script/SVG/MathML and undeclared properties
func checkPropertyDeclarations(ep *epub.EPUB, data []byte, location string, item epub.ManifestItem, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	hasScript := false
	hasSVG := false
	hasMathML := false

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if se.Name.Local == "script" {
			hasScript = true
		}
		if se.Name.Local == "svg" || se.Name.Space == "http://www.w3.org/2000/svg" {
			hasSVG = true
		}
		if se.Name.Local == "math" || se.Name.Space == "http://www.w3.org/1998/Math/MathML" {
			hasMathML = true
		}
	}

	if hasScript && !hasProperty(item.Properties, "scripted") {
		r.AddWithLocation(report.Error, "HTM-005",
			"Property 'scripted' should be declared in the manifest for scripted content",
			location)
	}
	if hasSVG && !hasProperty(item.Properties, "svg") {
		r.AddWithLocation(report.Error, "HTM-006",
			"Property 'svg' should be declared in the manifest for content with inline SVG",
			location)
	}
	if hasMathML && !hasProperty(item.Properties, "mathml") {
		r.AddWithLocation(report.Error, "HTM-007",
			"Property 'mathml' should be declared in the manifest for content with MathML",
			location)
	}
}

// HTM-013/HTM-014: Fixed-layout viewport checks
func checkFXLViewport(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	hasViewport := false
	viewportContent := ""

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if se.Name.Local == "meta" {
			var name, content string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "name":
					name = attr.Value
				case "content":
					content = attr.Value
				}
			}
			if name == "viewport" {
				hasViewport = true
				viewportContent = content
			}
		}

		// Stop after head
		if se.Name.Local == "body" {
			break
		}
	}

	if !hasViewport {
		r.AddWithLocation(report.Error, "HTM-013",
			"Fixed-layout content document has no viewport meta element",
			location)
		return
	}

	// HTM-014: viewport must have width and height
	hasWidth := false
	hasHeight := false
	viewportRe := regexp.MustCompile(`(?i)(width|height)\s*=\s*\d+`)
	matches := viewportRe.FindAllStringSubmatch(viewportContent, -1)
	for _, m := range matches {
		switch strings.ToLower(m[1]) {
		case "width":
			hasWidth = true
		case "height":
			hasHeight = true
		}
	}
	if !hasWidth || !hasHeight {
		r.AddWithLocation(report.Error, "HTM-014",
			"Viewport metadata must specify both width and height dimensions",
			location)
	}
}

// RSC-003: fragment identifiers must resolve
func checkFragmentIdentifiers(ep *epub.EPUB, data []byte, fullPath string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	itemDir := path.Dir(fullPath)

	// Collect all id attributes in the document for self-references
	ids := collectIDs(data)

	decoder = xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if se.Name.Local == "a" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "href" {
					checkFragmentRef(ep, attr.Value, itemDir, fullPath, ids, r)
				}
			}
		}
	}
}

func checkFragmentRef(ep *epub.EPUB, href, itemDir, location string, localIDs map[string]bool, r *report.Report) {
	if href == "" {
		return
	}

	u, err := url.Parse(href)
	if err != nil || u.Scheme != "" {
		return
	}

	fragment := u.Fragment
	if fragment == "" {
		return // No fragment to check
	}

	refPath := u.Path
	if refPath == "" {
		// Self-reference fragment
		if !localIDs[fragment] {
			r.AddWithLocation(report.Error, "RSC-003",
				fmt.Sprintf("Fragment identifier is not defined: '#%s'", fragment),
				location)
		}
		return
	}

	// Cross-document fragment reference
	target := resolvePath(itemDir, refPath)
	targetData, err := ep.ReadFile(target)
	if err != nil {
		return // File missing, handled by HTM-008
	}

	targetIDs := collectIDs(targetData)
	if !targetIDs[fragment] {
		r.AddWithLocation(report.Error, "RSC-003",
			fmt.Sprintf("Fragment identifier is not defined: '%s#%s'", refPath, fragment),
			location)
	}
}

func collectIDs(data []byte) map[string]bool {
	ids := make(map[string]bool)
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			for _, attr := range se.Attr {
				if attr.Name.Local == "id" {
					ids[attr.Value] = true
				}
			}
		}
	}
	return ids
}

// RSC-004: no remote resources / RSC-008: no remote stylesheets
func checkNoRemoteResources(ep *epub.EPUB, data []byte, location string, item epub.ManifestItem, r *report.Report) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		// Check <img src="http://...">
		if se.Name.Local == "img" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isRemoteURL(attr.Value) {
					r.AddWithLocation(report.Error, "RSC-004",
						fmt.Sprintf("Remote resource reference is not allowed: '%s'", attr.Value),
						location)
				}
			}
		}

		// Check <audio src="http://..."> and <video src="http://...">
		if se.Name.Local == "audio" || se.Name.Local == "video" || se.Name.Local == "source" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isRemoteURL(attr.Value) {
					r.AddWithLocation(report.Error, "RSC-004",
						fmt.Sprintf("Remote resource reference is not allowed: '%s'", attr.Value),
						location)
				}
			}
		}

		// Check <link rel="stylesheet" href="http://...">
		if se.Name.Local == "link" {
			var href, rel string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "href":
					href = attr.Value
				case "rel":
					rel = attr.Value
				}
			}
			if rel == "stylesheet" && isRemoteURL(href) {
				r.AddWithLocation(report.Error, "RSC-008",
					fmt.Sprintf("Remote resource reference is not allowed: '%s'", href),
					location)
			}
		}
	}
}

func isRemoteURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// checkContentReferences finds href/src attributes in XHTML and validates them.
func checkContentReferences(ep *epub.EPUB, data []byte, fullPath, itemHref string, manifestPaths map[string]bool, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	itemDir := path.Dir(fullPath)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		// Check <a href="..."> for internal links
		if se.Name.Local == "a" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "href" {
					checkHyperlink(ep, attr.Value, itemDir, fullPath, r)
				}
			}
		}

		// Check <img src="..."> for image references
		if se.Name.Local == "img" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" {
					checkResourceRef(ep, attr.Value, itemDir, fullPath, manifestPaths, r)
				}
			}
		}
	}
}

// checkHyperlink validates a hyperlink reference from a content document.
func checkHyperlink(ep *epub.EPUB, href, itemDir, location string, r *report.Report) {
	if href == "" {
		return
	}

	u, err := url.Parse(href)
	if err != nil {
		return
	}
	if u.Scheme != "" {
		return
	}

	refPath := u.Path
	if refPath == "" {
		return // fragment-only reference
	}

	target := resolvePath(itemDir, refPath)
	if _, exists := ep.Files[target]; !exists {
		r.AddWithLocation(report.Error, "HTM-008",
			fmt.Sprintf("Hyperlink reference '%s' (%s) was not found in the container", refPath, target),
			location)
	}
}

// checkResourceRef validates a resource reference (img src, etc.) from a content document.
func checkResourceRef(ep *epub.EPUB, src, itemDir, location string, manifestPaths map[string]bool, r *report.Report) {
	if src == "" {
		return
	}

	u, err := url.Parse(src)
	if err != nil {
		return
	}
	if u.Scheme != "" {
		return
	}

	refPath := u.Path
	if refPath == "" {
		return
	}

	target := resolvePath(itemDir, refPath)
	if manifestPaths[target] {
		return
	}
	if _, exists := ep.Files[target]; !exists {
		r.AddWithLocation(report.Error, "RSC-007",
			fmt.Sprintf("Referenced resource '%s' (%s) was not found in the container", src, target),
			location)
	}
}

// resolvePath resolves a relative path against a base directory.
func resolvePath(baseDir, rel string) string {
	if path.IsAbs(rel) {
		return rel[1:] // strip leading /
	}
	return path.Clean(baseDir + "/" + rel)
}

// HTM-016: IDs must be unique within a content document
func checkUniqueIDs(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	seen := make(map[string]bool)
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "id" {
				if seen[attr.Value] {
					r.AddWithLocation(report.Error, "HTM-016",
						fmt.Sprintf("Duplicate ID '%s'", attr.Value),
						location)
				}
				seen[attr.Value] = true
			}
		}
	}
}

// HTM-018: content document must have exactly one body element
func checkSingleBody(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	bodyCount := 0
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "body" {
				bodyCount++
			}
		}
	}
	if bodyCount > 1 {
		r.AddWithLocation(report.Error, "HTM-018",
			"Element body is not allowed here: content documents must have exactly one body element",
			location)
	}
}

// HTM-019: content document must have html as root element.
// Returns true if the root element is html.
func checkHTMLRootElement(data []byte, location string, r *report.Report) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return false
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		// First element should be html
		if se.Name.Local != "html" {
			r.AddWithLocation(report.Error, "HTM-019",
				fmt.Sprintf("Element body is not allowed here: expected element 'html' as root, but found '%s'", se.Name.Local),
				location)
			return false
		}
		return true
	}
}

// HTM-022: object data references must exist
func checkObjectReferences(ep *epub.EPUB, data []byte, fullPath string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	itemDir := path.Dir(fullPath)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "object" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "data" && attr.Value != "" {
					u, err := url.Parse(attr.Value)
					if err != nil || u.Scheme != "" {
						continue
					}
					target := resolvePath(itemDir, u.Path)
					if _, exists := ep.Files[target]; !exists {
						r.AddWithLocation(report.Error, "HTM-022",
							fmt.Sprintf("Referenced resource '%s' could not be found in the container", attr.Value),
							fullPath)
					}
				}
			}
		}
	}
}

// HTM-003: hyperlink href attributes must not be empty
func checkEmptyHrefAttributes(data []byte, location string, r *report.Report) {
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
		if se.Name.Local == "a" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "href" && attr.Value == "" {
					r.AddWithLocation(report.Warning, "HTM-003",
						"Hyperlink href attribute must not be empty",
						location)
				}
			}
		}
	}
}

// HTM-009: base element should not be used in EPUB content documents
func checkNoBaseElement(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "base" {
				r.AddWithLocation(report.Warning, "HTM-009",
					"The 'base' element is not allowed in EPUB content documents",
					location)
				return
			}
		}
	}
}

// HTM-010: EPUB 3 content documents must use HTML5 DOCTYPE or no DOCTYPE.
// Returns true if a non-HTML5 DOCTYPE was detected (to skip HTM-011 which overlaps).
func checkDoctypeHTML5(data []byte, location string, r *report.Report) bool {
	content := string(data)
	idx := strings.Index(strings.ToUpper(content), "<!DOCTYPE")
	if idx == -1 {
		return false // No DOCTYPE is fine
	}
	endIdx := strings.Index(content[idx:], ">")
	if endIdx == -1 {
		return false
	}
	doctype := strings.ToUpper(content[idx : idx+endIdx+1])
	// HTML5 DOCTYPE is just <!DOCTYPE html> (case-insensitive, optionally with system)
	// If it contains XHTML DTD identifiers, it's wrong
	if strings.Contains(doctype, "XHTML") || strings.Contains(doctype, "DTD") {
		r.AddWithLocation(report.Error, "HTM-010",
			"Irregular DOCTYPE: EPUB 3 content documents must use the HTML5 DOCTYPE (<!DOCTYPE html>) or no DOCTYPE",
			location)
		return true
	}
	return false
}

// Valid epub:type values from the EPUB structural semantics vocabulary
var validEpubTypes = map[string]bool{
	"abstract": true, "acknowledgments": true, "afterword": true, "answer": true,
	"answers": true, "appendix": true, "assessment": true, "assessments": true,
	"backmatter": true, "balloon": true, "biblioentry": true, "bibliography": true,
	"biblioref": true, "bodymatter": true, "bridgehead": true, "chapter": true,
	"colophon": true, "concluding-sentence": true, "conclusion": true, "contributors": true,
	"copyright-page": true, "cover": true, "covertitle": true, "credit": true,
	"credits": true, "dedication": true, "division": true, "endnote": true,
	"endnotes": true, "epigraph": true, "epilogue": true, "errata": true,
	"figure": true, "fill-in-the-blank-problem": true, "footnote": true, "footnotes": true,
	"foreword": true, "frontmatter": true, "fulltitle": true, "general-problem": true,
	"glossary": true, "glossdef": true, "glossref": true, "glossterm": true,
	"halftitle": true, "halftitlepage": true, "help": true, "imprimatur": true,
	"imprint": true, "index": true, "index-editor-note": true, "index-entry": true,
	"index-entry-list": true, "index-group": true, "index-headnotes": true,
	"index-legend": true, "index-locator": true, "index-locator-list": true,
	"index-locator-range": true, "index-term": true, "index-term-categories": true,
	"index-term-category": true, "index-xref-preferred": true, "index-xref-related": true,
	"introduction": true, "keyword": true, "keywords": true, "label": true,
	"landmarks": true, "learning-objective": true, "learning-objectives": true,
	"learning-outcome": true, "learning-outcomes": true, "learning-resource": true,
	"learning-resources": true, "learning-standard": true, "learning-standards": true,
	"list": true, "list-item": true, "loa": true, "loi": true, "lot": true, "lov": true,
	"match-problem": true, "multiple-choice-problem": true, "noteref": true,
	"notice": true, "ordinal": true, "other-credits": true, "page-list": true,
	"pagebreak": true, "panel": true, "panel-group": true, "part": true,
	"practice": true, "practices": true, "preamble": true, "preface": true,
	"prologue": true, "pullquote": true, "qna": true, "question": true,
	"revision-history": true, "se:short-story": true, "sound-area": true,
	"subchapter": true, "subtitle": true, "table": true, "table-cell": true,
	"table-row": true, "text-area": true, "tip": true, "title": true,
	"titlepage": true, "toc": true, "toc-brief": true, "topic-sentence": true,
	"true-false-problem": true, "volume": true, "warning": true,
}

// HTM-015: epub:type values must be valid
func checkEpubTypeValid(data []byte, location string, r *report.Report) {
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
		for _, attr := range se.Attr {
			if attr.Name.Local == "type" && attr.Name.Space == "http://www.idpf.org/2007/ops" {
				for _, val := range strings.Fields(attr.Value) {
					// Skip prefixed values (e.g., "dp:footnote") - those use custom vocabularies
					if strings.Contains(val, ":") {
						continue
					}
					if !validEpubTypes[val] {
						r.AddWithLocation(report.Warning, "HTM-015",
							fmt.Sprintf("epub:type value '%s' is not a recognized structural semantics value", val),
							location)
					}
				}
			}
		}
	}
}

// HTM-020: processing instructions should not be used in EPUB content documents
func checkNoProcessingInstructions(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if pi, ok := tok.(xml.ProcInst); ok {
			// Skip the xml declaration itself
			if pi.Target == "xml" {
				continue
			}
			r.AddWithLocation(report.Warning, "HTM-020",
				fmt.Sprintf("Processing instruction '%s' should not be used in EPUB content documents", pi.Target),
				location)
		}
	}
}

// HTM-021: position:absolute in content documents may cause rendering issues
func checkNoPositionAbsolute(data []byte, location string, r *report.Report) {
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
		for _, attr := range se.Attr {
			if attr.Name.Local == "style" {
				if strings.Contains(strings.ToLower(attr.Value), "position") &&
					strings.Contains(strings.ToLower(attr.Value), "absolute") {
					r.AddWithLocation(report.Warning, "HTM-021",
						"Use of 'position:absolute' in content documents may cause rendering issues in reading systems",
						location)
					return
				}
			}
		}
	}
}

// HTM-023: links must not escape the container via parent directory traversal
func checkNoParentDirLinks(ep *epub.EPUB, data []byte, fullPath string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	itemDir := path.Dir(fullPath)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		var hrefs []string
		for _, attr := range se.Attr {
			if attr.Name.Local == "href" || attr.Name.Local == "src" {
				hrefs = append(hrefs, attr.Value)
			}
		}

		for _, href := range hrefs {
			if href == "" {
				continue
			}
			u, err := url.Parse(href)
			if err != nil || u.Scheme != "" {
				continue
			}
			if u.Path == "" {
				continue
			}
			resolved := resolvePath(itemDir, u.Path)
			if strings.HasPrefix(resolved, "..") || strings.HasPrefix(resolved, "/") {
				r.AddWithLocation(report.Error, "HTM-023",
					fmt.Sprintf("Referenced resource '%s' leaks outside the container", href),
					fullPath)
			}
		}
	}
}

// HTM-024: XHTML content documents must have a head element
func checkContentHasHead(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "head" {
				return
			}
		}
	}
	r.AddWithLocation(report.Error, "HTM-024",
		"Content document is missing required element 'head'",
		location)
}

// HTM-025: embed element src must reference existing resource
func checkEmbedReferences(ep *epub.EPUB, data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	contentDir := path.Dir(location)
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "embed" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && attr.Value != "" {
					u, err := url.Parse(attr.Value)
					if err != nil || u.Scheme != "" {
						continue
					}
					target := resolvePath(contentDir, u.Path)
					if _, exists := ep.Files[target]; !exists {
						r.AddWithLocation(report.Error, "HTM-025",
							fmt.Sprintf("Referenced resource '%s' could not be found in the container", attr.Value),
							location)
					}
				}
			}
		}
	}
}

// HTM-026: lang and xml:lang must have the same value when both present
func checkLangXMLLangMatch(data []byte, location string, r *report.Report) {
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
		var lang, xmlLang string
		hasLang, hasXMLLang := false, false
		for _, attr := range se.Attr {
			if attr.Name.Local == "lang" && attr.Name.Space == "" {
				lang = attr.Value
				hasLang = true
			}
			if attr.Name.Local == "lang" && attr.Name.Space == "http://www.w3.org/XML/1998/namespace" {
				xmlLang = attr.Value
				hasXMLLang = true
			}
		}
		if hasLang && hasXMLLang && !strings.EqualFold(lang, xmlLang) {
			r.AddWithLocation(report.Error, "HTM-026",
				fmt.Sprintf("Attributes lang and xml:lang must have the same value when both are present, but found '%s' and '%s'", lang, xmlLang),
				location)
			return
		}
	}
}

// HTM-027: video poster attribute must reference existing resource
func checkVideoPosterExists(ep *epub.EPUB, data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	contentDir := path.Dir(location)
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "video" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "poster" && attr.Value != "" {
					u, err := url.Parse(attr.Value)
					if err != nil || u.Scheme != "" {
						continue
					}
					target := resolvePath(contentDir, u.Path)
					if _, exists := ep.Files[target]; !exists {
						r.AddWithLocation(report.Error, "HTM-027",
							fmt.Sprintf("Referenced resource '%s' could not be found in the container", attr.Value),
							location)
					}
				}
			}
		}
	}
}

// HTM-028: audio src must reference existing resource
func checkAudioSrcExists(ep *epub.EPUB, data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	contentDir := path.Dir(location)
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "audio" || se.Name.Local == "source" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && attr.Value != "" {
					u, err := url.Parse(attr.Value)
					if err != nil || u.Scheme != "" {
						continue
					}
					target := resolvePath(contentDir, u.Path)
					if _, exists := ep.Files[target]; !exists {
						r.AddWithLocation(report.Error, "HTM-028",
							fmt.Sprintf("Referenced resource '%s' could not be found in the container", attr.Value),
							location)
					}
				}
			}
		}
	}
}

// HTM-030: img src attribute must not be empty
func checkImgSrcNotEmpty(data []byte, location string, r *report.Report) {
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
		if se.Name.Local == "img" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && attr.Value == "" {
					r.AddWithLocation(report.Error, "HTM-030",
						"The value of attribute 'src' is invalid; the value must be a string with length at least 1",
						location)
				}
			}
		}
	}
}

// HTM-031: SSML namespace must not be used
func checkSSMLNamespace(data []byte, location string, r *report.Report) {
	ssmlNS := "http://www.w3.org/2001/10/synthesis"
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			for _, attr := range se.Attr {
				if strings.Contains(attr.Value, ssmlNS) || attr.Name.Space == ssmlNS {
					r.AddWithLocation(report.Error, "HTM-031",
						"Custom attribute namespace must not include SSML namespace",
						location)
					return
				}
			}
		}
	}
}

// HTM-032: CSS in inline style elements must be syntactically valid
func checkStyleElementValid(data []byte, location string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "style" {
			continue
		}
		// Read the style content
		var cssContent string
		for {
			inner, err := decoder.Token()
			if err != nil {
				break
			}
			if cd, ok := inner.(xml.CharData); ok {
				cssContent += string(cd)
			}
			if _, ok := inner.(xml.EndElement); ok {
				break
			}
		}
		// Check for basic CSS syntax errors
		if strings.Contains(cssContent, "{") {
			// Check for empty values (property: ;)
			emptyVal := regexp.MustCompile(`:\s*;`)
			if emptyVal.MatchString(cssContent) {
				r.AddWithLocation(report.Error, "HTM-032",
					"An error occurred while parsing the CSS in style element",
					location)
			}
			// Check for missing closing braces
			opens := strings.Count(cssContent, "{")
			closes := strings.Count(cssContent, "}")
			if opens != closes {
				r.AddWithLocation(report.Error, "HTM-032",
					"An error occurred while parsing the CSS in style element: mismatched braces",
					location)
			}
		}
	}
}

// HTM-033: RDF metadata elements should not be used in EPUB content documents
func checkNoRDFElements(data []byte, location string, r *report.Report) {
	rdfNS := "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Space == rdfNS || se.Name.Local == "RDF" {
				r.AddWithLocation(report.Error, "HTM-033",
					"RDF metadata elements should not be used in EPUB content documents",
					location)
				return
			}
		}
	}
}
