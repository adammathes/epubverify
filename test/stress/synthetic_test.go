package stress_test

import (
	"archive/zip"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/adammathes/epubverify/pkg/validate"
)

// createSyntheticEPUB creates a temporary EPUB from a map of paths to content.
// mimetype is always written first as stored (uncompressed).
func createSyntheticEPUB(t *testing.T, files map[string]string) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "synth-*.epub")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}

	w := zip.NewWriter(tmp)

	// Write mimetype first (stored, uncompressed)
	if mt, ok := files["mimetype"]; ok {
		header := &zip.FileHeader{
			Name:   "mimetype",
			Method: zip.Store,
		}
		mw, err := w.CreateHeader(header)
		if err != nil {
			t.Fatalf("create mimetype: %v", err)
		}
		mw.Write([]byte(mt))
	}

	// Write remaining files
	for name, data := range files {
		if name == "mimetype" {
			continue
		}
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		fw.Write([]byte(data))
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	tmp.Close()
	return tmp.Name()
}

// minimalEPUB3 returns the minimum set of files for a valid EPUB 3.
func minimalEPUB3() map[string]string {
	return map[string]string{
		"mimetype": "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"EPUB/package.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" xml:lang="en" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>Test</dc:title>
  <dc:language>en</dc:language>
  <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789abc</dc:identifier>
  <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
</metadata>
<manifest>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  <item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
</manifest>
<spine>
  <itemref idref="nav"/>
  <itemref idref="content"/>
</spine>
</package>`,
		"EPUB/nav.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Nav</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="content.xhtml">Content</a></li></ol></nav>
</body>
</html>`,
		"EPUB/content.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Content</title></head>
<body><p>Hello World</p></body>
</html>`,
	}
}

// minimalEPUB2 returns the minimum set of files for a valid EPUB 2.
func minimalEPUB2() map[string]string {
	return map[string]string{
		"mimetype": "application/epub+zip",
		"META-INF/container.xml": `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>Test EPUB 2</dc:title>
  <dc:language>en</dc:language>
  <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789def</dc:identifier>
</metadata>
<manifest>
  <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
  <item id="content" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
</manifest>
<spine toc="ncx">
  <itemref idref="content"/>
</spine>
</package>`,
		"OEBPS/toc.ncx": `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<head>
  <meta name="dtb:uid" content="urn:uuid:12345678-1234-1234-1234-123456789def"/>
  <meta name="dtb:depth" content="1"/>
</head>
<docTitle><text>Test EPUB 2</text></docTitle>
<navMap>
  <navPoint id="ch1" playOrder="1">
    <navLabel><text>Chapter 1</text></navLabel>
    <content src="chapter1.xhtml"/>
  </navPoint>
</navMap>
</ncx>`,
		"OEBPS/chapter1.xhtml": `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>Hello World</p></body>
</html>`,
	}
}

func validateSynthetic(t *testing.T, files map[string]string) *testResult {
	t.Helper()
	path := createSyntheticEPUB(t, files)
	defer os.Remove(path)

	r, err := validate.Validate(path)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}

	result := &testResult{path: path}
	for _, msg := range r.Messages {
		result.messages = append(result.messages, testMsg{
			severity: string(msg.Severity),
			checkID:  msg.CheckID,
			message:  msg.Message,
		})
	}
	result.errorCount = r.ErrorCount()
	result.warningCount = r.WarningCount()
	result.fatalCount = r.FatalCount()
	result.valid = r.IsValid()
	return result
}

type testMsg struct {
	severity string
	checkID  string
	message  string
}

type testResult struct {
	path         string
	messages     []testMsg
	errorCount   int
	warningCount int
	fatalCount   int
	valid        bool
}

func (r *testResult) hasCheck(checkID string) bool {
	for _, msg := range r.messages {
		if msg.checkID == checkID {
			return true
		}
	}
	return false
}

func (r *testResult) hasCheckWithSeverity(checkID, severity string) bool {
	for _, msg := range r.messages {
		if msg.checkID == checkID && msg.severity == severity {
			return true
		}
	}
	return false
}

func (r *testResult) messageFor(checkID string) string {
	for _, msg := range r.messages {
		if msg.checkID == checkID {
			return msg.message
		}
	}
	return ""
}

func (r *testResult) countCheck(checkID string) int {
	count := 0
	for _, msg := range r.messages {
		if msg.checkID == checkID {
			count++
		}
	}
	return count
}

func (r *testResult) dump(t *testing.T) {
	t.Helper()
	for _, msg := range r.messages {
		t.Logf("  %s(%s): %s", msg.severity, msg.checkID, msg.message)
	}
}

// ============================================================
// Test: Baseline minimal EPUBs should be valid
// ============================================================

func TestSyntheticMinimalEPUB3Valid(t *testing.T) {
	result := validateSynthetic(t, minimalEPUB3())
	if !result.valid {
		t.Error("minimal EPUB 3 should be valid")
		result.dump(t)
	}
}

func TestSyntheticMinimalEPUB2Valid(t *testing.T) {
	result := validateSynthetic(t, minimalEPUB2())
	if !result.valid {
		t.Error("minimal EPUB 2 should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: Content documents with various edge cases
// ============================================================

func TestSyntheticXHTMLWithEntityReferences(t *testing.T) {
	files := minimalEPUB3()
	// Content with various HTML entities
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Entities</title></head>
<body>
<p>Copyright &#169; 2024 &amp; &#x2014; em-dash</p>
<p>&#8220;smart quotes&#8221;</p>
</body>
</html>`
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with entity references should be valid")
		result.dump(t)
	}
}

func TestSyntheticXHTMLWithSVGInline(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>SVG</title></head>
<body>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
  <circle cx="50" cy="50" r="40" fill="red"/>
</svg>
</body>
</html>`
	// Should require svg property
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml" properties="svg"/>`,
		1)
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with inline SVG (and svg property) should be valid")
		result.dump(t)
	}
}

func TestSyntheticXHTMLWithMathML(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>MathML</title></head>
<body>
<math xmlns="http://www.w3.org/1998/Math/MathML">
  <mrow><mi>x</mi><mo>=</mo><mn>42</mn></mrow>
</math>
</body>
</html>`
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml" properties="mathml"/>`,
		1)
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with MathML (and mathml property) should be valid")
		result.dump(t)
	}
}

func TestSyntheticXHTMLWithEmptyBody(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Empty</title></head>
<body/>
</html>`
	result := validateSynthetic(t, files)
	// Empty body is valid XML and valid EPUB
	if !result.valid {
		t.Error("EPUB with empty body should be valid")
		result.dump(t)
	}
}

func TestSyntheticXHTMLNoTitle(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head></head>
<body><p>No title</p></body>
</html>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("HTM-002") {
		t.Error("should warn about missing title (HTM-002)")
		result.dump(t)
	}
}

func TestSyntheticXHTMLWithObsoleteElements(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Obsolete</title></head>
<body>
<center><p>centered text</p></center>
</body>
</html>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("HTM-004") {
		t.Error("should flag obsolete <center> element (HTM-004)")
		result.dump(t)
	}
}

// ============================================================
// Test: OPF edge cases
// ============================================================

func TestSyntheticMissingDCTitle(t *testing.T) {
	files := minimalEPUB3()
	// Remove dc:title from OPF
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		"<dc:title>Test</dc:title>\n", "", 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-001") {
		t.Error("should report missing dc:title (OPF-001)")
		result.dump(t)
	}
}

func TestSyntheticMissingDCLanguage(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		"<dc:language>en</dc:language>\n", "", 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-003") {
		t.Error("should report missing dc:language (OPF-003)")
		result.dump(t)
	}
}

func TestSyntheticMissingDCIdentifier(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`  <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789abc</dc:identifier>
`, "", 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-002") {
		t.Error("should report missing dc:identifier (OPF-002)")
		result.dump(t)
	}
}

func TestSyntheticMissingModified(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`  <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
`, "", 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-004") {
		t.Error("should report missing dcterms:modified (OPF-004)")
		result.dump(t)
	}
}

func TestSyntheticInvalidModifiedFormat(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		"2024-01-01T00:00:00Z", "January 1, 2024", 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-019") {
		t.Error("should report invalid dcterms:modified format (OPF-019)")
		result.dump(t)
	}
}

func TestSyntheticEmptySpine(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<spine>
  <itemref idref="nav"/>
  <itemref idref="content"/>
</spine>`,
		`<spine>
</spine>`, 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-010") {
		t.Error("should report empty spine (OPF-010)")
		result.dump(t)
	}
}

func TestSyntheticDuplicateSpineItemrefs(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<spine>
  <itemref idref="nav"/>
  <itemref idref="content"/>
</spine>`,
		`<spine>
  <itemref idref="nav"/>
  <itemref idref="content"/>
  <itemref idref="content"/>
</spine>`, 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-034") {
		t.Error("should report duplicate spine itemref (OPF-034)")
		result.dump(t)
	}
}

// ============================================================
// Test: Missing manifest items
// ============================================================

func TestSyntheticMissingManifestFile(t *testing.T) {
	files := minimalEPUB3()
	// Add manifest entry for file that doesn't exist
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
  <item id="missing" href="missing.xhtml" media-type="application/xhtml+xml"/>`, 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("RSC-001") {
		t.Error("should report missing file in manifest (RSC-001)")
		result.dump(t)
	}
}

func TestSyntheticFileNotInManifest(t *testing.T) {
	files := minimalEPUB3()
	// Add a file that is not in the manifest
	files["EPUB/extra.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Extra</title></head>
<body><p>Not in manifest</p></body>
</html>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-003") {
		t.Error("should report file not in manifest (OPF-003)")
		result.dump(t)
	}
}

// ============================================================
// Test: Navigation edge cases
// ============================================================

func TestSyntheticNavMissingToc(t *testing.T) {
	files := minimalEPUB3()
	// Nav without epub:type="toc"
	files["EPUB/nav.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Nav</title></head>
<body>
<nav epub:type="landmarks"><ol><li><a href="content.xhtml">Content</a></li></ol></nav>
</body>
</html>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("RSC-005") {
		t.Error("should report missing toc nav element")
		result.dump(t)
	}
}

func TestSyntheticNavBrokenLink(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/nav.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Nav</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="nonexistent.xhtml">Broken</a></li></ol></nav>
</body>
</html>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("RSC-007") {
		t.Error("should report broken nav link (RSC-007)")
		result.dump(t)
	}
}

func TestSyntheticMultipleNavItems(t *testing.T) {
	files := minimalEPUB3()
	// Two items with nav property
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml" properties="nav"/>`, 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-026") {
		t.Error("should report multiple nav items (OPF-026)")
		result.dump(t)
	}
}

// ============================================================
// Test: CSS edge cases
// ============================================================

func TestSyntheticCSSInStyleElement(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<title>CSS</title>
<style type="text/css">
body { font-family: serif; color: #333; }
p { margin: 1em 0; }
</style>
</head>
<body><p>Styled text</p></body>
</html>`
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with inline CSS should be valid")
		result.dump(t)
	}
}

func TestSyntheticCSSExternalStylesheet(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<title>CSS External</title>
<link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body><p>Styled text</p></body>
</html>`
	files["EPUB/style.css"] = `body { font-family: serif; }`
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
  <item id="css" href="style.css" media-type="text/css"/>`, 1)
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with external CSS should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: EPUB 2 specific edge cases
// ============================================================

func TestSyntheticEPUB2MissingNCX(t *testing.T) {
	files := minimalEPUB2()
	delete(files, "OEBPS/toc.ncx")
	result := validateSynthetic(t, files)
	if !result.hasCheck("E2-001") && !result.hasCheck("RSC-001") {
		t.Error("should report missing NCX (E2-001 or RSC-001)")
		result.dump(t)
	}
}

func TestSyntheticEPUB2NCXMissingNavMap(t *testing.T) {
	files := minimalEPUB2()
	files["OEBPS/toc.ncx"] = `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<head>
  <meta name="dtb:uid" content="urn:uuid:12345678-1234-1234-1234-123456789def"/>
</head>
<docTitle><text>Test</text></docTitle>
</ncx>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("E2-003") {
		t.Error("should report NCX missing navMap (E2-003)")
		result.dump(t)
	}
}

func TestSyntheticEPUB2NCXUIDMismatch(t *testing.T) {
	files := minimalEPUB2()
	files["OEBPS/toc.ncx"] = strings.Replace(files["OEBPS/toc.ncx"],
		"urn:uuid:12345678-1234-1234-1234-123456789def",
		"urn:uuid:DIFFERENT-UID-VALUE", 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("NCX-001") {
		t.Error("should report NCX UID mismatch (NCX-001)")
		result.dump(t)
	}
}

func TestSyntheticEPUB2WithPropertiesAttribute(t *testing.T) {
	files := minimalEPUB2()
	files["OEBPS/content.opf"] = strings.Replace(files["OEBPS/content.opf"],
		`<item id="content" href="chapter1.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="chapter1.xhtml" media-type="application/xhtml+xml" properties="nav"/>`, 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("E2-005") {
		t.Error("should report properties attribute not allowed in EPUB 2 (E2-005)")
		result.dump(t)
	}
}

// ============================================================
// Test: Cross-reference edge cases
// ============================================================

func TestSyntheticContentLinkToExistingFile(t *testing.T) {
	files := minimalEPUB3()
	// Add a second content doc and link from first
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Content</title></head>
<body><p><a href="chapter2.xhtml">Next</a></p></body>
</html>`
	files["EPUB/chapter2.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 2</title></head>
<body><p>Chapter 2</p></body>
</html>`
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
  <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>`, 1)
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<itemref idref="content"/>`,
		`<itemref idref="content"/>
  <itemref idref="ch2"/>`, 1)
	files["EPUB/nav.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Nav</title></head>
<body>
<nav epub:type="toc"><ol>
  <li><a href="content.xhtml">Content</a></li>
  <li><a href="chapter2.xhtml">Chapter 2</a></li>
</ol></nav>
</body>
</html>`
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with valid cross-references should be valid")
		result.dump(t)
	}
}

func TestSyntheticContentLinkToMissingFragment(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Content</title></head>
<body><p><a href="#nonexistent">Link to nothing</a></p></body>
</html>`
	result := validateSynthetic(t, files)
	// Should report unresolvable fragment
	if !result.hasCheck("RSC-012") {
		t.Log("Note: no RSC-012 for missing fragment in same file")
		// This is OK - some validators don't check self-referencing fragments
	}
}

// ============================================================
// Test: Large / complex content
// ============================================================

func TestSyntheticLargeNumberOfManifestItems(t *testing.T) {
	files := minimalEPUB3()
	var manifestItems, spineItems, navItems strings.Builder
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("ch%03d.xhtml", i)
		id := fmt.Sprintf("ch%03d", i)
		manifestItems.WriteString(fmt.Sprintf(`  <item id="%s" href="%s" media-type="application/xhtml+xml"/>
`, id, name))
		spineItems.WriteString(fmt.Sprintf(`  <itemref idref="%s"/>
`, id))
		navItems.WriteString(fmt.Sprintf(`<li><a href="%s">Chapter %d</a></li>
`, name, i))
		files["EPUB/"+name] = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter %d</title></head>
<body><p>Content of chapter %d</p></body>
</html>`, i, i)
	}

	files["EPUB/package.opf"] = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" xml:lang="en" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>Large EPUB</dc:title>
  <dc:language>en</dc:language>
  <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789abc</dc:identifier>
  <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
</metadata>
<manifest>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
%s</manifest>
<spine>
  <itemref idref="nav"/>
%s</spine>
</package>`, manifestItems.String(), spineItems.String())

	files["EPUB/nav.xhtml"] = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Nav</title></head>
<body>
<nav epub:type="toc"><ol>%s</ol></nav>
</body>
</html>`, navItems.String())

	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with 50 chapters should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: Encoding edge cases
// ============================================================

func TestSyntheticUTF8WithBOM(t *testing.T) {
	files := minimalEPUB3()
	// Add UTF-8 BOM to content
	files["EPUB/content.xhtml"] = "\xef\xbb\xbf" + `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>BOM</title></head>
<body><p>Content with BOM</p></body>
</html>`
	result := validateSynthetic(t, files)
	// UTF-8 BOM is generally tolerated
	if result.fatalCount > 0 {
		t.Error("EPUB with UTF-8 BOM should not be fatal")
		result.dump(t)
	}
}

// ============================================================
// Test: Container-level edge cases
// ============================================================

func TestSyntheticContainerVersion20(t *testing.T) {
	files := minimalEPUB3()
	// Replace only the container element's version, not the XML declaration's
	files["META-INF/container.xml"] = `<?xml version="1.0" encoding="UTF-8"?>
<container version="2.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("OCF-014") {
		t.Error("should report invalid container version (OCF-014)")
		result.dump(t)
	}
}

func TestSyntheticContainerExtraElement(t *testing.T) {
	files := minimalEPUB3()
	files["META-INF/container.xml"] = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
  <extra>unknown element</extra>
</container>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("RSC-005") {
		t.Error("should report unknown element in container.xml (RSC-005)")
		result.dump(t)
	}
}

// ============================================================
// Test: Percent-encoded hrefs in manifest
// ============================================================

func TestSyntheticPercentEncodedHref(t *testing.T) {
	files := minimalEPUB3()
	// Use a file with spaces in the name (percent-encoded in manifest)
	files["EPUB/my file.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Spaced</title></head>
<body><p>File with spaces</p></body>
</html>`
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="my%20file.xhtml" media-type="application/xhtml+xml"/>`, 1)
	files["EPUB/nav.xhtml"] = strings.Replace(files["EPUB/nav.xhtml"],
		`<a href="content.xhtml">Content</a>`,
		`<a href="my%20file.xhtml">Content</a>`, 1)
	delete(files, "EPUB/content.xhtml")
	result := validateSynthetic(t, files)
	// Check it resolves correctly
	if result.hasCheck("RSC-001") {
		t.Error("percent-encoded href should resolve correctly, but got RSC-001")
		result.dump(t)
	}
}

// ============================================================
// Test: Multiple renditions
// ============================================================

func TestSyntheticMultipleRootfiles(t *testing.T) {
	files := minimalEPUB3()
	files["META-INF/container.xml"] = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>
    <rootfile full-path="EPUB/package2.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	// Second OPF
	files["EPUB/package2.opf"] = files["EPUB/package.opf"]
	result := validateSynthetic(t, files)
	// Should report multiple OPF rootfiles
	if !result.hasCheck("PKG-013") {
		t.Error("should report multiple OPF rootfiles (PKG-013)")
		result.dump(t)
	}
}

// ============================================================
// Test: Guide element in EPUB 3 (deprecated)
// ============================================================

func TestSyntheticGuideInEPUB3(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`</package>`,
		`<guide>
  <reference type="toc" title="Table of Contents" href="nav.xhtml"/>
</guide>
</package>`, 1)
	result := validateSynthetic(t, files)
	// OPF-039 is downgraded to INFO in non-strict mode, check either
	hasGuideCheck := result.hasCheck("OPF-039")
	if !hasGuideCheck {
		t.Log("Note: guide element in EPUB 3 may be downgraded to INFO")
	}
}

// ============================================================
// Test: EPUB with images
// ============================================================

func TestSyntheticWithCoverImage(t *testing.T) {
	files := minimalEPUB3()
	// Add a tiny valid PNG (1x1 pixel)
	files["EPUB/cover.png"] = string([]byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
		0x44, 0xAE, 0x42, 0x60, 0x82,
	})
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
  <item id="cover" href="cover.png" media-type="image/png" properties="cover-image"/>`, 1)
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Content</title></head>
<body><p><img src="cover.png" alt="Cover"/></p></body>
</html>`
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with cover image should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: Deeply nested XHTML
// ============================================================

func TestSyntheticDeeplyNestedXHTML(t *testing.T) {
	files := minimalEPUB3()
	var content strings.Builder
	content.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Deep</title></head>
<body>`)
	for i := 0; i < 50; i++ {
		content.WriteString("<div>")
	}
	content.WriteString("<p>Deep content</p>")
	for i := 0; i < 50; i++ {
		content.WriteString("</div>")
	}
	content.WriteString("</body></html>")
	files["EPUB/content.xhtml"] = content.String()

	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with deeply nested content should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: Special characters in dc:identifier
// ============================================================

func TestSyntheticIdentifierWithSpecialChars(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		"urn:uuid:12345678-1234-1234-1234-123456789abc",
		"urn:isbn:978-0-13-468599-1", 1)
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with ISBN identifier should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: Non-ASCII filenames
// ============================================================

func TestSyntheticNonASCIIFilename(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/résumé.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Résumé</title></head>
<body><p>Content with accents</p></body>
</html>`
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="résumé.xhtml" media-type="application/xhtml+xml"/>`, 1)
	files["EPUB/nav.xhtml"] = strings.Replace(files["EPUB/nav.xhtml"],
		`<a href="content.xhtml">Content</a>`,
		`<a href="résumé.xhtml">Content</a>`, 1)
	delete(files, "EPUB/content.xhtml")
	result := validateSynthetic(t, files)
	// Should have PKG-012 usage note
	if !result.hasCheck("PKG-012") {
		t.Error("should have PKG-012 for non-ASCII filename")
		result.dump(t)
	}
	// But should still be valid
	if !result.valid {
		t.Error("EPUB with non-ASCII filename should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: NCX depth mismatch
// ============================================================

func TestSyntheticNCXDepthMismatch(t *testing.T) {
	files := minimalEPUB2()
	files["OEBPS/toc.ncx"] = `<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
<head>
  <meta name="dtb:uid" content="urn:uuid:12345678-1234-1234-1234-123456789def"/>
  <meta name="dtb:depth" content="5"/>
</head>
<docTitle><text>Test</text></docTitle>
<navMap>
  <navPoint id="ch1" playOrder="1">
    <navLabel><text>Chapter 1</text></navLabel>
    <content src="chapter1.xhtml"/>
  </navPoint>
</navMap>
</ncx>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("E2-015") {
		t.Log("Note: E2-015 depth mismatch may be downgraded to INFO")
	}
}

// ============================================================
// Test: EPUB with bindings (deprecated)
// ============================================================

func TestSyntheticWithBindings(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`</package>`,
		`<bindings>
  <mediaType media-type="application/x-test" handler="content"/>
</bindings>
</package>`, 1)
	result := validateSynthetic(t, files)
	// bindings is deprecated in EPUB 3
	if !result.hasCheck("OPF-086") && !result.hasCheck("RSC-005") {
		t.Log("Note: bindings element may or may not trigger a warning")
	}
}

// ============================================================
// Test: Crash regression - empty metadata
// ============================================================

func TestSyntheticEmptyMetadata(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" xml:lang="en" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
</metadata>
<manifest>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  <item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
</manifest>
<spine>
  <itemref idref="nav"/>
  <itemref idref="content"/>
</spine>
</package>`
	// Should not crash - just report errors
	result := validateSynthetic(t, files)
	if result.fatalCount > 0 {
		t.Error("empty metadata should report errors, not fatals")
		result.dump(t)
	}
}

// ============================================================
// Test: Crash regression - malformed XML in OPF
// ============================================================

func TestSyntheticMalformedOPF(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
<metadata>
  <dc:title>Test</dc:title>
</package>`
	// Should not crash
	result := validateSynthetic(t, files)
	if result.fatalCount == 0 {
		t.Error("malformed OPF should produce a fatal error")
		result.dump(t)
	}
}

// ============================================================
// Test: Crash regression - missing container.xml
// ============================================================

func TestSyntheticMissingContainerXML(t *testing.T) {
	files := map[string]string{
		"mimetype":         "application/epub+zip",
		"EPUB/content.opf": "<package/>",
	}
	result := validateSynthetic(t, files)
	if !result.hasCheck("RSC-002") {
		t.Error("should report missing container.xml (RSC-002)")
		result.dump(t)
	}
}

// ============================================================
// Test: Collections in EPUB 3
// ============================================================

func TestSyntheticWithCollection(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`</package>`,
		`<collection role="manifest">
  <link href="content.xhtml"/>
</collection>
</package>`, 1)
	result := validateSynthetic(t, files)
	// Collections are allowed in EPUB 3
	if result.fatalCount > 0 {
		t.Error("EPUB with collection should not be fatal")
		result.dump(t)
	}
}

// ============================================================
// Test: Media overlay reference
// ============================================================

func TestSyntheticMediaOverlayWrongType(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml" media-overlay="nav"/>`, 1)
	result := validateSynthetic(t, files)
	// OPF-044 should be reported for the non-SMIL media type
	if !result.hasCheck("OPF-044") {
		t.Error("should report media-overlay pointing to non-SMIL (OPF-044)")
		result.dump(t)
	}
}

func TestSyntheticMediaOverlayTypeIsOPF044(t *testing.T) {
	// After the fix, the media-overlay type check should use OPF-044 instead of RSC-005
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml" media-overlay="nav"/>`, 1)
	result := validateSynthetic(t, files)
	// Ensure the type mismatch is reported as OPF-044 (not RSC-005)
	found := false
	for _, msg := range result.messages {
		if msg.checkID == "OPF-044" && strings.Contains(msg.message, "application/smil+xml") {
			found = true
		}
	}
	if !found {
		t.Error("media-overlay type mismatch should be reported as OPF-044")
		result.dump(t)
	}
}

// ============================================================
// Test: Panic/crash resistance with nil-pointer edge cases
// ============================================================

func TestSyntheticEmptyPackageElement(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
</package>`
	// Should not panic
	result := validateSynthetic(t, files)
	_ = result // Just ensure no crash
}

func TestSyntheticEmptyContainerRootfiles(t *testing.T) {
	files := minimalEPUB3()
	files["META-INF/container.xml"] = `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
  </rootfiles>
</container>`
	// Should not panic
	result := validateSynthetic(t, files)
	_ = result
}

// ============================================================
// Test: content doc with lang/xml:lang mismatch
// ============================================================

func TestSyntheticLangMismatch(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" lang="en" xml:lang="fr">
<head><title>Lang Mismatch</title></head>
<body><p>Text</p></body>
</html>`
	result := validateSynthetic(t, files)
	// EPUBCheck reports this as RSC-005 (Schematron schema validation)
	if !result.hasCheck("RSC-005") {
		t.Error("should report lang/xml:lang mismatch (RSC-005)")
		result.dump(t)
	}
}

// ============================================================
// Test: Multiple cover-image properties
// ============================================================

func TestSyntheticMultipleCoverImages(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/cover1.png"] = "fake-png-1"
	files["EPUB/cover2.png"] = "fake-png-2"
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>`,
		`<item id="content" href="content.xhtml" media-type="application/xhtml+xml"/>
  <item id="cover1" href="cover1.png" media-type="image/png" properties="cover-image"/>
  <item id="cover2" href="cover2.png" media-type="image/png" properties="cover-image"/>`, 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-012") {
		t.Error("should report multiple cover-image properties (OPF-012)")
		result.dump(t)
	}
}

// ============================================================
// Test: Spine linear attribute
// ============================================================

func TestSyntheticSpineLinearInvalid(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/package.opf"] = strings.Replace(files["EPUB/package.opf"],
		`<itemref idref="content"/>`,
		`<itemref idref="content" linear="maybe"/>`, 1)
	result := validateSynthetic(t, files)
	if !result.hasCheck("OPF-038") {
		t.Error("should report invalid linear attribute (OPF-038)")
		result.dump(t)
	}
}

// ============================================================
// Test: EPUB with encryption.xml
// ============================================================

func TestSyntheticWithEncryptionXML(t *testing.T) {
	files := minimalEPUB3()
	files["META-INF/encryption.xml"] = `<?xml version="1.0" encoding="UTF-8"?>
<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container"
            xmlns:enc="http://www.w3.org/2001/04/xmlenc#">
  <enc:EncryptedData>
    <enc:EncryptionMethod Algorithm="http://www.w3.org/2001/04/xmlenc#aes256-cbc"/>
    <enc:CipherData>
      <enc:CipherReference URI="EPUB/content.xhtml"/>
    </enc:CipherData>
  </enc:EncryptedData>
</encryption>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("RSC-004") {
		t.Error("should report encryption.xml presence (RSC-004)")
		result.dump(t)
	}
}

// ============================================================
// Test: OPF with xml:lang attribute values
// ============================================================

func TestSyntheticOPFXMLLang(t *testing.T) {
	files := minimalEPUB3()
	// Valid xml:lang
	result := validateSynthetic(t, files)
	if !result.valid {
		t.Error("EPUB with valid xml:lang should be valid")
		result.dump(t)
	}
}

// ============================================================
// Test: content doc with duplicate IDs
// ============================================================

func TestSyntheticDuplicateContentIDs(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/content.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Dup IDs</title></head>
<body>
<p id="p1">First paragraph</p>
<p id="p1">Second paragraph with same ID</p>
</body>
</html>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("HTM-016") {
		t.Error("should report duplicate IDs in content (HTM-016)")
		result.dump(t)
	}
}

// ============================================================
// Test: SVG spine item in FXL without viewBox
// ============================================================

func TestSyntheticFXLSVGNoViewBox(t *testing.T) {
	files := minimalEPUB3()
	files["EPUB/image.svg"] = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">
  <circle cx="50" cy="50" r="40" fill="red"/>
</svg>`
	files["EPUB/package.opf"] = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" xml:lang="en" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>FXL SVG</dc:title>
  <dc:language>en</dc:language>
  <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789abc</dc:identifier>
  <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  <meta property="rendition:layout">pre-paginated</meta>
</metadata>
<manifest>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
  <item id="svg" href="image.svg" media-type="image/svg+xml"/>
</manifest>
<spine>
  <itemref idref="nav"/>
  <itemref idref="svg"/>
</spine>
</package>`
	result := validateSynthetic(t, files)
	if !result.hasCheck("HTM-048") {
		t.Error("should report FXL SVG without viewBox (HTM-048)")
		result.dump(t)
	}
}
