package doctor

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// Fix represents a single applied fix.
type Fix struct {
	CheckID     string
	Description string
	File        string // which file was modified (empty for zip-level fixes)
}

// fixMimetype ensures the mimetype file has the correct content.
// Fixes OCF-003. OCF-002/004/005 are handled by the writer.
func fixMimetype(files map[string][]byte) []Fix {
	var fixes []Fix
	expected := []byte("application/epub+zip")

	current, exists := files["mimetype"]
	if !exists {
		files["mimetype"] = expected
		fixes = append(fixes, Fix{
			CheckID:     "OCF-001",
			Description: "Added missing mimetype file",
		})
		return fixes
	}

	if !bytes.Equal(current, expected) {
		files["mimetype"] = expected
		fixes = append(fixes, Fix{
			CheckID:     "OCF-003",
			Description: fmt.Sprintf("Fixed mimetype content from '%s' to 'application/epub+zip'", strings.TrimSpace(string(current))),
		})
	}

	return fixes
}

// fixDCTermsModified adds a dcterms:modified element if missing in EPUB 3.
// Fixes OPF-004.
func fixDCTermsModified(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil || ep.Package.Version < "3.0" {
		return nil
	}
	if ep.Package.Metadata.Modified != "" {
		return nil
	}

	opfData, ok := files[ep.RootfilePath]
	if !ok {
		return nil
	}

	content := string(opfData)
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	// Insert before </metadata>
	metaClose := strings.Index(content, "</metadata>")
	if metaClose == -1 {
		// Try with namespace prefix
		metaClose = findClosingTag(content, "metadata")
	}
	if metaClose == -1 {
		return nil
	}

	insertion := fmt.Sprintf("    <meta property=\"dcterms:modified\">%s</meta>\n  ", now)
	newContent := content[:metaClose] + insertion + content[metaClose:]
	files[ep.RootfilePath] = []byte(newContent)

	return []Fix{{
		CheckID:     "OPF-004",
		Description: fmt.Sprintf("Added dcterms:modified with value '%s'", now),
		File:        ep.RootfilePath,
	}}
}

// fixMediaTypes corrects manifest media-type attributes that don't match actual content.
// Fixes OPF-024 and MED-001.
func fixMediaTypes(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil {
		return nil
	}

	var fixes []Fix

	// Image magic bytes
	pngMagic := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	jpegMagic := []byte{0xff, 0xd8, 0xff}
	gifMagic := []byte{0x47, 0x49, 0x46, 0x38}

	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.MediaType == "\x00MISSING" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)

		// Check extension-based mismatch
		ext := strings.ToLower(path.Ext(item.Href))
		expectedByExt := extensionToMediaType(ext)

		// Check magic-byte-based mismatch for images
		var detectedByMagic string
		if data, ok := files[fullPath]; ok && strings.HasPrefix(item.MediaType, "image/") && item.MediaType != "image/svg+xml" {
			if len(data) >= 8 {
				if bytes.HasPrefix(data, pngMagic) {
					detectedByMagic = "image/png"
				} else if bytes.HasPrefix(data, jpegMagic) {
					detectedByMagic = "image/jpeg"
				} else if bytes.HasPrefix(data, gifMagic) {
					detectedByMagic = "image/gif"
				}
			}
		}

		// Determine the correct type — prefer magic bytes for images, fall back to extension
		correctType := ""
		if detectedByMagic != "" && detectedByMagic != item.MediaType {
			correctType = detectedByMagic
		} else if expectedByExt != "" && expectedByExt != item.MediaType {
			// Only fix extension-based mismatches for non-images (images use magic bytes)
			if !strings.HasPrefix(item.MediaType, "image/") || !strings.HasPrefix(expectedByExt, "image/") {
				correctType = expectedByExt
			}
		}

		if correctType != "" {
			fixes = append(fixes, Fix{
				CheckID:     "OPF-024",
				Description: fmt.Sprintf("Fixed media-type for '%s' from '%s' to '%s'", item.Href, item.MediaType, correctType),
				File:        ep.RootfilePath,
			})
			// Apply fix in OPF
			opfData := files[ep.RootfilePath]
			opfStr := string(opfData)
			// Replace the specific media-type for this item's href
			// Match: href="<href>" ... media-type="<old>"  or  media-type="<old>" ... href="<href>"
			opfStr = fixManifestItemMediaType(opfStr, item.Href, item.MediaType, correctType)
			files[ep.RootfilePath] = []byte(opfStr)
		}
	}

	return fixes
}

// fixManifestProperties adds missing scripted/svg/mathml properties to manifest items.
// Fixes HTM-005, HTM-006, HTM-007.
func fixManifestProperties(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil || ep.Package.Version < "3.0" {
		return nil
	}

	var fixes []Fix

	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.MediaType != "application/xhtml+xml" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)
		data, ok := files[fullPath]
		if !ok {
			continue
		}

		// Skip nav documents
		if hasProperty(item.Properties, "nav") {
			continue
		}

		hasScript, hasSVG, hasMathML := detectContentFeatures(data)
		var missing []string

		if hasScript && !hasProperty(item.Properties, "scripted") {
			missing = append(missing, "scripted")
		}
		if hasSVG && !hasProperty(item.Properties, "svg") {
			missing = append(missing, "svg")
		}
		if hasMathML && !hasProperty(item.Properties, "mathml") {
			missing = append(missing, "mathml")
		}

		if len(missing) == 0 {
			continue
		}

		newProps := item.Properties
		for _, m := range missing {
			if newProps == "" {
				newProps = m
			} else {
				newProps = newProps + " " + m
			}
		}

		opfData := files[ep.RootfilePath]
		opfStr := string(opfData)
		opfStr = fixManifestItemProperties(opfStr, item.ID, item.Properties, newProps)
		files[ep.RootfilePath] = []byte(opfStr)

		for _, m := range missing {
			checkID := "HTM-005"
			if m == "svg" {
				checkID = "HTM-006"
			} else if m == "mathml" {
				checkID = "HTM-007"
			}
			fixes = append(fixes, Fix{
				CheckID:     checkID,
				Description: fmt.Sprintf("Added '%s' property to manifest item '%s'", m, item.ID),
				File:        ep.RootfilePath,
			})
		}
	}

	return fixes
}

// fixDoctype replaces XHTML/DTD doctypes with HTML5 DOCTYPE in EPUB 3 content docs.
// Fixes HTM-010 and HTM-011.
func fixDoctype(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil || ep.Package.Version < "3.0" {
		return nil
	}

	var fixes []Fix
	doctypeRe := regexp.MustCompile(`(?i)<!DOCTYPE[^>]*>`)

	for _, item := range ep.Package.Manifest {
		if item.MediaType != "application/xhtml+xml" || item.Href == "\x00MISSING" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)
		data, ok := files[fullPath]
		if !ok {
			continue
		}

		content := string(data)
		match := doctypeRe.FindString(content)
		if match == "" {
			continue
		}

		upper := strings.ToUpper(match)
		if strings.Contains(upper, "XHTML") || strings.Contains(upper, "DTD") {
			newContent := doctypeRe.ReplaceAllString(content, "<!DOCTYPE html>")
			files[fullPath] = []byte(newContent)
			fixes = append(fixes, Fix{
				CheckID:     "HTM-010",
				Description: fmt.Sprintf("Replaced non-HTML5 DOCTYPE with <!DOCTYPE html>"),
				File:        fullPath,
			})
		}
	}

	return fixes
}

// detectZipFixes checks the before-report for OCF issues that are fixed
// by construction when the writer rewrites the ZIP (mimetype ordering,
// compression, extra field). These don't modify the in-memory files but
// the writer's output will fix them.
func detectZipFixes(r *report.Report) []Fix {
	var fixes []Fix
	for _, msg := range r.Messages {
		switch msg.CheckID {
		case "OCF-002":
			fixes = append(fixes, Fix{
				CheckID:     "OCF-002",
				Description: "Reordered mimetype as first ZIP entry",
			})
		case "OCF-004":
			fixes = append(fixes, Fix{
				CheckID:     "OCF-004",
				Description: "Removed extra field from mimetype ZIP entry",
			})
		case "OCF-005":
			fixes = append(fixes, Fix{
				CheckID:     "OCF-005",
				Description: "Changed mimetype from compressed to stored",
			})
		}
	}
	return fixes
}

// --- Helper functions ---

func extensionToMediaType(ext string) string {
	switch ext {
	case ".xhtml", ".html", ".htm":
		return "application/xhtml+xml"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".ncx":
		return "application/x-dtbncx+xml"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".smil":
		return "application/smil+xml"
	default:
		return ""
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

func detectContentFeatures(data []byte) (hasScript, hasSVG, hasMathML bool) {
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
		if se.Name.Local == "script" {
			hasScript = true
		}
		if se.Name.Local == "svg" || se.Name.Space == "http://www.w3.org/2000/svg" {
			hasSVG = true
		}
		if se.Name.Local == "math" || se.Name.Space == "http://www.w3.org/1998/Math/MathML" {
			hasMathML = true
		}
	}
	return
}

// fixManifestItemMediaType replaces the media-type attribute for a manifest item matching href.
func fixManifestItemMediaType(opf, href, oldType, newType string) string {
	// Strategy: find the <item> element that contains this href and replace its media-type.
	// We look for the item element containing href="<href>" and replace media-type="<old>" with media-type="<new>".
	// This is done carefully to avoid false matches.

	// Escape for regex
	escapedHref := regexp.QuoteMeta(href)
	escapedOld := regexp.QuoteMeta(oldType)

	// Pattern: <item ... href="HREF" ... media-type="OLD" ...> (attributes in any order)
	// We'll find the <item ...> that contains this href
	itemRe := regexp.MustCompile(`<item\s[^>]*href="` + escapedHref + `"[^>]*>`)
	match := itemRe.FindString(opf)
	if match == "" {
		// Try single quotes
		itemRe = regexp.MustCompile(`<item\s[^>]*href='` + escapedHref + `'[^>]*>`)
		match = itemRe.FindString(opf)
	}
	if match == "" {
		return opf
	}

	// Replace media-type within this specific match
	oldAttr := regexp.MustCompile(`media-type=["']` + escapedOld + `["']`)
	newMatch := oldAttr.ReplaceAllString(match, `media-type="`+newType+`"`)
	return strings.Replace(opf, match, newMatch, 1)
}

// fixManifestItemProperties updates the properties attribute for a manifest item by ID.
func fixManifestItemProperties(opf, itemID, oldProps, newProps string) string {
	escapedID := regexp.QuoteMeta(itemID)

	// Find the <item> element with this ID
	itemRe := regexp.MustCompile(`<item\s[^>]*id="` + escapedID + `"[^>]*/?>`)
	match := itemRe.FindString(opf)
	if match == "" {
		itemRe = regexp.MustCompile(`<item\s[^>]*id='` + escapedID + `'[^>]*/?>`)
		match = itemRe.FindString(opf)
	}
	if match == "" {
		return opf
	}

	var newMatch string
	if oldProps == "" {
		// No existing properties attribute — add one before the closing /> or >
		if strings.HasSuffix(match, "/>") {
			newMatch = match[:len(match)-2] + ` properties="` + newProps + `"/>`
		} else {
			newMatch = match[:len(match)-1] + ` properties="` + newProps + `">`
		}
	} else {
		// Replace existing properties value
		escapedOld := regexp.QuoteMeta(oldProps)
		propRe := regexp.MustCompile(`properties=["']` + escapedOld + `["']`)
		newMatch = propRe.ReplaceAllString(match, `properties="`+newProps+`"`)
	}

	return strings.Replace(opf, match, newMatch, 1)
}

func findClosingTag(content, tagName string) int {
	// Try variants: </tagName>, </ns:tagName>, </dc:tagName>
	idx := strings.Index(content, "</"+tagName+">")
	if idx != -1 {
		return idx
	}
	// Try with any namespace prefix
	re := regexp.MustCompile(`</\w+:` + regexp.QuoteMeta(tagName) + `>`)
	loc := re.FindStringIndex(content)
	if loc != nil {
		return loc[0]
	}
	return -1
}

// --- Tier 2 fixes ---

// fixGuideElement removes the <guide> element from EPUB 3 OPF documents.
// Fixes OPF-039.
func fixGuideElement(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil || ep.Package.Version < "3.0" || !ep.Package.HasGuide {
		return nil
	}

	opfData, ok := files[ep.RootfilePath]
	if !ok {
		return nil
	}

	content := string(opfData)
	// Match <guide>...</guide> or <guide.../> including any namespace prefix
	guideRe := regexp.MustCompile(`(?s)\s*<guide\b[^>]*>.*?</guide>`)
	if !guideRe.MatchString(content) {
		// Try self-closing
		guideRe = regexp.MustCompile(`(?s)\s*<guide\b[^/]*/\s*>`)
		if !guideRe.MatchString(content) {
			return nil
		}
	}

	newContent := guideRe.ReplaceAllString(content, "")
	files[ep.RootfilePath] = []byte(newContent)

	return []Fix{{
		CheckID:     "OPF-039",
		Description: "Removed deprecated <guide> element from EPUB 3 package document",
		File:        ep.RootfilePath,
	}}
}

// fixEmptyHref removes empty href="" attributes from <a> elements in XHTML content.
// Fixes HTM-003.
func fixEmptyHref(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil {
		return nil
	}

	var fixes []Fix
	// Match <a ... href="" ...> and remove the href="" part
	emptyHrefRe := regexp.MustCompile(`(<a\b[^>]*?)\s+href\s*=\s*["'](\s*)["']`)

	for _, item := range ep.Package.Manifest {
		if item.MediaType != "application/xhtml+xml" || item.Href == "\x00MISSING" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)
		data, ok := files[fullPath]
		if !ok {
			continue
		}

		content := string(data)
		if !emptyHrefRe.MatchString(content) {
			continue
		}

		// Count fixes before replacing
		matches := emptyHrefRe.FindAllString(content, -1)
		newContent := emptyHrefRe.ReplaceAllString(content, "$1")
		files[fullPath] = []byte(newContent)

		fixes = append(fixes, Fix{
			CheckID:     "HTM-003",
			Description: fmt.Sprintf("Removed %d empty href attribute(s) from <a> elements", len(matches)),
			File:        fullPath,
		})
	}

	return fixes
}

// fixDCDateFormat reformats dc:date values that don't follow W3CDTF.
// Fixes OPF-036.
func fixDCDateFormat(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil || len(ep.Package.Metadata.Dates) == 0 {
		return nil
	}

	opfData, ok := files[ep.RootfilePath]
	if !ok {
		return nil
	}

	w3cdtfRe := regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2}(T\d{2}:\d{2}(:\d{2})?(Z|[+-]\d{2}:\d{2})?)?)?)?$`)

	var fixes []Fix
	content := string(opfData)

	for _, date := range ep.Package.Metadata.Dates {
		if w3cdtfRe.MatchString(date) {
			continue
		}

		reformatted := tryReformatDate(date)
		if reformatted == "" || reformatted == date {
			continue
		}

		// Replace the date value in the OPF
		dateRe := regexp.MustCompile(`(<dc:date[^>]*>)\s*` + regexp.QuoteMeta(date) + `\s*(</dc:date>)`)
		if dateRe.MatchString(content) {
			content = dateRe.ReplaceAllString(content, "${1}"+reformatted+"${2}")
			fixes = append(fixes, Fix{
				CheckID:     "OPF-036",
				Description: fmt.Sprintf("Reformatted dc:date from '%s' to '%s'", date, reformatted),
				File:        ep.RootfilePath,
			})
		}
	}

	if len(fixes) > 0 {
		files[ep.RootfilePath] = []byte(content)
	}

	return fixes
}

// tryReformatDate attempts to parse common non-W3CDTF date formats and
// returns a W3CDTF-compliant string, or "" if unparseable.
func tryReformatDate(s string) string {
	s = strings.TrimSpace(s)

	// Common patterns in real-world EPUBs:
	// "January 1, 2024" / "Jan 1, 2024"
	// "1/15/2024" or "01/15/2024" (US format)
	// "2024/01/15"
	// "2024.01.15"
	// "15 January 2024"

	months := map[string]string{
		"january": "01", "february": "02", "march": "03", "april": "04",
		"may": "05", "june": "06", "july": "07", "august": "08",
		"september": "09", "october": "10", "november": "11", "december": "12",
		"jan": "01", "feb": "02", "mar": "03", "apr": "04",
		"jun": "06", "jul": "07", "aug": "08", "sep": "09",
		"oct": "10", "nov": "11", "dec": "12",
	}

	// "Month Day, Year" or "Month Day Year"
	monthDayYear := regexp.MustCompile(`(?i)^(\w+)\s+(\d{1,2}),?\s+(\d{4})$`)
	if m := monthDayYear.FindStringSubmatch(s); m != nil {
		month, ok := months[strings.ToLower(m[1])]
		if ok {
			return fmt.Sprintf("%s-%s-%02s", m[3], month, zeroPad(m[2]))
		}
	}

	// "Day Month Year" (e.g., "15 January 2024")
	dayMonthYear := regexp.MustCompile(`(?i)^(\d{1,2})\s+(\w+)\s+(\d{4})$`)
	if m := dayMonthYear.FindStringSubmatch(s); m != nil {
		month, ok := months[strings.ToLower(m[2])]
		if ok {
			return fmt.Sprintf("%s-%s-%02s", m[3], month, zeroPad(m[1]))
		}
	}

	// "YYYY/MM/DD" or "YYYY.MM.DD"
	slashDot := regexp.MustCompile(`^(\d{4})[/.](\d{1,2})[/.](\d{1,2})$`)
	if m := slashDot.FindStringSubmatch(s); m != nil {
		return fmt.Sprintf("%s-%02s-%02s", m[1], zeroPad(m[2]), zeroPad(m[3]))
	}

	// "MM/DD/YYYY" (US format) — only if year is 4 digits
	usDate := regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})$`)
	if m := usDate.FindStringSubmatch(s); m != nil {
		return fmt.Sprintf("%s-%02s-%02s", m[3], zeroPad(m[1]), zeroPad(m[2]))
	}

	return ""
}

func zeroPad(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

// fixFilesNotInManifest adds manifest entries for container files not listed in the OPF.
// Fixes RSC-002.
func fixFilesNotInManifest(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil {
		return nil
	}

	manifestPaths := make(map[string]bool)
	manifestIDs := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestPaths[ep.ResolveHref(item.Href)] = true
		}
		if item.ID != "" {
			manifestIDs[item.ID] = true
		}
	}

	// Files that are expected to be outside the manifest
	ignorePaths := map[string]bool{
		"mimetype":                true,
		"META-INF/container.xml":  true,
		"META-INF/encryption.xml": true,
		"META-INF/manifest.xml":   true,
		"META-INF/metadata.xml":   true,
		"META-INF/rights.xml":     true,
		"META-INF/signatures.xml": true,
	}

	opfData, ok := files[ep.RootfilePath]
	if !ok {
		return nil
	}

	var fixes []Fix
	content := string(opfData)

	// Find insertion point: just before </manifest>
	manifestClose := strings.Index(content, "</manifest>")
	if manifestClose == -1 {
		manifestClose = findClosingTag(content, "manifest")
	}
	if manifestClose == -1 {
		return nil
	}

	var insertions []string
	for name := range files {
		if ignorePaths[name] {
			continue
		}
		if strings.HasPrefix(name, "META-INF/") {
			continue
		}
		if name == ep.RootfilePath {
			continue
		}
		if manifestPaths[name] {
			continue
		}

		// Generate a unique ID
		id := generateUniqueID(name, manifestIDs)
		manifestIDs[id] = true

		// Determine relative href from OPF directory
		href := relativeHref(ep.RootfilePath, name)

		// Guess media type from extension
		ext := strings.ToLower(path.Ext(name))
		mediaType := extensionToMediaType(ext)
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}

		insertions = append(insertions, fmt.Sprintf(`    <item id="%s" href="%s" media-type="%s"/>`, id, href, mediaType))
		fixes = append(fixes, Fix{
			CheckID:     "RSC-002",
			Description: fmt.Sprintf("Added '%s' to manifest (id='%s', media-type='%s')", name, id, mediaType),
			File:        ep.RootfilePath,
		})
	}

	if len(insertions) == 0 {
		return nil
	}

	// Sort for deterministic output
	sortStrings(insertions)
	insertion := strings.Join(insertions, "\n") + "\n  "
	newContent := content[:manifestClose] + insertion + content[manifestClose:]
	files[ep.RootfilePath] = []byte(newContent)

	return fixes
}

// generateUniqueID creates a unique manifest item ID based on the filename.
func generateUniqueID(filePath string, existing map[string]bool) string {
	// Use the filename without extension as the base
	base := strings.TrimSuffix(path.Base(filePath), path.Ext(filePath))
	// Sanitize: only allow alphanumeric, hyphens, underscores
	sanitized := regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(base, "_")
	if sanitized == "" {
		sanitized = "item"
	}
	// Ensure it starts with a letter (XML ID requirement)
	if sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "item_" + sanitized
	}

	id := sanitized
	counter := 2
	for existing[id] {
		id = fmt.Sprintf("%s_%d", sanitized, counter)
		counter++
	}
	return id
}

// relativeHref computes the relative path from the OPF file to a target file.
func relativeHref(opfPath, targetPath string) string {
	opfDir := path.Dir(opfPath)
	if opfDir == "." {
		return targetPath
	}
	// If target is under the same directory as OPF, strip the prefix
	if strings.HasPrefix(targetPath, opfDir+"/") {
		return strings.TrimPrefix(targetPath, opfDir+"/")
	}
	// Otherwise, use ../ navigation
	return "../" + targetPath
}

// sortStrings sorts a slice of strings in place (simple insertion sort to avoid importing sort).
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// fixObsoleteElements replaces common obsolete HTML elements with styled equivalents.
// Fixes HTM-004.
func fixObsoleteElements(files map[string][]byte, ep *epub.EPUB) []Fix {
	if ep.Package == nil {
		return nil
	}

	// Define replacements for common obsolete elements
	type replacement struct {
		openTag  string // replacement opening tag
		closeTag string // replacement closing tag
	}

	// These are the safe, common replacements
	replacements := map[string]replacement{
		"center":  {openTag: `<div style="text-align: center;">`, closeTag: "</div>"},
		"big":     {openTag: `<span style="font-size: larger;">`, closeTag: "</span>"},
		"strike":  {openTag: `<span style="text-decoration: line-through;">`, closeTag: "</span>"},
		"tt":      {openTag: `<span style="font-family: monospace;">`, closeTag: "</span>"},
		"acronym": {openTag: "<abbr>", closeTag: "</abbr>"},
		"dir":     {openTag: "<ul>", closeTag: "</ul>"},
	}

	var fixes []Fix

	for _, item := range ep.Package.Manifest {
		if item.MediaType != "application/xhtml+xml" || item.Href == "\x00MISSING" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)
		data, ok := files[fullPath]
		if !ok {
			continue
		}

		content := string(data)
		modified := false
		var replaced []string

		for elemName, repl := range replacements {
			// Match opening tag with optional attributes
			openRe := regexp.MustCompile(`<` + elemName + `(\s[^>]*)?>`)
			closeRe := regexp.MustCompile(`</` + elemName + `\s*>`)

			if !openRe.MatchString(content) {
				continue
			}

			// For elements like <center class="foo">, preserve the style approach
			// but for <acronym title="...">, we want to preserve attributes
			if elemName == "acronym" {
				// Preserve attributes for acronym → abbr
				content = openRe.ReplaceAllString(content, "<abbr${1}>")
			} else if elemName == "dir" {
				content = openRe.ReplaceAllString(content, "<ul${1}>")
			} else {
				// For styled replacements, any existing attributes get merged into the replacement
				content = openRe.ReplaceAllStringFunc(content, func(match string) string {
					// Extract existing attributes
					attrs := openRe.FindStringSubmatch(match)
					if len(attrs) > 1 && strings.TrimSpace(attrs[1]) != "" {
						existingAttrs := strings.TrimSpace(attrs[1])
						// If there's an existing style attribute, merge
						if strings.Contains(existingAttrs, "style=") {
							styleRe := regexp.MustCompile(`style\s*=\s*["']([^"']*)["']`)
							if sm := styleRe.FindStringSubmatch(existingAttrs); sm != nil {
								newStyle := strings.TrimSuffix(strings.TrimSpace(sm[1]), ";")
								// Extract the style from our replacement
								replStyleRe := regexp.MustCompile(`style="([^"]*)"`)
								if rm := replStyleRe.FindStringSubmatch(repl.openTag); rm != nil {
									mergedStyle := newStyle + "; " + rm[1]
									return replStyleRe.ReplaceAllString(repl.openTag, `style="`+mergedStyle+`"`)
								}
							}
						}
					}
					return repl.openTag
				})
			}
			content = closeRe.ReplaceAllString(content, repl.closeTag)
			modified = true
			replaced = append(replaced, elemName)
		}

		if modified {
			files[fullPath] = []byte(content)
			sortStrings(replaced)
			fixes = append(fixes, Fix{
				CheckID:     "HTM-004",
				Description: fmt.Sprintf("Replaced obsolete element(s) %s with modern equivalents", strings.Join(replaced, ", ")),
				File:        fullPath,
			})
		}
	}

	return fixes
}

// navDocHasToc checks whether a navigation document has epub:type="toc".
// Used by doctor mode to scan content features.
func navDocHasToc(data []byte) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
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
					if attr.Name.Local == "type" {
						for _, t := range strings.Fields(attr.Value) {
							if t == "toc" {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}
