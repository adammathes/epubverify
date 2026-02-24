package validate

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// Options configures validation behavior.
type Options struct {
	// Strict enables checks that follow the EPUB spec more closely,
	// even when the reference epubcheck tool doesn't flag them.
	// This includes PKG-005 (compressed mimetype) and RSC-002w (file not in manifest).
	Strict bool

	// Accessibility enables accessibility metadata and best-practice checks (ACC-*).
	// These are not flagged by epubcheck without --profile and are off by default.
	Accessibility bool

	// SingleFileMode enables single-file validation mode (e.g., validating a
	// standalone .opf wrapped in a minimal EPUB). Suppresses cross-reference
	// and content checks that don't apply to single-file validation.
	SingleFileMode bool
}

// Validate runs all validation checks on an EPUB file and returns a report.
func Validate(path string) (*report.Report, error) {
	return ValidateWithOptions(path, Options{})
}

// ValidateWithOptions runs validation with the given options.
func ValidateWithOptions(epubPath string, opts Options) (*report.Report, error) {
	r := report.NewReport()

	// PKG-016: warn if the .epub extension is not all lowercase
	ext := filepath.Ext(epubPath)
	if strings.EqualFold(ext, ".epub") && ext != ".epub" {
		r.Add(report.Warning, "PKG-016", "The '.epub' file extension should use lowercase characters")
	}

	ep, err := epub.Open(epubPath)
	if err != nil {
		errMsg := err.Error()
		// Differentiate between empty file (PKG-003), wrong file type (PKG-004), and
		// other ZIP errors (e.g. truncated ZIP with valid magic but no end directory).
		fi, statErr := os.Stat(epubPath)
		if statErr == nil && fi.Size() == 0 {
			// Empty file
			r.Add(report.Error, "PKG-003", "The EPUB publication must be a valid ZIP archive (zip file is empty)")
		} else if !hasZIPMagic(epubPath) {
			// File does not start with ZIP local file header magic → corrupted ZIP header
			r.Add(report.Fatal, "PKG-004", "Fatal error in opening ZIP container (corrupted ZIP header)")
		}
		// Always report PKG-008 for any ZIP open failure
		r.Add(report.Fatal, "PKG-008", "Unable to read EPUB file: "+errMsg)
		return r, nil
	}
	defer ep.Close()

	// Phase 1: OCF container checks
	if fatal := checkOCF(ep, r, opts); fatal {
		return r, nil
	}

	// Phase 2: Parse and check OPF
	if fatal := checkOPF(ep, r, opts); fatal {
		return r, nil
	}

	if !opts.SingleFileMode {
		// Phase 3: Cross-reference checks
		checkReferences(ep, r, opts)

		// Phase 4: Navigation document checks
		checkNavigation(ep, r)

		// Phase 5: Encoding checks (before content to identify bad files)
		badEncoding := checkEncoding(ep, r)

		// Phase 6: Content document checks
		checkContentWithSkips(ep, r, badEncoding)

		// Phase 7: CSS checks
		checkCSS(ep, r)

		// Phase 8: Fixed-layout checks
		checkFXL(ep, r)

		// Phase 9: Media checks
		checkMedia(ep, r)

		// Phase 10: EPUB 2 specific checks
		checkEPUB2(ep, r)

		// Phase 10b: Legacy NCX checks (for any publication with an NCX)
		checkLegacyNCXForAll(ep, r)

		// Phase 11: Accessibility checks (opt-in, not flagged by epubcheck without --profile)
		if opts.Accessibility {
			checkAccessibility(ep, r)
		}
	} else {
		// Single-file mode: run targeted content checks on XHTML/SVG content
		checkSingleFileContent(ep, r)
	}

	// Post-processing: in single-file mode, remap specific check IDs to RSC-005
	// to match EPUBCheck's schema validation behavior.
	if opts.SingleFileMode {
		r.RemapToRSC005(rsc005Mapping)
	}

	// Post-processing: when not in Strict mode, downgrade certain warnings
	// to INFO for checks that epubcheck does not flag. This aligns output
	// with the epubverify-spec test suite while keeping the checks active
	// for doctor mode (which uses Strict).
	if !opts.Strict {
		r.DowngradeToInfo(divergenceChecks)
	}

	// Post-downgrade: emit legacy media type warnings AFTER DowngradeToInfo
	// so they are not downgraded to INFO. These are real EPUBCheck warnings.
	if ep.Package != nil {
		if ep.IsLegacyOEBPS12 && ep.Package.Version != "" {
			checkLegacyOEBPS12MediaTypes(ep.Package, r)
		} else if ep.Package.Version < "3.0" {
			// Also check EPUB 2 packages for deprecated OEB media types
			checkLegacyOEBPS12MediaTypes(ep.Package, r)
		}
	}

	return r, nil
}

// rsc005Mapping maps specific check IDs to RSC-005 with EPUBCheck-compatible
// messages. Each entry transforms the original message text into the format
// expected by EPUBCheck's schema validation (RelaxNG).
var rsc005Mapping = map[string]func(string) string{
	"OPF-001": func(msg string) string {
		if strings.Contains(msg, "dc:title") {
			return `missing required element "dc:title"`
		}
		if strings.Contains(msg, "version") {
			return `missing required attribute "version"`
		}
		return msg
	},
	"OPF-002": func(msg string) string {
		return `missing required element "dc:identifier"`
	},
	"OPF-003": func(msg string) string {
		return `missing required element "dc:language"`
	},
	"OPF-004": func(msg string) string {
		return `missing required element "dcterms:modified"`
	},
	"OPF-007": func(msg string) string {
		return `missing required attribute "media-type"`
	},
	"OPF-010": func(msg string) string {
		return `missing required element "itemref"`
	},
	"OPF-019": func(msg string) string {
		return `value of property "dcterms:modified" must be of the form "CCYY-MM-DDThh:mm:ssZ"`
	},
	"OPF-030": func(msg string) string {
		return `"unique-identifier" attribute does not resolve to a dc:identifier element`
	},
	"OPF-031": func(msg string) string {
		return `must be a string with length at least 1`
	},
	"OPF-032": func(msg string) string {
		return `must be a string with length at least 1`
	},
	"OPF-034": func(msg string) string {
		return `Itemref refers to the same manifest entry as a previous itemref`
	},
	"OPF-008": func(msg string) string {
		return `missing required attribute "unique-identifier"`
	},
	"OPF-037": func(msg string) string {
		if strings.Contains(msg, "must be a relative URL") {
			return `@refines must be a relative URL`
		}
		if strings.Contains(msg, "refines missing target") {
			return `@refines missing target id`
		}
		return "" // don't remap other OPF-037 messages (e.g., deprecated media types)
	},
	"OPF-042": func(msg string) string {
		return `The value of the "rendition:flow" property must be either "paginated", "scrolled-doc", "scrolled-continuous", or "auto"`
	},
	"OPF-044": func(msg string) string {
		return `The media-overlay attribute must refer to an item with media type "application/smil+xml"; must be of the "application/smil+xml" type`
	},
	"OPF-012": func(msg string) string {
		if strings.Contains(msg, "at most once") {
			return `The 'cover-image' property must not occur more than one time`
		}
		return "" // don't remap other OPF-012 messages
	},
	"OPF-050": func(msg string) string {
		if strings.Contains(msg, "toc attribute must be set") {
			return `When an NCX document is included in the EPUB, the toc attribute must be set on the spine element`
		}
		return "" // don't remap other OPF-050 messages
	},
	"OPF-025a": func(msg string) string {
		return `value of attribute "scheme" is invalid; must be an NMTOKEN`
	},
}

// divergenceChecks lists check IDs where epubverify flags issues that
// epubcheck 5.3.0 does not. In non-Strict mode these are downgraded
// from WARNING to INFO so they don't affect warning_count.
var divergenceChecks = map[string]bool{
	"HTM-003":  true, // empty href attribute
	"HTM-009":  true, // base element
	"HTM-021":  true, // position:absolute in inline style
	"NAV-009":  true, // hidden attribute on nav
	"CSS-005":  true, // @import rules
	"CSS-011":  true, // @font-face missing src
	"OPF-039":  true, // deprecated guide element in EPUB 3
	"MED-012":  true, // video non-core media type
	"E2-012":   true, // invalid guide reference type
	"E2-015":   true, // NCX depth mismatch
}

// ValidateFile validates a single file (.opf, .xhtml, .svg, .smil) by wrapping
// it in a minimal EPUB container. Returns a validation report.
func ValidateFile(filePath string) (*report.Report, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	name := filepath.Base(filePath)

	switch ext {
	case ".opf":
		return validateSingleOPF(name, data)
	case ".xhtml":
		return validateSingleXHTML(name, data)
	case ".svg":
		return validateSingleXHTML(name, data) // SVG uses same wrapper
	case ".smil":
		return validateSingleSMIL(name, data)
	default:
		return nil, fmt.Errorf("unsupported single-file type: %s", ext)
	}
}

// validateSingleOPF wraps an OPF file in a minimal EPUB and validates it.
func validateSingleOPF(name string, data []byte) (*report.Report, error) {
	container := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="EPUB/%s" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`, name)

	files := map[string][]byte{
		"mimetype":               []byte("application/epub+zip"),
		"META-INF/container.xml": []byte(container),
		"EPUB/" + name:           data,
	}

	tmpPath, err := createTempEPUB(files)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpPath)

	return ValidateWithOptions(tmpPath, Options{SingleFileMode: true})
}

// validateSingleXHTML wraps an XHTML/SVG file in a minimal EPUB for content validation.
func validateSingleXHTML(name string, data []byte) (*report.Report, error) {
	// Determine media type from extension
	mediaType := "application/xhtml+xml"
	if strings.HasSuffix(strings.ToLower(name), ".svg") {
		mediaType = "image/svg+xml"
	}

	opf := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" xml:lang="en" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>Single File Validation</dc:title>
  <dc:language>en</dc:language>
  <dc:identifier id="uid">urn:uuid:00000000-0000-0000-0000-000000000000</dc:identifier>
  <meta property="dcterms:modified">2000-01-01T00:00:00Z</meta>
</metadata>
<manifest>
  <item id="content" href="%s" media-type="%s" properties="nav"/>
</manifest>
<spine>
  <itemref idref="content"/>
</spine>
</package>`, name, mediaType)

	container := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	files := map[string][]byte{
		"mimetype":               []byte("application/epub+zip"),
		"META-INF/container.xml": []byte(container),
		"EPUB/package.opf":       []byte(opf),
		"EPUB/" + name:           data,
	}

	tmpPath, err := createTempEPUB(files)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpPath)

	return ValidateWithOptions(tmpPath, Options{SingleFileMode: true})
}

// validateSingleSMIL wraps a SMIL file in a minimal EPUB for media overlay validation.
func validateSingleSMIL(name string, data []byte) (*report.Report, error) {
	opf := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" xml:lang="en" unique-identifier="uid">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>Single File Validation</dc:title>
  <dc:language>en</dc:language>
  <dc:identifier id="uid">urn:uuid:00000000-0000-0000-0000-000000000000</dc:identifier>
  <meta property="dcterms:modified">2000-01-01T00:00:00Z</meta>
</metadata>
<manifest>
  <item id="overlay" href="%s" media-type="application/smil+xml"/>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
</manifest>
<spine>
  <itemref idref="nav"/>
</spine>
</package>`, name)

	nav := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Nav</title></head>
<body><nav epub:type="toc"><ol><li><a href="#">Start</a></li></ol></nav></body>
</html>`

	container := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	files := map[string][]byte{
		"mimetype":               []byte("application/epub+zip"),
		"META-INF/container.xml": []byte(container),
		"EPUB/package.opf":       []byte(opf),
		"EPUB/nav.xhtml":         []byte(nav),
		"EPUB/" + name:           data,
	}

	tmpPath, err := createTempEPUB(files)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpPath)

	return ValidateWithOptions(tmpPath, Options{SingleFileMode: true})
}

// createTempEPUB creates a temporary EPUB file from a map of paths to data.
func createTempEPUB(files map[string][]byte) (string, error) {
	tmp, err := os.CreateTemp("", "epubverify-single-*.epub")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	w := zip.NewWriter(tmp)

	// Write mimetype first (stored, not compressed)
	if mt, ok := files["mimetype"]; ok {
		header := &zip.FileHeader{
			Name:   "mimetype",
			Method: zip.Store,
		}
		mw, err := w.CreateHeader(header)
		if err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return "", err
		}
		mw.Write(mt)
	}

	// Write remaining files
	for name, data := range files {
		if name == "mimetype" {
			continue
		}
		fw, err := w.Create(name)
		if err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return "", err
		}
		fw.Write(data)
	}

	if err := w.Close(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	tmp.Close()

	return tmpPath, nil
}

// hasZIPMagic returns true if the file starts with the ZIP local file header signature (PK\x03\x04).
func hasZIPMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return false
	}
	return magic[0] == 0x50 && magic[1] == 0x4B && magic[2] == 0x03 && magic[3] == 0x04
}
