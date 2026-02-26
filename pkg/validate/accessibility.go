package validate

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// checkAccessibility runs accessibility checks (ACC-001 through ACC-010).
func checkAccessibility(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil || ep.Package.Version < "3.0" {
		return
	}

	// Collect all meta properties for accessibility metadata checks
	metaProps := make(map[string]bool)
	for _, mr := range ep.Package.MetaRefines {
		metaProps[mr.Property] = true
	}
	// Also check non-refines metas from the raw OPF scan
	// The accessibility metadata uses schema.org properties
	a11yMeta := collectAccessibilityMeta(ep)

	// ACC-001: accessibility metadata should be present
	if !a11yMeta.hasAny {
		r.Add(report.Usage, "ACC-001",
			"EPUB publication should include accessibility metadata (schema.org properties)")
	}

	// ACC-005: schema:accessMode
	if !a11yMeta.hasAccessMode {
		r.Add(report.Usage, "ACC-005",
			"EPUB should declare schema:accessMode metadata")
	}

	// ACC-006: schema:accessModeSufficient
	if !a11yMeta.hasAccessModeSufficient {
		r.Add(report.Usage, "ACC-006",
			"EPUB should declare schema:accessModeSufficient metadata")
	}

	// ACC-007: schema:accessibilitySummary
	if !a11yMeta.hasAccessibilitySummary {
		r.Add(report.Usage, "ACC-007",
			"EPUB should declare schema:accessibilitySummary metadata")
	}

	// ACC-008: schema:accessibilityFeature
	if !a11yMeta.hasAccessibilityFeature {
		r.Add(report.Usage, "ACC-008",
			"EPUB should declare schema:accessibilityFeature metadata")
	}

	// ACC-009: schema:accessibilityHazard
	if !a11yMeta.hasAccessibilityHazard {
		r.Add(report.Usage, "ACC-009",
			"EPUB should declare schema:accessibilityHazard metadata")
	}

	// ACC-002: img elements should have alt text
	checkImgAltText(ep, r)

	// ACC-003: html element should declare language
	checkHTMLLangPresent(ep, r)

	// ACC-004: dc:source present means page-list should exist
	checkPageSourceHasPageList(ep, r)

	// ACC-010: landmarks navigation should be present
	checkLandmarksNavPresent(ep, r)
}

type accessibilityMeta struct {
	hasAny                    bool
	hasAccessMode             bool
	hasAccessModeSufficient   bool
	hasAccessibilitySummary   bool
	hasAccessibilityFeature   bool
	hasAccessibilityHazard    bool
}

func collectAccessibilityMeta(ep *epub.EPUB) accessibilityMeta {
	meta := accessibilityMeta{}

	// Read the raw OPF data and search for schema.org accessibility properties
	if ep.RootfilePath == "" {
		return meta
	}
	data, err := ep.ReadFile(ep.RootfilePath)
	if err != nil {
		return meta
	}

	content := string(data)
	// Look for meta elements with schema.org properties
	if strings.Contains(content, "schema:accessMode") || strings.Contains(content, "accessMode") {
		meta.hasAccessMode = true
		meta.hasAny = true
	}
	if strings.Contains(content, "schema:accessModeSufficient") || strings.Contains(content, "accessModeSufficient") {
		meta.hasAccessModeSufficient = true
		meta.hasAny = true
	}
	if strings.Contains(content, "schema:accessibilitySummary") || strings.Contains(content, "accessibilitySummary") {
		meta.hasAccessibilitySummary = true
		meta.hasAny = true
	}
	if strings.Contains(content, "schema:accessibilityFeature") || strings.Contains(content, "accessibilityFeature") {
		meta.hasAccessibilityFeature = true
		meta.hasAny = true
	}
	if strings.Contains(content, "schema:accessibilityHazard") || strings.Contains(content, "accessibilityHazard") {
		meta.hasAccessibilityHazard = true
		meta.hasAny = true
	}
	return meta
}

// ACC-002: img elements should have an alt attribute
func checkImgAltText(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.MediaType != "application/xhtml+xml" || item.Href == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue
		}
		decoder := newXHTMLDecoder(strings.NewReader(string(data)))
		for {
			tok, err := decoder.Token()
			if err != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok || se.Name.Local != "img" {
				continue
			}
			hasAlt := false
			for _, attr := range se.Attr {
				if attr.Name.Local == "alt" {
					hasAlt = true
					break
				}
			}
			if !hasAlt {
				r.AddWithLocation(report.Usage, "ACC-002",
					"Image element is missing 'alt' attribute for accessibility",
					fullPath)
			}
		}
	}
}

// ACC-003: html element should declare language
func checkHTMLLangPresent(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.MediaType != "application/xhtml+xml" || item.Href == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue
		}
		decoder := newXHTMLDecoder(strings.NewReader(string(data)))
		for {
			tok, err := decoder.Token()
			if err != nil {
				break
			}
			se, ok := tok.(xml.StartElement)
			if !ok || se.Name.Local != "html" {
				continue
			}
			hasLang := false
			for _, attr := range se.Attr {
				if attr.Name.Local == "lang" {
					hasLang = true
					break
				}
			}
			if !hasLang {
				r.AddWithLocation(report.Usage, "ACC-003",
					"Content document html element should declare a language via 'lang' or 'xml:lang'",
					fullPath)
			}
			break // Only check the html element
		}
	}
}

// ACC-004: when dc:source is present, page-list navigation should exist
func checkPageSourceHasPageList(ep *epub.EPUB, r *report.Report) {
	if len(ep.Package.Metadata.Sources) == 0 {
		return
	}

	// Check if navigation document has page-list
	var navHref string
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			navHref = item.Href
			break
		}
	}
	if navHref == "" {
		r.Add(report.Usage, "ACC-004",
			"EPUB has dc:source (indicating print source) but no page-list navigation was found")
		return
	}

	fullPath := ep.ResolveHref(navHref)
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return
	}

	if !navDocHasPageList(data) {
		r.Add(report.Usage, "ACC-004",
			"EPUB has dc:source (indicating print source) but no page-list navigation was found")
	}
}

func navDocHasPageList(data []byte) bool {
	decoder := newXHTMLDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "nav" {
				for _, attr := range se.Attr {
					if attr.Name.Local == "type" && containsToken(attr.Value, "page-list") {
						return true
					}
				}
			}
		}
	}
	return false
}

// ACC-010: landmarks navigation should be present
func checkLandmarksNavPresent(ep *epub.EPUB, r *report.Report) {
	var navHref string
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			navHref = item.Href
			break
		}
	}
	if navHref == "" {
		r.Add(report.Usage, "ACC-010",
			fmt.Sprintf("Navigation document should include a landmarks nav element"))
		return
	}

	fullPath := ep.ResolveHref(navHref)
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return
	}

	if !navDocHasLandmarks(data) {
		r.Add(report.Usage, "ACC-010",
			"Navigation document should include a landmarks nav element")
	}
}

func navDocHasLandmarks(data []byte) bool {
	decoder := newXHTMLDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "nav" {
				for _, attr := range se.Attr {
					if attr.Name.Local == "type" && containsToken(attr.Value, "landmarks") {
						return true
					}
				}
			}
		}
	}
	return false
}
