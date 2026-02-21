package doctor

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/adammathes/epubverify/pkg/validate"
)

// TestDoctorIntegrationMultipleProblems creates an EPUB with many simultaneous
// problems and verifies the doctor fixes all of them.
func TestDoctorIntegrationMultipleProblems(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "broken.epub")
	output := filepath.Join(dir, "fixed.epub")

	// Build a broken EPUB with many issues:
	// 1. mimetype not first (OCF-002)
	// 2. mimetype content wrong (OCF-003)
	// 3. mimetype compressed (OCF-005)
	// 4. missing dcterms:modified (OPF-004)
	// 5. XHTML 1.1 DOCTYPE (HTM-010)
	// 6. script without 'scripted' property (HTM-005)
	// 7. image media-type mismatch (OPF-024/MED-001)
	f, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	// Write container first (mimetype not first = OCF-002)
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// Mimetype with wrong content and compressed (OCF-003, OCF-005)
	header := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Deflate, // Wrong: should be Store
	}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip-WRONG")) // Wrong content

	// OPF without dcterms:modified and with wrong media-type for image
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Broken Test Book</dc:title>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    <item id="cover" href="cover.jpg" media-type="image/png"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
  </spine>
</package>`))

	// Nav doc
	nw, _ := w.Create("OEBPS/nav.xhtml")
	nw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter 1</a></li></ol></nav>
</body>
</html>`))

	// Chapter with XHTML 1.1 DOCTYPE and script (no scripted property)
	chw, _ := w.Create("OEBPS/chapter1.xhtml")
	chw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body>
<script>console.log("hello");</script>
<p>Hello world</p>
</body>
</html>`))

	// JPEG image declared as PNG
	coverw, _ := w.Create("OEBPS/cover.jpg")
	coverw.Write([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00})

	w.Close()
	f.Close()

	// Validate the broken EPUB first
	beforeReport, err := validate.Validate(input)
	if err != nil {
		t.Fatalf("Pre-validation failed: %v", err)
	}
	beforeErrors := beforeReport.ErrorCount() + beforeReport.FatalCount()
	t.Logf("Before: %d errors, %d warnings", beforeErrors, beforeReport.WarningCount())
	for _, msg := range beforeReport.Messages {
		t.Logf("  %s", msg)
	}

	// Run doctor
	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	t.Logf("\nApplied %d fixes:", len(result.Fixes))
	for _, fix := range result.Fixes {
		t.Logf("  [%s] %s", fix.CheckID, fix.Description)
	}

	afterErrors := result.AfterReport.ErrorCount() + result.AfterReport.FatalCount()
	t.Logf("\nAfter: %d errors, %d warnings", afterErrors, result.AfterReport.WarningCount())
	for _, msg := range result.AfterReport.Messages {
		t.Logf("  %s", msg)
	}

	// Verify fixes were applied
	expectedFixes := map[string]bool{
		"OCF-002": false, // mimetype not first
		"OCF-003": false, // wrong mimetype content
		"OPF-004": false, // missing dcterms:modified
		"OPF-024": false, // wrong media-type
		"HTM-005": false, // missing scripted property
		"HTM-010": false, // wrong DOCTYPE
	}
	for _, fix := range result.Fixes {
		if _, ok := expectedFixes[fix.CheckID]; ok {
			expectedFixes[fix.CheckID] = true
		}
	}
	for checkID, found := range expectedFixes {
		if !found {
			t.Errorf("Expected fix for %s but it was not applied", checkID)
		}
	}

	// Verify error count decreased
	if afterErrors >= beforeErrors {
		t.Errorf("Expected fewer errors after fix: before=%d, after=%d", beforeErrors, afterErrors)
	}

	// Verify the repaired EPUB is a valid ZIP
	zr, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("Output is not a valid ZIP: %v", err)
	}
	defer zr.Close()

	// Verify mimetype is first and stored
	if zr.File[0].Name != "mimetype" {
		t.Errorf("mimetype not first entry: got %s", zr.File[0].Name)
	}
	if zr.File[0].Method != zip.Store {
		t.Errorf("mimetype not stored: method=%d", zr.File[0].Method)
	}
}

// TestDoctorIntegrationTier2Problems creates an EPUB with multiple Tier 2
// issues and verifies the doctor fixes all of them simultaneously.
func TestDoctorIntegrationTier2Problems(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "broken2.epub")
	output := filepath.Join(dir, "fixed2.epub")

	// Build an EPUB with Tier 2 issues:
	// 1. Deprecated <guide> element (OPF-039)
	// 2. Bad dc:date format (OPF-036)
	// 3. Empty href="" on <a> (HTM-003)
	// 4. Obsolete <center> and <big> elements (HTM-004)
	// 5. Extra file not in manifest (RSC-002)
	f, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	// Mimetype (correct)
	header := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip"))

	// Container
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// OPF with guide and bad date
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Tier 2 Test Book</dc:title>
    <dc:language>en</dc:language>
    <dc:date>March 15, 2024</dc:date>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
  </spine>
  <guide>
    <reference type="cover" title="Cover" href="chapter1.xhtml"/>
  </guide>
</package>`))

	// Nav doc
	nw, _ := w.Create("OEBPS/nav.xhtml")
	nw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter 1</a></li></ol></nav>
</body>
</html>`))

	// Chapter with empty href, obsolete center and big
	chw, _ := w.Create("OEBPS/chapter1.xhtml")
	chw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body>
<a href="">click here</a>
<center>Centered content</center>
<big>Big text</big>
<p>Hello world</p>
</body>
</html>`))

	// Extra CSS file not in manifest (RSC-002)
	ew, _ := w.Create("OEBPS/orphan.css")
	ew.Write([]byte(`body { font-size: 1em; }`))

	w.Close()
	f.Close()

	// Validate before
	beforeReport, err := validate.Validate(input)
	if err != nil {
		t.Fatalf("Pre-validation failed: %v", err)
	}
	beforeErrors := beforeReport.ErrorCount() + beforeReport.FatalCount()
	beforeWarnings := beforeReport.WarningCount()
	t.Logf("Before: %d errors, %d warnings", beforeErrors, beforeWarnings)
	for _, msg := range beforeReport.Messages {
		t.Logf("  %s", msg)
	}

	// Run doctor
	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	t.Logf("\nApplied %d fixes:", len(result.Fixes))
	for _, fix := range result.Fixes {
		t.Logf("  [%s] %s", fix.CheckID, fix.Description)
	}

	afterErrors := result.AfterReport.ErrorCount() + result.AfterReport.FatalCount()
	afterWarnings := result.AfterReport.WarningCount()
	t.Logf("\nAfter: %d errors, %d warnings", afterErrors, afterWarnings)
	for _, msg := range result.AfterReport.Messages {
		t.Logf("  %s", msg)
	}

	// Verify all expected Tier 2 fixes were applied
	expectedFixes := map[string]bool{
		"OPF-039": false, // deprecated guide
		"OPF-036": false, // bad date format
		"HTM-003": false, // empty href
		"HTM-004": false, // obsolete elements
		"RSC-002": false, // file not in manifest
	}
	for _, fix := range result.Fixes {
		if _, ok := expectedFixes[fix.CheckID]; ok {
			expectedFixes[fix.CheckID] = true
		}
	}
	for checkID, found := range expectedFixes {
		if !found {
			t.Errorf("Expected fix for %s but it was not applied", checkID)
		}
	}

	// Verify combined issue count improved
	totalBefore := beforeErrors + beforeWarnings
	totalAfter := afterErrors + afterWarnings
	if totalAfter >= totalBefore {
		t.Errorf("Expected fewer total issues after fix: before=%d, after=%d", totalBefore, totalAfter)
	}

	// Verify the output is a valid ZIP
	zr, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("Output is not a valid ZIP: %v", err)
	}
	zr.Close()
}

// TestDoctorIntegrationTier3Problems creates an EPUB with Tier 3 issues:
// CSS @import and non-UTF-8 encoding declaration.
func TestDoctorIntegrationTier3Problems(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "broken3.epub")
	output := filepath.Join(dir, "fixed3.epub")

	f, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	// Mimetype
	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip"))

	// Container
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
    <dc:title>Tier 3 Test</dc:title>
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

	// Nav
	nw, _ := w.Create("OEBPS/nav.xhtml")
	nw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol><li><a href="chapter1.xhtml">Chapter 1</a></li></ol></nav>
</body>
</html>`))

	// Chapter with iso-8859-1 encoding declaration (but actually UTF-8 content)
	chw, _ := w.Create("OEBPS/chapter1.xhtml")
	chw.Write([]byte(`<?xml version="1.0" encoding="iso-8859-1"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title><link rel="stylesheet" href="styles/main.css"/></head>
<body><p>Hello world</p></body>
</html>`))

	// CSS with @import
	cssw, _ := w.Create("OEBPS/styles/main.css")
	cssw.Write([]byte(`@import "base.css";
body { color: #333; }
`))

	// Base CSS
	basew, _ := w.Create("OEBPS/styles/base.css")
	basew.Write([]byte(`html { font-size: 100%; }
`))

	w.Close()
	f.Close()

	// Validate before
	beforeReport, err := validate.Validate(input)
	if err != nil {
		t.Fatalf("Pre-validation failed: %v", err)
	}
	beforeErrors := beforeReport.ErrorCount() + beforeReport.FatalCount()
	beforeWarnings := beforeReport.WarningCount()
	t.Logf("Before: %d errors, %d warnings", beforeErrors, beforeWarnings)
	for _, msg := range beforeReport.Messages {
		t.Logf("  %s", msg)
	}

	// Run doctor
	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	t.Logf("\nApplied %d fixes:", len(result.Fixes))
	for _, fix := range result.Fixes {
		t.Logf("  [%s] %s", fix.CheckID, fix.Description)
	}

	afterErrors := result.AfterReport.ErrorCount() + result.AfterReport.FatalCount()
	afterWarnings := result.AfterReport.WarningCount()
	t.Logf("\nAfter: %d errors, %d warnings", afterErrors, afterWarnings)
	for _, msg := range result.AfterReport.Messages {
		t.Logf("  %s", msg)
	}

	// Verify expected fixes
	expectedFixes := map[string]bool{
		"CSS-005": false, // @import inlined
		"ENC-001": false, // encoding declaration fixed
	}
	for _, fix := range result.Fixes {
		if _, ok := expectedFixes[fix.CheckID]; ok {
			expectedFixes[fix.CheckID] = true
		}
	}
	for checkID, found := range expectedFixes {
		if !found {
			t.Errorf("Expected fix for %s but it was not applied", checkID)
		}
	}

	// Verify issue count improved
	totalBefore := beforeErrors + beforeWarnings
	totalAfter := afterErrors + afterWarnings
	if totalAfter >= totalBefore {
		t.Errorf("Expected fewer total issues after fix: before=%d, after=%d", totalBefore, totalAfter)
	}
}

// TestDoctorIntegrationTier4Problems creates an EPUB with multiple Tier 4
// issues and verifies the doctor fixes all of them simultaneously.
// Uses two chapters: ch1 for OPF-033 (fragment href), ch2 for XHTML fixes.
func TestDoctorIntegrationTier4Problems(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "broken4.epub")
	output := filepath.Join(dir, "fixed4.epub")

	f, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}

	w := zip.NewWriter(f)

	// Mimetype (correct)
	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip"))

	// Container
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// OPF with multiple Tier 4 OPF-level issues:
	// 1. Duplicate dcterms:modified (OPF-028)
	// 2. Fragment in manifest href for ch1 (OPF-033)
	// 3. Duplicate spine idref for ch2 (OPF-017)
	// 4. Invalid linear="true" on ch2 (OPF-038)
	// XHTML issues are on ch2 (which has a clean manifest href):
	// 5. Processing instruction (HTM-020)
	// 6. lang/xml:lang mismatch (HTM-026)
	// 7. Missing <title> (HTM-002)
	// 8. <base> element (HTM-009)
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:12345678-1234-1234-1234-123456789012</dc:identifier>
    <dc:title>Tier 4 Test</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
    <meta property="dcterms:modified">2024-06-15T12:00:00Z</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="ch1" href="chapter1.xhtml#intro" media-type="application/xhtml+xml"/>
    <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="ch1"/>
    <itemref idref="ch2" linear="true"/>
    <itemref idref="ch2"/>
  </spine>
</package>`))

	// Nav doc
	nw, _ := w.Create("OEBPS/nav.xhtml")
	nw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Navigation</title></head>
<body>
<nav epub:type="toc"><ol>
<li><a href="chapter1.xhtml">Chapter 1</a></li>
<li><a href="chapter2.xhtml">Chapter 2</a></li>
</ol></nav>
</body>
</html>`))

	// Chapter 1 — clean content, used for OPF-033 fragment test
	chw1, _ := w.Create("OEBPS/chapter1.xhtml")
	chw1.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Chapter 1</title></head>
<body><p id="intro">Chapter one content</p></body>
</html>`))

	// Chapter 2 — has all XHTML-level Tier 4 issues
	chw2, _ := w.Create("OEBPS/chapter2.xhtml")
	chw2.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<?oxygen RNGSchema="test.rng"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="en-US" xml:lang="en-GB">
<head><base href="http://example.com/"/></head>
<body><p>Chapter two content</p></body>
</html>`))

	w.Close()
	f.Close()

	// Validate before
	beforeReport, err := validate.Validate(input)
	if err != nil {
		t.Fatalf("Pre-validation failed: %v", err)
	}
	beforeErrors := beforeReport.ErrorCount() + beforeReport.FatalCount()
	beforeWarnings := beforeReport.WarningCount()
	t.Logf("Before: %d errors, %d warnings", beforeErrors, beforeWarnings)
	for _, msg := range beforeReport.Messages {
		t.Logf("  %s", msg)
	}

	// Run doctor
	result, err := Repair(input, output)
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	t.Logf("\nApplied %d fixes:", len(result.Fixes))
	for _, fix := range result.Fixes {
		t.Logf("  [%s] %s", fix.CheckID, fix.Description)
	}

	afterErrors := result.AfterReport.ErrorCount() + result.AfterReport.FatalCount()
	afterWarnings := result.AfterReport.WarningCount()
	t.Logf("\nAfter: %d errors, %d warnings", afterErrors, afterWarnings)
	for _, msg := range result.AfterReport.Messages {
		t.Logf("  %s", msg)
	}

	// Verify all expected Tier 4 fixes were applied
	expectedFixes := map[string]bool{
		"OPF-028": false, // duplicate dcterms:modified
		"OPF-033": false, // fragment in manifest href
		"OPF-017": false, // duplicate spine idref
		"OPF-038": false, // invalid linear value
		"HTM-020": false, // processing instruction
		"HTM-026": false, // lang/xml:lang mismatch
		"HTM-002": false, // missing title
		"HTM-009": false, // base element
	}
	for _, fix := range result.Fixes {
		if _, ok := expectedFixes[fix.CheckID]; ok {
			expectedFixes[fix.CheckID] = true
		}
	}
	for checkID, found := range expectedFixes {
		if !found {
			t.Errorf("Expected fix for %s but it was not applied", checkID)
		}
	}

	// Verify combined issue count improved
	totalBefore := beforeErrors + beforeWarnings
	totalAfter := afterErrors + afterWarnings
	if totalAfter >= totalBefore {
		t.Errorf("Expected fewer total issues after fix: before=%d, after=%d", totalBefore, totalAfter)
	}

	// Verify output is a valid ZIP
	zr, err := zip.OpenReader(output)
	if err != nil {
		t.Fatalf("Output is not a valid ZIP: %v", err)
	}
	zr.Close()
}

// TestDoctorRoundTrip verifies that running doctor on a valid EPUB
// produces identical validation results.
func TestDoctorRoundTrip(t *testing.T) {
	opts := defaultOpts()
	input := createTestEPUB(t, opts)

	report1, err := validate.Validate(input)
	if err != nil {
		t.Fatal(err)
	}

	// Doctor should not produce output for valid EPUB
	result, err := Repair(input, filepath.Join(t.TempDir(), "output.epub"))
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Fixes) != 0 {
		t.Errorf("Expected no fixes, got %d", len(result.Fixes))
	}

	// Before and after should be the same report
	if report1.ErrorCount() != result.BeforeReport.ErrorCount() {
		t.Errorf("Reports don't match: %d vs %d errors",
			report1.ErrorCount(), result.BeforeReport.ErrorCount())
	}
}
