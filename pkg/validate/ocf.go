package validate

import (
	"archive/zip"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
	"golang.org/x/text/unicode/norm"
)

// checkOCF runs all OCF container checks. Returns true if a fatal error
// was found that prevents further processing.
func checkOCF(ep *epub.EPUB, r *report.Report, opts Options) bool {
	fatal := false

	// PKG-027: all file names must be valid UTF-8 (check first, fatal if found)
	if checkFilenameUTF8(ep, r) {
		return true
	}

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

	// RSC-005: container.xml must have a valid content model (no unknown elements)
	checkContainerContentModel(ep, r)

	// container.xml must have a rootfile
	if !checkContainerHasRootfile(ep, r) {
		fatal = true
	}

	// OPF-002: rootfile target must exist
	if !fatal && !checkRootfileExists(ep, r) {
		return true
	}

	// encryption.xml and signatures.xml checks
	checkEncryptionXMLFull(ep, r)
	checkSignaturesXML(ep, r)

	// all rootfiles must exist
	if checkAllRootfilesExist(ep, r) {
		return true
	}

	// RSC-003: rootfile media-type must be correct
	checkRootfileMediaType(ep, r)

	// container.xml version must be 1.0
	checkContainerVersion(ep, r)

	// PKG-009: filenames must not contain restricted characters
	checkFilenameValidChars(ep, r)

	// PKG-010: warn about spaces in file names
	checkFilenameSpaces(ep, r)

	// PKG-011: filenames must not end with a full stop
	checkFilenameTrailingDot(ep, r)

	// PKG-012: usage note for non-ASCII characters in file names
	checkFilenameNonASCII(ep, r)

	// PKG-014: warn about empty directories
	checkEmptyDirectories(ep, r)

	// PKG-025: checked in references phase after OPF is loaded

	// OPF-060: duplicate filenames after case folding / NFC normalization
	checkDuplicateFilenames(ep, r)

	// file paths should not exceed 65535 bytes
	checkFilenameLength(ep, r)

	return fatal
}

// PKG-027: all ZIP entry names must be valid UTF-8.
// Reports PKG-027 once (for the first invalid entry) and returns true (fatal condition).
func checkFilenameUTF8(ep *epub.EPUB, r *report.Report) bool {
	for _, f := range ep.ZipFile.File {
		if !utf8.ValidString(f.Name) {
			r.Add(report.Fatal, "PKG-027", "File name is not a valid UTF-8 encoded string")
			return true
		}
	}
	return false
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

// checkContainerContentModel validates that container.xml only contains allowed elements.
// The OCF schema allows only: container > rootfiles > rootfile, and container > links > link.
func checkContainerContentModel(ep *epub.EPUB, r *report.Report) {
	if ep.ContainerData == nil {
		return
	}
	containerNS := "urn:oasis:names:tc:opendocument:xmlns:container"
	// Track element stack to know the parent context
	var stack []string
	decoder := xml.NewDecoder(strings.NewReader(string(ep.ContainerData)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			ns := t.Name.Space
			// Only validate elements in the container namespace (or no namespace)
			if ns != "" && ns != containerNS {
				stack = append(stack, local)
				continue
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			allowed := isAllowedContainerElement(local, parent)
			if !allowed {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("container.xml: element \"%s\" not allowed anywhere", local))
			}
			stack = append(stack, local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

// isAllowedContainerElement checks if an element local name is allowed in the OCF container schema.
func isAllowedContainerElement(local, parent string) bool {
	switch parent {
	case "":
		return local == "container"
	case "container":
		return local == "rootfiles" || local == "links"
	case "rootfiles":
		return local == "rootfile"
	case "links":
		return local == "link"
	default:
		return false
	}
}

// checkRootfileFullPathAttributes checks rootfile elements for missing/empty full-path attributes.
// Returns (hasValidRootfile, emittedAttributeError) to control subsequent checks.
func checkRootfileFullPathAttributes(ep *epub.EPUB, r *report.Report) (hasValidRootfile bool, emittedAttrError bool) {
	if ep.ContainerData == nil {
		return false, false
	}
	decoder := xml.NewDecoder(strings.NewReader(string(ep.ContainerData)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "rootfile" {
			continue
		}
		// Check if full-path attribute is present and non-empty
		hasFullPath := false
		fullPathEmpty := false
		for _, attr := range se.Attr {
			if attr.Name.Local == "full-path" {
				hasFullPath = true
				if attr.Value == "" {
					fullPathEmpty = true
				}
				break
			}
		}
		if !hasFullPath {
			r.Add(report.Error, "OPF-016",
				"The rootfile element is missing the required 'full-path' attribute")
			emittedAttrError = true
		} else if fullPathEmpty {
			r.Add(report.Error, "OPF-017",
				"The rootfile element has an empty 'full-path' attribute")
			emittedAttrError = true
		} else {
			hasValidRootfile = true
		}
	}
	return hasValidRootfile, emittedAttrError
}

// PKG-013: Only one OPF rootfile is allowed
func checkSingleOPFRootfile(ep *epub.EPUB, r *report.Report) {
	count := 0
	for _, rf := range ep.AllRootfiles {
		if rf.MediaType == "application/oebps-package+xml" {
			count++
		}
	}
	if count > 1 {
		r.Add(report.Error, "PKG-013",
			"Only one OPF rootfile is allowed in the container")
	}
}

// container.xml must contain a rootfile element
func checkContainerHasRootfile(ep *epub.EPUB, r *report.Report) bool {
	if ep.RootfilePath == "" {
		// Check for missing/empty full-path attributes
		hasValid, emittedAttrError := checkRootfileFullPathAttributes(ep, r)
		if emittedAttrError {
			// When full-path is missing/empty, also emit RSC-003 because the
			// rootfile can't be resolved to a valid OPF file
			r.Add(report.Error, "RSC-003",
				"No rootfile tag with media type 'application/oebps-package+xml' found")
		} else if !hasValid {
			r.Add(report.Error, "RSC-005", "container.xml does not contain a rootfile element")
		}
		return false
	}
	// Even with a valid rootfile, check for attribute issues on other rootfiles
	checkRootfileFullPathAttributes(ep, r)
	// Check for multiple OPF rootfiles
	checkSingleOPFRootfile(ep, r)
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

// checkEncryptionXMLFull validates META-INF/encryption.xml per the EPUB spec:
//   - RSC-004 INFO: encryption is present (and valid)
//   - RSC-005 ERROR: content model errors, duplicate IDs, invalid compression metadata
func checkEncryptionXMLFull(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["META-INF/encryption.xml"]
	if !exists {
		return
	}
	data, err := ep.ReadFile("META-INF/encryption.xml")
	if err != nil {
		return
	}

	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	rootChecked := false
	idCounts := make(map[string]int)
	var inEncProp bool

	wellFormed := true
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			wellFormed = false
			break
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			if ee, ok2 := tok.(xml.EndElement); ok2 {
				if ee.Name.Local == "EncryptionProperty" {
					inEncProp = false
				}
			}
			continue
		}

		local := se.Name.Local

		// RSC-005: root element must be "encryption"
		if !rootChecked {
			rootChecked = true
			if local != "encryption" {
				r.Add(report.Error, "RSC-005",
					fmt.Sprintf("META-INF/encryption.xml: expected element \"encryption\" but found \"%s\"", local))
				return
			}
		}

		// Track IDs for duplicate check (report for each element with a duplicate ID)
		for _, attr := range se.Attr {
			if attr.Name.Local == "Id" {
				id := attr.Value
				idCounts[id]++
				if idCounts[id] == 2 {
					// Report for both occurrences when first duplicate is found
					r.Add(report.Error, "RSC-005",
						fmt.Sprintf("META-INF/encryption.xml: Duplicate ID \"%s\"", id))
					r.Add(report.Error, "RSC-005",
						fmt.Sprintf("META-INF/encryption.xml: Duplicate ID \"%s\"", id))
				} else if idCounts[id] > 2 {
					r.Add(report.Error, "RSC-005",
						fmt.Sprintf("META-INF/encryption.xml: Duplicate ID \"%s\"", id))
				}
			}
		}

		switch local {
		case "EncryptionProperty":
			inEncProp = true
		case "Compression":
			if inEncProp {
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "Method":
						if attr.Value != "0" && attr.Value != "8" {
							r.Add(report.Error, "RSC-005",
								fmt.Sprintf("META-INF/encryption.xml: value of attribute \"Method\" is invalid: \"%s\"", attr.Value))
						}
					case "OriginalLength":
						if strings.TrimSpace(attr.Value) == "" {
							r.Add(report.Error, "RSC-005",
								"META-INF/encryption.xml: value of attribute \"OriginalLength\" is invalid: must not be empty")
						}
					}
				}
			}
		}
	}

	if !wellFormed {
		return
	}

	// RSC-004 INFO: encryption is present and appears valid
	r.Add(report.Info, "RSC-004",
		"META-INF/encryption.xml is present; encryption support may limit validation")
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

// checkSignaturesXML validates META-INF/signatures.xml content model.
// RSC-005 is reported if the root element is not "signatures".
func checkSignaturesXML(ep *epub.EPUB, r *report.Report) {
	_, exists := ep.Files["META-INF/signatures.xml"]
	if !exists {
		return
	}
	data, err := ep.ReadFile("META-INF/signatures.xml")
	if err != nil {
		return
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local != "signatures" {
			r.Add(report.Error, "RSC-005",
				fmt.Sprintf("META-INF/signatures.xml: expected element \"signatures\" but found \"%s\"", se.Name.Local))
		}
		return
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

// isFilenameSpaceChar returns true if the character is a space character per the EPUB spec.
// Space characters get PKG-010 (warning) rather than PKG-009 (error).
func isFilenameSpaceChar(c rune) bool {
	switch c {
	case 0x0009, // TAB
		0x000A, // LF
		0x000C, // FF
		0x000D, // CR
		0x0020, // SPACE
		0x2009: // THIN SPACE
		return true
	}
	return false
}

// formatCodePoint formats a rune for PKG-009 error messages: "U+XXXX (desc)"
func formatCodePoint(c rune) string {
	switch {
	case c == '"':
		return "U+0022 (\")"
	case c == '*':
		return "U+002A (*)"
	case c == ':':
		return "U+003A (:)"
	case c == '<':
		return "U+003C (<)"
	case c == '>':
		return "U+003E (>)"
	case c == '?':
		return "U+003F (?)"
	case c == '\\':
		return "U+005C (\\)"
	case c == '|':
		return "U+007C (|)"
	case c == 0x7F:
		return "U+007F (CONTROL)"
	case c >= 0x80 && c <= 0x9F:
		return fmt.Sprintf("U+%04X (CONTROL)", c)
	case c == 0xFFFD:
		return "U+FFFD REPLACEMENT CHARACTER (SPECIALS)"
	case c >= 0xE000 && c <= 0xF8FF:
		return fmt.Sprintf("U+%04X (PRIVATE USE)", c)
	case c >= 0xF0000 && c <= 0xFFFFF:
		return fmt.Sprintf("U+%05X (PRIVATE USE)", c)
	case c >= 0x100000 && c <= 0x10FFFF:
		return fmt.Sprintf("U+%06X (PRIVATE USE)", c)
	case c >= 0xFDD0 && c <= 0xFDEF:
		return fmt.Sprintf("U+%04X (NON CHARACTER)", c)
	case c == 0xFFFE || c == 0xFFFF:
		return fmt.Sprintf("U+%04X (NON CHARACTER)", c)
	case (c&0xFFFF) == 0xFFFE || (c&0xFFFF) == 0xFFFF:
		return fmt.Sprintf("U+%X (NON CHARACTER)", c)
	case c == 0xE0001:
		return "U+E0001 LANGUAGE TAG (DEPRECATED)"
	case c < 0x20:
		return fmt.Sprintf("U+%04X (CONTROL)", c)
	default:
		return fmt.Sprintf("U+%04X (%c)", c, c)
	}
}

// ValidateFilenameString validates a single filename string (without a full EPUB).
// Used by the filename-checker step definitions. Checks PKG-009, PKG-010, PKG-011.
func ValidateFilenameString(name string, epub2 bool) *report.Report {
	r := report.NewReport()

	// PKG-010: warn about space characters in filenames
	for _, c := range name {
		if isFilenameSpaceChar(c) {
			r.Add(report.Warning, "PKG-010",
				fmt.Sprintf("Filename contains spaces, which is discouraged: '%s'", name))
			break
		}
	}

	// PKG-009: forbidden characters (skip space chars — they get PKG-010 instead)
	seen := make(map[rune]bool)
	var forbidden []rune
	for _, c := range name {
		if isFilenameSpaceChar(c) {
			continue
		}
		if isForbiddenFilenameChar(c, epub2) && !seen[c] {
			seen[c] = true
			forbidden = append(forbidden, c)
		}
	}
	if len(forbidden) > 0 {
		var parts []string
		for _, c := range forbidden {
			parts = append(parts, formatCodePoint(c))
		}
		r.Add(report.Error, "PKG-009",
			fmt.Sprintf("File name contains characters forbidden in OCF file names: %s.", strings.Join(parts, ", ")))
	}

	// PKG-011: filename must not end with a full stop
	if strings.HasSuffix(name, ".") {
		r.Add(report.Error, "PKG-011",
			fmt.Sprintf("File name must not end with a full stop: '%s'", name))
	}

	return r
}

// isForbiddenFilenameChar returns true if the character is forbidden in EPUB file names (PKG-009).
// In EPUB 2, | is not forbidden; in EPUB 3 it is.
func isForbiddenFilenameChar(c rune, epub2 bool) bool {
	// Control characters < 0x20
	if c < 0x20 {
		return true
	}
	// DEL (0x7F) and C1 controls (0x80-0x9F)
	if c == 0x7F || (c >= 0x80 && c <= 0x9F) {
		return true
	}
	// Forbidden printable ASCII chars
	switch c {
	case '"', '*', ':', '<', '>', '?', '\\':
		return true
	case '|':
		return !epub2 // | is forbidden in EPUB 3 only
	}
	// Unicode non-characters: U+FDD0-U+FDEF
	if c >= 0xFDD0 && c <= 0xFDEF {
		return true
	}
	// Unicode non-characters: U+FFFE, U+FFFF and similar in higher planes
	if (c & 0xFFFF) == 0xFFFE || (c & 0xFFFF) == 0xFFFF {
		return true
	}
	// Unicode replacement character
	if c == 0xFFFD {
		return true
	}
	// Private use areas: U+E000-U+F8FF, U+F0000-U+FFFFF, U+100000-U+10FFFF
	if (c >= 0xE000 && c <= 0xF8FF) ||
		(c >= 0xF0000 && c <= 0xFFFFF) ||
		(c >= 0x100000 && c <= 0x10FFFF) {
		return true
	}
	// U+E0001 LANGUAGE TAG is deprecated and forbidden
	// Note: U+E0020-U+E007F (tag characters for emoji tag sequences) are ALLOWED
	if c == 0xE0001 {
		return true
	}
	return false
}

// PKG-009: filenames must not contain restricted characters
func checkFilenameValidChars(ep *epub.EPUB, r *report.Report) {
	isEPUB2 := ep.Package != nil && ep.Package.Version == "2.0"
	for _, f := range ep.ZipFile.File {
		// Collect all unique forbidden chars in this filename
		seen := make(map[rune]bool)
		var forbidden []rune
		for _, c := range f.Name {
			if isForbiddenFilenameChar(c, isEPUB2) && !seen[c] {
				seen[c] = true
				forbidden = append(forbidden, c)
			}
		}
		if len(forbidden) == 0 {
			continue
		}
		// Format the list of forbidden chars
		var parts []string
		for _, c := range forbidden {
			parts = append(parts, formatCodePoint(c))
		}
		msg := strings.Join(parts, ", ")
		r.Add(report.Error, "PKG-009",
			fmt.Sprintf("File name contains characters forbidden in OCF file names: %s", msg))
	}
}

// PKG-010: warn about spaces in file names (any Unicode space character)
func checkFilenameSpaces(ep *epub.EPUB, r *report.Report) {
	for _, f := range ep.ZipFile.File {
		if f.Name == "mimetype" {
			continue
		}
		for _, c := range f.Name {
			if isFilenameSpaceChar(c) {
				r.Add(report.Warning, "PKG-010",
					fmt.Sprintf("Filename contains spaces, which is discouraged: '%s'", f.Name))
				break
			}
		}
	}
}

// PKG-011: filenames must not end with a full stop (.)
func checkFilenameTrailingDot(ep *epub.EPUB, r *report.Report) {
	for _, f := range ep.ZipFile.File {
		if strings.HasSuffix(f.Name, ".") {
			r.Add(report.Error, "PKG-011",
				fmt.Sprintf("File name must not end with a full stop: '%s'", f.Name))
		}
	}
}

// PKG-012: usage note for non-ASCII characters in file names
func checkFilenameNonASCII(ep *epub.EPUB, r *report.Report) {
	for _, f := range ep.ZipFile.File {
		for _, c := range f.Name {
			if c > 0x7F {
				r.Add(report.Usage, "PKG-012",
					fmt.Sprintf("Filename contains non-ASCII characters, which may cause interoperability issues: '%s'", f.Name))
				break
			}
		}
	}
}

// PKG-014: warn about empty directories in the container
func checkEmptyDirectories(ep *epub.EPUB, r *report.Report) {
	// Build a set of all file names (non-directory entries) for quick lookup
	fileNames := make(map[string]bool)
	for _, f := range ep.ZipFile.File {
		if !strings.HasSuffix(f.Name, "/") {
			fileNames[f.Name] = true
		}
	}
	for _, f := range ep.ZipFile.File {
		if !strings.HasSuffix(f.Name, "/") || f.UncompressedSize64 != 0 {
			continue
		}
		// Check if any file has this directory as a prefix (i.e., is a child)
		hasChildren := false
		for name := range fileNames {
			if strings.HasPrefix(name, f.Name) {
				hasChildren = true
				break
			}
		}
		if !hasChildren {
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
		if item.Href == "\x00MISSING" || item.Href == "" {
			continue
		}
		// Skip remote resources
		if strings.HasPrefix(item.Href, "http://") || strings.HasPrefix(item.Href, "https://") {
			continue
		}
		// ResolveHref already normalizes ".." segments via path.Clean
		fullPath := ep.ResolveHref(item.Href)
		if strings.HasPrefix(fullPath, "META-INF/") {
			r.Add(report.Error, "PKG-025",
				fmt.Sprintf("Publication resources must not be located in the META-INF directory: '%s'", fullPath))
		}
	}
}

// fullCaseFold applies a simple full Unicode case folding.
// strings.ToLower handles most cases but misses ß → ss.
func fullCaseFold(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "ß", "ss")
	s = strings.ReplaceAll(s, "ﬀ", "ff")
	s = strings.ReplaceAll(s, "ﬁ", "fi")
	s = strings.ReplaceAll(s, "ﬂ", "fl")
	s = strings.ReplaceAll(s, "ﬃ", "ffi")
	s = strings.ReplaceAll(s, "ﬄ", "ffl")
	s = strings.ReplaceAll(s, "ﬅ", "st")
	s = strings.ReplaceAll(s, "ﬆ", "st")
	return s
}

// OPF-060: duplicate filenames after Unicode case folding or NFC normalization
func checkDuplicateFilenames(ep *epub.EPUB, r *report.Report) {
	// We check duplicates using NFC normalization + full case folding.
	// NFC normalization catches files like "Á" (NFC) and "Á" (NFD decomposed).
	// Full case folding catches ß → ss and other Unicode case folding.
	seen := make(map[string]string) // normalized key → original filename
	reported := make(map[string]bool) // avoid duplicate reports

	for _, f := range ep.ZipFile.File {
		name := f.Name
		// Normalize to NFC first, then apply full case folding
		key := fullCaseFold(norm.NFC.String(name))

		if existing, ok := seen[key]; ok {
			reportKey := existing + "|" + name
			if existing != name && !reported[reportKey] {
				reported[reportKey] = true
				r.Add(report.Error, "OPF-060",
					fmt.Sprintf("Duplicate entry: file names must be unique after Unicode case folding: '%s' and '%s'", existing, name))
			}
		} else {
			seen[key] = name
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
