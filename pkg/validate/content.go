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

	// Build map of manifest item ID -> spine itemref properties for rendition overrides
	spineProps := make(map[string]string)
	spineItemIDs := make(map[string]bool)
	for _, ref := range ep.Package.Spine {
		spineProps[ref.IDRef] = ref.Properties
		spineItemIDs[ref.IDRef] = true
	}

	// Check SVG content documents for remote-resources property
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		if item.MediaType != "image/svg+xml" {
			continue
		}
		if ep.Package.Version < "3.0" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue
		}
		checkSVGPropertyDeclarations(ep, data, fullPath, item, r)
		checkNoRemoteResources(ep, data, fullPath, item, r)
	}

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

		// HTM-013/HTM-014: FXL viewport checks (only for spine items)
		if ep.Package.Version >= "3.0" && spineItemIDs[item.ID] {
			// Determine if this specific item is fixed-layout, considering
			// per-spine-item rendition overrides
			itemIsFXL := isFXL
			if props, ok := spineProps[item.ID]; ok {
				if hasProperty(props, "rendition:layout-reflowable") {
					itemIsFXL = false
				} else if hasProperty(props, "rendition:layout-pre-paginated") {
					itemIsFXL = true
				}
			}
			// Skip nav document from FXL viewport checks
			if itemIsFXL && !hasProperty(item.Properties, "nav") {
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

		// HTM-031: custom attribute namespaces must be valid
		if ep.Package.Version >= "3.0" {
			checkCustomAttributeNamespaces(data, fullPath, r)
		}

		// HTM-032: style element CSS syntax
		checkStyleElementValid(data, fullPath, r)

		// HTM-033: no RDF elements in content
		checkNoRDFElements(data, fullPath, r)

		// RSC-032: foreign resources referenced from content must have fallbacks
		if ep.Package.Version >= "3.0" && !isNav {
			checkForeignResourceFallbacks(ep, data, fullPath, r)
		}
	}
}

// sourceRef holds a buffered <source> element for deferred audio/video checking.
type sourceRef struct {
	href      string
	mediaType string // from type attribute (may be empty)
}

// RSC-032: foreign resources (non-core media types) referenced from content
// documents must have proper fallbacks (manifest fallback or HTML fallback).
func checkForeignResourceFallbacks(ep *epub.EPUB, data []byte, location string, r *report.Report) {
	// Build manifest maps
	manifestByHref := make(map[string]epub.ManifestItem) // resolved path -> item
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestByHref[ep.ResolveHref(item.Href)] = item
		}
	}

	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	itemDir := path.Dir(location)
	inPicture := false
	// Track audio/video context: when non-empty, we're inside that element
	// and buffer <source> elements to check as a group (HTML fallback mechanism)
	mediaParent := "" // "audio" or "video" or ""
	var bufferedSources []sourceRef

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		// Track end elements
		if ee, ok := tok.(xml.EndElement); ok {
			switch ee.Name.Local {
			case "picture":
				inPicture = false
			case "audio", "video":
				if mediaParent == ee.Name.Local {
					// Process buffered sources: if any source resolves to a core type,
					// that's the HTML fallback — foreign sources in the same element are OK.
					checkAudioVideoSources(ep, bufferedSources, itemDir, location, manifestByHref, r)
					mediaParent = ""
					bufferedSources = nil
				}
			}
			continue
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch se.Name.Local {
		case "picture":
			inPicture = true
		case "img":
			if inPicture {
				// MED-003: img inside picture must reference core media types
				checkPictureImgRef(ep, se, itemDir, location, manifestByHref, r)
			} else {
				for _, attr := range se.Attr {
					if attr.Name.Local == "src" {
						checkForeignRef(ep, attr.Value, itemDir, location, manifestByHref, "img", r)
					}
				}
			}
		case "audio", "video":
			mediaParent = se.Name.Local
			bufferedSources = nil
			// Check direct src attribute (not the <source> fallback mechanism)
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" {
					checkForeignRef(ep, attr.Value, itemDir, location, manifestByHref, se.Name.Local, r)
				}
				if se.Name.Local == "video" && attr.Name.Local == "poster" {
					checkForeignRef(ep, attr.Value, itemDir, location, manifestByHref, "poster", r)
				}
			}
		case "source":
			if inPicture {
				// MED-007: source in picture without type attr for foreign resource
				checkPictureSourceRef(ep, se, itemDir, location, manifestByHref, r)
			} else if mediaParent != "" {
				// Buffer for deferred audio/video HTML fallback check
				var href, typeAttr string
				for _, attr := range se.Attr {
					if attr.Name.Local == "src" {
						href = attr.Value
					}
					if attr.Name.Local == "type" {
						typeAttr = attr.Value
					}
				}
				if href != "" {
					bufferedSources = append(bufferedSources, sourceRef{href: href, mediaType: typeAttr})
				}
			} else {
				for _, attr := range se.Attr {
					if attr.Name.Local == "src" {
						checkForeignRef(ep, attr.Value, itemDir, location, manifestByHref, "source", r)
					}
				}
			}
		case "embed":
			var embedSrc, embedType string
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" {
					embedSrc = attr.Value
				}
				if attr.Name.Local == "type" {
					embedType = attr.Value
				}
			}
			if embedSrc != "" {
				if !checkTypeMismatch(embedSrc, embedType, itemDir, location, manifestByHref, r) {
					checkForeignRef(ep, embedSrc, itemDir, location, manifestByHref, "embed", r)
				}
			}
		case "object":
			var objectData, objectType string
			for _, attr := range se.Attr {
				if attr.Name.Local == "data" {
					objectData = attr.Value
				}
				if attr.Name.Local == "type" {
					objectType = attr.Value
				}
			}
			if objectData != "" {
				if !checkTypeMismatch(objectData, objectType, itemDir, location, manifestByHref, r) {
					checkForeignRef(ep, objectData, itemDir, location, manifestByHref, "object", r)
				}
			}
		case "input":
			var inputType, src string
			for _, attr := range se.Attr {
				if attr.Name.Local == "type" {
					inputType = attr.Value
				}
				if attr.Name.Local == "src" {
					src = attr.Value
				}
			}
			if inputType == "image" && src != "" {
				checkForeignRef(ep, src, itemDir, location, manifestByHref, "input-image", r)
			}
		case "math":
			for _, attr := range se.Attr {
				if attr.Name.Local == "altimg" {
					checkForeignRef(ep, attr.Value, itemDir, location, manifestByHref, "math-altimg", r)
				}
			}
		}
	}
}

// checkAudioVideoSources processes buffered <source> elements from an <audio> or <video>.
// If any source resolves to a core media type, foreign sources are OK (HTML fallback).
// Otherwise RSC-032 fires for each foreign source with no manifest fallback.
func checkAudioVideoSources(ep *epub.EPUB, sources []sourceRef, itemDir, location string, manifestByHref map[string]epub.ManifestItem, r *report.Report) {
	if len(sources) == 0 {
		return
	}

	// Check if any source is a core media type (HTML fallback present)
	hasCoreSource := false
	for _, src := range sources {
		if src.href == "" || isRemoteURL(src.href) {
			continue
		}
		// Check via type attribute first
		if src.mediaType != "" {
			mt := src.mediaType
			if idx := strings.Index(mt, ";"); idx >= 0 {
				mt = strings.TrimSpace(mt[:idx])
			}
			if coreMediaTypes[mt] {
				hasCoreSource = true
				break
			}
		}
		// Check via manifest
		u, err := url.Parse(src.href)
		if err != nil {
			continue
		}
		target := resolvePath(itemDir, u.Path)
		item, ok := manifestByHref[target]
		if !ok {
			continue
		}
		mt := item.MediaType
		if idx := strings.Index(mt, ";"); idx >= 0 {
			mt = strings.TrimSpace(mt[:idx])
		}
		if coreMediaTypes[mt] {
			hasCoreSource = true
			break
		}
	}

	if hasCoreSource {
		return // HTML fallback mechanism satisfied
	}

	// No core source — check each foreign source individually
	for _, src := range sources {
		checkForeignRef(ep, src.href, itemDir, location, manifestByHref, "source", r)
	}
}


// checkPictureImgRef checks img src/srcset inside <picture> — must be core types (MED-003).
// Reports once per unique foreign resource (deduplicates src vs srcset references).
func checkPictureImgRef(ep *epub.EPUB, se xml.StartElement, itemDir, location string, manifestByHref map[string]epub.ManifestItem, r *report.Report) {
	reported := make(map[string]bool)
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "src":
			if !reported[attr.Value] {
				reported[attr.Value] = true
				checkPictureForeignRef(ep, attr.Value, itemDir, location, manifestByHref, r)
			}
		case "srcset":
			// Parse srcset: "url [descriptor], url [descriptor], ..."
			for _, entry := range strings.Split(attr.Value, ",") {
				parts := strings.Fields(strings.TrimSpace(entry))
				if len(parts) > 0 && parts[0] != "" {
					if !reported[parts[0]] {
						reported[parts[0]] = true
						checkPictureForeignRef(ep, parts[0], itemDir, location, manifestByHref, r)
					}
				}
			}
		}
	}
}

// checkPictureSourceRef checks source elements inside <picture>.
// OPF-013 if type attr doesn't match manifest media-type.
// MED-007 if no type attr for foreign resource.
func checkPictureSourceRef(ep *epub.EPUB, se xml.StartElement, itemDir, location string, manifestByHref map[string]epub.ManifestItem, r *report.Report) {
	var typeAttr, srcset string
	for _, attr := range se.Attr {
		if attr.Name.Local == "type" {
			typeAttr = attr.Value
		}
		if attr.Name.Local == "srcset" {
			srcset = attr.Value
		}
	}
	if srcset == "" {
		return
	}
	// Get the first URL from srcset for type mismatch check
	firstHref := ""
	for _, entry := range strings.Split(srcset, ",") {
		parts := strings.Fields(strings.TrimSpace(entry))
		if len(parts) > 0 && parts[0] != "" {
			firstHref = parts[0]
			break
		}
	}
	// OPF-013: type attribute doesn't match manifest media-type
	if typeAttr != "" && firstHref != "" {
		if checkTypeMismatch(firstHref, typeAttr, itemDir, location, manifestByHref, r) {
			return
		}
		return // type matches or no manifest entry — skip MED-007
	}
	// No type attribute: check if any srcset URL references a foreign resource → MED-007
	// No type attribute: check if any srcset URL references a foreign resource
	for _, entry := range strings.Split(srcset, ",") {
		parts := strings.Fields(strings.TrimSpace(entry))
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		href := parts[0]
		if isRemoteURL(href) {
			continue
		}
		u, err := url.Parse(href)
		if err != nil {
			continue
		}
		target := resolvePath(itemDir, u.Path)
		item, ok := manifestByHref[target]
		if !ok {
			continue
		}
		mt := item.MediaType
		if idx := strings.Index(mt, ";"); idx >= 0 {
			mt = strings.TrimSpace(mt[:idx])
		}
		if !coreMediaTypes[mt] {
			r.AddWithLocation(report.Error, "MED-007",
				fmt.Sprintf("The `source` element references a foreign resource '%s' but does not declare its media type in a 'type' attribute", href),
				location)
			return // Report once per source element
		}
	}
}

// checkPictureForeignRef checks a reference inside <picture><img> — foreign types → MED-003.
func checkPictureForeignRef(ep *epub.EPUB, href, itemDir, location string, manifestByHref map[string]epub.ManifestItem, r *report.Report) {
	if href == "" || isRemoteURL(href) {
		return
	}
	u, err := url.Parse(href)
	if err != nil {
		return
	}
	refPath := u.Path
	if refPath == "" {
		return
	}
	target := resolvePath(itemDir, refPath)
	item, ok := manifestByHref[target]
	if !ok {
		return
	}
	mt := item.MediaType
	if idx := strings.Index(mt, ";"); idx >= 0 {
		mt = strings.TrimSpace(mt[:idx])
	}
	if coreMediaTypes[mt] {
		return
	}
	r.AddWithLocation(report.Error, "MED-003",
		fmt.Sprintf("The `picture` element's `img` fallback references a foreign resource '%s' of type '%s'", href, item.MediaType),
		location)
}

// checkTypeMismatch checks if the element's type attribute matches the manifest media-type.
// If they don't match, OPF-013 is emitted and true is returned (caller should skip RSC-032).
func checkTypeMismatch(href, typeAttr, itemDir, location string, manifestByHref map[string]epub.ManifestItem, r *report.Report) bool {
	if typeAttr == "" || href == "" || isRemoteURL(href) {
		return false
	}
	u, err := url.Parse(href)
	if err != nil || u.Path == "" {
		return false
	}
	target := resolvePath(itemDir, u.Path)
	item, ok := manifestByHref[target]
	if !ok {
		return false
	}
	manifestMT := item.MediaType
	if idx := strings.Index(manifestMT, ";"); idx >= 0 {
		manifestMT = strings.TrimSpace(manifestMT[:idx])
	}
	declaredMT := typeAttr
	if idx := strings.Index(declaredMT, ";"); idx >= 0 {
		declaredMT = strings.TrimSpace(declaredMT[:idx])
	}
	if !strings.EqualFold(manifestMT, declaredMT) {
		r.AddWithLocation(report.Warning, "OPF-013",
			fmt.Sprintf("'type' attribute value '%s' does not match the resource's manifest media type '%s'", typeAttr, item.MediaType),
			location)
		return true
	}
	return false
}

func checkForeignRef(ep *epub.EPUB, href, itemDir, location string, manifestByHref map[string]epub.ManifestItem, context string, r *report.Report) {
	if href == "" || isRemoteURL(href) || strings.HasPrefix(href, "data:") {
		// Remote resources and data URIs handled separately
		if strings.HasPrefix(href, "data:") {
			// data: URIs with foreign types need reporting
			if strings.HasPrefix(href, "data:image/") {
				mt := strings.SplitN(href[5:], ";", 2)[0]
				mt = strings.SplitN(mt, ",", 2)[0]
				if !coreMediaTypes[mt] {
					r.AddWithLocation(report.Error, "RSC-032",
						fmt.Sprintf("Fallback must be provided for foreign resource: data URI with media type '%s'", mt),
						location)
				}
			}
		}
		return
	}

	u, err := url.Parse(href)
	if err != nil {
		return
	}
	refPath := u.Path
	if refPath == "" {
		return
	}
	target := resolvePath(itemDir, refPath)
	item, ok := manifestByHref[target]
	if !ok {
		return // Not in manifest - handled by RSC-007
	}

	// Check if the media type is foreign (non-core)
	// Strip parameters (e.g., "audio/ogg ; codecs=opus" -> "audio/ogg")
	mt := item.MediaType
	if idx := strings.Index(mt, ";"); idx >= 0 {
		mt = strings.TrimSpace(mt[:idx])
	}
	if coreMediaTypes[mt] {
		return // Core media type, no fallback needed
	}

	// Exempt: fonts, video (in video/img), audio (in audio/source) are
	// allowed to use foreign types without fallbacks per EPUB spec
	if isFontMediaType(mt) {
		return // Font types are always exempt
	}
	if strings.HasPrefix(mt, "video/") && (context == "video" || context == "source" || context == "img" || context == "object") {
		return // Video foreign types are exempt in video/source/img/object context
	}
	// Note: audio in <source> is NOT exempt - non-core audio types still need fallbacks

	// Check for manifest fallback
	if item.Fallback != "" {
		return // Has manifest fallback
	}

	r.AddWithLocation(report.Error, "RSC-032",
		fmt.Sprintf("Fallback must be provided for foreign resource '%s' of type '%s'", href, item.MediaType),
		location)
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

// checkPropertyDeclarations: check for script/SVG/MathML/switch/form/remote-resources
// and verify declared manifest properties match actual content.
// OPF-014: property needed but not declared
// OPF-015: property declared but not needed
func checkPropertyDeclarations(ep *epub.EPUB, data []byte, location string, item epub.ManifestItem, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	hasScript := false
	hasSVG := false
	hasMathML := false
	hasSwitch := false
	hasForm := false
	hasRemoteResources := false
	hasRemoteInlineCSS := false

	// Build set of linked CSS hrefs to check for remote resources
	var linkedCSSHrefs []string

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
			// Per HTML spec, <script type="text/plain"> and other non-JS types
			// are data blocks, not executable scripts. Only count as scripted
			// if type is absent or a JavaScript-compatible MIME type.
			scriptType := ""
			for _, attr := range se.Attr {
				if attr.Name.Local == "type" {
					scriptType = strings.TrimSpace(strings.ToLower(attr.Value))
				}
			}
			if isExecutableScriptType(scriptType) {
				hasScript = true
			}
		}
		if se.Name.Local == "svg" || se.Name.Space == "http://www.w3.org/2000/svg" {
			hasSVG = true
		}
		if se.Name.Local == "math" || se.Name.Space == "http://www.w3.org/1998/Math/MathML" {
			hasMathML = true
		}
		// epub:switch detection
		if se.Name.Local == "switch" && (se.Name.Space == "http://www.idpf.org/2007/ops" ||
			strings.HasPrefix(getAttrVal(se, "xmlns:epub"), "http://www.idpf.org/2007/ops")) {
			hasSwitch = true
		}
		// Form elements count as scripted per epubcheck
		if se.Name.Local == "form" {
			hasForm = true
		}
		// Check for remote resource references in content elements
		switch se.Name.Local {
		case "audio", "video", "source", "img", "iframe", "object", "embed":
			for _, attr := range se.Attr {
				if (attr.Name.Local == "src" || attr.Name.Local == "poster" || attr.Name.Local == "data") && isRemoteURL(attr.Value) {
					hasRemoteResources = true
				}
			}
		case "script":
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isRemoteURL(attr.Value) {
					hasRemoteResources = true
				}
			}
		}
		// Collect linked CSS stylesheet hrefs
		if se.Name.Local == "link" {
			rel := ""
			href := ""
			for _, attr := range se.Attr {
				if attr.Name.Local == "rel" {
					rel = attr.Value
				}
				if attr.Name.Local == "href" {
					href = attr.Value
				}
			}
			if strings.Contains(rel, "stylesheet") && href != "" {
				linkedCSSHrefs = append(linkedCSSHrefs, href)
			}
		}
		// Check inline <style> for remote resources
		if se.Name.Local == "style" {
			// Read style content
			var styleContent string
			for {
				t, err := decoder.Token()
				if err != nil {
					break
				}
				if cd, ok := t.(xml.CharData); ok {
					styleContent += string(cd)
				}
				if _, ok := t.(xml.EndElement); ok {
					break
				}
			}
			if hasRemoteURLInCSS(styleContent) {
				hasRemoteInlineCSS = true
			}
		}
	}

	// Check linked CSS files for remote resources
	itemDir := path.Dir(location)
	for _, href := range linkedCSSHrefs {
		if isRemoteURL(href) {
			continue
		}
		cssPath := resolvePath(itemDir, href)
		cssData, err := ep.ReadFile(cssPath)
		if err != nil {
			continue
		}
		if hasRemoteURLInCSS(string(cssData)) {
			hasRemoteResources = true
			break
		}
	}
	if hasRemoteInlineCSS {
		hasRemoteResources = true
	}

	// OPF-014: property needed but not declared
	if hasScript && !hasProperty(item.Properties, "scripted") {
		r.AddWithLocation(report.Error, "OPF-014",
			"Property 'scripted' should be declared in the manifest for scripted content",
			location)
	}
	if hasForm && !hasProperty(item.Properties, "scripted") && !hasScript {
		r.AddWithLocation(report.Error, "OPF-014",
			"Property 'scripted' should be declared in the manifest for content with form elements",
			location)
	}
	if hasSVG && !hasProperty(item.Properties, "svg") {
		r.AddWithLocation(report.Error, "OPF-014",
			"Property 'svg' should be declared in the manifest for content with inline SVG",
			location)
	}
	if hasMathML && !hasProperty(item.Properties, "mathml") {
		r.AddWithLocation(report.Error, "OPF-014",
			"Property 'mathml' should be declared in the manifest for content with MathML",
			location)
	}
	if hasSwitch {
		// RSC-017: epub:switch is deprecated
		r.AddWithLocation(report.Warning, "RSC-017",
			`The "epub:switch" element is deprecated`,
			location)
		if !hasProperty(item.Properties, "switch") {
			r.AddWithLocation(report.Error, "OPF-014",
				"Property 'switch' should be declared in the manifest for content with epub:switch",
				location)
		}
	}
	if hasRemoteResources && !hasProperty(item.Properties, "remote-resources") {
		r.AddWithLocation(report.Error, "OPF-014",
			"Property 'remote-resources' should be declared in the manifest for content with remote resources",
			location)
	}

	// OPF-015: property declared but not needed
	hasScriptOrForm := hasScript || hasForm
	if hasProperty(item.Properties, "scripted") && !hasScriptOrForm {
		r.AddWithLocation(report.Error, "OPF-015",
			"Property 'scripted' is declared in the manifest but the content does not contain scripted elements",
			location)
	}
	if hasProperty(item.Properties, "svg") && !hasSVG {
		r.AddWithLocation(report.Error, "OPF-015",
			"Property 'svg' is declared in the manifest but the content does not contain SVG elements",
			location)
	}
	if hasProperty(item.Properties, "remote-resources") && !hasRemoteResources {
		// OPF-018 in epubcheck for unnecessary remote-resources is a warning
		r.AddWithLocation(report.Warning, "OPF-018",
			"Property 'remote-resources' is declared in the manifest but is not needed",
			location)
	}
}

// hasRemoteURLInCSS checks if CSS content contains any remote URL references
// (excluding @namespace and @import declarations which are handled separately)
func hasRemoteURLInCSS(css string) bool {
	// Remove @namespace lines (use url() for identifiers, not resources)
	namespaceRe := regexp.MustCompile(`(?m)@namespace\s+[^\n;]+;`)
	cleaned := namespaceRe.ReplaceAllString(css, "")
	// Remove @import lines (remote @import is handled by RSC-006, not remote-resources)
	importRe := regexp.MustCompile(`(?m)@import\s+[^\n;]+;`)
	cleaned = importRe.ReplaceAllString(cleaned, "")
	urlRe := regexp.MustCompile(`url\(['"]?(https?://[^'"\)\s]+)['"]?\)`)
	return urlRe.MatchString(cleaned)
}

// getAttrVal gets an attribute value by local name from a start element
func getAttrVal(se xml.StartElement, name string) string {
	for _, attr := range se.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

// checkSVGPropertyDeclarations checks SVG content documents for remote resources
// and verifies the remote-resources manifest property is declared.
func checkSVGPropertyDeclarations(ep *epub.EPUB, data []byte, location string, item epub.ManifestItem, r *report.Report) {
	content := string(data)
	hasRemote := false

	// Strip XML processing instructions — their remote hrefs are disallowed (RSC-006)
	// and should not trigger the remote-resources property requirement (OPF-014).
	piRe := regexp.MustCompile(`<\?[^?]*\?>`)
	stripped := piRe.ReplaceAllString(content, "")

	// Check for remote URLs in SVG element attributes (href, xlink:href)
	remoteRe := regexp.MustCompile(`(?:href|xlink:href)\s*=\s*["'](https?://[^"']+)["']`)
	if remoteRe.MatchString(stripped) {
		hasRemote = true
	}
	// hasRemoteURLInCSS already strips @import (which fire RSC-006, not OPF-014)
	if hasRemoteURLInCSS(stripped) {
		hasRemote = true
	}

	if hasRemote && !hasProperty(item.Properties, "remote-resources") {
		r.AddWithLocation(report.Error, "OPF-014",
			"Property 'remote-resources' should be declared in the manifest for content with remote resources",
			location)
	}
}

// viewportDim represents a parsed key=value pair from a viewport meta content.
type viewportDim struct {
	key   string
	value string
	hasEq bool // true if '=' was present in the source
}

// viewportUnits matches a trailing CSS unit or % on a dimension value.
var viewportUnitRe = regexp.MustCompile(`(?i)(px|em|ex|rem|%|vw|vh|pt|pc|cm|mm|in)$`)

// parseViewportDims splits a viewport content string into key[=value] pairs.
func parseViewportDims(content string) []viewportDim {
	var dims []viewportDim
	for _, part := range strings.Split(content, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.IndexByte(part, '=')
		if idx < 0 {
			dims = append(dims, viewportDim{key: strings.ToLower(strings.TrimSpace(part))})
		} else {
			key := strings.ToLower(strings.TrimSpace(part[:idx]))
			val := part[idx+1:] // keep original spacing for whitespace-only detection
			dims = append(dims, viewportDim{key: key, value: val, hasEq: true})
		}
	}
	return dims
}

// HTM-046/047/056/057/059: Fixed-layout XHTML viewport checks
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
		r.AddWithLocation(report.Error, "HTM-046",
			"Fixed-layout content document has no viewport meta element",
			location)
		return
	}

	dims := parseViewportDims(viewportContent)

	// HTM-047: key= with empty or all-whitespace value (syntax invalid)
	for _, d := range dims {
		if d.hasEq && strings.TrimSpace(d.value) == "" {
			r.AddWithLocation(report.Error, "HTM-047",
				fmt.Sprintf("The viewport meta element has an invalid value for dimension '%s'", d.key),
				location)
			return
		}
	}

	// HTM-059: duplicate width or height keys
	seen := make(map[string]int)
	for _, d := range dims {
		if d.key == "width" || d.key == "height" {
			seen[d.key]++
		}
	}
	for _, key := range []string{"width", "height"} {
		if seen[key] > 1 {
			r.AddWithLocation(report.Error, "HTM-059",
				fmt.Sprintf("The viewport meta element declares '%s' more than once", key),
				location)
		}
	}
	if seen["width"] > 1 || seen["height"] > 1 {
		return
	}

	// HTM-057: dimension present but value has units or no value (key without =)
	for _, d := range dims {
		if d.key != "width" && d.key != "height" {
			continue
		}
		val := strings.TrimSpace(d.value)
		if !d.hasEq || val == "" {
			// key with no = at all (empty value treated as HTM-057)
			r.AddWithLocation(report.Error, "HTM-057",
				fmt.Sprintf("The value of viewport dimension '%s' must be a number without units", d.key),
				location)
		} else if viewportUnitRe.MatchString(val) {
			r.AddWithLocation(report.Error, "HTM-057",
				fmt.Sprintf("The value of viewport dimension '%s' must be a number without units", d.key),
				location)
		}
	}

	// HTM-056: missing width or height
	hasWidth := false
	hasHeight := false
	for _, d := range dims {
		if d.key == "width" {
			hasWidth = true
		} else if d.key == "height" {
			hasHeight = true
		}
	}
	if !hasWidth || !hasHeight {
		r.AddWithLocation(report.Error, "HTM-056",
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

	// Skip media fragment URIs (EPUB Region-Based Navigation, Media Fragments).
	// These use schemes like #xywh=, #xyn=, #t= and are not HTML element IDs.
	if strings.HasPrefix(fragment, "xywh=") || strings.HasPrefix(fragment, "xyn=") ||
		strings.HasPrefix(fragment, "t=") || strings.HasPrefix(fragment, "epubcfi(") {
		return
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

// checkNoRemoteResources validates remote resource usage.
// Per EPUB spec:
// - Remote audio/video are ALLOWED (just need remote-resources property)
// - Remote fonts (in CSS/SVG) are ALLOWED
// - Remote images, iframes, scripts, stylesheets, objects are NOT allowed (RSC-006)
func checkNoRemoteResources(ep *epub.EPUB, data []byte, location string, item epub.ManifestItem, r *report.Report) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	// Match href="..." in processing instruction data
	piHrefRe := regexp.MustCompile(`href\s*=\s*["']([^"']+)["']`)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		// RSC-006: Remote stylesheet in SVG <?xml-stylesheet href="...">
		if pi, ok := tok.(xml.ProcInst); ok {
			if pi.Target == "xml-stylesheet" {
				m := piHrefRe.FindSubmatch(pi.Inst)
				if m != nil && isRemoteURL(string(m[1])) {
					r.AddWithLocation(report.Error, "RSC-006",
						fmt.Sprintf("Remote resource reference is not allowed: '%s'", string(m[1])),
						location)
				}
			}
			continue
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		// RSC-006: Remote image resources are not allowed
		if se.Name.Local == "img" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isRemoteURL(attr.Value) {
					r.AddWithLocation(report.Error, "RSC-006",
						fmt.Sprintf("Remote resource reference is not allowed: '%s'", attr.Value),
						location)
				}
			}
		}

		// RSC-006: Remote iframe/embed resources are not allowed
		if se.Name.Local == "iframe" || se.Name.Local == "embed" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isRemoteURL(attr.Value) {
					r.AddWithLocation(report.Error, "RSC-006",
						fmt.Sprintf("Remote resource reference is not allowed: '%s'", attr.Value),
						location)
				}
			}
		}
		// RSC-006: Remote object data is not allowed
		if se.Name.Local == "object" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "data" && isRemoteURL(attr.Value) {
					// Check if this remote object references audio/video (allowed)
					var objType string
					for _, a2 := range se.Attr {
						if a2.Name.Local == "type" {
							objType = a2.Value
						}
					}
					if strings.HasPrefix(objType, "audio/") || strings.HasPrefix(objType, "video/") {
						// Remote audio/video via object is allowed
						// Property check handled by checkPropertyDeclarations
					} else {
						r.AddWithLocation(report.Error, "RSC-006",
							fmt.Sprintf("Remote resource reference is not allowed: '%s'", attr.Value),
							location)
					}
				}
			}
		}

		// RSC-006: Remote script references are not allowed
		if se.Name.Local == "script" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isRemoteURL(attr.Value) {
					r.AddWithLocation(report.Error, "RSC-006",
						fmt.Sprintf("Remote resource reference is not allowed: '%s'", attr.Value),
						location)
				}
			}
		}

		// Remote audio/video resources are ALLOWED in EPUB 3.
		// RSC-031: warn when using http:// instead of https:// for remote resources.
		if se.Name.Local == "audio" || se.Name.Local == "video" || se.Name.Local == "source" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && isNonHTTPSRemote(attr.Value) {
					r.AddWithLocation(report.Warning, "RSC-031",
						fmt.Sprintf("Remote resource uses insecure 'http' scheme: '%s'", attr.Value),
						location)
				}
			}
		}

		// RSC-006: Remote stylesheet references are not allowed
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

		// RSC-006: Remote stylesheet in SVG inline <style> @import
		if se.Name.Local == "style" {
			inner, _ := decoder.Token()
			if cd, ok2 := inner.(xml.CharData); ok2 {
				css := string(cd)
				importRe := regexp.MustCompile(`@import\s+(?:url\(['"]?|['"])([^'")\s]+)`)
				for _, m := range importRe.FindAllStringSubmatch(css, -1) {
					if isRemoteURL(m[1]) {
						r.AddWithLocation(report.Error, "RSC-006",
							fmt.Sprintf("Remote resource reference is not allowed: '%s'", m[1]),
							location)
					}
				}
			}
		}
	}
}

func isRemoteURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func isFileURL(s string) bool {
	return strings.HasPrefix(s, "file://") || strings.HasPrefix(s, "file:/")
}

func isNonHTTPSRemote(s string) bool {
	return strings.HasPrefix(s, "http://")
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

		// Check <img src="..."> and <img srcset="..."> for image references
		if se.Name.Local == "img" {
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "src":
					checkResourceRef(ep, attr.Value, itemDir, fullPath, manifestPaths, r)
				case "srcset":
					// RSC-008: srcset resources must be declared in the manifest
					checkSrcsetRef(ep, attr.Value, itemDir, fullPath, manifestPaths, r)
				}
			}
		}

		// RSC-007: Check <iframe src="...">, <embed src="...">, <object data="...">
		if se.Name.Local == "iframe" || se.Name.Local == "embed" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" {
					checkResourceRef(ep, attr.Value, itemDir, fullPath, manifestPaths, r)
				}
			}
		}
		if se.Name.Local == "object" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "data" {
					checkResourceRef(ep, attr.Value, itemDir, fullPath, manifestPaths, r)
				}
			}
		}

		// RSC-007: Check <audio src="...">, <video src="...">, <source src="...">, <track src="...">
		if se.Name.Local == "audio" || se.Name.Local == "video" || se.Name.Local == "source" || se.Name.Local == "track" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" {
					checkResourceRef(ep, attr.Value, itemDir, fullPath, manifestPaths, r)
				}
			}
		}

		// RSC-007: Check <blockquote cite="...">, <q cite="...">, <ins cite="...">, <del cite="...">
		if se.Name.Local == "blockquote" || se.Name.Local == "q" || se.Name.Local == "ins" || se.Name.Local == "del" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "cite" {
					checkResourceRef(ep, attr.Value, itemDir, fullPath, manifestPaths, r)
				}
			}
		}

		// RSC-007: Check <link rel="stylesheet" href="..."> for missing stylesheets
		if se.Name.Local == "link" {
			rel := ""
			href := ""
			for _, attr := range se.Attr {
				if attr.Name.Local == "rel" {
					rel = attr.Value
				}
				if attr.Name.Local == "href" {
					href = attr.Value
				}
			}
			if strings.Contains(strings.ToLower(rel), "stylesheet") && href != "" && !isRemoteURL(href) {
				// RSC-013: stylesheet URLs must not contain fragment identifiers
				if u, err := url.Parse(href); err == nil && u.Fragment != "" {
					r.AddWithLocation(report.Error, "RSC-013",
						fmt.Sprintf("Fragment identifier is not allowed in stylesheet URL: '%s'", href),
						fullPath)
				} else {
					target := resolvePath(itemDir, href)
					if _, exists := ep.Files[target]; !exists {
						r.AddWithLocation(report.Error, "RSC-007",
							fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
							fullPath)
					}
				}
			}
		}
	}
}

// checkHyperlink validates a hyperlink reference from a content document.
func checkHyperlink(ep *epub.EPUB, href, itemDir, location string, r *report.Report) {
	if strings.TrimSpace(href) == "" {
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

	// Skip absolute paths (starting with /) — these are not valid EPUB
	// container references and typically come from embedded web content
	// (e.g., Wikipedia articles with /wiki/... links).
	if strings.HasPrefix(refPath, "/") {
		return
	}

	target := resolvePath(itemDir, refPath)
	if _, exists := ep.Files[target]; !exists {
		r.AddWithLocation(report.Error, "RSC-007",
			fmt.Sprintf("Referenced resource '%s' could not be found in the container", refPath),
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
		return // remote URL - handled by remote resource checks
	}

	refPath := u.Path
	if refPath == "" {
		return // fragment-only reference
	}

	// Skip absolute paths
	if strings.HasPrefix(refPath, "/") {
		return
	}

	target := resolvePath(itemDir, refPath)
	if manifestPaths[target] {
		return // good - exists in container and in manifest
	}
	if _, exists := ep.Files[target]; !exists {
		// RSC-007: not found in container at all
		r.AddWithLocation(report.Error, "RSC-007",
			fmt.Sprintf("Referenced resource '%s' could not be found in the container", src),
			location)
	} else {
		// RSC-006: exists in container but not declared in manifest
		r.AddWithLocation(report.Error, "RSC-006",
			fmt.Sprintf("Referenced resource '%s' is not declared in the OPF manifest", src),
			location)
	}
}

// checkSrcsetRef checks each URL in a srcset attribute.
// RSC-007: resource not found in container.
// RSC-008: resource in container but not declared in manifest.
func checkSrcsetRef(ep *epub.EPUB, srcset, itemDir, location string, manifestPaths map[string]bool, r *report.Report) {
	reported := make(map[string]bool)
	for _, entry := range strings.Split(srcset, ",") {
		parts := strings.Fields(strings.TrimSpace(entry))
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		href := parts[0]
		if reported[href] || isRemoteURL(href) {
			continue
		}
		reported[href] = true
		u, err := url.Parse(href)
		if err != nil || u.Path == "" {
			continue
		}
		target := resolvePath(itemDir, u.Path)
		if manifestPaths[target] {
			continue // declared in manifest — OK
		}
		if _, exists := ep.Files[target]; !exists {
			r.AddWithLocation(report.Error, "RSC-007",
				fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
				location)
		} else {
			r.AddWithLocation(report.Error, "RSC-008",
				fmt.Sprintf("Referenced resource '%s' is not declared in the OPF manifest", href),
				location)
		}
	}
}

// isExecutableScriptType returns true if the script type attribute value
// indicates executable JavaScript. Per HTML spec, a <script> is executable
// if type is absent/empty, or matches a JavaScript MIME type. Non-JS types
// like "text/plain" or "application/ld+json" are data blocks.
func isExecutableScriptType(t string) bool {
	if t == "" {
		return true // no type = JavaScript
	}
	jsTypes := map[string]bool{
		"text/javascript":        true,
		"application/javascript": true,
		"text/ecmascript":        true,
		"application/ecmascript": true,
		"module":                 true,
	}
	return jsTypes[t]
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
	"answers": true, "antonym-group": true, "appendix": true, "aside": true,
	"assessment": true, "assessments": true,
	"backlink": true, "backmatter": true, "balloon": true,
	"biblioentry": true, "bibliography": true, "biblioref": true,
	"bodymatter": true, "bridgehead": true,
	"chapter": true, "colophon": true, "concluding-sentence": true,
	"conclusion": true, "condensed-entry": true, "contributors": true,
	"copyright-page": true, "cover": true, "covertitle": true,
	"credit": true, "credits": true,
	"dedication": true, "def": true, "dictentry": true, "dictionary": true,
	"division": true,
	"endnote": true, "endnotes": true, "epigraph": true, "epilogue": true,
	"errata": true, "etymology": true, "example": true,
	"figure": true, "fill-in-the-blank-problem": true,
	"footnote": true, "footnotes": true, "foreword": true,
	"frontmatter": true, "fulltitle": true,
	"general-problem": true, "glossary": true, "glossdef": true,
	"glossref": true, "glossterm": true, "gram-info": true,
	"halftitle": true, "halftitlepage": true, "help": true,
	"idiom": true, "imprimatur": true, "imprint": true,
	"index": true, "index-editor-note": true, "index-entry": true,
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
	"part-of-speech": true, "part-of-speech-group": true, "part-of-speech-list": true,
	"phonetic-transcription": true, "phrase-group": true, "phrase-list": true,
	"practice": true, "practices": true, "preamble": true, "preface": true,
	"prologue": true, "pullquote": true, "qna": true, "question": true,
	"referrer": true, "revision-history": true,
	"sense-group": true, "sense-list": true, "sound-area": true,
	"subchapter": true, "subtitle": true, "synonym-group": true,
	"table": true, "table-cell": true, "table-row": true,
	"text-area": true, "tip": true, "title": true, "titlepage": true,
	"toc": true, "toc-brief": true, "topic-sentence": true,
	"tran": true, "tran-info": true, "true-false-problem": true,
	"volume": true, "warning": true,
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
						r.AddWithLocation(report.Info, "HTM-015",
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
			r.AddWithLocation(report.Info, "HTM-020",
				fmt.Sprintf("Processing instruction '%s' found in EPUB content document", pi.Target),
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
			r.AddWithLocation(report.Error, "RSC-005",
				fmt.Sprintf("lang and xml:lang attributes must have the same value when both are present, but found '%s' and '%s'", lang, xmlLang),
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

// Allowed attribute namespaces in EPUB XHTML content documents.
var allowedAttrNamespaces = map[string]bool{
	"":                                     true, // no namespace (plain HTML attributes)
	"xmlns":                                true, // namespace declarations (Go xml parser representation)
	"http://www.w3.org/1999/xhtml":         true, // XHTML
	"http://www.w3.org/XML/1998/namespace":  true, // xml: prefix
	"http://www.w3.org/2000/xmlns/":         true, // xmlns: declarations
	"http://www.idpf.org/2007/ops":          true, // epub: prefix
	"http://www.w3.org/2001/10/synthesis":   true, // ssml: prefix (TTS pronunciation)
	"http://www.w3.org/2000/svg":            true, // SVG namespace
	"http://www.w3.org/1998/Math/MathML":    true, // MathML namespace
	"http://www.w3.org/1999/xlink":          true, // XLink (used in SVG)
}

// HTM-031: custom attribute namespaces must be valid.
// Attributes using non-standard namespaces (e.g., a misspelled SSML namespace)
// are flagged. Valid SSML (ssml:ph, ssml:alphabet) is permitted for TTS.
func checkCustomAttributeNamespaces(data []byte, location string, r *report.Report) {
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
			ns := attr.Name.Space
			if ns != "" && !allowedAttrNamespaces[ns] {
				r.AddWithLocation(report.Error, "HTM-031",
					fmt.Sprintf("Custom attribute namespace '%s' must not include non-standard namespaces", ns),
					location)
				return
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
