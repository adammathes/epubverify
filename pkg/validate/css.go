package validate

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// checkCSS validates CSS files referenced in the manifest.
func checkCSS(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil {
		return
	}

	manifestHrefs := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestHrefs[ep.ResolveHref(item.Href)] = true
		}
	}

	for _, item := range ep.Package.Manifest {
		if item.MediaType != "text/css" {
			continue
		}
		if item.Href == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue // Missing file handled by RSC-005
		}

		cssContent := string(data)

		// CSS-001: CSS syntax errors
		checkCSSSyntax(cssContent, fullPath, r)

		// CSS-004: no remote font sources
		checkCSSRemoteFonts(ep, cssContent, fullPath, item, r)

		// CSS-006: font file referenced in CSS must exist
		checkCSSFontFileExists(ep, cssContent, fullPath, r)

		// CSS-007: background-image referenced file must exist
		checkCSSBackgroundImageExists(ep, cssContent, fullPath, r)

		// CSS-008: CSS-referenced resources must be in manifest
		checkCSSResourceInManifest(ep, cssContent, fullPath, manifestHrefs, r)
	}
}

// CSS-001: basic CSS syntax validation
var cssSyntaxErrorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`:\s*;`),          // empty value like "color: ;"
	regexp.MustCompile(`[^:}\s]\s*\}`),   // missing colon before }
}

func checkCSSSyntax(css string, location string, r *report.Report) {
	// Check for properties without values (like "color: ;")
	re := regexp.MustCompile(`:\s*;`)
	if matches := re.FindAllString(css, -1); len(matches) > 0 {
		r.AddWithLocation(report.Error, "CSS-001",
			"An error occurred while parsing the CSS: empty property value",
			location)
	}

	// Check for properties without colon (like "font-size }")
	lines := strings.Split(css, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "@") ||
			strings.HasPrefix(trimmed, "}") || trimmed == "{" {
			continue
		}
		// Inside a rule: should have "property: value" pattern
		if !strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "{") &&
			!strings.Contains(trimmed, "}") && !strings.HasPrefix(trimmed, ".") &&
			!strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "*") &&
			len(trimmed) > 0 {
			// Looks like a property without colon
			if strings.ContainsAny(trimmed, "abcdefghijklmnopqrstuvwxyz-") && !strings.Contains(trimmed, ",") {
				r.AddWithLocation(report.Error, "CSS-001",
					fmt.Sprintf("An error occurred while parsing the CSS: '%s'", trimmed),
					location)
			}
		}
	}
}

// CSS-004: no remote font sources in @font-face
func checkCSSRemoteFonts(ep *epub.EPUB, css string, location string, item epub.ManifestItem, r *report.Report) {
	fontFaceRe := regexp.MustCompile(`@font-face\s*\{([^}]*)\}`)
	urlRe := regexp.MustCompile(`url\(['"]?(https?://[^'")\s]+)['"]?\)`)

	matches := fontFaceRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		urls := urlRe.FindAllStringSubmatch(match[1], -1)
		for range urls {
			r.AddWithLocation(report.Error, "CSS-004",
				"The property 'remote-resources' should be declared in the OPF manifest",
				location)
		}
	}
}

// CSS-006: font file sources must exist
func checkCSSFontFileExists(ep *epub.EPUB, css string, location string, r *report.Report) {
	fontFaceRe := regexp.MustCompile(`@font-face\s*\{([^}]*)\}`)
	urlRe := regexp.MustCompile(`url\(['"]?([^'")\s]+)['"]?\)`)

	cssDir := path.Dir(location)

	matches := fontFaceRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		urls := urlRe.FindAllStringSubmatch(match[1], -1)
		for _, u := range urls {
			href := u[1]
			if isRemoteURL(href) {
				continue // Handled by CSS-004
			}
			parsed, err := url.Parse(href)
			if err != nil {
				continue
			}
			target := resolvePath(cssDir, parsed.Path)
			if _, exists := ep.Files[target]; !exists {
				r.AddWithLocation(report.Error, "CSS-006",
					fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
					location)
			}
		}
	}
}

// CSS-007: background-image referenced files must exist
func checkCSSBackgroundImageExists(ep *epub.EPUB, css string, location string, r *report.Report) {
	bgRe := regexp.MustCompile(`background(?:-image)?\s*:\s*url\(['"]?([^'")\s]+)['"]?\)`)
	cssDir := path.Dir(location)

	matches := bgRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		href := match[1]
		if isRemoteURL(href) {
			continue
		}
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		target := resolvePath(cssDir, parsed.Path)
		if _, exists := ep.Files[target]; !exists {
			r.AddWithLocation(report.Error, "CSS-007",
				fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
				location)
		}
	}
}

// CSS-008: CSS-referenced resources must be declared in the OPF manifest
func checkCSSResourceInManifest(ep *epub.EPUB, css string, location string, manifestHrefs map[string]bool, r *report.Report) {
	bgRe := regexp.MustCompile(`url\(['"]?([^'")\s]+)['"]?\)`)
	cssDir := path.Dir(location)

	matches := bgRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		href := match[1]
		if isRemoteURL(href) {
			continue
		}
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		target := resolvePath(cssDir, parsed.Path)
		if _, exists := ep.Files[target]; exists {
			if !manifestHrefs[target] {
				r.AddWithLocation(report.Error, "CSS-008",
					fmt.Sprintf("Referenced resource '%s' is not declared in the OPF manifest", href),
					location)
			}
		}
	}
}
