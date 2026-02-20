package validate

import (
	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// Validate runs all validation checks on an EPUB file and returns a report.
func Validate(path string) (*report.Report, error) {
	r := report.NewReport()

	ep, err := epub.Open(path)
	if err != nil {
		r.Add(report.Fatal, "PKG-000", "Could not open EPUB: "+err.Error())
		return r, nil
	}
	defer ep.Close()

	// Phase 1: OCF container checks
	if fatal := checkOCF(ep, r); fatal {
		return r, nil
	}

	// Phase 2: Parse and check OPF
	if fatal := checkOPF(ep, r); fatal {
		return r, nil
	}

	// Phase 3: Cross-reference checks
	checkReferences(ep, r)

	// Phase 4: Content document checks
	checkContent(ep, r)

	return r, nil
}
