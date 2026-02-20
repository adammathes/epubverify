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

// checkContent validates XHTML content documents.
func checkContent(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil {
		return
	}

	// Build set of manifest-declared resources (resolved full paths).
	// Resources already in the manifest are covered by RSC-001 if missing,
	// so we skip RSC-007 for those to avoid duplicate reporting.
	manifestPaths := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestPaths[ep.ResolveHref(item.Href)] = true
		}
	}

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
				fmt.Sprintf("Content document is not well-formed XML: element not terminated by the matching end-tag"),
				location)
			return false
		}
	}
	return true
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

	// Skip external URLs
	u, err := url.Parse(href)
	if err != nil {
		return
	}
	if u.Scheme != "" {
		return
	}

	// Strip fragment
	refPath := u.Path
	if refPath == "" {
		return // fragment-only reference
	}

	// Resolve relative path
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
	// Skip if already declared in manifest — RSC-001 handles missing manifest resources
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
