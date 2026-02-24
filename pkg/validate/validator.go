package validate

import (
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
	if fatal := checkOPF(ep, r); fatal {
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
	}

	// Post-processing: when not in Strict mode, downgrade certain warnings
	// to INFO for checks that epubcheck does not flag. This aligns output
	// with the epubverify-spec test suite while keeping the checks active
	// for doctor mode (which uses Strict).
	if !opts.Strict {
		r.DowngradeToInfo(divergenceChecks)
	}

	// Post-downgrade: emit OEBPS 1.2 legacy media type warnings AFTER DowngradeToInfo
	// so they are not downgraded to INFO. These are real EPUBCheck warnings.
	if ep.IsLegacyOEBPS12 && ep.Package != nil && ep.Package.Version != "" {
		checkLegacyOEBPS12MediaTypes(ep.Package, r)
	}

	return r, nil
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
