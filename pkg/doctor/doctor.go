// Package doctor implements an EPUB repair mode ("doctor") that applies
// safe, mechanical fixes for common validation errors.
//
// The approach:
//  1. Open the EPUB and read all files into memory
//  2. Run the standard validator to identify problems
//  3. Apply Tier 1 fixes (safe, deterministic, content-preserving)
//  4. Write a new EPUB with all fixes applied
//  5. Re-validate the output to confirm fixes worked
//
// Tier 1 fixes (safe, deterministic, content-preserving):
//   - PKG-006/007/005: mimetype file issues — all handled by correct ZIP writing
//   - OPF-004: missing dcterms:modified — adds current timestamp
//   - OPF-024/MED-001: media-type mismatch — corrects based on file magic bytes
//   - HTM-005/006/007: missing manifest properties — adds scripted/svg/mathml
//   - HTM-010/011: wrong DOCTYPE — replaces with <!DOCTYPE html>
//
// Tier 2 fixes (low-to-medium complexity, still safe):
//   - OPF-039: deprecated <guide> element in EPUB 3 — removes it
//   - OPF-036: bad dc:date format — reformats to W3CDTF
//   - RSC-002: files in container but not in manifest — adds manifest entries
//   - HTM-003: empty href="" on <a> elements — removes the href attribute
//   - HTM-004: obsolete HTML elements (center, big, strike, tt, etc.) — replaces with styled modern equivalents
//
// Tier 3 fixes (higher complexity):
//   - CSS-005: @import rules — inlines imported CSS content
//   - ENC-001: non-UTF-8 encoding declaration — transcodes (iso-8859-1, windows-1252) or fixes declaration
//   - ENC-002: UTF-16 encoded content — transcodes to UTF-8
//
// Tier 4 fixes (cleanup and consistency):
//   - OPF-028: multiple dcterms:modified — removes duplicates
//   - OPF-033: fragment in manifest href — strips fragment identifier
//   - OPF-017: duplicate spine idrefs — removes duplicate itemrefs
//   - OPF-038: invalid spine linear value — normalizes to "yes"/"no"
//   - HTM-009: <base> element present — removes it
//   - HTM-020: processing instructions — removes non-XML PIs
//   - HTM-026: lang/xml:lang mismatch — syncs lang to match xml:lang
//   - HTM-002: missing <title> element — adds <title>Untitled</title>
package doctor

import (
	"fmt"
	"io"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
	"github.com/adammathes/epubverify/pkg/validate"
)

// Result holds the outcome of a doctor run.
type Result struct {
	Fixes       []Fix
	BeforeReport *report.Report
	AfterReport  *report.Report
}

// Repair opens an EPUB, applies fixes, and writes the repaired version.
// If outputPath is empty, it writes to inputPath with a ".fixed.epub" suffix.
func Repair(inputPath, outputPath string) (*Result, error) {
	if outputPath == "" {
		outputPath = inputPath + ".fixed.epub"
	}

	// Step 1: Open and validate original
	ep, err := epub.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("opening epub: %w", err)
	}

	beforeReport, err := validate.ValidateWithOptions(inputPath, validate.Options{Strict: true})
	if err != nil {
		ep.Close()
		return nil, fmt.Errorf("validating: %w", err)
	}

	// If already valid with no warnings or usage notes, nothing to do
	if beforeReport.IsValid() && beforeReport.WarningCount() == 0 && beforeReport.UsageCount() == 0 {
		ep.Close()
		return &Result{
			BeforeReport: beforeReport,
			AfterReport:  beforeReport,
		}, nil
	}

	// Step 2: Read all files into memory
	files := make(map[string][]byte)
	for name, f := range ep.Files {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		files[name] = data
	}

	// Need to parse container and OPF for fix functions
	// (the ep already has these parsed from Open + validate)
	ep.ParseContainer()
	ep.ParseOPF()

	// Step 3: Apply fixes
	var allFixes []Fix

	// ZIP-level: ensure correct mimetype (also fixes PKG-006 if missing)
	allFixes = append(allFixes, fixMimetype(files)...)

	// Detect ZIP-structural issues fixed by construction (the writer always
	// writes mimetype first, stored, with no extra field).
	allFixes = append(allFixes, detectZipFixes(beforeReport)...)

	// OPF-level: add missing dcterms:modified
	allFixes = append(allFixes, fixDCTermsModified(files, ep)...)

	// OPF-level: correct media-type mismatches
	allFixes = append(allFixes, fixMediaTypes(files, ep)...)

	// OPF-level: add missing manifest properties (scripted/svg/mathml)
	allFixes = append(allFixes, fixManifestProperties(files, ep)...)

	// Content-level: fix DOCTYPE declarations
	allFixes = append(allFixes, fixDoctype(files, ep)...)

	// --- Tier 2 fixes ---

	// OPF-level: remove deprecated <guide> element (EPUB 3)
	allFixes = append(allFixes, fixGuideElement(files, ep)...)

	// OPF-level: reformat bad dc:date values
	allFixes = append(allFixes, fixDCDateFormat(files, ep)...)

	// OPF-level: add unlisted container files to manifest
	allFixes = append(allFixes, fixFilesNotInManifest(files, ep)...)

	// Content-level: remove empty href attributes
	allFixes = append(allFixes, fixEmptyHref(files, ep)...)

	// Content-level: replace obsolete HTML elements
	allFixes = append(allFixes, fixObsoleteElements(files, ep)...)

	// --- Tier 3 fixes ---

	// CSS-level: inline @import rules
	allFixes = append(allFixes, fixCSSImports(files, ep)...)

	// Encoding: fix non-UTF-8 encoding declarations and transcode
	allFixes = append(allFixes, fixEncodingDeclaration(files, ep)...)

	// --- Tier 4 fixes ---

	// OPF-level: remove extra dcterms:modified elements
	allFixes = append(allFixes, fixExtraDCTermsModified(files, ep)...)

	// OPF-level: strip fragment identifiers from manifest hrefs
	allFixes = append(allFixes, fixManifestHrefFragment(files, ep)...)

	// OPF-level: remove duplicate spine idrefs
	allFixes = append(allFixes, fixDuplicateSpineIdrefs(files, ep)...)

	// OPF-level: fix invalid spine linear attribute values
	allFixes = append(allFixes, fixInvalidLinear(files, ep)...)

	// Content-level: remove <base> elements
	allFixes = append(allFixes, fixBaseElement(files, ep)...)

	// Content-level: remove processing instructions
	allFixes = append(allFixes, fixProcessingInstructions(files, ep)...)

	// Content-level: sync lang/xml:lang mismatch
	allFixes = append(allFixes, fixLangXMLLangMismatch(files, ep)...)

	// Content-level: add missing <title> element
	allFixes = append(allFixes, fixMissingTitle(files, ep)...)

	if len(allFixes) == 0 {
		ep.Close()
		return &Result{
			BeforeReport: beforeReport,
			AfterReport:  beforeReport,
		}, nil
	}

	// Step 4: Write repaired EPUB
	// The writer handles PKG-007 (mimetype first), PKG-005 (no extra field),
	// and PKG-005 (stored not compressed) by construction.
	if err := writeEPUB(outputPath, files, ep.ZipFile); err != nil {
		ep.Close()
		return nil, fmt.Errorf("writing repaired epub: %w", err)
	}

	ep.Close()

	// Step 5: Re-validate to confirm (Strict mode to see all warnings)
	afterReport, err := validate.ValidateWithOptions(outputPath, validate.Options{Strict: true})
	if err != nil {
		return nil, fmt.Errorf("validating repaired epub: %w", err)
	}

	return &Result{
		Fixes:        allFixes,
		BeforeReport: beforeReport,
		AfterReport:  afterReport,
	}, nil
}

// Note on PKG-005/PKG-007:
// These are "fixed by construction" — the writeEPUB function always writes
// mimetype as the first entry, stored (not compressed), with no extra field.
// So any EPUB that passes through doctor mode gets these fixed automatically,
// even though we don't emit explicit Fix entries for them unless the original
// had different issues.
