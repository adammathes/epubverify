package validate

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// checkOCF runs all OCF container checks. Returns true if a fatal error
// was found that prevents further processing.
func checkOCF(ep *epub.EPUB, r *report.Report, opts Options) bool {
	fatal := false

	// OCF-001: mimetype file must be present
	checkMimetypePresent(ep, r)

	// OCF-002: mimetype must be first entry
	checkMimetypeFirst(ep, r)

	// OCF-003: mimetype content must be exactly "application/epub+zip"
	checkMimetypeContent(ep, r)

	// OCF-004: mimetype must not have extra field in local header
	checkMimetypeNoExtraField(ep, r)

	// OCF-005: mimetype must be stored, not compressed
	// epubcheck 5.3.0 does not flag compressed mimetype entries.
	// Only check in strict mode to better follow the spec.
	if opts.Strict {
		checkMimetypeStored(ep, r)
	}

	// OCF-006: container.xml must be present
	if !checkContainerPresent(ep, r) {
		return true
	}

	// OCF-007: container.xml must be well-formed XML
	if !checkContainerWellFormed(ep, r) {
		return true
	}

	// OCF-008: container.xml must have a rootfile
	if !checkContainerHasRootfile(ep, r) {
		fatal = true
	}

	// OCF-009: rootfile target must exist
	if !fatal && !checkRootfileExists(ep, r) {
		return true
	}

	// OCF-010: META-INF/encryption.xml must be valid if present
	checkEncryptionXML(ep, r)

	// OCF-011: all rootfiles must exist
	if checkAllRootfilesExist(ep, r) {
		return true
	}

	// OCF-012: rootfile media-type must be correct
	checkRootfileMediaType(ep, r)

	return fatal
}

// OCF-001: mimetype file must be present
func checkMimetypePresent(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["mimetype"]
	if !exists {
		r.Add(report.Error, "OCF-001", "Required mimetype file not found in the OCF container")
	}
}

// OCF-002: mimetype must be the first entry in the zip
func checkMimetypeFirst(ep *epub.EPUB, r *report.Report) {
	if len(ep.ZipFile.File) == 0 {
		return
	}
	first := ep.ZipFile.File[0]
	if first.Name != "mimetype" {
		// Only report if mimetype exists but isn't first (OCF-001 covers missing case)
		if _, exists := ep.Files["mimetype"]; exists {
			r.Add(report.Error, "OCF-002", "The mimetype file must be the first entry in the zip archive")
		}
	}
}

// OCF-003: mimetype must contain exactly "application/epub+zip"
func checkMimetypeContent(ep *epub.EPUB, r *report.Report) {
	f, exists := ep.Files["mimetype"]
	if !exists {
		return
	}
	data, err := readZipFile(f)
	if err != nil {
		return
	}
	content := string(data)
	if content != "application/epub+zip" {
		r.Add(report.Error, "OCF-003",
			"The mimetype file must contain exactly 'application/epub+zip' but was '"+strings.TrimSpace(content)+"'")
	}
}

// OCF-004: mimetype must not have an extra field in its local header.
// Go's archive/zip reads Extra from the central directory, which may differ
// from the local file header. We read the raw local header to check.
func checkMimetypeNoExtraField(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["mimetype"]
	if !exists {
		return
	}

	hasExtra, err := mimetypeLocalHeaderHasExtra(ep.Path)
	if err != nil {
		return
	}
	if hasExtra {
		r.Add(report.Error, "OCF-004", "The mimetype zip entry must not have an extra field in its local header")
	}
}

// mimetypeLocalHeaderHasExtra reads the raw zip bytes to check if the first
// local file header (which should be the mimetype entry) has a non-zero
// extra field length.
func mimetypeLocalHeaderHasExtra(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// ZIP local file header structure:
	// 0-3:   signature (0x04034b50)
	// 4-5:   version needed
	// 6-7:   general purpose bit flag
	// 8-9:   compression method
	// 10-11: last mod file time
	// 12-13: last mod file date
	// 14-17: crc-32
	// 18-21: compressed size
	// 22-25: uncompressed size
	// 26-27: file name length
	// 28-29: extra field length

	header := make([]byte, 30)
	_, err = f.Read(header)
	if err != nil {
		return false, err
	}

	// Verify it's a local file header
	sig := binary.LittleEndian.Uint32(header[0:4])
	if sig != 0x04034b50 {
		return false, nil
	}

	extraLen := binary.LittleEndian.Uint16(header[28:30])
	return extraLen > 0, nil
}

// OCF-005: mimetype must be stored, not compressed (strict mode only)
func checkMimetypeStored(ep *epub.EPUB, r *report.Report) {
	f, exists := ep.Files["mimetype"]
	if !exists {
		return
	}
	if f.Method != zip.Store {
		r.Add(report.Error, "OCF-005", "The mimetype file must be stored (not compressed) in the zip archive")
	}
}

// OCF-006: META-INF/container.xml must be present
func checkContainerPresent(ep *epub.EPUB, r *report.Report) bool {
	_, exists := ep.Files["META-INF/container.xml"]
	if !exists {
		r.Add(report.Fatal, "OCF-006", "Required META-INF/container.xml not found in the OCF container")
		return false
	}
	return true
}

// OCF-007: container.xml must be well-formed XML
func checkContainerWellFormed(ep *epub.EPUB, r *report.Report) bool {
	err := ep.ParseContainer()
	if err != nil {
		r.Add(report.Fatal, "OCF-007", "META-INF/container.xml is not well-formed: XML document structures must be well-formed")
		return false
	}
	return true
}

// OCF-008: container.xml must contain a rootfile element
func checkContainerHasRootfile(ep *epub.EPUB, r *report.Report) bool {
	if ep.RootfilePath == "" {
		r.Add(report.Error, "OCF-008", "container.xml does not contain a rootfile element")
		return false
	}
	return true
}

// OCF-009: rootfile full-path must point to an existing file
func checkRootfileExists(ep *epub.EPUB, r *report.Report) bool {
	if ep.RootfilePath == "" {
		return false
	}
	_, exists := ep.Files[ep.RootfilePath]
	if !exists {
		r.Add(report.Fatal, "OCF-009", "The rootfile '"+ep.RootfilePath+"' ("+ep.RootfilePath+") was not found in the container")
		return false
	}
	return true
}

// OCF-010: META-INF/encryption.xml must be complete if present
func checkEncryptionXML(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["META-INF/encryption.xml"]
	if !exists {
		return
	}
	data, err := ep.ReadFile("META-INF/encryption.xml")
	if err != nil {
		return
	}
	// Check if encryption.xml has actual content (EncryptedData elements)
	content := string(data)
	if !strings.Contains(content, "EncryptedData") && !strings.Contains(content, "EncryptionMethod") {
		r.Add(report.Error, "OCF-010",
			"META-INF/encryption.xml is incomplete: no encryption data found")
	}
}

// OCF-011: all rootfile elements must point to existing files
func checkAllRootfilesExist(ep *epub.EPUB, r *report.Report) bool {
	if len(ep.AllRootfiles) <= 1 {
		return false
	}
	for _, rf := range ep.AllRootfiles {
		if rf.FullPath == ep.RootfilePath {
			continue // Already checked by OCF-009
		}
		if _, exists := ep.Files[rf.FullPath]; !exists {
			r.Add(report.Fatal, "OCF-011",
				fmt.Sprintf("Rootfile '%s' was not found in the container", rf.FullPath))
			return true
		}
	}
	return false
}

// OCF-012: rootfile media-type must be application/oebps-package+xml
func checkRootfileMediaType(ep *epub.EPUB, r *report.Report) {
	hasCorrectMediaType := false
	for _, rf := range ep.AllRootfiles {
		if rf.MediaType == "application/oebps-package+xml" {
			hasCorrectMediaType = true
			break
		}
	}
	if len(ep.AllRootfiles) > 0 && !hasCorrectMediaType {
		r.Add(report.Error, "OCF-012",
			"No rootfile tag with media type 'application/oebps-package+xml' found")
	}
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var buf []byte
	b := make([]byte, 1024)
	for {
		n, err := rc.Read(b)
		buf = append(buf, b[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
