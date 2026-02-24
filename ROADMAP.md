# Roadmap

Status as of February 2026.

## Current State

**Godog BDD tests**: 826 passing, 41 failing, 35 pending (902 total scenarios; 91.6% pass rate on non-pending)
**Unit tests**: all passing
**External dependencies removed**: tests no longer require `epubverify-spec`

Progress: started at 605/903 (67%), reached 861/903 (95.3%), now tracking 826/902
(the count shift reflects removal of one duplicate scenario and reclassification of
pending viewport-parser scenarios).

### Failure breakdown (41 remaining scenarios)

| Category | Count | Key Tests |
|----------|-------|-----------|
| OPF metadata/refines | 3 | refines relative URL, refines fragment ID, unique-identifier resolution |
| OPF manifest/spine | 5 | unknown item property (full-pub), nav property OPF-012, OPF-043 spine fallback, OPF-091 dup, RSC-020 URLs |
| OPF collections/guide | 4 | OPF-070 invalid role URL, manifest collection nesting, guide duplicate RSC-017 (x2) |
| OPF file names (PKG) | 3 | PKG-009/010/012 in single-file mode |
| OPF package metadata | 1 | package element missing metadata child |
| Content: EPUB2 | 3 | HTM-004 DOCTYPE/entity in EPUB2, HTML5 elements/DOCTYPE in OPS |
| Content: XHTML | 10 | epub:switch/trigger validation, microdata, MathML encoding, custom attrs HTM-054, URL host, title content |
| Content: entities | 2 | Unknown entity references in XHTML |
| CSS | 1 | RSC-030 file URL count (3 vs 2) |
| Prefix/vocabulary | 6 | Prefix validation in SVG/SMIL, empty namespace, reserved prefix overriding |
| Obsolete public ID | 1 | HTM-004 obsolete doctype public identifier |
| Item paths with spaces | 2 | item paths with spaces (full-pub mode) |

## Completed

- [x] Migrate from epubverify-spec to self-contained godog/Gherkin test suite
- [x] Port all epubcheck feature files and fixtures into `testdata/`
- [x] Implement godog step definitions for full-publication and single-file checks
- [x] Update CI to run godog tests (no external repo clone needed)
- [x] Remove stale references to EPUBCHECK_SPEC_DIR and epubcheck-spec
- [x] Update Makefile, README, and testing-strategy docs
- [x] Fix all 693 godog BDD scenarios (100% pass rate on full-EPUB tests)
- [x] Implement PENDING step definitions:
  - Filename-checker steps (PKG-009/010/011/012) with `ValidateFilenameString`
  - Usage severity steps (`usage CODE is reported [N times]`)
  - `checkFilesInManifest` changed to OPF-003 (Usage), matching epubcheck
- [x] Implement usage checks:
  - OPF-090, HTM-060a/b, NCX-006, OPF-018b/RSC-006b, OPF-096b,
    MED-015, CSS-029
- [x] Add `UsageCount()` to report; update doctor early-exit condition
- [x] Rewrite spec-update-runbook for testdata workflow
- [x] CI improvements: split unit/BDD jobs, Go 1.24+1.25 matrix, branch triggers
- [x] Single-file validation support (`ValidateFile` for .opf/.xhtml/.svg/.smil)
  - Creates temporary EPUB wrapper, validates with `SingleFileMode`
  - Skips container-dependent checks (OPF-024, OPF-093, OPF-096, RSC-032)
  - Enables 442 previously-PENDING scenarios
- [x] RSC-005 schema mapping for single-file validation
  - Maps OPF-001/031/038/039b/042/088 and HTM-004/009 to RSC-005
  - Convergence/divergence tracking for error code differences
- [x] Rendition metadata validation (OPF-085/086/092)
  - layout, spread, flow, orientation property validation
  - Deprecated rendition values, invalid rendition contexts
- [x] OPF metadata validation
  - Metadata refines validation, MARC relator codes, legacy media types
  - Link relation/properties validation, date format checks
  - Nav/element-order checks, cover-image uniqueness
- [x] Single-file content checks (content.go + opf.go expansion)
  - XHTML: XML version, encoding, well-formedness, namespace, IDs, obsolete attrs
  - XHTML: external entities, SSML, URL schemes, ARIA, epub:type/switch/trigger
  - SVG: foreignObject, viewBox, prefix, epub:type, element validation
  - SMIL: media overlay content checks, par/seq structure, clock values
- [x] OPF pre-parse checks
  - Encoding detection: UTF-16 BOM, UCS-4, ISO-8859-1 (RSC-027/028/016)
  - Namespace validation, DOCTYPE checks, bindings deprecation
  - XML duplicate ID tracking, unknown element detection
- [x] Media overlay metadata validation
  - media:duration clock value parsing and sum verification (MED-016)
  - media-overlay attribute cross-referencing
- [x] Add AGENTS.md with TDD workflow and reference documentation
- [x] Move ROADMAP.md to repo root

## Next Steps (41 remaining failures)

### 1. Quick OPF fixes (~8 scenarios)

Message and error code mismatches that are straightforward:
- Fix missing `metadata` child message format
- Fix OPF-023 -> OPF-043 for spine non-content fallback
- Add OPF-012 alongside RSC-005 for nav property on wrong media type
- Fix RSC-030 double-counting (remove manifest item check from opf.go, keep references.go)
- Remove duplicate OPF-091 check (`checkManifestHrefNoFragment` + `checkManifestHrefFragment`)
- Fix refines: RSC-005 for absolute URL, RSC-017 for non-fragment resource path
- Fix unique-identifier resolution in EPUB2 single-file mode

### 2. New OPF checks (~6 scenarios)

- RSC-020: manifest item href URL encoding validation
- OPF-027: unknown prefixed manifest item properties in full-publication mode
- OPF-070: collection role URL validation
- RSC-005: manifest collection nesting check
- RSC-017: guide duplicate entries detection
- PKG-009/010/012: file name checks in single-file OPF mode

### 3. Content document checks (~15 scenarios)

- HTM-004: DOCTYPE and entity reference checks in EPUB2 XHTML
- epub:switch/trigger validation: child element order, ID references
- MathML annotation `encoding` attribute validation
- Microdata attribute validation (RSC-005)
- HTM-054: custom attributes using reserved namespace strings
- URL host parsing (RSC-020 count)
- SVG title content HTML validation
- Obsolete doctype public identifier
- Item paths with spaces (full-pub mode)

### 4. Prefix/vocabulary checks (~6 scenarios)

- Prefix attribute validation on SVG content documents and embedded SVG
- Empty namespace in prefix declarations
- Reserved prefix overriding in XHTML content documents
- Undeclared prefix in Media Overlays epub:type

### 5. Viewport meta tag parsing step definitions (pending, not failing)

The `F-viewport-meta-tag/viewport-syntax.feature` scenarios remain PENDING:
- `parsing viewport <vp>` -- expose viewport parser as standalone function
- `the parsed viewport equals <vp>` -- assert parsed result
- `error <error> is returned` / `no error is returned`

### 6. Doctor mode BDD tests

Currently doctor mode is only tested via Go unit tests. Consider adding
Gherkin scenarios for doctor mode.
