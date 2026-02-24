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

// utf16BEBom and utf16LEBom are the byte order marks for UTF-16 encodings.
var utf16BEBom = []byte{0xFE, 0xFF}
var utf16LEBom = []byte{0xFF, 0xFE}

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

		// CSS-003: warn if CSS is UTF-16 encoded (detected by BOM); skip further checks
		if len(data) >= 2 && (data[0] == 0xFE && data[1] == 0xFF ||
			data[0] == 0xFF && data[1] == 0xFE) {
			r.AddWithLocation(report.Warning, "CSS-003",
				"CSS document is encoded in UTF-16; UTF-8 encoding is recommended",
				fullPath)
			continue // Can't reliably parse UTF-16 as UTF-8 text
		}

		cssContent := string(data)

		// CSS-004: @charset must be UTF-8 or UTF-16 if present
		checkCSSCharset(cssContent, fullPath, r)

		// CSS-001: forbidden CSS properties (direction, unicode-bidi)
		checkCSSForbiddenProperties(cssContent, fullPath, r)

		// CSS-008: CSS syntax errors (unclosed braces)
		checkCSSSyntax(cssContent, fullPath, r)

		// CSS-002: @font-face empty URL reference
		checkCSSFontFaceEmptyURL(cssContent, fullPath, r)

		// CSS-011: @font-face must have src descriptor; CSS-019: empty @font-face
		checkCSSFontFaceHasSrc(cssContent, fullPath, r)

		// OPF-014 / RSC-006: remote font sources
		checkCSSRemoteFonts(ep, cssContent, fullPath, item, r)

		// CSS-005: no @import rules; RSC-001/RSC-008: imported files must exist
		checkCSSNoImport(cssContent, fullPath, r)
		checkCSSImportTargets(ep, cssContent, fullPath, manifestHrefs, r)

		// RSC-007: font file referenced in CSS must exist in the container
		checkCSSFontFileExists(ep, cssContent, fullPath, r)

		// RSC-007: background-image referenced file must exist in the container
		checkCSSBackgroundImageExists(ep, cssContent, fullPath, r)

		// RSC-008: CSS-referenced resources must be in manifest
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
	"adobe-text-layout": true,
	// Modern CSS properties
	"text-wrap": true, "hanging-punctuation": true,
	"aspect-ratio": true, "accent-color": true,
	"contain": true, "container": true, "container-name": true, "container-type": true,
	"text-decoration-thickness": true, "text-underline-offset": true,
	"scroll-margin": true, "scroll-padding": true,
	"inset": true, "inset-block": true, "inset-inline": true,
	"margin-block": true, "margin-inline": true,
	"padding-block": true, "padding-inline": true,
	"border-block": true, "border-inline": true,
	"inline-size": true, "block-size": true,
	"max-inline-size": true, "max-block-size": true,
	"min-inline-size": true, "min-block-size": true,
}

// checkCSSCharset validates the @charset declaration if present.
// CSS-004: if @charset is present, it must be UTF-8 or UTF-16.
func checkCSSCharset(css string, location string, r *report.Report) {
	charsetRe := regexp.MustCompile(`(?i)@charset\s+["']([^"']+)["']`)
	m := charsetRe.FindStringSubmatch(css)
	if m == nil {
		return
	}
	enc := strings.ToLower(strings.TrimSpace(m[1]))
	if enc != "utf-8" && enc != "utf-16" && enc != "utf-16be" && enc != "utf-16le" {
		r.AddWithLocation(report.Error, "CSS-004",
			fmt.Sprintf("The CSS @charset declaration must be 'UTF-8' or 'UTF-16', but found '%s'", m[1]),
			location)
	}
}

// checkCSSForbiddenProperties reports use of CSS properties that are not
// allowed in EPUB content documents.
// CSS-001: 'direction' and 'unicode-bidi' properties are forbidden.
func checkCSSForbiddenProperties(css string, location string, r *report.Report) {
	// Strip comments
	commentRe := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	css = commentRe.ReplaceAllString(css, "")

	// Match property name at start of a declaration (after whitespace or ;)
	propRe := regexp.MustCompile(`(?m)(?:^|;)\s*(direction|unicode-bidi)\s*:`)
	matches := propRe.FindAllStringSubmatch(css, -1)
	for _, m := range matches {
		prop := strings.TrimSpace(m[1])
		r.AddWithLocation(report.Error, "CSS-001",
			fmt.Sprintf("CSS property '%s' is not allowed in EPUB content documents", prop),
			location)
	}
}

// checkCSSFontFaceEmptyURL reports @font-face rules with empty URL references.
// CSS-002: @font-face src URL must not be empty.
func checkCSSFontFaceEmptyURL(css string, location string, r *report.Report) {
	fontFaceRe := regexp.MustCompile(`@font-face\s*\{([^}]*)\}`)
	emptyURLRe := regexp.MustCompile(`url\(\s*['"]{0,1}\s*['"]{0,1}\s*\)`)

	matches := fontFaceRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		body := match[1]
		if emptyURLRe.MatchString(body) {
			r.AddWithLocation(report.Error, "CSS-002",
				"@font-face 'src' has an empty URL reference",
				location)
		}
	}
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

// CSS-011: @font-face rules must include a src descriptor
// CSS-019: @font-face rules must not be empty
func checkCSSFontFaceHasSrc(css string, location string, r *report.Report) {
	fontFaceRe := regexp.MustCompile(`@font-face\s*\{([^}]*)\}`)
	matches := fontFaceRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		body := match[1]
		trimmed := strings.TrimSpace(body)
		if trimmed == "" {
			// Empty @font-face block
			r.AddWithLocation(report.Warning, "CSS-019",
				"@font-face rule is empty",
				location)
		} else if !strings.Contains(body, "src") {
			r.AddWithLocation(report.Warning, "CSS-011",
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

// checkCSSImportTargets validates @import target files:
//   - RSC-001: imported file is in manifest but not in container
//   - RSC-008: imported file is in container but not declared in manifest
func checkCSSImportTargets(ep *epub.EPUB, css string, location string, manifestHrefs map[string]bool, r *report.Report) {
	// Match @import "url" or @import url("url")
	importRe := regexp.MustCompile(`@import\s+(?:url\(['"]?|['"])([^'")\s]+)`)
	cssDir := path.Dir(location)

	for _, match := range importRe.FindAllStringSubmatch(css, -1) {
		href := match[1]
		if isRemoteURL(href) {
			continue
		}
		parsed, err := url.Parse(href)
		if err != nil {
			continue
		}
		target := resolvePath(cssDir, parsed.Path)
		_, inContainer := ep.Files[target]
		inManifest := manifestHrefs[target]

		if !inContainer {
			if inManifest {
				// RSC-001: in manifest but missing from container — handled by checkManifestFilesExist
				// Don't double-report here
			} else {
				// Not in container, not in manifest - RSC-007
				r.AddWithLocation(report.Error, "RSC-007",
					fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
					location)
			}
		} else if !inManifest {
			// RSC-008: in container but not in manifest
			r.AddWithLocation(report.Error, "RSC-008",
				fmt.Sprintf("Referenced resource '%s' is in the container but not declared in the OPF manifest", href),
				location)
		}
	}
}

// CSS-008: CSS syntax errors (unclosed braces)
func checkCSSSyntax(css string, location string, r *report.Report) {
	// Strip comments before analyzing
	commentRe := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	css = commentRe.ReplaceAllString(css, "")

	// Count unclosed braces: each unmatched '{' is a syntax error
	depth := 0
	maxDepth := 0
	for _, ch := range css {
		if ch == '{' {
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		} else if ch == '}' {
			if depth > 0 {
				depth--
			}
		}
	}
	// Each unclosed brace is one CSS-008 error
	for i := 0; i < depth; i++ {
		r.AddWithLocation(report.Error, "CSS-008",
			"An error occurred while parsing the CSS: unclosed block",
			location)
	}
}

// checkCSSRemoteFonts: remote fonts in CSS are allowed per EPUB spec,
// but the referencing content document needs the remote-resources property.
// We check for remote URLs in the CSS and report OPF-014 if no content document
// declaring remote-resources references this CSS.
// RSC-031: warn if remote URL uses http:// instead of https://.
// RSC-030: error if file:// URL is used.
func checkCSSRemoteFonts(ep *epub.EPUB, css string, location string, item epub.ManifestItem, r *report.Report) {
	// Strip @namespace lines (use url() for namespace identifiers, not resources)
	namespaceRe := regexp.MustCompile(`(?m)@namespace\s+[^\n;]+;`)
	strippedCSS := namespaceRe.ReplaceAllString(css, "")

	// RSC-031: http:// remote URLs in CSS are insecure
	httpURLRe := regexp.MustCompile(`url\(['"]?(http://[^'"\)\s]+)['"]?\)`)
	for _, m := range httpURLRe.FindAllStringSubmatch(strippedCSS, -1) {
		r.AddWithLocation(report.Warning, "RSC-031",
			fmt.Sprintf("Remote resource uses insecure 'http' scheme: '%s'", m[1]),
			location)
	}

	// RSC-030: file:// URLs are not allowed
	fileURLRe := regexp.MustCompile(`url\(['"]?(file:[^'"\)\s]+)['"]?\)`)
	for _, m := range fileURLRe.FindAllStringSubmatch(strippedCSS, -1) {
		r.AddWithLocation(report.Error, "RSC-030",
			fmt.Sprintf("Use of 'file' URL scheme is prohibited: '%s'", m[1]),
			location)
	}

	// Remove @namespace lines as they use url() for namespace identifiers, not resources
	cleaned := strippedCSS
	urlRe := regexp.MustCompile(`url\(['"]?(https?://[^'"\)\s]+)['"]?\)`)
	if !urlRe.MatchString(cleaned) {
		return
	}

	// If the CSS item itself has remote-resources property, no error
	if hasProperty(item.Properties, "remote-resources") {
		return
	}

	// The CSS has remote URLs - find which content documents reference this CSS
	// and check if any of them has the remote-resources property
	found := false
	anyHasProperty := false
	for _, mItem := range ep.Package.Manifest {
		if mItem.MediaType != "application/xhtml+xml" && mItem.MediaType != "image/svg+xml" {
			continue
		}
		if mItem.Href == "\x00MISSING" {
			continue
		}
		contentPath := ep.ResolveHref(mItem.Href)
		data, err := ep.ReadFile(contentPath)
		if err != nil {
			continue
		}
		content := string(data)
		// Check if the content document references this CSS
		if strings.Contains(content, item.Href) || strings.Contains(content, path.Base(item.Href)) {
			found = true
			if hasProperty(mItem.Properties, "remote-resources") {
				anyHasProperty = true
			} else {
				r.AddWithLocation(report.Error, "OPF-014",
					"Property 'remote-resources' should be declared in the manifest for content with remote resources",
					contentPath)
			}
		}
	}
	// If no content document explicitly references the CSS, report on the CSS item itself
	if !found && !anyHasProperty {
		r.AddWithLocation(report.Error, "OPF-014",
			"Property 'remote-resources' should be declared in the manifest for content with remote resources",
			location)
	}
}

// checkCSSFontFileExists: font file sources must exist in the container.
// RSC-007: referenced resource not found in the container.
// RSC-008: remote font not declared in the manifest.
// CSS-007: info when font uses a non-standard (non-core) media type.
func checkCSSFontFileExists(ep *epub.EPUB, css string, location string, r *report.Report) {
	fontFaceRe := regexp.MustCompile(`@font-face\s*\{([^}]*)\}`)
	urlRe := regexp.MustCompile(`url\(['"]?([^'")\s]+)['"]?\)`)

	cssDir := path.Dir(location)

	// Build manifest lookups
	manifestPaths := make(map[string]bool)
	manifestByPath := make(map[string]string) // path -> media-type
	remoteManifestURLs := make(map[string]bool)
	if ep.Package != nil {
		for _, item := range ep.Package.Manifest {
			if item.Href != "\x00MISSING" {
				fp := ep.ResolveHref(item.Href)
				manifestPaths[fp] = true
				manifestByPath[fp] = item.MediaType
			}
			if isRemoteURL(item.Href) {
				remoteManifestURLs[item.Href] = true
			}
		}
	}

	matches := fontFaceRe.FindAllStringSubmatch(css, -1)
	for _, match := range matches {
		urls := urlRe.FindAllStringSubmatch(match[1], -1)
		for _, u := range urls {
			href := u[1]
			if isRemoteURL(href) {
				// RSC-008: remote font not declared in manifest.
				// Strip fragment (e.g. https://example.org/svg#font → https://example.org/svg)
				// since manifest items reference the base resource URL.
				baseURL := href
				if idx := strings.Index(href, "#"); idx >= 0 {
					baseURL = href[:idx]
				}
				if !remoteManifestURLs[baseURL] {
					r.AddWithLocation(report.Error, "RSC-008",
						fmt.Sprintf("Remote resource '%s' is not declared in the package document", href),
						location)
				}
				continue
			}
			// Skip file:// URLs (handled by RSC-030)
			if isFileURL(href) {
				continue
			}
			// Skip empty URLs (handled by CSS-002)
			if strings.TrimSpace(href) == "" {
				continue
			}
			parsed, err := url.Parse(href)
			if err != nil {
				continue
			}
			// Skip fragment-only URLs
			if parsed.Path == "" {
				continue
			}
			target := resolvePath(cssDir, parsed.Path)
			if _, exists := ep.Files[target]; !exists {
				// Skip if in manifest - RSC-001 will report it
				if manifestPaths[target] {
					continue
				}
				r.AddWithLocation(report.Error, "RSC-007",
					fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
					location)
			} else {
				// CSS-007: info when font uses a non-standard (non-core) media type
				if mt, ok := manifestByPath[target]; ok {
					if isFontMediaType(mt) && !coreMediaTypes[mt] && !acceptedFontTypeAliases[mt] {
						r.AddWithLocation(report.Info, "CSS-007",
							fmt.Sprintf("Font '%s' uses a non-standard media type '%s'", href, mt),
							location)
					}
				}
			}
		}
	}
}

// checkCSSBackgroundImageExists: background-image referenced files must exist.
// RSC-007: referenced resource not found in the container.
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
		// Skip fragment-only URLs
		if parsed.Path == "" {
			continue
		}
		target := resolvePath(cssDir, parsed.Path)
		if _, exists := ep.Files[target]; !exists {
			r.AddWithLocation(report.Error, "RSC-007",
				fmt.Sprintf("Referenced resource '%s' could not be found in the container", href),
				location)
		}
	}
}

// checkCSSResourceInManifest: CSS-referenced resources must be declared in the OPF manifest.
// RSC-008: resource in container but not declared in the manifest.
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
		// Skip fragment-only URLs
		if parsed.Path == "" {
			continue
		}
		target := resolvePath(cssDir, parsed.Path)
		if _, exists := ep.Files[target]; exists {
			if !manifestHrefs[target] {
				r.AddWithLocation(report.Error, "RSC-008",
					fmt.Sprintf("Referenced resource '%s' is not declared in the OPF manifest", href),
					location)
			}
		}
	}
}
