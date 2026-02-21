package validate

import (
	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// Options configures validation behavior.
type Options struct {
	// Strict enables checks that follow the EPUB spec more closely,
	// even when the reference epubcheck tool doesn't flag them.
	// This includes OCF-005 (compressed mimetype) and RSC-002 (file not in manifest).
	Strict bool
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
		r.Add(report.Fatal, "PKG-000", "Could not open EPUB: "+err.Error())
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

	// Phase 3: Cross-reference checks
	checkReferences(ep, r, opts)

	// Phase 4: Navigation document checks
	checkNavigation(ep, r)

	// Phase 5: Content document checks
	checkContent(ep, r)

	// Phase 6: EPUB 2 specific checks
	checkEPUB2(ep, r)

	return r, nil
}
