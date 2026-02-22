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

		// CSS-002: unknown CSS property names
		checkCSSValidProperties(cssContent, fullPath, r)

		// CSS-003: @font-face must have src descriptor
		checkCSSFontFaceHasSrc(cssContent, fullPath, r)

		// CSS-004: no remote font sources
		checkCSSRemoteFonts(ep, cssContent, fullPath, item, r)

		// CSS-005: no @import rules
		checkCSSNoImport(cssContent, fullPath, r)

		// CSS-006: font file referenced in CSS must exist
		checkCSSFontFileExists(ep, cssContent, fullPath, r)

		// CSS-007: background-image referenced file must exist
		checkCSSBackgroundImageExists(ep, cssContent, fullPath, r)

		// CSS-008: CSS-referenced resources must be in manifest
		checkCSSResourceInManifest(ep, cssContent, fullPath, manifestHrefs, r)
	}
}

// Common CSS property names (comprehensive but not exhaustive)
var knownCSSProperties = map[string]bool{
	"align-content": true, "align-items": true, "align-self": true,
	"all": true, "animation": true, "animation-delay": true,
	"animation-direction": true, "animation-duration": true,
	"animation-fill-mode": true, "animation-iteration-count": true,
	"animation-name": true, "animation-play-state": true,
	"animation-timing-function": true, "backface-visibility": true,
	"background": true, "background-attachment": true,
	"background-blend-mode": true, "background-clip": true,
	"background-color": true, "background-image": true,
	"background-origin": true, "background-position": true,
	"background-repeat": true, "background-size": true,
	"border": true, "border-bottom": true, "border-bottom-color": true,
	"border-bottom-left-radius": true, "border-bottom-right-radius": true,
	"border-bottom-style": true, "border-bottom-width": true,
	"border-collapse": true, "border-color": true,
	"border-image": true, "border-image-outset": true,
	"border-image-repeat": true, "border-image-slice": true,
	"border-image-source": true, "border-image-width": true,
	"border-left": true, "border-left-color": true,
	"border-left-style": true, "border-left-width": true,
	"border-radius": true, "border-right": true,
	"border-right-color": true, "border-right-style": true,
	"border-right-width": true, "border-spacing": true,
	"border-style": true, "border-top": true,
	"border-top-color": true, "border-top-left-radius": true,
	"border-top-right-radius": true, "border-top-style": true,
	"border-top-width": true, "border-width": true,
	"bottom": true, "box-decoration-break": true,
	"box-shadow": true, "box-sizing": true, "break-after": true,
	"break-before": true, "break-inside": true, "caption-side": true,
	"clear": true, "clip": true, "clip-path": true, "color": true,
	"column-count": true, "column-fill": true, "column-gap": true,
	"column-rule": true, "column-rule-color": true,
	"column-rule-style": true, "column-rule-width": true,
	"column-span": true, "column-width": true, "columns": true,
	"content": true, "counter-increment": true, "counter-reset": true,
	"cursor": true, "direction": true, "display": true,
	"empty-cells": true, "filter": true, "flex": true,
	"flex-basis": true, "flex-direction": true, "flex-flow": true,
	"flex-grow": true, "flex-shrink": true, "flex-wrap": true,
	"float": true, "font": true, "font-family": true,
	"font-feature-settings": true, "font-kerning": true,
	"font-size": true, "font-size-adjust": true,
	"font-stretch": true, "font-style": true,
	"font-variant": true, "font-variant-caps": true,
	"font-variant-east-asian": true, "font-variant-ligatures": true,
	"font-variant-numeric": true, "font-weight": true,
	"gap": true, "grid": true, "grid-area": true,
	"grid-auto-columns": true, "grid-auto-flow": true,
	"grid-auto-rows": true, "grid-column": true,
	"grid-column-end": true, "grid-column-gap": true,
	"grid-column-start": true, "grid-gap": true,
	"grid-row": true, "grid-row-end": true,
	"grid-row-gap": true, "grid-row-start": true,
	"grid-template": true, "grid-template-areas": true,
	"grid-template-columns": true, "grid-template-rows": true,
	"height": true, "hyphens": true, "justify-content": true,
	"justify-items": true, "justify-self": true, "left": true,
	"letter-spacing": true, "line-height": true,
	"list-style": true, "list-style-image": true,
	"list-style-position": true, "list-style-type": true,
	"margin": true, "margin-bottom": true, "margin-left": true,
	"margin-right": true, "margin-top": true,
	"max-height": true, "max-width": true, "min-height": true,
	"min-width": true, "mix-blend-mode": true,
	"object-fit": true, "object-position": true,
	"opacity": true, "order": true, "orphans": true,
	"outline": true, "outline-color": true,
	"outline-offset": true, "outline-style": true,
	"outline-width": true, "overflow": true,
	"overflow-wrap": true, "overflow-x": true, "overflow-y": true,
	"padding": true, "padding-bottom": true, "padding-left": true,
	"padding-right": true, "padding-top": true,
	"page-break-after": true, "page-break-before": true,
	"page-break-inside": true, "perspective": true,
	"perspective-origin": true, "place-content": true,
	"place-items": true, "place-self": true,
	"pointer-events": true, "position": true, "quotes": true,
	"resize": true, "right": true, "row-gap": true,
	"scroll-behavior": true, "tab-size": true,
	"table-layout": true, "text-align": true,
	"text-align-last": true, "text-decoration": true,
	"text-decoration-color": true, "text-decoration-line": true,
	"text-decoration-style": true, "text-indent": true,
	"text-justify": true, "text-overflow": true,
	"text-shadow": true, "text-transform": true, "top": true,
	"transform": true, "transform-origin": true,
	"transform-style": true, "transition": true,
	"transition-delay": true, "transition-duration": true,
	"transition-property": true, "transition-timing-function": true,
	"unicode-bidi": true, "user-select": true,
	"vertical-align": true, "visibility": true,
	"white-space": true, "widows": true, "width": true,
	"word-break": true, "word-spacing": true, "word-wrap": true,
	"writing-mode": true, "z-index": true,
	// @font-face descriptors
	"src": true, "font-display": true, "unicode-range": true,
	// Vendor prefixed (common ones)
	"-webkit-appearance": true, "-moz-appearance": true,
	"-webkit-text-size-adjust": true, "-moz-osx-font-smoothing": true,
	"-webkit-font-smoothing": true, "-webkit-overflow-scrolling": true,
	"-webkit-hyphens": true, "-moz-hyphens": true, "-ms-hyphens": true,
	"-epub-hyphens": true, "-epub-writing-mode": true,
	"-webkit-writing-mode": true, "-ms-writing-mode": true,
	"oeb-column-number": true, "adobe-hyphenate": true,
}

// CSS-002: CSS stylesheets should use valid CSS property names
func checkCSSValidProperties(css string, location string, r *report.Report) {
	// Remove comments
	commentRe := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	css = commentRe.ReplaceAllString(css, "")

	// Extract property declarations only from inside rule blocks.
	// We track brace depth to skip selectors (outside blocks).
	inBlock := 0
	propRe := regexp.MustCompile(`^\s*([\w-]+)\s*:`)
	for _, line := range strings.Split(css, "\n") {
		trimmed := strings.TrimSpace(line)
		// Count braces to track whether we're inside a rule block
		for _, ch := range trimmed {
			if ch == '{' {
				inBlock++
			} else if ch == '}' {
				if inBlock > 0 {
					inBlock--
				}
			}
		}
		// Only look for properties when inside a block and the line
		// doesn't contain a brace (selector lines like "a:hover {")
		if inBlock > 0 && !strings.ContainsAny(trimmed, "{}") && trimmed != "" {
			if match := propRe.FindStringSubmatch(line); match != nil {
				prop := strings.TrimSpace(match[1])
				if strings.HasPrefix(prop, "-") {
					continue
				}
				if !knownCSSProperties[prop] {
					r.AddWithLocation(report.Warning, "CSS-002",
						fmt.Sprintf("CSS property '%s' is not a recognized property name", prop),
						location)
				}
			}
		}
	}
}

// CSS-003: @font-face rules must include a src descriptor
func checkCSSFontFaceHasSrc(css string, location string, r *report.Report) {
	fontFaceRe := regexp.MustCompile(`@font-face\s*\{([^}]*)\}`)
	matches := fontFaceRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		body := match[1]
		if !strings.Contains(body, "src") {
			r.AddWithLocation(report.Warning, "CSS-003",
				"@font-face rule is missing required 'src' descriptor",
				location)
		}
	}
}

// CSS-005: @import rules should not be used in EPUB CSS stylesheets
func checkCSSNoImport(css string, location string, r *report.Report) {
	importRe := regexp.MustCompile(`@import\s+`)
	if importRe.MatchString(css) {
		r.AddWithLocation(report.Warning, "CSS-005",
			"@import rules should not be used in EPUB CSS stylesheets",
			location)
	}
}

// CSS-001: basic CSS syntax validation
var cssSyntaxErrorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`:\s*;`),          // empty value like "color: ;"
	regexp.MustCompile(`[^:}\s]\s*\}`),   // missing colon before }
}

func checkCSSSyntax(css string, location string, r *report.Report) {
	// Strip comments before analyzing
	commentRe := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	css = commentRe.ReplaceAllString(css, "")

	// Check for properties without values (like "color: ;")
	re := regexp.MustCompile(`:\s*;`)
	if matches := re.FindAllString(css, -1); len(matches) > 0 {
		r.AddWithLocation(report.Error, "CSS-001",
			"An error occurred while parsing the CSS: empty property value",
			location)
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
			r.AddWithLocation(report.Warning, "CSS-007",
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
