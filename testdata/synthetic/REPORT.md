# Synthetic EPUB Testing Report

## Overview

Generated 100 randomized synthetic EPUBs with injected faults and compared
epubverify results against epubcheck 5.1.0 to identify bugs.

- **Total EPUBs tested:** 100
- **Validity matches:** 94 (94%)
- **Validity mismatches:** 6 (all false negatives)
- **Crashes:** 0 from either tool
- **Check differences:** 64 (both agree invalid, but flag different specific checks)

## Bugs Found (False Negatives)

These are cases where epubverify says VALID but epubcheck correctly reports
ERROR. These represent real bugs in epubverify.

### Bug 1: `page-progression-direction` attribute not rejected for EPUB 2

**Files:** synth_012.epub, synth_014.epub
**Severity:** ERROR missed

The `page-progression-direction` attribute on `<spine>` was introduced in EPUB 3.
In EPUB 2, it is not a valid attribute. epubcheck correctly reports
`RSC-005: attribute "page-progression-direction" not allowed here`.

**Root cause:** `pkg/validate/opf.go:1165` — `checkPageProgressionDirection()`
returns early when `pkg.Version < "3.0"`, which means it silently ignores the
attribute for EPUB 2 instead of flagging it as an invalid attribute.

```go
func checkPageProgressionDirection(pkg *epub.Package, r *report.Report) {
    if pkg.Version < "3.0" || pkg.PageProgressionDirection == "" {
        return  // BUG: should report error for EPUB 2
    }
```

**Fix:** For EPUB 2, if `PageProgressionDirection` is non-empty, report an error
that the attribute is not allowed.

---

### Bug 2: Missing `<title>` in EPUB 2 XHTML reported as WARNING, should be ERROR

**Files:** synth_081.epub, synth_014.epub
**Severity:** WARNING should be ERROR

In EPUB 2 (XHTML 1.1), the `<title>` element is required inside `<head>` by
the DTD. epubverify reports `HTM-002` as WARNING, but epubcheck correctly
reports `RSC-005: element "head" incomplete; missing required element "title"`
as ERROR.

**Root cause:** `pkg/validate/content.go:783` — `checkContentHasTitle()` always
uses `report.Warning`:

```go
r.AddWithLocation(report.Warning, "HTM-002",
    "Missing title element in content document head", location)
```

**Fix:** For EPUB 2, the severity of HTM-002 should be ERROR since XHTML 1.1
requires `<title>`. For EPUB 3 (HTML5), it could remain a WARNING since the
spec is more lenient.

---

### Bug 3: Empty `href` on `<a>` in EPUB 2 not caught as content model error

**Files:** synth_078.epub, synth_089.epub, synth_077.epub
**Severity:** ERROR missed (downgraded to INFO)

When an `<a href="">` appears directly inside `<body>` in EPUB 2, it violates
the XHTML strict content model (inline elements like `<a>` cannot be direct
children of `<body>`). epubcheck reports `RSC-005: element "a" not allowed here`.

epubverify catches the empty href as `HTM-003` (WARNING) but then the
`divergenceChecks` map in `pkg/validate/validator.go:246` downgrades it to INFO.
More importantly, epubverify doesn't perform content model validation, so the
structural error goes undetected.

**Root cause:** `pkg/validate/validator.go:246` — HTM-003 is in the
`divergenceChecks` set and gets downgraded from WARNING to INFO. The deeper
issue is that epubverify lacks XHTML strict content model validation.

---

### Bug 4: CSS value-level parse errors (CSS-008) not detected

**File:** synth_077.epub
**Severity:** ERROR missed

CSS like `body { color: ; font-size: px; }` has valid brace structure but
contains value-level errors (empty property values). epubcheck reports
`CSS-008: Token ";" not allowed here, expecting a property value` as ERROR.

**Root cause:** `pkg/validate/css.go:374` — `checkCSSSyntax()` only counts
unclosed braces. It does not parse CSS property values, so `color: ;` and
`font-size: px;` are not detected as errors. The CSS `body { color: ; font-size: px; } .broken { }}}` has balanced outer braces (the extra `}}` are excess
closing braces, not unclosed ones).

**Fix:** Implement more thorough CSS parsing that detects:
- Empty property values (`property: ;`)
- Excess closing braces
- Invalid value tokens

---

## Notable Check Differences (Both Invalid, Different Checks)

Even when both tools agree an EPUB is invalid, they often flag different
specific check IDs. Key patterns:

| Check ID | epubcheck-only | Notes |
|----------|---------------|-------|
| RSC-005  | 28 times | Schema validation errors — epubverify lacks full schema validation |
| RSC-008  | 15 times | Referenced resources not in manifest — different resource tracking |
| RSC-016  | 10 times | Fatal XML parse errors — epubverify uses HTM-001 instead |
| PKG-006  | 6 times  | Mimetype ordering — epubverify uses PKG-007 for this |
| OPF-043  | 3 times  | Non-standard media-type without fallback |
| RSC-003  | 4 times  | Container.xml rootfile issues |

| Check ID | epubverify-only | Notes |
|----------|----------------|-------|
| NAV-001  | 7 times | Missing nav document — epubverify more strict |
| HTM-001  | 6 times | Malformed XML — different error ID than epubcheck |
| PKG-007  | 6 times | Mimetype check — same concept as PKG-006 in epubcheck |
| OPF-001  | 5 times | Missing dc:title — epubverify catches even when OPF can't be fully parsed |
| RSC-010  | 5 times | Invalid manifest URL — epubverify checks more URL patterns |

## Tools

- **Generator:** `cmd/epubfuzz/main.go` — produces randomized synthetic EPUBs
  with 50+ possible injected faults (OCF, OPF, spine, manifest, content,
  navigation, CSS, EPUB 2-specific)
- **Comparator:** `cmd/epubcompare/main.go` — runs both tools and produces
  structured JSON comparison results
- **Results:** `testdata/synthetic/comparison_results.json`
- **Manifest:** `testdata/synthetic/manifest.json`

## Reproduction

```bash
# Build tools
go build -o epubfuzz ./cmd/epubfuzz/
go build -o epubcompare ./cmd/epubcompare/

# Generate 100 synthetic EPUBs (deterministic, seed=42)
./epubfuzz testdata/synthetic

# Run comparison (requires epubcheck jar)
./epubcompare testdata/synthetic ./epubverify /path/to/epubcheck.jar
```
