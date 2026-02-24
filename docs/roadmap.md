# Roadmap

Status as of February 2026.

## Current State

**Godog BDD tests**: 605 passing, 298 failing (67% pass rate on 903 total scenarios)
**Unit tests**: all passing
**External dependencies removed**: tests no longer require `epubverify-spec`

The 298 failing scenarios are all from newly-enabled single-file validation
(.opf, .xhtml, .svg, .smil). The 693 original full-EPUB scenarios all pass
with no regressions.

### Failure breakdown (single-file scenarios)

| Category | Approx Count | Description |
|----------|-------------|-------------|
| RSC-005 schema validation | ~150 | OPF/XHTML/SVG schema checks — epubcheck uses RelaxNG, we use programmatic checks with different IDs |
| OPF-086/OPF-092 | ~30 | Rendition metadata property validation (layout, spread, flow, orientation) |
| RSC-016/RSC-017 | ~40 | XML encoding detection (UTF-16 BOM, UCS-4, ISO-8859-1, unknown encoding) |
| OPF-027/OPF-028 | ~24 | unique-identifier resolution, dcterms:modified count |
| HTM-004 | ~12 | Obsolete HTML elements in single-file XHTML |
| RSC-029/RSC-028/RSC-033 | ~30 | Data URL, file URL, URL encoding issues |
| Other (OPF-053, RSC-020, etc.) | ~12 | Miscellaneous missing checks |

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

## Next Steps

### 1. Fix RSC-005 schema validation gap (~150 scenarios)

The biggest gap: epubcheck uses RelaxNG schemas and reports RSC-005 for schema
violations. We use programmatic checks with specific IDs (OPF-001, OPF-031, etc.).
Scenarios expect RSC-005 with specific message patterns.

Options:
- Map specific check IDs to RSC-005 in single-file mode
- Implement a lightweight OPF schema validator
- Accept as known divergence

### 2. Rendition metadata validation (OPF-086/OPF-092, ~30 scenarios)

Single-file OPF scenarios test `rendition:layout`, `rendition:spread`,
`rendition:flow`, `rendition:orientation` properties as meta elements
with `refines`. Need to implement:
- OPF-086: deprecated rendition property values
- OPF-092: rendition properties used in wrong context (refining resources)

### 3. XML encoding detection (RSC-016/RSC-017, ~40 scenarios)

Single-file validation needs encoding detection for:
- RSC-016: fatal encoding errors (UCS-4, ISO-8859-1, unknown encoding)
- RSC-017: encoding warnings (UTF-16 with/without BOM)

### 4. URL validation (RSC-029/RSC-028/RSC-020, ~30 scenarios)

- RSC-029: data URL in manifest/content
- RSC-028: non-conforming URL
- RSC-020: EPUB CFI URL validation

### 5. Viewport meta tag parsing step definitions

The `F-viewport-meta-tag/viewport-syntax.feature` scenarios remain PENDING:
- `parsing viewport <vp>` — expose viewport parser as standalone function
- `the parsed viewport equals <vp>` — assert parsed result
- `error <error> is returned` / `no error is returned`

### 6. Doctor mode BDD tests

Currently doctor mode is only tested via Go unit tests. Consider adding
Gherkin scenarios for doctor mode.
