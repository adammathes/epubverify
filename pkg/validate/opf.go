package validate

import (
	"fmt"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// checkOPF parses the OPF and runs all package document checks.
// Returns true if a fatal error prevents further processing.
func checkOPF(ep *epub.EPUB, r *report.Report) bool {
	if err := ep.ParseOPF(); err != nil {
		r.Add(report.Fatal, "OPF-000", "Could not parse package document: "+err.Error())
		return true
	}

	pkg := ep.Package

	// OPF-001: dc:title must be present
	checkDCTitle(pkg, r)

	// OPF-002: dc:identifier must be present
	checkDCIdentifier(pkg, r)

	// OPF-003: dc:language must be present
	checkDCLanguage(pkg, r)

	// OPF-004: dcterms:modified must be present (EPUB 3)
	checkDCTermsModified(pkg, r)

	// OPF-005: manifest item IDs must be unique
	checkManifestUniqueIDs(pkg, r)

	// OPF-006: manifest items must have href
	checkManifestHrefRequired(pkg, r)

	// OPF-007: manifest items must have media-type
	checkManifestMediaTypeRequired(pkg, r)

	// OPF-008: unique-identifier must resolve
	checkUniqueIdentifierResolves(pkg, r)

	// OPF-009: spine itemrefs must reference valid manifest items
	checkSpineIdrefResolves(pkg, r)

	// OPF-010: spine must not be empty
	checkSpineNotEmpty(pkg, r)

	return false
}

// OPF-001
func checkDCTitle(pkg *epub.Package, r *report.Report) {
	if len(pkg.Metadata.Titles) == 0 {
		r.Add(report.Error, "OPF-001", "Package metadata is missing required element dc:title")
	}
}

// OPF-002
func checkDCIdentifier(pkg *epub.Package, r *report.Report) {
	if len(pkg.Metadata.Identifiers) == 0 {
		r.Add(report.Error, "OPF-002", "Package metadata is missing required element dc:identifier")
	}
}

// OPF-003
func checkDCLanguage(pkg *epub.Package, r *report.Report) {
	if len(pkg.Metadata.Languages) == 0 {
		r.Add(report.Error, "OPF-003", "Package metadata is missing required element dc:language")
	}
}

// OPF-004
func checkDCTermsModified(pkg *epub.Package, r *report.Report) {
	if pkg.Version >= "3.0" && pkg.Metadata.Modified == "" {
		r.Add(report.Error, "OPF-004", "Package metadata is missing required element dcterms:modified")
	}
}

// OPF-005
func checkManifestUniqueIDs(pkg *epub.Package, r *report.Report) {
	seen := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID == "" {
			continue
		}
		if seen[item.ID] {
			r.Add(report.Error, "OPF-005",
				fmt.Sprintf("Duplicate manifest item id '%s'", item.ID))
		}
		seen[item.ID] = true
	}
}

// OPF-006
func checkManifestHrefRequired(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.Href == "\x00MISSING" {
			r.Add(report.Error, "OPF-006",
				fmt.Sprintf("Manifest item '%s' is missing required attribute 'href'", item.ID))
		}
	}
}

// OPF-007
func checkManifestMediaTypeRequired(pkg *epub.Package, r *report.Report) {
	for _, item := range pkg.Manifest {
		if item.MediaType == "\x00MISSING" {
			r.Add(report.Error, "OPF-007",
				fmt.Sprintf("Manifest item '%s' is missing required attribute 'media-type'", item.ID))
		}
	}
}

// OPF-008
func checkUniqueIdentifierResolves(pkg *epub.Package, r *report.Report) {
	if pkg.UniqueIdentifier == "" {
		r.Add(report.Error, "OPF-008", "Package element is missing unique-identifier attribute")
		return
	}
	for _, id := range pkg.Metadata.Identifiers {
		if id.ID == pkg.UniqueIdentifier {
			return
		}
	}
	r.Add(report.Error, "OPF-008",
		fmt.Sprintf("The unique-identifier '%s' was not found among dc:identifier elements", pkg.UniqueIdentifier))
}

// OPF-009
func checkSpineIdrefResolves(pkg *epub.Package, r *report.Report) {
	manifestIDs := make(map[string]bool)
	for _, item := range pkg.Manifest {
		if item.ID != "" {
			manifestIDs[item.ID] = true
		}
	}
	for _, ref := range pkg.Spine {
		if ref.IDRef == "" {
			continue
		}
		if !manifestIDs[ref.IDRef] {
			r.Add(report.Error, "OPF-009",
				fmt.Sprintf("Spine itemref '%s' not found in manifest", ref.IDRef))
		}
	}
}

// OPF-010
func checkSpineNotEmpty(pkg *epub.Package, r *report.Report) {
	if len(pkg.Spine) == 0 {
		r.Add(report.Error, "OPF-010", "The spine is incomplete: it must contain at least one itemref element")
	}
}
