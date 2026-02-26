# Roadmap

## DATE UPDATED

February 26, 2026

---

## STATUS

### Test Results

| Suite | Result | Notes |
|-------|--------|-------|
| **Godog BDD scenarios** | 923/924 passing (1 pending) | 100% pass rate on non-pending scenarios |
| **Unit tests** | All passing | 35 doctor tests, 39 content model tests, epub/validate tests |
| **Stress tests** | 77/77 match epubcheck | Independent real-world EPUBs |
| **Synthetic EPUBs** | 29/29 match epubcheck | Purpose-built edge cases |

### Where We Have Confidence

**High confidence — EPUB 3.3 validation core.** The 923 passing BDD scenarios are ported directly from epubcheck's own test suite and cover OCF container checks, OPF package document validation, XHTML/SVG/SMIL content document checks, navigation document validation, CSS checks, media overlay validation, fixed-layout checks, accessibility checks, cross-reference resolution, encoding detection, EPUB Dictionary/Index/Preview collections, EDUPUB profile checks, and Search Key Map validation. The stress test corpus of 77 independently-downloaded real-world EPUBs (from Project Gutenberg, IDPF samples, Standard Ebooks, DAISY, Feedbooks, wareid, readium) all produce the same valid/invalid verdict as epubcheck 5.1.0.

**High confidence — EPUB 2.0.1 validation.** 7 feature files covering NCX, OCF, OPF, and OPS checks for EPUB 2. The stress test corpus includes EPUB 2 books.

**High confidence — Doctor mode.** 24 fix types across 4 tiers, all with unit tests and integration tests that take broken EPUBs to 0 errors.

**High confidence — HTML5 content model (RSC-005).** The Tier 1 RelaxNG gap analysis closed 62 of 63 identified gaps. We now enforce: block-in-phrasing (div inside p/h1-h6/span/etc.), restricted children (ul/ol/table/select/dl/hgroup), void element children, table content model, interactive nesting, transparent content model inheritance, figcaption position, and picture structure. Only remaining gap: `<input>` type-specific attribute validation (low priority).

**High confidence — Schematron rule coverage (Tier 2).** The Tier 2 Schematron audit covers all 118 patterns from epubcheck's 10 core .sch files: 167 checks implemented, 13 partial, 0 missing. New checks added: disallowed descendant nesting (address/form/progress/meter/caption/header/footer/label), required ancestors (area→map, img[ismap]→a[href]), bdo dir attribute, SSML ph nesting, duplicate map names, select multiple validation, meta charset uniqueness, link sizes validation, and IDREF attribute checking.

**High confidence — Java code check coverage (Tier 3).** The Tier 3 Java audit covers all 315 MessageId codes defined in epubcheck: 223 implemented (including EPUB Dictionaries, EDUPUB, collections), 9 suppressed (disabled in epubcheck), 83 wontfix (niche/defunct features like DTBook, scripting checks, dead code). See `scripts/java-audit.py` (run with `--json` for machine-readable output).

### Where We Have Less Confidence

**Medium confidence — Edge-case error codes.** A handful of epubcheck error codes rely on schema validation or features we haven't implemented: RSC-007 (mailto links), RSC-020 (CFI URLs), OPF-007c (prefix redeclaration), PKG-026 (font obfuscation), OPF-043 (complex fallback chains).

**Lower confidence — Non-English and exotic EPUBs.** The test corpus is heavily English-biased. CJK, Arabic/RTL, Cyrillic, and Indic script coverage is limited to a few IDPF samples.

**Lower confidence — Very large EPUBs.** Testing has been on typical-sized books. Memory usage and performance on 50MB+ EPUBs is untested.

### Progress History

Started at 605/903 (67%) → 826/903 (91.6%) → 867/902 → 901/902 → **923/924 (100% non-pending)**

---

## APPROVED

### Increase Real-World EPUB Test Coverage

Before we move on to the long running epub scouring stress test, let's expand from 77 to 200 epubs with more diversity from:

Expand the stress test corpus beyond the current 77 EPUBs with diverse sources:

- **Standard Ebooks** (~700 titles): High quality EPUB3, rich accessibility metadata, custom `se:*` vocabulary
- **OAPEN scholarly books**: Complex structure, footnotes, indexes, math
- **Non-English EPUBs**: CJK, Arabic/RTL, Cyrillic, Indic scripts
- **Very large EPUBs**: 50MB+ image-heavy books for performance testing
- **Calibre/Sigil-generated**: Tool-specific output patterns
- **EPUB2 depth**: More EPUB2-only books for legacy path coverage

### Long-Running EPUB Scouring Stress Test

**Goal:** Create an automated, long-running process that continuously discovers EPUBs on the internet, validates them with both epubverify and epubcheck, and files bugs when discrepancies are found.

**Motivation:** Our current stress test (77 EPUBs) covers a good range of sources but is a point-in-time snapshot. A continuous scouring process would catch regressions, discover new edge cases, and build confidence that epubverify handles the full diversity of real-world EPUBs. We already have most of the infrastructure — download scripts, comparison runner, analysis tools.

**Design:**

The system has three components: a **crawler** that discovers and downloads EPUBs, a **validator** that runs both tools and compares results, and a **reporter** that logs results and files issues.

**1. Crawler (`scripts/epub-crawler.sh` or `cmd/epubcrawl/`)**

Sources to crawl (in order of reliability):
- **Project Gutenberg** — bulk catalog at `gutenberg.org/cache/epub/feeds/`, thousands of EPUBs. Parse the catalog, download EPUBs we haven't seen before. Rate-limit to 1 request/second.
- **Standard Ebooks GitHub releases** — each book has a GitHub release with the EPUB. Use the GitHub API to enumerate releases.
- **OAPEN** — scholarly open-access EPUBs. OAPEN has an OAI-PMH feed and direct download links.
- **Internet Archive** — search API at `archive.org/advancedsearch.php` with `mediatype:texts` and `format:epub`. Rate-limit carefully.
- **Open Library** — lending library EPUBs via the Open Library API.
- **Feedbooks public domain** — catalog at `feedbooks.com/publicdomain`.

Each downloaded EPUB gets a SHA-256 hash. We maintain a manifest (`stress-test/crawl-manifest.json`) tracking:
```json
{
  "sha256": "abc123...",
  "source_url": "https://gutenberg.org/...",
  "downloaded_at": "2026-02-25T12:00:00Z",
  "size_bytes": 123456,
  "epubcheck_verdict": "valid",
  "epubverify_verdict": "valid",
  "match": true,
  "discrepancy_issue": null
}
```

**EPUBs are never committed to the repo.** The manifest tracks what we've tested. EPUBs are stored in a local (gitignored) directory or a cloud bucket.

**2. Validator (`scripts/crawl-validate.sh`)**

For each new EPUB:
1. Run `epubverify book.epub --json -` and capture output
2. Run `java -jar epubcheck.jar book.epub --json -` and capture output
3. Compare verdicts (valid/invalid) and error codes
4. Log result to the manifest

On discrepancy:
- If epubverify says VALID but epubcheck says INVALID → **false negative** (we're missing checks). Log the specific error codes we missed.
- If epubverify says INVALID but epubcheck says VALID → **false positive** (we're over-reporting). This is a bug in epubverify.

**3. Reporter (`scripts/crawl-report.sh`)**

- Generates a summary of all crawl runs: total EPUBs tested, match rate, discrepancies by category
- For new discrepancies: edits a document in the repo with the EPUB source URL, both validator outputs, and the specific error code differences to review and fix
- Optionally files an ISSUE in the github repo
- Optionally: if the discrepancy involves an error code we've seen before, adds a comment to the existing issue instead of creating a new one
- Optionally: for false negatives, generates a synthetic test fixture that reproduces the issue and adds it to `testdata/synthetic/`

**Workflow Integration:**

This could run as:
- A **GitHub Actions scheduled workflow** (e.g., weekly) that downloads N new EPUBs, validates, and opens issues
- A **local long-running script** for deeper testing sessions
- Both — the scheduled workflow for steady-state coverage, manual runs for deep dives

**What we already have:**
- `stress-test/download-epubs.sh` — downloads from Gutenberg and IDPF
- `stress-test/run-comparison.sh` — runs both validators
- `stress-test/analyze-results.sh` — compares results
- `stress-test/epub-sources.txt` — URL catalog
- `cmd/epubcompare/` — Go tool for comparing outputs

**What we'd need to build:**
- Crawler logic that enumerates sources and discovers new EPUBs
- Manifest tracking (what we've tested, deduplication by SHA-256)
- GitHub issue filing (via `gh` CLI)
- Scheduled GitHub Actions workflow
- Optional: synthetic fixture generation from discrepancies

**Estimated scope:** Medium. Most infrastructure exists. Main new work is the crawler and the automated issue filing.

---

### Systematic Gap Extraction from Epubcheck's Three Validation Tiers ✅ ALL THREE TIERS COMPLETE

**Goal:** Methodically analyze each of epubcheck's three validation tiers — RelaxNG schemas, Schematron rules, and Java code — to identify every check that epubverify doesn't currently implement, prioritize the gaps, and systematically close them.

**Motivation:** We've already had success using the schematron audit (`scripts/schematron-audit.py`) to identify gaps. That audit found checks we were missing and led to new test fixtures. The same approach can be applied more thoroughly to all three tiers.

**Background: Epubcheck's Three Validation Tiers**

Epubcheck validates EPUBs using three complementary mechanisms:

1. **RelaxNG schemas** (`src/main/resources/com/adobe/epubcheck/schema/`): XML grammar validation. These define what elements and attributes are allowed where. Epubcheck uses Jing to validate against these schemas and maps violations to error codes (primarily RSC-005). The schemas cover:
   - EPUB 2 OPF (`20/`)
   - EPUB 3 OPF (`30/`)
   - EPUB 2/3 content documents (XHTML, SVG, DTBook)
   - NCX, media overlays, OCF container
   - Custom EPUB extensions of HTML5 (the `epub-xhtml-30.rnc` schema extends the base HTML5 schema with epub-specific attributes)

2. **Schematron rules** (`src/main/resources/com/adobe/epubcheck/schema/`): Semantic constraint checking. These express rules that can't be captured in a grammar — things like "if attribute X is present, attribute Y must also be present" or "element Z must contain at least one child of type W." The schematron rules map to specific epubcheck error codes (OPF-xxx, HTM-xxx, etc.).

3. **Java code** (`src/main/java/com/adobe/epubcheck/`): Programmatic checks. These handle everything the schemas can't — cross-file references, ZIP structure, encoding detection, media type sniffing, complex business logic.

**Approach:**

**Tier 1: RelaxNG Schema Analysis** ✅ DONE

Audit script (`scripts/relaxng-audit.py`) parses epubcheck's 34 .rnc schemas, extracts 115 element definitions and 305 patterns, identified 63 content model gaps — **62 of 63 closed** (PR #27). See `testdata/fixtures/relaxng-gaps/gap-analysis.json` for the full machine-readable report.

**Completed (Phase 1 + Phase 2):**
- Schema inventory and parser (34 files, 115 elements, 87 content rules)
- Block-in-phrasing detection: `<div>` in `<p>`, `<table>` in `<span>`, etc.
- Restricted children: `<ul>`/`<ol>` → `<li>`, `<tr>` → `<td>`/`<th>`, `<select>` → `<option>`, etc.
- Void element children (br/hr/img with content)
- Table content model checks
- dl/hgroup restricted children
- Interactive nesting (button, input, select nested in `<a>`, etc.)
- Transparent content model inheritance (a, ins, del, object)
- Figcaption position checks
- Picture element structure (source* then img)
- 29 unit tests, 16 test fixtures, gap analysis JSON report

**Remaining (low priority):**
- SVG/MathML full content model
- `<input>` type-specific attribute validation (13+ type variants)

**Tier 2: Schematron Rule Analysis** ✅ DONE

Audit script (`scripts/schematron-audit.py`) — parses epubcheck's 10 core Schematron .sch files, extracts 118 patterns and 180 checks, compares against epubverify implementation. **All 118 patterns accounted for: 167 implemented, 13 partial, 0 missing.**

**Completed:**
- Complete mapping of all Schematron assertions to KNOWN_CHECKS manifest
- Cross-referenced with existing godog scenarios and implementation
- Implemented 8 new check functions for 43 previously-missing XHTML patterns
- 28 new unit tests, all passing
- 0 regressions on initial implementation

**New check functions (Tier 2):**

| Check | What it catches |
|-------|----------------|
| `checkDisallowedDescendants` | address/form/progress/meter/caption/header/footer/label nesting |
| `checkRequiredAncestor` | area without map ancestor, img[ismap] without a[href] ancestor |
| `checkBdoDir` | bdo element missing required dir attribute |
| `checkSSMLPhNesting` | Nested ssml:ph attributes |
| `checkDuplicateMapName` | Multiple map elements with same name |
| `checkSelectMultiple` | Multiple selected options without @multiple |
| `checkMetaCharset` | More than one meta charset element |
| `checkLinkSizes` | sizes attribute on link without rel=icon |

**Already-implemented patterns now tracked:**
- Interactive nesting patterns (a, button, audio, video) — `checkInteractiveNesting`
- IDREF/IDREFS validation (13 patterns) — existing `checkIDReferences`

**Remaining (wontfix — too niche):**
- OCF metadata patterns (9): multi-rendition metadata.xml; OPF equivalents exist
- distributable-object collection: very rare EDUPUB-specific feature
- Multi-rendition selection/mapping (4): experimental multi-rendition container extensions

**Tier 3: Java Code Analysis** ✅ DONE

Audit script (`scripts/java-audit.py`) — parses epubcheck's `MessageId.java` (315 message IDs), `DefaultSeverities.java`, and `MessageBundle.properties`, then greps all Java source files for `MessageId.XXX` references to map every error code to its emitting Java class. Cross-references against epubverify's Go source and BDD feature files. **All 315 message IDs accounted for: 223 implemented, 9 suppressed, 83 wontfix, 0 missing.**

**New check functions (Tier 3, Phase 1):**

| Check | What it catches |
|-------|----------------|
| `checkLinkNotInManifest` | OPF-067: resource listed as both metadata link and manifest item |
| `checkEmptyMetadataElements` | OPF-072: empty dc:source metadata elements |
| `checkCSSFontFaceUsage` | CSS-028: use of @font-face declaration (USAGE report) |
| `checkSpinePageMap` (extended) | OPF-062: Adobe page-map attribute on spine element |

**New check functions (Tier 3, Phase 2 — Known Gaps Closure):**

| Check | What it catches |
|-------|----------------|
| `checkCSSPositionFixed` | CSS-006: `position:fixed` usage (USAGE report) |
| `checkEmptyHrefUsage` | HTM-045: empty href attribute encountered (USAGE) |
| `checkRegionBasedProperty` | HTM-052: region-based epub:type on non-data-nav |
| `checkNCXUIDWhitespace` | NCX-004: dtb:uid leading/trailing whitespace (USAGE) |
| `checkUnreferencedManifestItems` | OPF-097: unreferenced manifest item (USAGE) |
| `checkMultiRenditionMetadata` | RSC-019: multi-rendition EPUB missing metadata.xml |
| `checkPaginationSourceMetadata` | OPF-066: missing pagination source metadata (EDUPUB) |
| `checkDataNavNotInSpine` | OPF-077: Data Navigation Document in spine |
| `checkCollections` (extended) | OPF-071/075/076: index, preview collection checks |
| `checkDictionaryCollection` | OPF-081/082/083/084: dictionary collection validation |
| `checkDictionaryHasContent` | OPF-078: dictionary collection needs dict content |
| `checkDictionaryDCType` | OPF-079: dict content without dc:type "dictionary" |
| `checkSKMFileExtension` | OPF-080: Search Key Map .xml extension |
| `checkSKMSpineReferences` | RSC-021: SKM must point to spine content docs |
| `checkMicrodataWithoutRDFa` | HTM-051: microdata without RDFa (EDUPUB) |

**Deliverables:**
- `scripts/java-audit.py` — comprehensive audit script (run with `--json` for machine-readable output)
- 19 new check functions total, 22 new BDD scenarios, 0 regressions: 923/924 BDD scenarios passing

**Consolidated known gaps (all tiers):**

The following checks are intentionally skipped. They're grouped by reason so future work can target a category. Codes marked SUPPRESSED are disabled by default in epubcheck itself.

**Could implement later (meaningful checks, deprioritized):**

| Code | Sev | What it checks | Why skipped |
|------|-----|----------------|-------------|
| `<input>` types | ERROR | Type-specific attribute validation (13+ variants) | Tier 1 RelaxNG; large surface area, low real-world impact |
| SVG/MathML models | ERROR | Full SVG/MathML content model enforcement | Tier 1 RelaxNG; complex schemas, rarely triggers |
| HTM-044 | USAGE | Unused namespace URI declared | Dead code in epubcheck (not actively emitted) |

**NAV-004 note:** Our NAV-004 implements "anchors must contain text" (ERROR), which differs from epubcheck's NAV-004 USAGE check for EDUPUB heading hierarchy. The EDUPUB version requires complex section/heading analysis for a defunct profile — not worth implementing.

**Multi-rendition container (Tier 2 Schematron — experimental spec extension):**

9 OCF metadata patterns + 4 selection/mapping patterns. OPF equivalents exist for single-rendition EPUBs (the common case).

**Dead code / already covered:**

| Code | Why skipped |
|------|-------------|
| OPF-011 | Commented out in epubcheck source (dead code); handled by OPF-088 |
| OPF-021 | DTBook-only href check (DAISY; not EPUB content) |
| OPF-047 | OEBPS 1.2 syntax info; already detected via `IsLegacyOEBPS12` |
| PKG-001 | Version mismatch info; handled by OPF-001 |
| PKG-015 | Unable to read contents; covered by PKG-008 |
| PKG-018 | File not found; handled by Go `os.Open` |
| PKG-020 | OPF not found; covered by OPF-002 |
| RSC-022 | Java version check; not applicable to Go |

**Epubcheck internal (CHK-001 through CHK-007):** Custom message override configuration errors. These are internal to epubcheck's message override system and have no equivalent in epubverify.

**SUPPRESSED in epubcheck (51 codes):** These are disabled by default in epubcheck itself — they defined checks that were later deemed too noisy or not spec-required. Includes all 10 SCP (scripting) codes, 12 CSS property checks, and various HTM/OPF codes. Full list: run `python3 scripts/java-audit.py --json | jq '.messages[] | select(.status=="suppressed" or (.status=="wontfix" and .severity=="SUPPRESSED"))'`.

---

---

## PROPOSED



### Update Doctor Mode

Given the more comprehensive tests, offer more fixes for checks that have a clear easy to implement adjustment to resolve in an epub.

### Doctor Mode BDD Tests

Add Gherkin scenarios for doctor mode. Currently only tested via Go unit tests. BDD scenarios would make the expected behavior more visible and could serve as documentation.

### Performance Benchmarking at Scale

Extend `make bench` to run against the full stress test corpus. Track per-book validation time, memory usage, startup overhead (JVM vs native Go), and batch throughput.

### CI Integration for Stress Tests

Add a CI job that downloads a cached set of test EPUBs, runs epubverify, compares against cached epubcheck results, and fails if any new disagreements appear.

---

## COMPLETED

### Tier 3 Java Code Analysis — Complete (Proposal 2)

Java code audit script (`scripts/java-audit.py`) — parses epubcheck's `MessageId.java` (315 message IDs), `DefaultSeverities.java` (severity mappings), and `MessageBundle.properties` (message texts). Greps all Java source files for `MessageId.XXX` references to map every error code to its emitting Java class. Cross-references against epubverify Go source and BDD features. **All 315 message IDs accounted for: 223 implemented, 9 suppressed, 83 wontfix, 0 missing.**

**Phase 1 — 4 new check functions:**

| Check | What it catches |
|-------|----------------|
| `checkLinkNotInManifest` | OPF-067: resource listed as both metadata link and manifest item |
| `checkEmptyMetadataElements` | OPF-072: empty dc:source metadata elements |
| `checkCSSFontFaceUsage` | CSS-028: use of @font-face declaration (USAGE report) |
| `checkSpinePageMap` (extended) | OPF-062: Adobe page-map attribute on spine element |

**Phase 2 — Known Gaps Closure (22 codes implemented):**

Systematically closed all "Could implement later", "EPUB Dictionaries/Index", and "EDUPUB/defunct profiles" gaps:

- **CSS/Content checks:** CSS-006 (position:fixed), HTM-045 (empty href), HTM-051 (microdata/RDFa), HTM-052 (region-based), NCX-004 (uid whitespace)
- **Package document:** OPF-066 (pagination source), OPF-077 (data-nav in spine), OPF-097 (unreferenced items), RSC-019 (multi-rendition metadata)
- **Collections:** OPF-071 (index XHTML), OPF-075 (preview content), OPF-076 (preview CFI)
- **EPUB Dictionaries:** OPF-078 (dict content), OPF-079 (dict dc:type), OPF-080 (SKM extension), OPF-081 (dict resource), OPF-082 (multiple SKM), OPF-083 (no SKM), OPF-084 (invalid resource), RSC-021 (SKM spine)
- Enhanced Collection type with Links field; added collection link parsing in OPF reader

- Machine-readable gap analysis: run `python3 scripts/java-audit.py --json`
- 22 new BDD scenarios, 0 regressions: 923/924 BDD scenarios passing

### Tier 2 Schematron Rule Analysis — Complete (Proposal 2)

Schematron audit script (`scripts/schematron-audit.py`) — parses epubcheck's 10 core Schematron .sch files covering 118 patterns and 180 individual checks. **All patterns accounted for: 167 implemented, 13 partial, 0 missing.**

**8 new check functions added:**

| Check | What it catches |
|-------|----------------|
| `checkDisallowedDescendants` | Forbidden nesting: address/form/progress/meter/caption/header/footer/label |
| `checkRequiredAncestor` | area requires map; img[ismap] requires a[href] |
| `checkBdoDir` | bdo missing required dir attribute |
| `checkSSMLPhNesting` | Nested ssml:ph attributes |
| `checkDuplicateMapName` | Duplicate map name values |
| `checkSelectMultiple` | Multiple selected without @multiple |
| `checkMetaCharset` | Duplicate meta charset elements |
| `checkLinkSizes` | sizes attribute on non-icon link |

- 28 new unit tests for Schematron checks, all passing
- 43 XHTML patterns implemented, 13 IDREF patterns mapped to existing checkIDReferences
- 15 multi-rendition/collection patterns marked wontfix (very niche features, OPF equivalents exist)
- 0 regressions on initial implementation

### Tier 1 RelaxNG Gap Analysis — Complete (Proposal 2)

RelaxNG schema audit script (`scripts/relaxng-audit.py`) — parses epubcheck's 34 RelaxNG .rnc schemas, extracts 115 element definitions and content model rules, compares against epubverify implementation. Initial audit identified **63 content model gaps → reduced to 1** (input type-specific attribute validation, low priority).

**8 new content model check functions:**

| Check | What it catches |
|-------|----------------|
| `checkBlockInPhrasing` | Block elements in phrasing-only parents (p, h1-h6, span, em, etc.) |
| `checkRestrictedChildren` | Invalid children of ul/ol, dl, hgroup, tr, thead/tbody/tfoot, select, etc. |
| `checkVoidElementChildren` | Child elements inside void elements (br, hr, img, input, etc.) |
| `checkTableContentModel` | Non-table children directly inside table |
| `checkInteractiveNesting` | Interactive elements nested in other interactive elements |
| `checkTransparentContentModel` | Transparent content model inheritance (a, ins, del, object, etc.) |
| `checkFigcaptionPosition` | Figcaption not first or last child of figure |
| `checkPictureContentModel` | Picture element structure (source* then img) |

- 29 unit tests for content model checks, all passing
- 16 test fixtures in `testdata/fixtures/relaxng-gaps/xhtml/`
- 0 regressions on initial implementation
- Only remaining gap: `<input>` type-specific attribute validation (low priority, 13+ type variants)

### Validation Engine (PRs #17–#22)

- Full EPUB 3.3 and 2.0.1 validation: OCF, OPF, XHTML, SVG, CSS, SMIL, navigation, accessibility, encoding, fixed-layout, cross-references
- 901/902 godog BDD scenarios passing (ported from epubcheck 5.3.0 test suite)
- Single-file validation support (`.opf`, `.xhtml`, `.svg`, `.smil`)
- Viewport meta tag parsing per EPUB 3.3 spec
- All 41 previously-failing complex scenarios fixed (DOCTYPE, entity refs, SVG, MathML, SSML, epub:switch, microdata, custom namespaces, URL conformance, prefix declarations)

### Doctor Mode (PR #21)

- 24 automatic fix types across 4 tiers (OCF/OPF/XHTML/CSS)
- Non-destructive: always writes to new file, re-validates output
- 35 unit tests + 5 integration tests
- Supports encoding transcoding (ISO-8859-1, Windows-1252, UTF-16 → UTF-8)

### Testing Infrastructure (PRs #23–#25)

- Self-contained godog/Gherkin test suite (no external repos needed)
- Stress test infrastructure: download scripts, comparison runner, analysis tools
- 77/77 real-world EPUBs match epubcheck verdict across 11 testing rounds
- 29 synthetic edge-case EPUBs
- Schematron audit script for coverage gap analysis
- Fuzzing tools (`cmd/epubfuzz/`)
- CI: GitHub Actions with godog BDD tests, Go version matrix

### Bug Fixes from Real-World Testing (11 Rounds)

Over 11 rounds of real-world testing, 30+ false-positive bugs were found and fixed:

| Round | EPUBs | Sources | Bugs Fixed |
|-------|-------|---------|------------|
| 1 | 5 | Gutenberg | 4 (OPF-037, CSS-002, HTM-015, NAV-010) |
| 2 | 25 | +Feedbooks | 4 (OPF-037, E2-007, OPF-036, RSC-002) |
| 3 | 30 | +Gutenberg EPUB2 | 0 |
| 4 | 42 | +IDPF epub3-samples | 7 (CSS-001, OPF-024, HTM-013, HTM-020, HTM-031, MED-004) |
| 5 | 49 | +DAISY, bmaupin | 0 |
| 6 | 86 | +28 IDPF, +11 Gutenberg | 7 (RSC-003, HTM-013, HTM-008, OPF-037, CSS-007, OPF-024, OPF-029) |
| 7 | 96 | +Gutenberg, +IDPF old | 0 |
| 8 | 122 | +Standard Ebooks | 3 (OPF-037, CSS-002, HTM-015) |
| 9 | 133 | +wareid, +readium | 0 |
| 10 | 162 | +29 synthetic | 5 (RSC-001, RSC-002, MED-001, HTM-005, RSC-004) |
| 11 | 77 new | Independent stress test | 3 (OPF-025b, RSC-005, RSC-032) |

### Documentation & Developer Experience

- AGENTS.md with TDD workflow guidelines
- Testing strategy doc with full bug history
- Spec update runbook for adding/debugging tests
- Doctor mode design docs
- Stress test README
