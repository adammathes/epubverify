# Roadmap

Status as of February 2026.

## Current State

**Godog BDD tests**: 901 passing, 0 failing, 1 pending (902 total scenarios; 100% pass rate on non-pending)
**Unit tests**: all passing
**External dependencies removed**: tests no longer require `epubverify-spec`

Progress: started at 605/903 (67%), reached 826/903 (91.6%), then 867/902, now **901/902 (100% non-pending pass rate)**.

### All previously-failing scenarios now fixed

All 41 previously-failing scenarios have been resolved across two sessions. Key fixes:

| Category | Fixes Applied |
|----------|--------------|
| EPUB2 DOCTYPE checks | `checkHTM004EPUB2Mode` - correct HTM-004 code (removed from RSC-005 remap) |
| Content: entity refs | `checkUnknownEntityRefs` - internal DTD entity extraction, correct error matching |
| Content: entities (no semicolon) | Separated from undefined-entity check; handled by regex `checkEntityReferences` |
| SVG title content | `checkSVGTitleContent` - standalone vs embedded mode, deferred nonPhrasingHTML check |
| MathML annotation | `checkMathMLContentOnly` - allow nested `<math>` inside `annotation-xml` |
| MathML annotation attrs | `checkMathMLAnnotation` - MathML-Presentation content tracking |
| epub:switch/trigger | Two-pass ID collection, case-after-default detection |
| Microdata attrs | `checkMicrodataAttrs` - itemprop without href/data |
| Custom NS attrs (HTM-054) | Reserved namespace host detection (w3.org/idpf.org) |
| URL conformance (RSC-020) | Single-slash URL detection |
| Prefix declarations | Added `msv`, `prism` to reserved prefix URIs |
| epub:prefix location | `checkPrefixAttrLocation` - html-only (not embedded svg) |
| SMIL prefix | `checkSMILPlainPrefixAttr` - plain `prefix=` vs `epub:prefix=` |
| SVG epub:prefix | Allow on root SVG element only |

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
- [x] Viewport meta tag parsing step definitions
  - Exported `ParseViewport` function in `pkg/validate/viewport.go`
  - Implements EPUB 3.3 viewport meta syntax parsing algorithm
  - Error detection: NULL_OR_EMPTY, ASSIGN_UNEXPECTED, VALUE_EMPTY, NAME_EMPTY, LEADING_SEPARATOR, TRAILING_SEPARATOR
  - Normalized output: semicolon-separated properties, comma-joined multi-values
  - All 34 viewport-syntax.feature scenarios now pass (20 valid + 14 invalid)

## Next Steps

### 1. Doctor mode BDD tests

Currently doctor mode is only tested via Go unit tests. Consider adding
Gherkin scenarios for doctor mode.
