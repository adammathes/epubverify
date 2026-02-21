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

// checkEPUB2 runs EPUB 2 specific checks.
func checkEPUB2(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil || ep.Package.Version >= "3.0" {
		return
	}

	// E2-004: spine must have toc attribute
	checkEPUB2SpineToc(ep, r)

	// E2-005: EPUB 2 must not have nav property
	checkEPUB2NoNavProperty(ep, r)

	// E2-006: EPUB 2 must not have dcterms:modified
	checkEPUB2NoDCTermsModified(ep, r)

	// E2-009: guide references must resolve
	checkEPUB2GuideRefs(ep, r)

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

	// E2-007: navPoint must have content element
	checkNCXNavPointContent(data, r)

	// E2-008: navPoint content src must resolve
	checkNCXContentSrcResolves(ep, data, ncxFullPath, r)

	// E2-010: NCX uid must match OPF uid
	checkNCXUIDMatchesOPF(ep, data, r)

	// E2-011: NCX IDs must be unique
	checkNCXUniqueIDs(data, r)
}

// E2-004: EPUB 2 spine must have toc attribute
func checkEPUB2SpineToc(ep *epub.EPUB, r *report.Report) {
	if ep.Package.SpineToc == "" {
		r.Add(report.Error, "E2-004",
			"EPUB 2 spine element is missing required attribute 'toc'")
	}
}

// E2-005: EPUB 2 must not have properties attribute on manifest items
func checkEPUB2NoNavProperty(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Properties != "" {
			r.Add(report.Error, "E2-005",
				fmt.Sprintf("Manifest item properties attribute '%s' is not allowed in EPUB 2", item.Properties))
		}
	}
}

// E2-006: EPUB 2 must not have dcterms:modified meta with property attribute
func checkEPUB2NoDCTermsModified(ep *epub.EPUB, r *report.Report) {
	if ep.Package.ModifiedCount > 0 {
		r.Add(report.Error, "E2-006",
			"The 'property' attribute on meta element is not allowed in EPUB 2")
	}
}

// E2-009: guide references must resolve
func checkEPUB2GuideRefs(ep *epub.EPUB, r *report.Report) {
	manifestHrefs := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestHrefs[ep.ResolveHref(item.Href)] = true
		}
	}

	for _, ref := range ep.Package.Guide {
		if ref.Href == "" {
			continue
		}
		u, err := url.Parse(ref.Href)
		if err != nil {
			continue
		}
		target := ep.ResolveHref(u.Path)
		if !manifestHrefs[target] {
			r.Add(report.Error, "E2-009",
				fmt.Sprintf("Guide reference '%s' is not declared in OPF manifest", ref.Href))
		}
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
			"NCX document is missing required element 'navMap'")
	}
}

// E2-007: navPoint elements must have a content child element
func checkNCXNavPointContent(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	inNavPoint := false
	hasContent := false
	depth := 0

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "navPoint" {
				if inNavPoint && !hasContent {
					r.Add(report.Error, "E2-007",
						"NCX navPoint element is incomplete: missing required child element 'content'")
				}
				inNavPoint = true
				hasContent = false
				depth++
			}
			if inNavPoint && t.Name.Local == "content" {
				hasContent = true
			}
		case xml.EndElement:
			if t.Name.Local == "navPoint" {
				if !hasContent {
					r.Add(report.Error, "E2-007",
						"NCX navPoint element is incomplete: missing required child element 'content'")
				}
				depth--
				if depth <= 0 {
					inNavPoint = false
				}
				hasContent = false
			}
		}
	}
}

// E2-008: navPoint content src must point to an existing resource
func checkNCXContentSrcResolves(ep *epub.EPUB, data []byte, ncxFullPath string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	ncxDir := path.Dir(ncxFullPath)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "content" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && attr.Value != "" {
					u, err := url.Parse(attr.Value)
					if err != nil || u.Scheme != "" {
						continue
					}
					target := resolvePath(ncxDir, u.Path)
					if _, exists := ep.Files[target]; !exists {
						r.Add(report.Error, "E2-008",
							fmt.Sprintf("Referenced resource '%s' could not be found in the container", attr.Value))
					}
				}
			}
		}
	}
}

// E2-010: NCX dtb:uid must match the OPF dc:identifier
func checkNCXUIDMatchesOPF(ep *epub.EPUB, data []byte, r *report.Report) {
	// Find OPF unique-identifier value
	opfUID := ""
	if ep.Package.UniqueIdentifier != "" {
		for _, id := range ep.Package.Metadata.Identifiers {
			if id.ID == ep.Package.UniqueIdentifier {
				opfUID = id.Value
				break
			}
		}
	}
	if opfUID == "" {
		return
	}

	// Find NCX dtb:uid
	ncxUID := ""
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
			if name == "dtb:uid" {
				ncxUID = content
				break
			}
		}
	}

	if ncxUID != "" && ncxUID != opfUID {
		r.Add(report.Error, "E2-010",
			fmt.Sprintf("NCX identifier '%s' does not match OPF identifier '%s'", ncxUID, opfUID))
	}
}

// E2-011: NCX id attributes must be unique
func checkNCXUniqueIDs(data []byte, r *report.Report) {
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
					r.Add(report.Error, "E2-011",
						fmt.Sprintf("The id attribute '%s' does not have a unique value", attr.Value))
				}
				seen[attr.Value] = true
			}
		}
	}
}
