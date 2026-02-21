package validate

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// checkEPUB2 runs EPUB 2 specific checks.
func checkEPUB2(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil || ep.Package.Version >= "3.0" {
		return
	}

	// E2-004: spine must have toc attribute
	checkEPUB2SpineToc(ep, r)

	// E2-001: NCX must be present
	ncxPath := findNCXPath(ep)
	if ncxPath == "" {
		return // Can't check further
	}

	ncxFullPath := ep.ResolveHref(ncxPath)
	if _, exists := ep.Files[ncxFullPath]; !exists {
		r.Add(report.Error, "E2-001",
			fmt.Sprintf("Referenced resource '%s' could not be found in the container", ncxPath))
		return
	}

	data, err := ep.ReadFile(ncxFullPath)
	if err != nil {
		return
	}

	// E2-002: NCX must be well-formed XML
	if !checkNCXWellFormed(data, r) {
		return
	}

	// E2-003: NCX must have navMap
	checkNCXHasNavMap(data, r)
}

// E2-004: EPUB 2 spine must have toc attribute
func checkEPUB2SpineToc(ep *epub.EPUB, r *report.Report) {
	if ep.Package.SpineToc == "" {
		r.Add(report.Error, "E2-004",
			"EPUB 2 spine element is missing required attribute 'toc'")
	}
}

// findNCXPath finds the NCX file path from the manifest.
func findNCXPath(ep *epub.EPUB) string {
	// First try the spine toc attribute
	if ep.Package.SpineToc != "" {
		for _, item := range ep.Package.Manifest {
			if item.ID == ep.Package.SpineToc {
				return item.Href
			}
		}
	}

	// Fallback: find by media-type
	for _, item := range ep.Package.Manifest {
		if item.MediaType == "application/x-dtbncx+xml" {
			return item.Href
		}
	}

	return ""
}

// E2-002: NCX must be well-formed XML
func checkNCXWellFormed(data []byte, r *report.Report) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			r.Add(report.Fatal, "E2-002",
				"NCX document is not well-formed: XML document structures must start and end within the same entity")
			return false
		}
	}
	return true
}

// E2-003: NCX must contain navMap
func checkNCXHasNavMap(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	hasNavMap := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "navMap" {
				hasNavMap = true
				break
			}
		}
	}

	if !hasNavMap {
		r.Add(report.Error, "E2-003",
			fmt.Sprintf("NCX document is missing required element 'navMap'"))
	}
}
