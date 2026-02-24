package validate

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
	"golang.org/x/text/unicode/norm"
)

// checkReferences validates cross-references between manifest and zip contents.
func checkReferences(ep *epub.EPUB, r *report.Report, opts Options) {
	pkg := ep.Package
	if pkg == nil {
		return
	}

	// PKG-025: publication resources must not be in META-INF
	checkNoResourcesInMetaInf(ep, r)

	// RSC-001: every manifest href must exist in the zip
	checkManifestFilesExist(ep, r)

	// RSC-010: manifest hrefs must be valid URLs
	checkManifestHrefValidURL(ep, r)

	// RSC-026: manifest hrefs must not use path traversal or absolute paths
	checkManifestNoPathTraversal(ep, r)

	// OPF-060: no duplicate zip entries
	checkNoDuplicateZipEntries(ep, r)

	// RSC-026: manifest hrefs must not be absolute paths
	checkManifestNoAbsolutePath(ep, r)

	// RSC-002: every file in the container should be in the manifest
	checkFilesInManifest(ep, r)

	// RSC-006: resources referenced in content must be in manifest
	checkResourcesInManifest(ep, r)

	// NAV-001: exactly one manifest item with properties="nav"
	checkNavDeclared(ep, r)

	// OPF-026: exactly one nav item (checks >1)
	checkSingleNavItem(ep, r)

	// NAV-002: nav document must have epub:type="toc"
	checkNavHasToc(ep, r)

	// PKG-026: IDPF-obfuscated resources must be core media type fonts
	checkObfuscatedResources(ep, r)
}

// RSC-001 / RSC-005 / RSC-009: manifest file existence checks
func checkManifestFilesExist(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue // Empty href handled by OPF-030
		}
		// Skip NCX files in EPUB 2 - handled by E2-001
		if ep.Package.Version < "3.0" && item.MediaType == "application/x-dtbncx+xml" {
			continue
		}
		// Skip absolute paths - handled by RSC-026
		if strings.HasPrefix(item.Href, "/") {
			continue
		}
		// Skip path traversal - handled by RSC-026
		if strings.Contains(item.Href, "..") {
			continue
		}
		// RSC-030: file:// URLs are not allowed in the manifest
		if strings.HasPrefix(item.Href, "file:") {
			r.Add(report.Error, "RSC-030",
				fmt.Sprintf("Use of 'file' URL scheme is prohibited: '%s'", item.Href))
			continue
		}
		// RSC-006: remote XHTML content documents in manifest are not allowed
		// (Remote SVG checked at content level; SVG can also be used as fonts)
		if strings.HasPrefix(item.Href, "http://") || strings.HasPrefix(item.Href, "https://") {
			if item.MediaType == "application/xhtml+xml" {
				r.Add(report.Error, "RSC-006",
					fmt.Sprintf("Remote resource reference is not allowed: '%s'", item.Href))
			}
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		if _, exists := ep.Files[fullPath]; !exists {
			r.Add(report.Error, "RSC-001",
				fmt.Sprintf("Referenced resource '%s' could not be found in the container", item.Href))
		}
	}
}

// RSC-005/RSC-006/RSC-009: resources referenced in content documents checks
func checkResourcesInManifest(ep *epub.EPUB, r *report.Report) {
	manifestHrefs := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestHrefs[ep.ResolveHref(item.Href)] = true
		}
	}

	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.MediaType != "application/xhtml+xml" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue
		}

		checkReferencedResourcesInManifest(ep, data, fullPath, manifestHrefs, r)
	}
}

// checkReferencedResourcesInManifest scans an XHTML doc for link[rel=stylesheet] href
// and checks that the referenced CSS/resource is in the manifest.
func checkReferencedResourcesInManifest(ep *epub.EPUB, data []byte, fullPath string, manifestHrefs map[string]bool, r *report.Report) {
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

		// Check <link rel="stylesheet" href="...">
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
			if rel == "stylesheet" && href != "" {
				u, err := url.Parse(href)
				if err != nil || u.Scheme != "" {
					continue // skip remote/invalid
				}
				target := resolvePath(itemDir, u.Path)
				if _, exists := ep.Files[target]; !exists {
					// RSC-005: CSS file doesn't exist
					r.AddWithLocation(report.Error, "RSC-005",
						fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
						fullPath)
				} else if !manifestHrefs[target] {
					// RSC-006: file exists in zip but is not in manifest
					r.AddWithLocation(report.Error, "RSC-006",
						fmt.Sprintf("Referenced resource '%s' is not declared in the OPF manifest", href),
						fullPath)
				}
			}
		}

		// Check font references via @font-face in inline styles or link tags
		// (font file missing is checked via RSC-009 in content checks)
	}
}

// NAV-001
func checkNavDeclared(ep *epub.EPUB, r *report.Report) {
	if ep.Package.Version < "3.0" {
		return
	}
	count := 0
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			count++
		}
	}
	if count == 0 {
		r.Add(report.Error, "NAV-001", "No manifest item found with nav property (exactly one is required)")
	}
}

// OPF-026: Exactly one manifest item must declare the nav property
func checkSingleNavItem(ep *epub.EPUB, r *report.Report) {
	if ep.Package.Version < "3.0" {
		return
	}
	count := 0
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			count++
		}
	}
	if count > 1 {
		r.Add(report.Error, "OPF-026",
			fmt.Sprintf("Exactly one manifest item must declare the nav property, but %d were found", count))
	}
}

// NAV-002
func checkNavHasToc(ep *epub.EPUB, r *report.Report) {
	if ep.Package.Version < "3.0" {
		return
	}

	var navHref string
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			navHref = item.Href
			break
		}
	}
	if navHref == "" {
		return
	}

	fullPath := ep.ResolveHref(navHref)
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return
	}

	if !navDocHasToc(data) {
		r.Add(report.Error, "NAV-002", "Required toc nav element (epub:type='toc') not found in navigation document")
	}
}

// checkNavigation validates the navigation document (Level 2+3 checks).
func checkNavigation(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil || ep.Package.Version < "3.0" {
		return
	}

	var navHref string
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			navHref = item.Href
			break
		}
	}
	if navHref == "" {
		return
	}

	fullPath := ep.ResolveHref(navHref)
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return
	}

	// NAV-011: nav document must be well-formed XHTML
	if !checkNavWellFormed(data, fullPath, r) {
		return // Can't check further
	}

	navInfo := parseNavDocument(ep, data, fullPath)

	// NAV-008: toc nav must have ol element
	if navInfo.tocCount > 0 && !navInfo.tocHasOl {
		r.Add(report.Error, "NAV-008",
			"Navigation document toc nav is missing required element 'ol'")
	}

	// NAV-003: TOC links must resolve
	for _, link := range navInfo.tocLinks {
		if link.href != "" {
			checkNavLinkResolves(ep, link.href, fullPath, "NAV-003", r)
		}
	}

	// NAV-004: nav anchors must contain text
	for _, link := range navInfo.tocLinks {
		if link.text == "" {
			r.Add(report.Error, "NAV-004",
				fmt.Sprintf("Anchors within nav elements must contain text content"))
		}
	}

	// NAV-005: exactly one toc nav
	if navInfo.tocCount > 1 {
		r.Add(report.Error, "NAV-005",
			fmt.Sprintf("Exactly one nav element with epub:type='toc' is required, but %d were found", navInfo.tocCount))
	}

	// NAV-006: landmarks links must resolve
	for _, link := range navInfo.landmarkLinks {
		if link.href != "" {
			checkNavLinkResolves(ep, link.href, fullPath, "NAV-006", r)
		}
	}

	// NAV-007: page-list links must resolve
	for _, link := range navInfo.pageListLinks {
		if link.href != "" {
			checkNavLinkResolves(ep, link.href, fullPath, "NAV-007", r)
		}
	}

	// NAV-009: hidden attribute on nav elements
	if navInfo.hasHiddenNav {
		r.AddWithLocation(report.Warning, "NAV-009",
			"The 'hidden' attribute on nav elements may affect reading system behavior",
			fullPath)
	}

	// NAV-010: external links are not allowed in toc, page-list, or landmarks nav elements
	for _, link := range navInfo.tocLinks {
		if isRemoteURL(link.href) {
			r.AddWithLocation(report.Error, "NAV-010",
				fmt.Sprintf("The 'toc' nav element must not contain links to remote resources: '%s'", link.href),
				fullPath)
		}
	}
	for _, link := range navInfo.landmarkLinks {
		if isRemoteURL(link.href) {
			r.AddWithLocation(report.Error, "NAV-010",
				fmt.Sprintf("The 'landmarks' nav element must not contain links to remote resources: '%s'", link.href),
				fullPath)
		}
	}
	for _, link := range navInfo.pageListLinks {
		if isRemoteURL(link.href) {
			r.AddWithLocation(report.Error, "NAV-010",
				fmt.Sprintf("The 'page-list' nav element must not contain links to remote resources: '%s'", link.href),
				fullPath)
		}
	}

	// RSC-011: toc nav links must point to documents in the spine
	// RSC-010: toc nav links must point to content documents (XHTML or SVG)
	checkNavTocSpineLinks(ep, navInfo, fullPath, r)

	// NAV-011: toc nav links must follow spine reading order
	checkNavTOCOrder(ep, navInfo, fullPath, r)
}

type navLink struct {
	href string
	text string
}

type navDocInfo struct {
	tocLinks      []navLink
	landmarkLinks []navLink
	pageListLinks []navLink
	landmarkTypes []string
	tocCount      int
	tocHasOl      bool
	hasHiddenNav  bool
	hasLandmarks  bool
}

// NAV-011: nav document must be well-formed XHTML
func checkNavWellFormed(data []byte, location string, r *report.Report) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			r.AddWithLocation(report.Fatal, "NAV-011",
				"Navigation document is not well-formed: element must be terminated by the matching end-tag",
				location)
			return false
		}
	}
	return true
}

func parseNavDocument(ep *epub.EPUB, data []byte, navPath string) navDocInfo {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	info := navDocInfo{}

	// Track nav element nesting
	var currentNavType string
	inNav := false
	inAnchor := false
	var currentHref string
	var currentText string

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "nav" {
				for _, attr := range t.Attr {
					if attr.Name.Local == "type" {
						if containsToken(attr.Value, "toc") {
							currentNavType = "toc"
							info.tocCount++
						} else if containsToken(attr.Value, "landmarks") {
							currentNavType = "landmarks"
							info.hasLandmarks = true
						} else if containsToken(attr.Value, "page-list") {
							currentNavType = "page-list"
						} else {
							currentNavType = ""
						}
						inNav = true
					}
					if attr.Name.Local == "hidden" {
						info.hasHiddenNav = true
					}
				}
			}
			if inNav && currentNavType == "toc" && t.Name.Local == "ol" {
				info.tocHasOl = true
			}
			if inNav && t.Name.Local == "a" {
				inAnchor = true
				currentHref = ""
				currentText = ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "href" {
						currentHref = attr.Value
					}
					// Capture epub:type on landmark anchors
					if currentNavType == "landmarks" && attr.Name.Local == "type" {
						for _, val := range strings.Fields(attr.Value) {
							info.landmarkTypes = append(info.landmarkTypes, val)
						}
					}
				}
			}
		case xml.CharData:
			if inAnchor {
				currentText += string(t)
			}
		case xml.EndElement:
			if t.Name.Local == "a" && inAnchor {
				link := navLink{
					href: currentHref,
					text: strings.TrimSpace(currentText),
				}
				switch currentNavType {
				case "toc":
					info.tocLinks = append(info.tocLinks, link)
				case "landmarks":
					info.landmarkLinks = append(info.landmarkLinks, link)
				case "page-list":
					info.pageListLinks = append(info.pageListLinks, link)
				}
				inAnchor = false
			}
			if t.Name.Local == "nav" {
				inNav = false
				currentNavType = ""
			}
		}
	}

	return info
}

func checkNavLinkResolves(ep *epub.EPUB, href, navFullPath, checkID string, r *report.Report) {
	u, err := url.Parse(href)
	if err != nil || u.Scheme != "" {
		return
	}

	refPath := u.Path
	if refPath == "" {
		// Fragment-only: check the fragment against the nav doc itself
		if u.Fragment != "" {
			navData, err := ep.ReadFile(navFullPath)
			if err == nil {
				ids := collectNavIDs(navData)
				if !ids[u.Fragment] {
					r.AddWithLocation(report.Error, "RSC-012",
						fmt.Sprintf("Fragment identifier is not defined: '#%s'", u.Fragment),
						navFullPath)
				}
			}
		}
		return
	}

	navDir := path.Dir(navFullPath)
	target := resolvePath(navDir, refPath)
	// Also try NFC-normalized path for diacritic filenames
	nfcTarget := norm.NFC.String(target)
	_, exists := ep.Files[target]
	if !exists && nfcTarget != target {
		_, exists = ep.Files[nfcTarget]
	}
	if !exists {
		r.AddWithLocation(report.Error, "RSC-007",
			fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
			navFullPath)
		return
	}

	// Check fragment identifier if present
	if u.Fragment != "" {
		// EPUB CFI fragments are always valid (they encode location within documents)
		if strings.HasPrefix(u.Fragment, "epubcfi(") {
			return
		}
		targetData, err := ep.ReadFile(target)
		if err != nil && nfcTarget != target {
			targetData, err = ep.ReadFile(nfcTarget)
		}
		if err == nil {
			ids := collectNavIDs(targetData)
			if !ids[u.Fragment] {
				r.AddWithLocation(report.Error, "RSC-012",
					fmt.Sprintf("Fragment identifier is not defined: '%s#%s'", refPath, u.Fragment),
					navFullPath)
			}
		}
	}
}

// collectNavIDs collects all id attributes from an HTML/XHTML document.
func collectNavIDs(data []byte) map[string]bool {
	return collectIDs(data)
}

// checkNavTocSpineLinks checks RSC-011 (toc link to non-spine doc) and
// RSC-010 (toc link to non-content-document type).
func checkNavTocSpineLinks(ep *epub.EPUB, navInfo navDocInfo, navPath string, r *report.Report) {
	navDir := path.Dir(navPath)

	// Build spine set: manifest full path → true
	spineSet := buildSpinePathSet(ep)

	// Build manifest path → media-type map
	manifestMT := make(map[string]string)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestMT[ep.ResolveHref(item.Href)] = item.MediaType
		}
	}

	for _, link := range navInfo.tocLinks {
		if link.href == "" || isRemoteURL(link.href) {
			continue
		}
		u, err := url.Parse(link.href)
		if err != nil || u.Scheme != "" || u.Path == "" {
			continue
		}
		target := resolvePath(navDir, u.Path)

		mt, inManifest := manifestMT[target]
		if !inManifest {
			continue // file not in manifest, already reported by RSC-007
		}

		// RSC-010: toc nav links must point to XHTML or SVG content documents
		isContentDoc := mt == "application/xhtml+xml" || mt == "image/svg+xml"
		if !isContentDoc {
			r.AddWithLocation(report.Error, "RSC-010",
				fmt.Sprintf("The 'toc' nav element links to a resource that is not a Content Document: '%s'", link.href),
				navPath)
			continue
		}

		// RSC-011: toc nav links to content docs must be in the spine
		if !spineSet[target] {
			r.AddWithLocation(report.Error, "RSC-011",
				fmt.Sprintf("Content document '%s' is referenced from the 'toc' nav but is not listed in the spine", link.href),
				navPath)
		}
	}
}

// buildSpinePathSet returns a set of full file paths for all spine items.
func buildSpinePathSet(ep *epub.EPUB) map[string]bool {
	// manifest ID (trimmed) → full path
	idToPath := make(map[string]string)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			idToPath[strings.TrimSpace(item.ID)] = ep.ResolveHref(item.Href)
		}
	}
	spineSet := make(map[string]bool)
	for _, ref := range ep.Package.Spine {
		if p := idToPath[strings.TrimSpace(ref.IDRef)]; p != "" {
			spineSet[p] = true
		}
	}
	return spineSet
}

// buildSpinePositions returns a map of full file path → spine index (0-based).
func buildSpinePositions(ep *epub.EPUB) map[string]int {
	idToPath := make(map[string]string)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			idToPath[strings.TrimSpace(item.ID)] = ep.ResolveHref(item.Href)
		}
	}
	positions := make(map[string]int)
	for i, ref := range ep.Package.Spine {
		if p := idToPath[strings.TrimSpace(ref.IDRef)]; p != "" {
			positions[p] = i
		}
	}
	return positions
}

// checkNavTOCOrder checks NAV-011: toc links must follow spine reading order
// and fragment document order.
func checkNavTOCOrder(ep *epub.EPUB, navInfo navDocInfo, navPath string, r *report.Report) {
	navDir := path.Dir(navPath)
	spinePositions := buildSpinePositions(ep)

	lastSpinePos := -1
	lastFragPos := make(map[string]int) // fullPath → last seen fragment byte offset

	for _, link := range navInfo.tocLinks {
		if link.href == "" || isRemoteURL(link.href) {
			continue
		}
		u, err := url.Parse(link.href)
		if err != nil || u.Scheme != "" || u.Path == "" {
			continue
		}
		target := resolvePath(navDir, u.Path)

		spinePos, inSpine := spinePositions[target]
		if !inSpine {
			continue // not in spine, already reported
		}

		fragOffset := 0 // no fragment = beginning of doc = offset 0
		if u.Fragment != "" {
			data, err := ep.ReadFile(target)
			if err == nil {
				elemPos := collectElementPositions(data)
				if pos, ok := elemPos[u.Fragment]; ok {
					fragOffset = pos
				} else {
					fragOffset = -1 // unknown fragment, skip fragment check
				}
			}
		}

		if spinePos < lastSpinePos {
			// Out of spine order
			r.AddWithLocation(report.Warning, "NAV-011",
				fmt.Sprintf("The 'toc' nav link to '%s' is not in spine reading order", link.href),
				navPath)
		} else if spinePos == lastSpinePos && fragOffset >= 0 {
			// Same document: check fragment order
			if fragOffset < lastFragPos[target] {
				r.AddWithLocation(report.Warning, "NAV-011",
					fmt.Sprintf("The 'toc' nav link to '%s' is not in document reading order", link.href),
					navPath)
			}
		}

		// Update tracking (always update to current position)
		lastSpinePos = spinePos
		if fragOffset >= 0 {
			lastFragPos[target] = fragOffset
		}
	}
}

// collectElementPositions returns a map of element id → byte offset in the document.
func collectElementPositions(data []byte) map[string]int {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	positions := make(map[string]int)
	for {
		offset := decoder.InputOffset()
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "id" && attr.Value != "" {
				positions[attr.Value] = int(offset)
				break
			}
		}
	}
	return positions
}

// RSC-010: manifest hrefs must be valid URLs
func checkManifestHrefValidURL(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		_, err := url.Parse(item.Href)
		if err != nil {
			r.Add(report.Error, "RSC-010",
				fmt.Sprintf("Manifest item href '%s' is not a valid URL", item.Href))
			continue
		}
		// Check for bad percent encoding
		if strings.Contains(item.Href, "%") {
			// Verify all percent-encoded sequences are valid
			for i := 0; i < len(item.Href); i++ {
				if item.Href[i] == '%' {
					if i+2 >= len(item.Href) || !isHexDigit(item.Href[i+1]) || !isHexDigit(item.Href[i+2]) {
						r.Add(report.Error, "RSC-010",
							fmt.Sprintf("Manifest item href '%s' is not a valid URL: bad percent-encoding", item.Href))
						break
					}
				}
			}
		}
	}
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// RSC-026: manifest hrefs must not use path traversal
func checkManifestNoPathTraversal(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if strings.Contains(item.Href, "..") {
			// Items that resolve to META-INF are reported as PKG-025, not RSC-026
			rawPath := ep.ResolveHref(item.Href)
			cleanPath := path.Clean(rawPath)
			if strings.HasPrefix(cleanPath, "META-INF/") {
				continue
			}
			r.Add(report.Error, "RSC-026",
				fmt.Sprintf("Referenced resource '%s' cannot be accessed (path traversal not allowed)", item.Href))
		}
	}
}

// OPF-060: no duplicate zip entries (exact same path appearing more than once)
func checkNoDuplicateZipEntries(ep *epub.EPUB, r *report.Report) {
	seen := make(map[string]bool)
	for _, f := range ep.ZipFile.File {
		if seen[f.Name] {
			r.Add(report.Error, "OPF-060",
				fmt.Sprintf("Duplicate entry in the ZIP file: '%s'", f.Name))
		}
		seen[f.Name] = true
	}
}

// RSC-026: manifest hrefs must not be absolute paths
func checkManifestNoAbsolutePath(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if strings.HasPrefix(item.Href, "/") {
			r.Add(report.Error, "RSC-026",
				fmt.Sprintf("Referenced resource '%s' cannot be accessed (absolute paths not allowed)", item.Href))
		}
	}
}

// RSC-002w: every content file in the container should be listed in the manifest
// (Using RSC-002w to distinguish from the fatal RSC-002 for missing container.xml)
func checkFilesInManifest(ep *epub.EPUB, r *report.Report) {
	manifestPaths := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestPaths[ep.ResolveHref(item.Href)] = true
		}
	}

	// Files that are expected to be outside the manifest
	ignorePaths := map[string]bool{
		"mimetype":                  true,
		"META-INF/container.xml":    true,
		"META-INF/encryption.xml":   true,
		"META-INF/manifest.xml":     true,
		"META-INF/metadata.xml":     true,
		"META-INF/rights.xml":       true,
		"META-INF/signatures.xml":   true,
	}

	// For multiple renditions: collect OPF paths and manifest hrefs from
	// other rootfiles so we don't flag their files as undeclared.
	otherRenditionPaths := collectOtherRenditionPaths(ep)

	for name := range ep.Files {
		// Skip directory entries in the ZIP archive
		if strings.HasSuffix(name, "/") {
			continue
		}
		if ignorePaths[name] {
			continue
		}
		if strings.HasPrefix(name, "META-INF/") {
			continue
		}
		// Skip the OPF file itself
		if name == ep.RootfilePath {
			continue
		}
		// Skip files belonging to other renditions
		if otherRenditionPaths[name] {
			continue
		}
		if !manifestPaths[name] {
			r.Add(report.Warning, "RSC-002w",
				fmt.Sprintf("File '%s' in container is not declared in the OPF manifest", name))
		}
	}
}

// collectOtherRenditionPaths parses additional rootfile OPFs (for multiple
// rendition EPUBs) and returns the set of all paths they reference.
// Also includes container-level links (e.g., mapping documents).
func collectOtherRenditionPaths(ep *epub.EPUB) map[string]bool {
	paths := make(map[string]bool)

	// Container-level links (mapping documents, etc.)
	for _, href := range ep.ContainerLinks {
		paths[href] = true
	}

	if len(ep.AllRootfiles) <= 1 {
		return paths
	}

	for _, rf := range ep.AllRootfiles {
		if rf.FullPath == ep.RootfilePath {
			continue
		}
		// The OPF file itself belongs to the other rendition
		paths[rf.FullPath] = true

		// Try to parse the other OPF to get its manifest entries
		data, err := ep.ReadFile(rf.FullPath)
		if err != nil {
			continue
		}
		otherDir := path.Dir(rf.FullPath)
		for _, href := range extractManifestHrefs(data) {
			decoded, err := url.PathUnescape(href)
			if err != nil {
				decoded = href
			}
			if otherDir == "." {
				paths[decoded] = true
			} else {
				paths[otherDir+"/"+decoded] = true
			}
		}
	}
	return paths
}

// extractManifestHrefs does a quick XML scan of an OPF to extract manifest item hrefs.
func extractManifestHrefs(data []byte) []string {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var hrefs []string
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "item" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "href" && attr.Value != "" {
					hrefs = append(hrefs, attr.Value)
				}
			}
		}
	}
	return hrefs
}

func hasProperty(properties, prop string) bool {
	for _, p := range strings.Fields(properties) {
		if p == prop {
			return true
		}
	}
	return false
}

// PKG-026: IDPF-obfuscated resources (Algorithm="http://www.idpf.org/2008/embedding")
// must be core media type fonts. Reports an error for each non-CMT or non-font resource.
func checkObfuscatedResources(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["META-INF/encryption.xml"]
	if !exists {
		return
	}
	data, err := ep.ReadFile("META-INF/encryption.xml")
	if err != nil {
		return
	}

	// Build manifest lookup by href and full path
	manifestByPath := make(map[string]epub.ManifestItem)
	for _, item := range ep.Package.Manifest {
		manifestByPath[item.Href] = item
		manifestByPath[ep.ResolveHref(item.Href)] = item
	}

	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var isIDPF bool
	var inEncData bool
	var currentURI string

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "EncryptedData":
				inEncData = true
				isIDPF = false
				currentURI = ""
			case "EncryptionMethod":
				if inEncData {
					for _, attr := range t.Attr {
						if attr.Name.Local == "Algorithm" &&
							attr.Value == "http://www.idpf.org/2008/embedding" {
							isIDPF = true
						}
					}
				}
			case "CipherReference":
				for _, attr := range t.Attr {
					if attr.Name.Local == "URI" {
						currentURI = attr.Value
					}
				}
			}
		case xml.EndElement:
			if t.Name.Local == "EncryptedData" {
				if isIDPF && currentURI != "" {
					item, found := manifestByPath[currentURI]
					if !found {
						item, found = manifestByPath[ep.ResolveHref(currentURI)]
					}
					if found {
						mt := item.MediaType
						if !coreMediaTypes[mt] || !isFontMediaType(mt) {
							r.Add(report.Error, "PKG-026",
								fmt.Sprintf("Obfuscated resource '%s' with media type '%s' is not an EPUB Core Media Type font", currentURI, mt))
						}
					}
				}
				inEncData = false
				isIDPF = false
				currentURI = ""
			}
		}
	}
}
