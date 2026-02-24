package doctor

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/validate"
)

// createTestEPUB builds a minimal EPUB 3 in a temp file and returns its path.
// The options allow injecting specific problems for doctor to fix.
type epubOpts struct {
	mimetypeContent  string // empty = correct
	mimetypeMethod   uint16 // 0 = Store
	mimetypeFirst    bool   // true = mimetype is first entry
	version          string // "3.0" or "2.0"
	includeDCModified bool
	doctype          string // empty = HTML5, "xhtml" = XHTML 1.1 doctype
	includeScript    bool   // add <script> to content but not property
	wrongMediaType   string // if non-empty, set this as media-type for the cover image
	// Tier 2 options
	includeGuide     bool   // add deprecated <guide> element
	emptyHref        bool   // add <a href=""> to content
	badDate          string // bad dc:date value (non-W3CDTF)
	extraFile        bool   // add a file not in manifest
	obsoleteElements bool   // add obsolete HTML elements (center, big, etc.)
}

func defaultOpts() epubOpts {
	return epubOpts{
		mimetypeContent:  "application/epub+zip",
		mimetypeMethod:   zip.Store,
		mimetypeFirst:    true,
		version:          "3.0",
		includeDCModified: true,
		doctype:          "",
	}
}

func createTestEPUB(t *testing.T, opts epubOpts) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.epub")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	mimetypeContent := opts.mimetypeContent
	if mimetypeContent == "" {
		mimetypeContent = "application/epub+zip"
	}

	// Write mimetype
	writeMimetype := func() {
		header := &zip.FileHeader{
			Name:   "mimetype",
			Method: opts.mimetypeMethod,
		}
		mw, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		mw.Write([]byte(mimetypeContent))
	}

	writeContainer := func() {
		cw, _ := w.Create("META-INF/container.xml")
		cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))
	}

	writeOPF := func() {
		modified := ""
		if opts.includeDCModified {
			modified = `    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>` + "\n"
		}

		coverItem := ""
		if opts.wrongMediaType != "" {
			coverItem = `    <item id="cover" href="cover.jpg" media-type="` + opts.wrongMediaType + `"/>` + "\n"
		}

		dateElem := ""
		if opts.badDate != "" {
			dateElem = `    <dc:date>` + opts.badDate + `</dc:date>` + "\n"
		}

		guideSection := ""
		if opts.includeGuide {
			guideSection = `
  <guide>
    <reference type="cover" title="Cover" href="chapter1.xhtml"/>
  </guide>`
		}

		cw, _ := w.Create("OEBPS/content.opf")
		cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="` + opts.version + `" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test Book</dc:title>
    <dc:language>en</dc:language>
` + dateElem + modified + `  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
` + coverItem + `  </manifest>
  <spine>
    <itemref idref="ch1"/>
  </spine>` + guideSection + `
</package>`))
	}

	writeNav := func() {
		cw, _ := w.Create("OEBPS/nav.xhtml")
		cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter 1</a></li></ol></nav>
</body>
</html>`))
	}

	writeChapter := func() {
		doctype := "<!DOCTYPE html>"
		if opts.doctype == "xhtml" {
			doctype = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">`
		}

		scriptTag := ""
		if opts.includeScript {
			scriptTag = `<script>console.log("test");</script>` + "\n"
		}

		emptyHrefTag := ""
		if opts.emptyHref {
			emptyHrefTag = `<a href="">click here</a>` + "\n"
		}

		obsoleteTags := ""
		if opts.obsoleteElements {
			obsoleteTags = `<center>Centered text</center>
<big>Big text</big>
<strike>Struck text</strike>
<tt>Monospace text</tt>
`
		}

		cw, _ := w.Create("OEBPS/chapter1.xhtml")
		cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
` + doctype + `
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body>
` + scriptTag + emptyHrefTag + obsoleteTags + `<p>Hello world</p>
</body>
</html>`))
	}

	writeCover := func() {
		// Write a valid JPEG file (just the magic bytes + minimal data)
		cw, _ := w.Create("OEBPS/cover.jpg")
		cw.Write([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00})
	}

	if opts.mimetypeFirst {
		writeMimetype()
		writeContainer()
	} else {
		writeContainer()
		writeMimetype()
	}

	writeOPF()
	writeNav()
	writeChapter()

	if opts.wrongMediaType != "" {
		writeCover()
	}

	if opts.extraFile {
		// Write a CSS file that's not in the manifest
		ew, _ := w.Create("OEBPS/extra-style.css")
		ew.Write([]byte(`body { margin: 1em; }`))
	}

	w.Close()
	f.Close()

	return path
}

func TestDoctorFixesMimetypeContent(t *testing.T) {
	opts := defaultOpts()
	opts.mimetypeContent = "wrong/type"
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	if len(result.Fixes) == 0 {
		t.Fatal("Expected fixes but got none")
	}

	foundMimeFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "PKG-007" {
			foundMimeFix = true
			break
		}
	}
	if !foundMimeFix {
		t.Error("Expected PKG-007 fix for mimetype content")
	}

	// Verify the output EPUB has correct mimetype
	zr, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	if zr.File[0].Name != "mimetype" {
		t.Errorf("mimetype is not first entry, got %s", zr.File[0].Name)
	}
	if zr.File[0].Method != zip.Store {
		t.Errorf("mimetype should be stored, got method %d", zr.File[0].Method)
	}
}

func TestDoctorFixesMissingDCModified(t *testing.T) {
	opts := defaultOpts()
	opts.includeDCModified = false
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundModFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-004" {
			foundModFix = true
			break
		}
	}
	if !foundModFix {
		t.Error("Expected OPF-004 fix for missing dcterms:modified")
	}

	// Verify the output no longer has the OPF-004 error
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "OPF-004" {
			t.Errorf("OPF-004 still present after fix: %s", msg.Message)
		}
	}
}

func TestDoctorFixesDoctype(t *testing.T) {
	opts := defaultOpts()
	opts.doctype = "xhtml"
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundDTFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "HTM-010" {
			foundDTFix = true
			break
		}
	}
	if !foundDTFix {
		t.Error("Expected HTM-010 fix for XHTML DOCTYPE")
	}

	// Verify the output content has HTML5 DOCTYPE
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/chapter1.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("<!DOCTYPE html>")) {
		t.Error("Expected HTML5 DOCTYPE in output")
	}
	if bytes.Contains(data, []byte("XHTML")) {
		t.Error("XHTML DOCTYPE should have been removed")
	}
}

func TestDoctorFixesMissingScriptedProperty(t *testing.T) {
	opts := defaultOpts()
	opts.includeScript = true
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundScriptFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-014" {
			foundScriptFix = true
			break
		}
	}
	if !foundScriptFix {
		t.Error("Expected OPF-014 fix for missing 'scripted' property")
	}

	// Verify the property was added in the output
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "OPF-014" && strings.Contains(msg.Message, "scripted") {
			t.Errorf("OPF-014 (scripted) still present after fix: %s", msg.Message)
		}
	}
}

func TestDoctorFixesMediaTypeMismatch(t *testing.T) {
	opts := defaultOpts()
	opts.wrongMediaType = "image/png" // file is actually JPEG
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundMediaFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-024" {
			foundMediaFix = true
			if !strings.Contains(fix.Description, "image/jpeg") {
				t.Errorf("Expected fix to correct to image/jpeg, got: %s", fix.Description)
			}
			break
		}
	}
	if !foundMediaFix {
		t.Error("Expected OPF-024 fix for media-type mismatch")
	}
}

func TestDoctorNoFixesOnValidEPUB(t *testing.T) {
	opts := defaultOpts()
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	// A valid EPUB should have no fixes to apply
	if len(result.Fixes) > 0 {
		for _, fix := range result.Fixes {
			t.Logf("Unexpected fix: [%s] %s", fix.CheckID, fix.Description)
		}
		t.Errorf("Expected no fixes on valid EPUB, got %d", len(result.Fixes))
	}
}

func TestDoctorOutputPassesValidation(t *testing.T) {
	// Create an EPUB with multiple problems
	opts := defaultOpts()
	opts.mimetypeContent = "wrong"
	opts.includeDCModified = false
	opts.doctype = "xhtml"
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	_, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	// Re-validate independently
	report, err := validate.Validate(output)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Should have fewer errors than before
	if report.FatalCount() > 0 {
		t.Errorf("Output has %d fatal errors", report.FatalCount())
		for _, msg := range report.Messages {
			t.Logf("  %s", msg)
		}
	}
}

func TestDoctorMimetypeNotFirst(t *testing.T) {
	opts := defaultOpts()
	opts.mimetypeFirst = false
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	_, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	// Verify output has mimetype as first entry
	zr, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	if len(zr.File) == 0 {
		t.Fatal("Output ZIP has no files")
	}
	if zr.File[0].Name != "mimetype" {
		t.Errorf("Expected mimetype as first entry, got '%s'", zr.File[0].Name)
	}
}

// --- Tier 2 unit tests ---

func TestDoctorFixesGuideElement(t *testing.T) {
	opts := defaultOpts()
	opts.includeGuide = true
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-039" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected OPF-039 fix for deprecated <guide> element")
	}

	// Verify the guide warning is gone after fix
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "OPF-039" {
			t.Errorf("OPF-039 still present after fix: %s", msg.Message)
		}
	}

	// Verify the OPF no longer has <guide>
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/content.opf")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "<guide") {
		t.Error("OPF still contains <guide> element after fix")
	}
}

func TestDoctorFixesEmptyHref(t *testing.T) {
	opts := defaultOpts()
	opts.emptyHref = true
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "HTM-003" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected HTM-003 fix for empty href attribute")
	}

	// Verify the warning is gone
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "HTM-003" {
			t.Errorf("HTM-003 still present after fix: %s", msg.Message)
		}
	}

	// Verify the content no longer has href=""
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/chapter1.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `href=""`) {
		t.Error("Content still contains href=\"\" after fix")
	}
	// But the <a> element should still be there (just without href)
	if !strings.Contains(string(data), "<a>") && !strings.Contains(string(data), "<a ") {
		t.Error("The <a> element was removed entirely instead of just removing href")
	}
}

func TestDoctorFixesBadDate(t *testing.T) {
	opts := defaultOpts()
	opts.badDate = "January 15, 2024"
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-036" {
			foundFix = true
			if !strings.Contains(fix.Description, "2024-01-15") {
				t.Errorf("Expected reformatted date to contain '2024-01-15', got: %s", fix.Description)
			}
			break
		}
	}
	if !foundFix {
		t.Error("Expected OPF-036 fix for bad dc:date format")
	}

	// Verify the OPF has the reformatted date
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/content.opf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "2024-01-15") {
		t.Error("OPF does not contain reformatted date '2024-01-15'")
	}
}

func TestDoctorFixesFileNotInManifest(t *testing.T) {
	opts := defaultOpts()
	opts.extraFile = true
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-003" {
			foundFix = true
			if !strings.Contains(fix.Description, "extra-style.css") {
				t.Errorf("Expected fix to mention 'extra-style.css', got: %s", fix.Description)
			}
			break
		}
	}
	if !foundFix {
		t.Error("Expected OPF-003 fix for file not in manifest")
	}

	// Verify the OPF now contains the extra file in manifest
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/content.opf")
	if err != nil {
		t.Fatal(err)
	}
	opf := string(data)
	if !strings.Contains(opf, "extra-style.css") {
		t.Error("OPF manifest does not contain 'extra-style.css' after fix")
	}
	if !strings.Contains(opf, "text/css") {
		t.Error("OPF manifest item for CSS file should have media-type text/css")
	}
}

func TestDoctorFixesObsoleteElements(t *testing.T) {
	opts := defaultOpts()
	opts.obsoleteElements = true
	input := createTestEPUB(t, opts)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "HTM-004" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected HTM-004 fix for obsolete elements")
	}

	// Verify the HTM-004 errors are gone
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "HTM-004" {
			t.Errorf("HTM-004 still present after fix: %s", msg.Message)
		}
	}

	// Verify the content has replacements
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/chapter1.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Should not have obsolete elements
	if strings.Contains(content, "<center>") {
		t.Error("Content still contains <center> after fix")
	}
	if strings.Contains(content, "<big>") {
		t.Error("Content still contains <big> after fix")
	}
	if strings.Contains(content, "<strike>") {
		t.Error("Content still contains <strike> after fix")
	}
	if strings.Contains(content, "<tt>") {
		t.Error("Content still contains <tt> after fix")
	}

	// Should have modern replacements
	if !strings.Contains(content, "text-align: center") {
		t.Error("Expected 'text-align: center' in replacement for <center>")
	}
	if !strings.Contains(content, "font-size: larger") {
		t.Error("Expected 'font-size: larger' in replacement for <big>")
	}
	if !strings.Contains(content, "text-decoration: line-through") {
		t.Error("Expected 'text-decoration: line-through' in replacement for <strike>")
	}
	if !strings.Contains(content, "font-family: monospace") {
		t.Error("Expected 'font-family: monospace' in replacement for <tt>")
	}
}

func TestDoctorDateReformatVariants(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"January 15, 2024", "2024-01-15"},
		{"Jan 1, 2024", "2024-01-01"},
		{"15 January 2024", "2024-01-15"},
		{"2024/01/15", "2024-01-15"},
		{"2024.01.15", "2024-01-15"},
		{"01/15/2024", "2024-01-15"},
		{"1/5/2024", "2024-01-05"},
	}

	for _, tt := range tests {
		result := tryReformatDate(tt.input)
		if result != tt.expected {
			t.Errorf("tryReformatDate(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDoctorDateReformatUnparseable(t *testing.T) {
	// These should return "" (can't parse)
	unparseable := []string{
		"not a date",
		"",
		"yesterday",
	}

	for _, s := range unparseable {
		result := tryReformatDate(s)
		if result != "" {
			t.Errorf("tryReformatDate(%q) = %q, want empty string", s, result)
		}
	}
}

// --- Tier 3 unit tests ---

// createEPUBWithCSSImport builds an EPUB where a CSS file uses @import.
func createEPUBWithCSSImport(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	epubPath := filepath.Join(dir, "test.epub")

	f, err := os.Create(epubPath)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	// mimetype
	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip"))

	// container
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// OPF with both CSS files in manifest
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>CSS Import Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="main-css" href="styles/main.css" media-type="text/css"/>
    <item id="base-css" href="styles/base.css" media-type="text/css"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
  </spine>
</package>`))

	// nav
	nw, _ := w.Create("OEBPS/nav.xhtml")
	nw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter 1</a></li></ol></nav>
</body>
</html>`))

	// chapter
	chw, _ := w.Create("OEBPS/chapter1.xhtml")
	chw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title><link rel="stylesheet" href="styles/main.css"/></head>
<body><p>Hello world</p></body>
</html>`))

	// main.css with @import
	cssw, _ := w.Create("OEBPS/styles/main.css")
	cssw.Write([]byte(`@import url("base.css");
body { margin: 1em; }
`))

	// base.css (the imported file)
	basew, _ := w.Create("OEBPS/styles/base.css")
	basew.Write([]byte(`html { font-size: 16px; }
p { line-height: 1.5; }
`))

	w.Close()
	f.Close()
	return epubPath
}

func TestDoctorFixesCSSImport(t *testing.T) {
	input := createEPUBWithCSSImport(t)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "CSS-005" {
			foundFix = true
			if !strings.Contains(fix.Description, "1 @import") {
				t.Errorf("Expected fix to mention inlined imports, got: %s", fix.Description)
			}
			break
		}
	}
	if !foundFix {
		t.Error("Expected CSS-005 fix for @import rule")
	}

	// Verify CSS-005 warning is gone
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "CSS-005" {
			t.Errorf("CSS-005 still present after fix: %s", msg.Message)
		}
	}

	// Verify the CSS file now contains the inlined content
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/styles/main.css")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "@import") {
		t.Error("CSS still contains @import after fix")
	}
	if !strings.Contains(content, "font-size: 16px") {
		t.Error("CSS should contain inlined content from base.css")
	}
	if !strings.Contains(content, "margin: 1em") {
		t.Error("CSS should still contain original content")
	}
}

// createEPUBWithBadEncoding builds an EPUB where an XHTML file declares non-UTF-8 encoding.
func createEPUBWithBadEncoding(t *testing.T, encoding string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	epubPath := filepath.Join(dir, "test.epub")

	f, err := os.Create(epubPath)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	// mimetype
	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip"))

	// container
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// OPF
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Encoding Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
  </spine>
</package>`))

	// nav
	nw, _ := w.Create("OEBPS/nav.xhtml")
	nw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter 1</a></li></ol></nav>
</body>
</html>`))

	// chapter with bad encoding
	chw, _ := w.Create("OEBPS/chapter1.xhtml")
	chw.Write(content)

	w.Close()
	f.Close()
	return epubPath
}

func TestDoctorFixesEncodingDeclaration(t *testing.T) {
	// Create an XHTML file that declares iso-8859-1 but is actually valid UTF-8
	content := []byte(`<?xml version="1.0" encoding="iso-8859-1"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>Hello world</p></body>
</html>`)

	input := createEPUBWithBadEncoding(t, "iso-8859-1", content)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "ENC-001" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected ENC-001 fix for non-UTF-8 encoding declaration")
	}

	// Verify ENC-001 is gone
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "ENC-001" {
			t.Errorf("ENC-001 still present after fix: %s", msg.Message)
		}
	}

	// Verify the encoding declaration was changed
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/chapter1.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "iso-8859-1") {
		t.Error("File still declares iso-8859-1 encoding after fix")
	}
	if !strings.Contains(string(data), "UTF-8") {
		t.Error("File should declare UTF-8 encoding after fix")
	}
}

func TestDoctorFixesWindows1252Transcoding(t *testing.T) {
	// Create XHTML with actual Windows-1252 bytes:
	// 0x93 = left double quote, 0x94 = right double quote in Windows-1252
	content := []byte(`<?xml version="1.0" encoding="windows-1252"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>`)
	// Append Windows-1252 encoded "smart quotes"
	content = append(content, 0x93) // left double quote
	content = append(content, []byte("Hello")...)
	content = append(content, 0x94) // right double quote
	content = append(content, []byte(` world</p></body>
</html>`)...)

	input := createEPUBWithBadEncoding(t, "windows-1252", content)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "ENC-001" {
			foundFix = true
			if !strings.Contains(fix.Description, "windows-1252") {
				t.Errorf("Expected fix to mention windows-1252, got: %s", fix.Description)
			}
			break
		}
	}
	if !foundFix {
		t.Error("Expected ENC-001 fix for windows-1252 transcoding")
	}

	// Verify transcoded content
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/chapter1.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "UTF-8") {
		t.Error("File should declare UTF-8 encoding after fix")
	}
	// Check that smart quotes were properly transcoded to UTF-8
	// U+201C = left double quotation mark, U+201D = right double quotation mark
	if !strings.Contains(s, "\u201c") {
		t.Error("Expected left double quote (U+201C) in transcoded output")
	}
	if !strings.Contains(s, "\u201d") {
		t.Error("Expected right double quote (U+201D) in transcoded output")
	}
}

func TestDoctorFixesUTF16(t *testing.T) {
	// Build UTF-16LE encoded XHTML with BOM
	xmlContent := `<?xml version="1.0" encoding="UTF-16"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p>Hello world</p></body>
</html>`

	// Encode as UTF-16LE with BOM
	var utf16Data bytes.Buffer
	utf16Data.Write([]byte{0xFF, 0xFE}) // BOM
	for _, r := range xmlContent {
		utf16Data.WriteByte(byte(r & 0xFF))
		utf16Data.WriteByte(byte(r >> 8))
	}

	input := createEPUBWithBadEncoding(t, "UTF-16", utf16Data.Bytes())
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "ENC-002" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected ENC-002 fix for UTF-16 transcoding")
	}

	// Verify the output is now valid UTF-8 and contains expected content
	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	data, err := ep.ReadFile("OEBPS/chapter1.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "Hello world") {
		t.Error("Transcoded content should contain 'Hello world'")
	}
	// Should not start with BOM
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		t.Error("Output should not have UTF-16 BOM")
	}
}

func TestTranscodeLatin1(t *testing.T) {
	// Latin-1 byte 0xE9 = é (U+00E9)
	input := []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x20, 0xE9} // "Hello é"
	result := transcodeLatin1ToUTF8(input)
	if !strings.Contains(string(result), "Hello") {
		t.Error("Expected 'Hello' in output")
	}
	if !strings.Contains(string(result), "é") {
		t.Errorf("Expected 'é' in output, got: %q", string(result))
	}
}

func TestTranscodeWindows1252(t *testing.T) {
	// Windows-1252: 0x80 = € (euro sign), 0x93 = " (left double quote)
	input := []byte{0x80, 0x20, 0x93}
	result := transcodeWindows1252ToUTF8(input)
	s := string(result)
	if !strings.Contains(s, "€") {
		t.Error("Expected euro sign in output")
	}
	if !strings.Contains(s, "\u201c") {
		t.Error("Expected left double quote in output")
	}
}

func TestTranscodeUTF16(t *testing.T) {
	// UTF-16LE BOM + "Hi" (H=0x48, i=0x69)
	data := []byte{0xFF, 0xFE, 0x48, 0x00, 0x69, 0x00}
	result, err := transcodeUTF16ToUTF8(data, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "Hi" {
		t.Errorf("Expected 'Hi', got %q", string(result))
	}

	// UTF-16BE BOM + "Hi"
	data = []byte{0xFE, 0xFF, 0x00, 0x48, 0x00, 0x69}
	result, err = transcodeUTF16ToUTF8(data, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "Hi" {
		t.Errorf("Expected 'Hi', got %q", string(result))
	}
}

// --- Tier 4 unit tests ---

// createCustomEPUB builds a custom EPUB from raw parts for targeted testing.
func createCustomEPUB(t *testing.T, opf, chapter string, extraFiles map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	epubPath := filepath.Join(dir, "test.epub")

	f, err := os.Create(epubPath)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip"))

	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(opf))

	nw, _ := w.Create("OEBPS/nav.xhtml")
	nw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter 1</a></li></ol></nav>
</body>
</html>`))

	chw, _ := w.Create("OEBPS/chapter1.xhtml")
	chw.Write([]byte(chapter))

	for name, data := range extraFiles {
		ew, _ := w.Create(name)
		ew.Write(data)
	}

	w.Close()
	f.Close()
	return epubPath
}

func TestDoctorFixesExtraDCTermsModified(t *testing.T) {
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
    <meta property="dcterms:modified">2024-06-15T12:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Ch</title></head><body><p>Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-028" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected OPF-028 fix for duplicate dcterms:modified")
	}

	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "OPF-028" {
			t.Errorf("OPF-028 still present after fix: %s", msg.Message)
		}
	}
}

func TestDoctorFixesManifestHrefFragment(t *testing.T) {
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml#intro" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Ch</title></head><body><p id="intro">Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-033" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected OPF-033 fix for fragment in manifest href")
	}

	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "OPF-033" {
			t.Errorf("OPF-033 still present after fix: %s", msg.Message)
		}
	}
}

func TestDoctorFixesDuplicateSpineIdrefs(t *testing.T) {
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
    <itemref idref="ch1"/>
  </spine>
</package>`
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Ch</title></head><body><p>Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-017" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected OPF-017 fix for duplicate spine idrefs")
	}

	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "OPF-017" {
			t.Errorf("OPF-017 still present after fix: %s", msg.Message)
		}
	}
}

func TestDoctorFixesInvalidLinear(t *testing.T) {
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="ch1" linear="true"/>
  </spine>
</package>`
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Ch</title></head><body><p>Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "OPF-038" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected OPF-038 fix for invalid linear value")
	}

	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "OPF-038" {
			t.Errorf("OPF-038 still present after fix: %s", msg.Message)
		}
	}
}

func TestDoctorFixesBaseElement(t *testing.T) {
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Ch</title><base href="http://example.com/"/></head>
<body><p>Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "HTM-009" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected HTM-009 fix for <base> element")
	}

	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()
	data, _ := ep.ReadFile("OEBPS/chapter1.xhtml")
	if strings.Contains(string(data), "<base") {
		t.Error("Output still contains <base> element")
	}
}

func TestDoctorFixesProcessingInstructions(t *testing.T) {
	// HTM-020 is now INFO severity (processing instructions are allowed per
	// the EPUB spec). The doctor still has the fix function, but it only runs
	// when there are other issues that make the EPUB invalid. When the EPUB's
	// only issue is a PI, the doctor correctly determines it's already valid
	// and skips fixes.
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	// Include a PI (INFO-level) and a missing <title> (WARNING) so the doctor runs
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<?oxygen RNGSchema="test.rng"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head></head>
<body><p>Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "HTM-020" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected HTM-020 fix for processing instructions")
	}

	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()
	data, _ := ep.ReadFile("OEBPS/chapter1.xhtml")
	if strings.Contains(string(data), "<?oxygen") {
		t.Error("Output still contains processing instruction")
	}
	// XML declaration should be preserved
	if !strings.Contains(string(data), "<?xml") {
		t.Error("XML declaration should be preserved")
	}
}

func TestDoctorFixesLangMismatch(t *testing.T) {
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="en-US" xml:lang="en-GB">
<head><title>Ch</title></head>
<body><p>Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "HTM-026" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected HTM-026 fix for lang/xml:lang mismatch")
	}

	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()
	data, _ := ep.ReadFile("OEBPS/chapter1.xhtml")
	content := string(data)
	// Both should now be en-GB
	if !strings.Contains(content, `lang="en-GB"`) {
		t.Error("Expected lang to be synced to en-GB")
	}
}

func TestDoctorFixesMissingTitle(t *testing.T) {
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`
	chapter := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head></head>
<body><p>Hi</p></body></html>`

	input := createCustomEPUB(t, opf, chapter, nil)
	output := filepath.Join(t.TempDir(), "fixed.epub")

	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	foundFix := false
	for _, fix := range result.Fixes {
		if fix.CheckID == "HTM-002" {
			foundFix = true
			break
		}
	}
	if !foundFix {
		t.Error("Expected HTM-002 fix for missing title")
	}

	ep, err := epub.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()
	data, _ := ep.ReadFile("OEBPS/chapter1.xhtml")
	if !strings.Contains(string(data), "<title>") {
		t.Error("Output should contain a <title> element")
	}
}
