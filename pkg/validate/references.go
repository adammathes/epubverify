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
)

// checkReferences validates cross-references between manifest and zip contents.
func checkReferences(ep *epub.EPUB, r *report.Report, opts Options) {
	pkg := ep.Package
	if pkg == nil {
		return
	}

	// RSC-001: every manifest href must exist in the zip
	checkManifestFilesExist(ep, r)

	// RSC-010: manifest hrefs must be valid URLs
	checkManifestHrefValidURL(ep, r)

	// RSC-011: manifest hrefs must not use path traversal
	checkManifestNoPathTraversal(ep, r)

	// RSC-012: no duplicate zip entries
	checkNoDuplicateZipEntries(ep, r)

	// RSC-013: manifest hrefs must not be absolute paths
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
		// Skip absolute paths - handled by RSC-013
		if strings.HasPrefix(item.Href, "/") {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		if _, exists := ep.Files[fullPath]; !exists {
			checkID := "RSC-001"
			if item.MediaType == "text/css" {
				checkID = "RSC-005"
			} else if isFontMediaType(item.MediaType) {
				checkID = "RSC-009"
			}
			r.Add(report.Error, checkID,
				fmt.Sprintf("Referenced resource '%s' could not be found in the container", item.Href))
		}
	}
}

func isFontMediaType(mt string) bool {
	return strings.HasPrefix(mt, "font/") ||
		mt == "application/font-woff" ||
		mt == "application/font-sfnt" ||
		mt == "application/vnd.ms-opentype"
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

	// NAV-010: landmark entries should use valid epub:type values
	// The EPUB 3 structural semantics vocabulary is extensible, so unknown
	// values are informational rather than violations.
	for _, t := range navInfo.landmarkTypes {
		if !validEpubTypes[t] && !strings.Contains(t, ":") {
			r.AddWithLocation(report.Info, "NAV-010",
				fmt.Sprintf("Landmark nav entry uses unknown epub:type value '%s'", t),
				fullPath)
		}
	}
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
		return // fragment-only
	}

	navDir := path.Dir(navFullPath)
	target := resolvePath(navDir, refPath)
	if _, exists := ep.Files[target]; !exists {
		r.AddWithLocation(report.Error, checkID,
			fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
			navFullPath)
	}
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

// RSC-011: manifest hrefs must not use path traversal
func checkManifestNoPathTraversal(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if strings.Contains(item.Href, "..") {
			r.Add(report.Error, "RSC-011",
				fmt.Sprintf("Referenced resource '%s' could not be found in the container: path traversal not allowed", item.Href))
		}
	}
}

// RSC-012: no duplicate zip entries
func checkNoDuplicateZipEntries(ep *epub.EPUB, r *report.Report) {
	// Check for files that map to the same case-insensitive path
	seen := make(map[string]string) // lowercase -> original
	for _, f := range ep.ZipFile.File {
		lower := strings.ToLower(f.Name)
		if existing, ok := seen[lower]; ok {
			if existing != f.Name {
				r.Add(report.Error, "RSC-012",
					fmt.Sprintf("Duplicate entry in the ZIP file: '%s' and '%s'", existing, f.Name))
			}
		}
		seen[lower] = f.Name
	}
}

// RSC-013: manifest hrefs must not be absolute paths
func checkManifestNoAbsolutePath(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		if strings.HasPrefix(item.Href, "/") {
			r.Add(report.Error, "RSC-013",
				fmt.Sprintf("Referenced resource '%s' leaks outside the container: absolute paths not allowed", item.Href))
		}
	}
}

// RSC-002: every content file in the container should be listed in the manifest
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
		if !manifestPaths[name] {
			r.Add(report.Warning, "RSC-002",
				fmt.Sprintf("File '%s' in container is not declared in the OPF manifest", name))
		}
	}
}

func hasProperty(properties, prop string) bool {
	for _, p := range strings.Fields(properties) {
		if p == prop {
			return true
		}
	}
	return false
}
