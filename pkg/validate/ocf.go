package validate

import (
	"archive/zip"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// checkOCF runs all OCF container checks. Returns true if a fatal error
// was found that prevents further processing.
func checkOCF(ep *epub.EPUB, r *report.Report, opts Options) bool {
	fatal := false

	// PKG-006: mimetype file must be present
	checkMimetypePresent(ep, r)

	// PKG-007: mimetype must be first entry (or wrong content)
	checkMimetypeFirst(ep, r)

	// PKG-007: mimetype content must be exactly "application/epub+zip"
	checkMimetypeContent(ep, r)

	// PKG-005: mimetype must not have extra field in local header
	checkMimetypeNoExtraField(ep, r)

	// mimetype must be stored, not compressed (strict mode only)
	if opts.Strict {
		checkMimetypeStored(ep, r)
	}

	// RSC-002: container.xml must be present
	if !checkContainerPresent(ep, r) {
		return true
	}

	// container.xml must be well-formed XML
	if !checkContainerWellFormed(ep, r) {
		return true
	}

	// container.xml must have a rootfile
	if !checkContainerHasRootfile(ep, r) {
		fatal = true
	}

	// OPF-002: rootfile target must exist
	if !fatal && !checkRootfileExists(ep, r) {
		return true
	}

	// encryption.xml checks
	checkEncryptionXML(ep, r)

	// all rootfiles must exist
	if checkAllRootfilesExist(ep, r) {
		return true
	}

	// RSC-003: rootfile media-type must be correct
	checkRootfileMediaType(ep, r)

	// encryption.xml must be well-formed XML if present
	checkEncryptionXMLWellFormed(ep, r)

	// container.xml version must be 1.0
	checkContainerVersion(ep, r)

	// PKG-009: filenames must not contain restricted characters
	checkFilenameValidChars(ep, r)

	// PKG-010: warn about spaces in file names
	checkFilenameSpaces(ep, r)

	// PKG-014: warn about empty directories
	checkEmptyDirectories(ep, r)

	// PKG-025: publication resources must not be in META-INF
	checkNoResourcesInMetaInf(ep, r)

	// OPF-060: duplicate filenames after case folding / NFC normalization
	checkDuplicateFilenames(ep, r)

	// file paths should not exceed 65535 bytes
	checkFilenameLength(ep, r)

	return fatal
}

// PKG-006: mimetype file must be present (epubcheck: PKG-006)
func checkMimetypePresent(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["mimetype"]
	if !exists {
		r.Add(report.Error, "PKG-006", "Required mimetype file not found in the OCF container")
	}
}

// PKG-007: mimetype must be the first entry in the zip
func checkMimetypeFirst(ep *epub.EPUB, r *report.Report) {
	if len(ep.ZipFile.File) == 0 {
		return
	}
	first := ep.ZipFile.File[0]
	if first.Name != "mimetype" {
		// Only report if mimetype exists but isn't first (PKG-006 covers missing case)
		if _, exists := ep.Files["mimetype"]; exists {
			r.Add(report.Error, "PKG-007", "The mimetype file must be the first entry in the zip archive")
		}
	}
}

// PKG-007: mimetype must contain exactly "application/epub+zip"
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
		r.Add(report.Error, "PKG-007",
			"The mimetype file must contain exactly 'application/epub+zip' but was '"+strings.TrimSpace(content)+"'")
	}
}

// PKG-005: mimetype must not have an extra field in its local header.
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
		r.Add(report.Error, "PKG-005", "The mimetype zip entry must not have an extra field in its local header")
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

// mimetype must be stored, not compressed (strict mode only)
func checkMimetypeStored(ep *epub.EPUB, r *report.Report) {
	f, exists := ep.Files["mimetype"]
	if !exists {
		return
	}
	if f.Method != zip.Store {
		r.Add(report.Error, "PKG-005", "The mimetype file must be stored (not compressed) in the zip archive")
	}
}

// RSC-002: META-INF/container.xml must be present
func checkContainerPresent(ep *epub.EPUB, r *report.Report) bool {
	_, exists := ep.Files["META-INF/container.xml"]
	if !exists {
		r.Add(report.Fatal, "RSC-002", "Required file META-INF/container.xml was not found in the container")
		return false
	}
	return true
}

// RSC-005: container.xml must be well-formed XML
func checkContainerWellFormed(ep *epub.EPUB, r *report.Report) bool {
	err := ep.ParseContainer()
	if err != nil {
		r.Add(report.Fatal, "RSC-005", "META-INF/container.xml is not well-formed: XML document structures must be well-formed")
		return false
	}
	return true
}

// container.xml must contain a rootfile element
func checkContainerHasRootfile(ep *epub.EPUB, r *report.Report) bool {
	if ep.RootfilePath == "" {
		r.Add(report.Error, "RSC-005", "container.xml does not contain a rootfile element")
		return false
	}
	return true
}

// OPF-002: rootfile full-path must point to an existing file
func checkRootfileExists(ep *epub.EPUB, r *report.Report) bool {
	if ep.RootfilePath == "" {
		return false
	}
	_, exists := ep.Files[ep.RootfilePath]
	if !exists {
		r.Add(report.Fatal, "OPF-002", "The package document '"+ep.RootfilePath+"' was not found in the container")
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

// RSC-003: rootfile media-type must be application/oebps-package+xml
func checkRootfileMediaType(ep *epub.EPUB, r *report.Report) {
	hasCorrectMediaType := false
	for _, rf := range ep.AllRootfiles {
		if rf.MediaType == "application/oebps-package+xml" {
			hasCorrectMediaType = true
			break
		}
	}
	if len(ep.AllRootfiles) > 0 && !hasCorrectMediaType {
		r.Add(report.Error, "RSC-003",
			"No rootfile tag with media type 'application/oebps-package+xml' found")
	}
}

// OCF-013: encryption.xml must be well-formed XML if present
func checkEncryptionXMLWellFormed(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["META-INF/encryption.xml"]
	if !exists {
		return
	}
	data, err := ep.ReadFile("META-INF/encryption.xml")
	if err != nil {
		return
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			r.Add(report.Fatal, "OCF-013",
				fmt.Sprintf("META-INF/encryption.xml is not well-formed: element must be followed by the '>' character (%s)", err.Error()))
			r.Add(report.Error, "OCF-013",
				"Encryption XML validation aborted due to malformed XML")
			return
		}
	}
}

// OCF-014: container.xml version attribute must be 1.0
func checkContainerVersion(ep *epub.EPUB, r *report.Report) {
	if ep.ContainerData == nil {
		return
	}
	decoder := xml.NewDecoder(strings.NewReader(string(ep.ContainerData)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "container" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "version" && attr.Value != "1.0" {
					r.Add(report.Error, "OCF-014",
						fmt.Sprintf("The container.xml version attribute value '%s' must be equal to '1.0'", attr.Value))
				}
			}
			return
		}
	}
}

// PKG-009: filenames must not contain restricted characters
func checkFilenameValidChars(ep *epub.EPUB, r *report.Report) {
	for _, f := range ep.ZipFile.File {
		for _, c := range f.Name {
			if c < 0x20 {
				r.Add(report.Error, "PKG-009",
					fmt.Sprintf("File name contains characters forbidden in OCF file names: '%s'", f.Name))
				break
			}
		}
	}
}

// PKG-010: warn about spaces in file names
func checkFilenameSpaces(ep *epub.EPUB, r *report.Report) {
	for _, f := range ep.ZipFile.File {
		if strings.Contains(f.Name, " ") && f.Name != "mimetype" {
			r.Add(report.Warning, "PKG-010",
				fmt.Sprintf("Filename contains spaces, which is discouraged: '%s'", f.Name))
		}
	}
}

// PKG-014: warn about empty directories in the container
func checkEmptyDirectories(ep *epub.EPUB, r *report.Report) {
	for _, f := range ep.ZipFile.File {
		if strings.HasSuffix(f.Name, "/") && f.UncompressedSize64 == 0 {
			r.Add(report.Warning, "PKG-014",
				fmt.Sprintf("File '%s' is a directory: empty directories are not allowed in an EPUB container", f.Name))
		}
	}
}

// PKG-025: publication resources must not be stored in META-INF
func checkNoResourcesInMetaInf(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil {
		return
	}
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		if strings.HasPrefix(fullPath, "META-INF/") {
			r.Add(report.Error, "PKG-025",
				fmt.Sprintf("Publication resources must not be located in the META-INF directory: '%s'", fullPath))
		}
	}
}

// OPF-060: duplicate filenames after Unicode case folding or NFC normalization
func checkDuplicateFilenames(ep *epub.EPUB, r *report.Report) {
	seen := make(map[string]string) // normalized → original
	for _, f := range ep.ZipFile.File {
		normalized := strings.ToLower(f.Name)
		if existing, ok := seen[normalized]; ok {
			if existing != f.Name {
				r.Add(report.Error, "OPF-060",
					fmt.Sprintf("Duplicate entry: file names must be unique after Unicode case folding: '%s' and '%s'", existing, f.Name))
			}
		} else {
			seen[normalized] = f.Name
		}
	}
}

// file paths should not exceed 65535 bytes
func checkFilenameLength(ep *epub.EPUB, r *report.Report) {
	for _, f := range ep.ZipFile.File {
		if len(f.Name) > 65535 {
			r.Add(report.Warning, "PKG-016",
				fmt.Sprintf("File path '%s...' exceeds recommended maximum of 65535 bytes", f.Name[:50]))
		}
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
