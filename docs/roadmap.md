# Roadmap

Status as of February 2026.

## Current State

**Godog BDD tests**: 693 passing, 0 failing (100% pass rate)
**Unit tests**: all passing
**External dependencies removed**: tests no longer require `epubverify-spec`

## Completed

- [x] Migrate from epubverify-spec to self-contained godog/Gherkin test suite
- [x] Port all epubcheck feature files and fixtures into `testdata/`
- [x] Implement godog step definitions for full-publication and single-file checks
- [x] Update CI to run godog tests (no external repo clone needed)
- [x] Remove stale references to EPUBCHECK_SPEC_DIR and epubcheck-spec
- [x] Update Makefile, README, and testing-strategy docs
- [x] Fix all 693 godog BDD scenarios (100% pass rate)
- [x] Implement PENDING step definitions:
  - Filename-checker steps (PKG-009/010/011/012) with `ValidateFilenameString`
  - Usage severity steps (`usage CODE is reported [N times]`)
  - `checkFilesInManifest` changed to OPF-003 (Usage), matching epubcheck
- [x] Implement usage checks newly exposed by pending step resolution:
  - OPF-090: non-preferred core media types
  - HTM-060a/b: viewport meta tag usage notes
  - NCX-006: empty NCX text labels
  - OPF-018b/RSC-006b: remote-resources on scripted content
  - OPF-096b: non-linear items potentially reachable via scripted content
  - MED-015: SMIL text elements not in content document DOM order
  - CSS-029: well-known media overlay class names not declared in package
- [x] Add `UsageCount()` to report; update doctor early-exit condition

## Next Steps

### 1. Spec-update-runbook refresh

The `docs/spec-update-runbook.md` still references the old epubverify-spec
workflow. It should be rewritten to describe:

- How to add new feature files and fixtures to `testdata/`
- How to port new scenarios from upstream epubcheck
- How to debug failing godog scenarios (run single feature, verbose output)

### 2. CI improvements

- Consider adding a CI job that reports the godog pass rate as a PR comment
- Add test matrix for Go versions (1.24, 1.25)
- Consider splitting unit tests and godog tests into separate CI jobs
  for faster feedback

### 3. Feature parity with epubcheck

Known detection gaps where epubverify differs from epubcheck:

- RSC-005: HTML5 schema validation (not implemented, would need embedded schemas)
- RSC-007: mailto link validation
- RSC-020: EPUB CFI URL validation
- OPF-007c: prefix redeclaration detection
- PKG-026: font obfuscation validation
- OPF-043: advanced fallback chain requirements

### 4. Viewport meta tag parsing step definitions

The `F-viewport-meta-tag/viewport-syntax.feature` scenarios remain PENDING:
- `parsing viewport <vp>` — expose viewport parser as standalone function
- `the parsed viewport equals <vp>` — assert parsed result
- `error <error> is returned` / `no error is returned`

### 5. Doctor mode BDD tests

Currently doctor mode is only tested via Go unit tests. Consider adding
Gherkin scenarios for doctor mode:

```gherkin
Scenario: Doctor fixes missing mimetype
  Given an EPUB with a wrong mimetype value
  When the doctor repairs the EPUB
  Then the output should be valid
  And the fix list should include "OCF-003"
```
