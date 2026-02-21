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
		if fix.CheckID == "OCF-003" {
			foundMimeFix = true
			break
		}
	}
	if !foundMimeFix {
		t.Error("Expected OCF-003 fix for mimetype content")
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
		if fix.CheckID == "HTM-005" {
			foundScriptFix = true
			break
		}
	}
	if !foundScriptFix {
		t.Error("Expected HTM-005 fix for missing 'scripted' property")
	}

	// Verify the property was added in the output
	for _, msg := range result.AfterReport.Messages {
		if msg.CheckID == "HTM-005" {
			t.Errorf("HTM-005 still present after fix: %s", msg.Message)
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
		if fix.CheckID == "RSC-002" {
			foundFix = true
			if !strings.Contains(fix.Description, "extra-style.css") {
				t.Errorf("Expected fix to mention 'extra-style.css', got: %s", fix.Description)
			}
			break
		}
	}
	if !foundFix {
		t.Error("Expected RSC-002 fix for file not in manifest")
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
