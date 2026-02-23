# Roadmap

Status after godog migration (February 2026).

## Current State

**Godog BDD tests**: 693 passing, 209 failing (77% pass rate)
**Unit tests**: all passing
**External dependencies removed**: tests no longer require `epubverify-spec`

## Completed

- [x] Migrate from epubverify-spec to self-contained godog/Gherkin test suite
- [x] Port all epubcheck feature files and fixtures into `testdata/`
- [x] Implement godog step definitions for full-publication and single-file checks
- [x] Update CI to run godog tests (no external repo clone needed)
- [x] Remove stale references to EPUBCHECK_SPEC_DIR and epubcheck-spec
- [x] Update Makefile, README, and testing-strategy docs

## Next Steps

### 1. Fix the 209 failing godog scenarios

The failing tests break down by category:

| Category | Approx Count | Description |
|----------|-------------|-------------|
| Remote resources | ~40 | Remote resource detection (RSC-004, RSC-010+), `remote-resources` property validation |
| Foreign resources / fallbacks | ~36 | Foreign media type fallback chain validation (MED-003+), `picture`/`source`/`audio` fallbacks |
| CSS validation | ~18 | CSS `@charset`, `@import` missing file, `url()` references, `@font-face` validation, `direction`/`unicode-bidi` |
| SVG | ~20 | SVG content document validation, SVG `use` elements, SVG fragment identifiers |
| Fixed-layout / viewport | ~21 | FXL viewport parsing edge cases, SVG viewbox checks |
| OCF / container / ZIP | ~19 | Missing mimetype, container.xml validation, ZIP corruption, encryption.xml, signatures.xml |
| Media overlays | ~7 | Overlay fragment resolution, active-class CSS checks |
| NCX (EPUB 2) | ~8 | NCX duplicate IDs, invalid IDs, pageTarget type, uid matching |
| Navigation | ~8 | Nav doc schema checks, toc-spine order, external links |
| Schema validation | ~4 | RelaxNG/Schematron schema errors in content documents |
| OPF | ~5 | Missing spine, page-map attribute, guide references |
| Encoding | ~4 | CSS UTF-16 encoding detection |
| Misc | ~10 | DOCTYPE external identifiers, epub extension case, data URLs, file URLs |

**Suggested approach**: Work category by category, starting with the
highest-value areas:

1. **OCF / container / ZIP** — many of these are likely fixture-path or
   step-definition issues rather than missing validation logic
2. **Remote resources** — large category, may need step-definition
   refinement for the `remote-resources` property checks
3. **Foreign resources / fallbacks** — related to remote resources,
   likely fixable together
4. **SVG** — may need SVG-specific step definitions
5. **CSS** — some checks may already work but step definitions may not
   match the validator output format
6. **Fixed-layout** — viewport parsing likely works, may be step-definition
   alignment issues

### 2. Improve godog step definitions

Some failures are likely caused by missing or incomplete step definitions
rather than missing validation logic. Areas to check:

- Single-file check modes (`checking XHTML document`, `checking SVG document`)
  may need refinement
- Error message matching may need looser patterns
- Some steps may need new matchers for specific check IDs or message formats

### 3. Spec-update-runbook refresh

The `docs/spec-update-runbook.md` still references the old epubverify-spec
workflow. It should be rewritten to describe:

- How to add new feature files and fixtures to `testdata/`
- How to port new scenarios from upstream epubcheck
- How to debug failing godog scenarios

### 4. Doctor mode BDD tests

Currently doctor mode is only tested via Go unit tests. Consider adding
Gherkin scenarios for doctor mode:

```gherkin
Scenario: Doctor fixes missing mimetype
  Given an EPUB with a wrong mimetype value
  When the doctor repairs the EPUB
  Then the output should be valid
  And the fix list should include "OCF-003"
```

### 5. CI improvements

- Consider adding a CI job that reports the godog pass rate as a PR comment
- Add test matrix for Go versions (1.24, 1.25)
- Consider splitting unit tests and godog tests into separate CI jobs
  for faster feedback

### 6. Feature parity with epubcheck

Beyond the 209 failing scenarios, there are detection gaps documented
in the testing-strategy.md bug-fix history. Key areas where epubverify
is known to differ from epubcheck:

- RSC-005: HTML5 schema validation (not implemented, would need embedded schemas)
- RSC-007: mailto link validation
- RSC-020: EPUB CFI URL validation
- OPF-007c: prefix redeclaration detection
- PKG-026: font obfuscation validation
- OPF-043: advanced fallback chain requirements
