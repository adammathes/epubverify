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

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// checkContent validates XHTML content documents.
func checkContent(ep *epub.EPUB, r *report.Report) {
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
		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue // Missing file reported by RSC-001
		}

		// HTM-001: XHTML must be well-formed XML
		if !checkXHTMLWellFormed(data, fullPath, r) {
			continue // Can't check further if not well-formed
		}

		// HTM-002: content should have title (WARNING)
		checkContentHasTitle(data, fullPath, r)

		// HTM-004: no obsolete elements
		checkNoObsoleteElements(data, fullPath, r)

		// HTM-010/HTM-011: DOCTYPE and namespace checks (EPUB 3 only)
		if ep.Package.Version >= "3.0" {
			checkDoctype(data, fullPath, r)
		}
		checkXHTMLNamespace(data, fullPath, r)

		// HTM-005/HTM-006/HTM-007: property declarations
		if ep.Package.Version >= "3.0" {
			checkPropertyDeclarations(ep, data, fullPath, item, r)
		}

		// HTM-013/HTM-014: FXL viewport checks
		if isFXL && ep.Package.Version >= "3.0" {
			// Skip nav document from FXL viewport checks
			if !hasProperty(item.Properties, "nav") {
				checkFXLViewport(data, fullPath, r)
			}
		}

		// RSC-003: fragment identifiers must resolve
		checkFragmentIdentifiers(ep, data, fullPath, r)

		// RSC-004: no remote resources (img src with http://)
		// RSC-008: no remote stylesheets
		checkNoRemoteResources(ep, data, fullPath, item, r)

		// HTM-008 / RSC-007: check internal links and resource references
		checkContentReferences(ep, data, fullPath, item.Href, manifestPaths, r)
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
			r.AddWithLocation(report.Fatal, "HTM-001",
				"Content document is not well-formed XML: element not terminated by the matching end-tag",
				location)
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

// HTM-010: DOCTYPE check for EPUB 3
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
		r.AddWithLocation(report.Error, "HTM-010",
			"Irregular DOCTYPE: EPUB 3 content documents should use <!DOCTYPE html>",
			location)
	}
}

// HTM-011: XHTML namespace check
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
					r.AddWithLocation(report.Error, "HTM-011",
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
			r.AddWithLocation(report.Error, "RSC-012",
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
		r.AddWithLocation(report.Error, "RSC-012",
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
					r.AddWithLocation(report.Error, "RSC-006",
						fmt.Sprintf("Remote resource reference is not allowed: '%s'", attr.Value),
						location)
				}
			}
		}

		// Check <audio src="http://..."> and <video src="http://...">
		if se.Name.Local == "audio" || se.Name.Local == "video" || se.Name.Local == "source" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isRemoteURL(attr.Value) {
					r.AddWithLocation(report.Error, "RSC-006",
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
				r.AddWithLocation(report.Error, "RSC-006",
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
