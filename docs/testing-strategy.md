# Testing Strategy

epubverify uses a two-tier testing approach: unit tests for internal logic and
godog/Gherkin BDD tests for spec compliance.

## Running Tests

```bash
# Unit tests
make test

# Godog spec compliance tests
make godog-test

# All tests
make test-all
```

## Test Architecture

### Unit tests (`pkg/*/`)

Standard Go `testing` package tests for internal logic:

- `pkg/epub/reader_test.go` — EPUB parsing, container/OPF parsing, href resolution
- `pkg/validate/content_test.go` — epub:type validation edge cases
- `pkg/validate/validator_test.go` — minimal EPUB 2/3 validation smoke tests
- `pkg/doctor/doctor_test.go` — doctor mode fix functions (all 4 tiers)
- `pkg/doctor/integration_test.go` — multi-problem integration tests

### Godog BDD tests (`test/godog/`)

Gherkin feature files in `testdata/features/` define spec compliance tests
using the [godog](https://github.com/cucumber/godog) framework. These are
ported from the upstream [w3c/epubcheck](https://github.com/w3c/epubcheck)
test suite.

- **Feature files**: `testdata/features/epub2/` and `testdata/features/epub3/`
- **Fixtures**: `testdata/fixtures/epub2/` and `testdata/fixtures/epub3/`
- **Step definitions**: `test/godog/epubcheck_test.go`

Everything is self-contained in the repo — no external dependencies or
environment variables needed.

## Bugs Found and Fixed

This section documents bugs discovered during real-world EPUB testing.
Each round expanded the test corpus and exposed new false positives.

### Round 1 (5 Gutenberg EPUBs)

All 5 passed epubcheck with 0 errors. epubverify reported false positives
on all 5. Four bugs identified and fixed:

| Check ID | Severity | Description | Fix |
|----------|----------|-------------|-----|
| OPF-037 | ERROR | `refines` target IDs on `dc:creator` not tracked | Added `ID` field to `DCCreator`; parser captures `id` attr; validator includes creator IDs |
| CSS-002 | WARNING | CSS selectors like `a:link` matched as properties | Rewrote to only match inside rule blocks |
| HTM-015 | WARNING | Unknown `epub:type` values flagged as warnings | Downgraded to INFO — vocabulary is extensible per spec |
| NAV-010 | WARNING | Unknown landmark `epub:type` values flagged | Downgraded to INFO — same rationale |

### Round 2 (expanded to 25 EPUBs: +16 Gutenberg, +4 Feedbooks)

New samples exposed 4 more false positives. 6 of 25 failed (epubverify
said INVALID, epubcheck said VALID). Three bugs identified and fixed:

| Check ID | Severity | Description | Fix |
|----------|----------|-------------|-----|
| OPF-037 | ERROR | `dc:contributor` element IDs not tracked as refines targets | Added `Contributors` field to `Metadata`; parser captures contributors; validator includes their IDs |
| E2-007 | ERROR | Nested `navPoint` elements in NCX incorrectly flagged | Rewrote with stack-based tracking for proper nesting |
| OPF-036 | WARNING | Fractional seconds in ISO 8601 dates rejected | Updated W3CDTF regex to allow `.\d+` fractional seconds |

Additionally, the Feedbooks EPUBs revealed a false positive for RSC-002
(flagging ZIP directory entries as unmanifested files):

| Check ID | Severity | Description | Fix |
|----------|----------|-------------|-----|
| RSC-002 | WARNING | ZIP directory entries (trailing `/`) flagged as unmanifested | Skip entries ending with `/` |

After all fixes: **25/25 samples match epubcheck's validity verdict.**

### Round 3 (expanded to 30 EPUBs: +3 Gutenberg EPUB 2, +2 Feedbooks)

No new bugs found. All 30 samples match epubcheck's validity verdict.

### Round 4 (expanded to 42 EPUBs: +12 IDPF epub3-samples)

The IDPF samples exercise exotic EPUB 3 features: fixed-layout, SVG in
spine, MathML, media overlays, SSML pronunciation, RTL text, web fonts,
bindings, and custom media types. These exposed 7 new false positives:

| Check ID | Severity | Description | Fix |
|----------|----------|-------------|-----|
| OPF-037 | ERROR | `dc:title` element IDs not tracked as refines targets | Changed `Titles` from `[]string` to `[]DCTitle` with `ID` field |
| CSS-001 | ERROR | CSS comments with special characters falsely parsed as syntax errors | Strip comments before analyzing CSS syntax |
| OPF-024 | ERROR | Font MIME types `application/vnd.ms-opentype` and `text/javascript` rejected | Added `mediaTypesEquivalent()` for font/JS/MP4 type aliases |
| HTM-013 | ERROR | FXL viewport check ignores per-spine-item `rendition:layout-reflowable` overrides | Check spine itemref properties for rendition overrides |
| HTM-020 | WARNING | Processing instructions flagged as warnings | Downgraded to INFO — PIs are allowed per EPUB spec |
| HTM-031 | ERROR | SSML namespace flagged as forbidden | Disabled check — SSML attributes are explicitly permitted in EPUB 3 |
| MED-004 | ERROR | Non-spine foreign resources (page templates, custom XML) flagged for missing fallback | Only require fallback for spine items |

After all fixes: **42/42 samples match epubcheck's validity verdict.**

### Round 5 (expanded to 49 EPUBs: +7 from IDPF, DAISY, bmaupin)

Added more IDPF samples (obfuscated fonts, Hebrew RTL, ultra-minimal),
DAISY accessibility test EPUBs, and minimal EPUB test files. Two
additional IDPF/ReadBeyond samples were excluded because they require
HTML5 schema validation (RSC-005) which we don't implement.

No new bugs found. **49/49 samples match epubcheck's validity verdict.**

### Round 6 (expanded to 86 EPUBs: +28 IDPF, +11 Gutenberg, +2 DAISY)

Downloaded all remaining IDPF epub3-samples (28 new), bringing the total
to all 43 available from the IDPF release. Also added 11 more Gutenberg
EPUBs and 2 more DAISY accessibility test EPUBs. This round exposed 7
new false positive categories:

| Check ID | Severity | Description | Fix |
|----------|----------|-------------|-----|
| RSC-003 | ERROR | Media fragment URIs (`#xywh=`, `#xyn=`, `#t=`, `epubcfi(`) treated as HTML element IDs | Skip known media fragment URI prefixes |
| HTM-013 | ERROR | FXL viewport check ran on non-spine XHTML fallback documents | Only check viewport for spine items |
| HTM-008 | ERROR | Absolute path hyperlinks (`/wiki/...`) from embedded web content flagged | Skip absolute paths (starting with `/`) |
| OPF-037 | ERROR | `<meta>` elements with IDs not tracked as refines targets | Collect IDs from all `<meta>` elements in metadata |
| CSS-007 | ERROR→WARNING | Missing CSS background-image reported as ERROR | Downgraded to WARNING (epubcheck doesn't validate CSS image refs) |
| OPF-024 | ERROR | WOFF fonts declared as `application/font-woff` not recognized | Added `font/woff`/`application/font-woff` to equivalence groups |
| OPF-029 | ERROR | `data-nav` manifest property not recognized | Added to valid manifest properties (EPUB Region-Based Navigation) |

3 IDPF samples excluded as false-negative gaps: `epub30-spec.epub`
(RSC-007 mailto), `georgia-cfi.epub` (RSC-020 CFI URLs),
`jlreq-in-english.epub` (RSC-007 mailto).

After all fixes: **86/86 samples match epubcheck's validity verdict.**

### Round 7 (expanded to 96 EPUBs: +8 Gutenberg, +2 IDPF old release)

Added 3 more EPUB 3 samples (Iliad, Emma, Thus Spake Zarathustra) and
5 more EPUB 2 samples (Great Expectations, Treasure Island, Grimm's
Fairy Tales, Aesop's Fables, Count of Monte Cristo) to deepen EPUB 2
coverage. Also added 2 samples from the older IDPF 20170606 release
for backward compatibility testing.

No new bugs found. **96/96 samples match epubcheck's validity verdict.**

### Round 8 (expanded to 122 EPUBs: +17 Standard Ebooks, +9 Gutenberg)

Added 17 Standard Ebooks samples — professionally typeset EPUB 3 with
rich accessibility metadata, custom `se:*` vocabulary, `<guide>` elements,
ONIX records, and complex `<meta refines>` chains targeting diverse DC
elements. Also added 9 more Gutenberg EPUB 3 samples. The SE samples
immediately exposed a major gap in our OPF-037 refines check — all 14
initially showed as INVALID.

Three bug categories fixed:

| Check ID | Severity | Description | Fix |
|----------|----------|-------------|-----|
| OPF-037 | ERROR | IDs on `dc:publisher`, `dc:subject`, `dc:description`, and other DC elements not tracked as valid refines targets | Collect `id` attributes from all elements inside `<metadata>` via new `DCElementIDs` field |
| CSS-002 | WARNING | Modern CSS properties (`text-wrap`, `hanging-punctuation`, `adobe-text-layout`, logical properties) not recognized | Added ~25 modern and vendor-specific properties to `knownCSSProperties` |
| HTM-015 | INFO | Missing epub:type values from W3C EPUB SSV 1.1 (`backlink`, dictionary terms, `referrer`, etc.) | Added ~30 missing values from the complete W3C specification |

Also reclassified 2 IDPF samples (WCAG, vertically-scrollable-manga)
from "known-invalid" to "known false-negative" — their OPF-037 errors
were actually false positives that are now fixed. Their real errors
(OPF-007c prefix redeclaration, RSC-007 mailto links) are detection
gaps in epubverify.

After all fixes: **122/122 samples match epubcheck's validity verdict**
(114 valid, 6 known-invalid, 2 known false-negatives pass as valid).

### Round 9 (expanded to 133 EPUBs: +11 wareid, +1 readium)

Added 11 purpose-built test EPUBs from wareid/EPUB3-tests (fixed-layout
templates, audio/video content, WOFF2 fonts, rendition properties,
accessibility vocabulary, page breaks, URL handling) and 1 MathML
conformance test from readium/readium-test-files.

12 additional samples from these repos were excluded as false negatives
(epubcheck INVALID due to RSC-005 schema validation, RSC-001 missing
files, PKG-026 obfuscation, OPF-043 fallback requirements).

No new bugs found. **133/133 samples match epubcheck's validity verdict.**

### Round 10 (added 29 synthetic EPUBs + 5 bug fixes)

Created 29 purpose-built synthetic EPUBs (8 custom edge-case + 21
reconstructed from w3c/epubcheck test fixtures) targeting under-tested
validation paths.

These exposed **5 new false-positive bug categories**, all fixed:

| Check ID | Severity | Description | Fix |
|----------|----------|-------------|-----|
| RSC-001 | ERROR | Percent-encoded manifest hrefs not decoded when looking up ZIP entries | `ResolveHref()` now URL-decodes hrefs with `url.PathUnescape()` |
| RSC-002 | WARNING | Files belonging to other renditions (multiple rootfiles) flagged as undeclared | Parse all rootfile OPFs and container `<links>` to build exclusion set |
| MED-001/MED-003 | ERROR | Foreign (non-core) image types checked for magic bytes / corruption | Skip image integrity checks for non-core media types |
| HTM-005 | ERROR | `<script type="text/plain">` data blocks flagged as scripted content | Only flag executable script types (JS/module), not data blocks |
| RSC-004 | ERROR | Remote `<video>`/`<audio>` sources flagged even when content doc has `remote-resources` property | Check `remote-resources` property before flagging |

After all fixes: **133/133 real-world + 29/29 synthetic = 162/162 match.**
