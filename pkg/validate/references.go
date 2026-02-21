package validate

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// checkReferences validates cross-references between manifest and zip contents.
func checkReferences(ep *epub.EPUB, r *report.Report, opts Options) {
	pkg := ep.Package
	if pkg == nil {
		return
	}

	// RSC-001: every manifest href must exist in the zip
	checkManifestFilesExist(ep, r)

	// RSC-006: resources referenced in content must be in manifest
	checkResourcesInManifest(ep, r)

	// NAV-001: exactly one manifest item with properties="nav"
	checkNavDeclared(ep, r)

	// OPF-026: exactly one nav item (checks >1)
	checkSingleNavItem(ep, r)

	// NAV-002: nav document must have epub:type="toc"
	checkNavHasToc(ep, r)
}

// RSC-001
func checkManifestFilesExist(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		if _, exists := ep.Files[fullPath]; !exists {
			r.Add(report.Error, "RSC-001",
				fmt.Sprintf("Referenced resource '%s' could not be found in the container", item.Href))
		}
	}
}

// RSC-006: resources referenced in content documents must be declared in the OPF manifest
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
				if !manifestHrefs[target] {
					// The file exists in the zip but is not in the manifest
					if _, exists := ep.Files[target]; exists {
						r.AddWithLocation(report.Error, "RSC-008",
							fmt.Sprintf("Referenced resource '%s' is not declared in the OPF manifest", href),
							fullPath)
					}
				}
			}
		}
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

// checkNavigation validates the navigation document (Level 2 checks).
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

	navInfo := parseNavDocument(ep, data, fullPath)

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
}

type navLink struct {
	href string
	text string
}

type navDocInfo struct {
	tocLinks      []navLink
	landmarkLinks []navLink
	pageListLinks []navLink
	tocCount      int
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
						} else if containsToken(attr.Value, "page-list") {
							currentNavType = "page-list"
						} else {
							currentNavType = ""
						}
						inNav = true
					}
				}
			}
			if inNav && t.Name.Local == "a" {
				inAnchor = true
				currentHref = ""
				currentText = ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "href" {
						currentHref = attr.Value
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

func hasProperty(properties, prop string) bool {
	for _, p := range strings.Fields(properties) {
		if p == prop {
			return true
		}
	}
	return false
}
