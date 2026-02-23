package validate

import (
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
func ValidateWithOptions(path string, opts Options) (*report.Report, error) {
	r := report.NewReport()

	ep, err := epub.Open(path)
	if err != nil {
		errMsg := err.Error()
		// PKG-008: Unable to read EPUB file (ZIP error)
		if strings.Contains(errMsg, "zip") || strings.Contains(errMsg, "EOF") ||
			strings.Contains(errMsg, "not a valid") || strings.Contains(errMsg, "opening epub") {
			r.Add(report.Fatal, "PKG-008", "Unable to read EPUB file: "+errMsg)
		} else {
			r.Add(report.Fatal, "PKG-008", "Unable to read EPUB file: "+errMsg)
		}
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

	return r, nil
}

// divergenceChecks lists check IDs where epubverify flags issues that
// epubcheck 5.3.0 does not. In non-Strict mode these are downgraded
// from WARNING to INFO so they don't affect warning_count.
var divergenceChecks = map[string]bool{
	"RSC-002w": true, // file in container not in manifest (our custom warning)
	"HTM-003":  true, // empty href attribute
	"HTM-009":  true, // base element
	"HTM-021":  true, // position:absolute in inline style
	"NAV-009":  true, // hidden attribute on nav
	"CSS-003":  true, // @font-face missing src
	"CSS-005":  true, // @import rules
	"OPF-039":  true, // deprecated guide element in EPUB 3
	"MED-012":  true, // video non-core media type
	"E2-012":   true, // invalid guide reference type
	"E2-015":   true, // NCX depth mismatch
}
