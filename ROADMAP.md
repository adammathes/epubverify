# Roadmap

## DATE UPDATED

February 25, 2026

---

## STATUS

### Test Results

| Suite | Result | Notes |
|-------|--------|-------|
| **Godog BDD scenarios** | 901/902 passing (1 pending) | 100% pass rate on non-pending scenarios |
| **Unit tests** | All passing | 35 doctor tests, 11 content model tests, epub/validate tests |
| **Stress tests** | 77/77 match epubcheck | Independent real-world EPUBs |
| **Synthetic EPUBs** | 29/29 match epubcheck | Purpose-built edge cases |

### Where We Have Confidence

**High confidence — EPUB 3.3 validation core.** The 901 passing BDD scenarios are ported directly from epubcheck's own test suite and cover OCF container checks, OPF package document validation, XHTML/SVG/SMIL content document checks, navigation document validation, CSS checks, media overlay validation, fixed-layout checks, accessibility checks, cross-reference resolution, and encoding detection. The stress test corpus of 77 independently-downloaded real-world EPUBs (from Project Gutenberg, IDPF samples, Standard Ebooks, DAISY, Feedbooks, wareid, readium) all produce the same valid/invalid verdict as epubcheck 5.1.0.

**High confidence — EPUB 2.0.1 validation.** 7 feature files covering NCX, OCF, OPF, and OPS checks for EPUB 2. The stress test corpus includes EPUB 2 books.

**High confidence — Doctor mode.** 24 fix types across 4 tiers, all with unit tests and integration tests that take broken EPUBs to 0 errors.

### Where We Have Less Confidence

**Medium confidence — Full HTML5 content model (RSC-005).** We now detect block-in-phrasing violations (e.g., `<div>` inside `<p>`, `<table>` inside `<span>`) and restricted children violations (e.g., `<div>` inside `<ul>`, `<p>` inside `<tr>`). The Tier 1 RelaxNG gap analysis (`scripts/relaxng-audit.py`) identified 63 content model rules; the highest-priority gaps (block-in-inline, restricted children) are implemented. Remaining gaps: void element children, transparent content models, interactive nesting beyond `<a>`, input type-specific attributes.

**Medium confidence — Edge-case error codes.** A handful of epubcheck error codes rely on schema validation or features we haven't implemented: RSC-007 (mailto links), RSC-020 (CFI URLs), OPF-007c (prefix redeclaration), PKG-026 (font obfuscation), OPF-043 (complex fallback chains).

**Lower confidence — Non-English and exotic EPUBs.** The test corpus is heavily English-biased. CJK, Arabic/RTL, Cyrillic, and Indic script coverage is limited to a few IDPF samples.

**Lower confidence — Very large EPUBs.** Testing has been on typical-sized books. Memory usage and performance on 50MB+ EPUBs is untested.

### Progress History

Started at 605/903 (67%) → 826/903 (91.6%) → 867/902 → **901/902 (100% non-pending)**

---

## APPROVED

_Large work items approved by human editor (adammathes)._

No items currently approved. See PROPOSED section for candidates.

---

## PROPOSED

_Large work items proposed by AI or human editors. Not yet approved._

### Proposal 1: Long-Running EPUB Scouring Stress Test

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
- For new discrepancies: creates a GitHub issue with the EPUB source URL, both validator outputs, and the specific error code differences
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

### Proposal 2: Systematic Gap Extraction from Epubcheck's Three Validation Tiers

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

**Tier 1: RelaxNG Schema Analysis** ✅ DONE (Phase 1)

Audit script (`scripts/relaxng-audit.py`) parses epubcheck's 34 .rnc schemas, extracts 115 element definitions and 305 patterns, identifies 63 content model gaps. High-priority gaps (block-in-phrasing, restricted children) are implemented. See `testdata/fixtures/relaxng-gaps/gap-analysis.json` for the full machine-readable report.

**Completed:**
- Schema inventory and parser (34 files, 115 elements, 87 content rules)
- Block-in-phrasing detection: `<div>` in `<p>`, `<table>` in `<span>`, etc.
- Restricted children: `<ul>`/`<ol>` → `<li>`, `<tr>` → `<td>`/`<th>`, `<select>` → `<option>`, etc.
- 8 unit tests, 16 test fixtures, gap analysis JSON report

**Remaining (Tier 1 Phase 2):**
- Void element children (br/hr/img with content)
- Transparent content model inheritance (a, ins, del, object)
- Interactive nesting beyond `<a>` (button, input, select in `<a>`)
- SVG/MathML full content model
- Input type-specific attribute validation

**Tier 2: Schematron Rule Analysis (extending existing audit)**

We already have `scripts/schematron-audit.py` which parses the schematron files and maps assertions to epubcheck error codes. Extend this:

1. **Complete the mapping.** Ensure every schematron assertion is mapped to an epubcheck error code.
2. **Cross-reference with our godog scenarios.** For each schematron rule, check whether we have a godog scenario that tests the corresponding error code.
3. **Identify untested rules.** Rules with no corresponding godog scenario are potential gaps.
4. **Create test fixtures.** For each untested rule, create a minimal EPUB fixture that triggers the rule and verify epubverify catches it.
5. **Fix gaps.** Implement missing checks.

The schematron audit already identified several gaps in earlier work. A second pass with the updated codebase may reveal new ones.

**Tier 3: Java Code Analysis**

The Java code checks are the hardest to audit systematically because they're scattered across many classes. Approach:

1. **Grep for error codes.** Search the epubcheck Java source for every `message(MessageId.XXX)` call. Build a complete list of error codes emitted by Java code (as opposed to schema validation).
2. **Cross-reference with our implementation.** For each error code, check whether epubverify emits it and whether we have a test for it.
3. **Categorize gaps.** Group missing error codes by category: cross-reference checks, media type checks, metadata checks, content checks, etc.
4. **Prioritize.** Focus on error codes that appear in real-world EPUBs (use the stress test corpus).

**Deliverables:**

- A gap analysis document listing every check we're missing, organized by tier and priority
- New test fixtures for each identified gap
- Incremental PRs closing the gaps, starting with the highest-priority items

**What we already have:**
- `scripts/schematron-audit.py` — parses schematron, maps to error codes
- `testdata/fixtures/schematron-audit/` — fixtures from the first audit pass
- The godog test suite itself — can be queried for which error codes have scenarios
- The stress test corpus — shows which error codes appear in practice

**What we'd need to build:**
- RelaxNG schema parser/analyzer (could be a Python script similar to the schematron audit)
- Java code error-code extractor (grep + analysis script)
- Gap analysis document
- New test fixtures and scenarios for identified gaps
- Implementation of missing checks

**Estimated scope:** Large. The RelaxNG analysis alone is significant. Best done incrementally — one tier at a time, highest-priority gaps first.

---

### Proposal 3: Increase Real-World EPUB Test Coverage

Expand the stress test corpus beyond the current 77 EPUBs with diverse sources:

- **Standard Ebooks** (~700 titles): High quality EPUB3, rich accessibility metadata, custom `se:*` vocabulary
- **OAPEN scholarly books**: Complex structure, footnotes, indexes, math
- **Non-English EPUBs**: CJK, Arabic/RTL, Cyrillic, Indic scripts
- **Very large EPUBs**: 50MB+ image-heavy books for performance testing
- **Calibre/Sigil-generated**: Tool-specific output patterns
- **EPUB2 depth**: More EPUB2-only books for legacy path coverage

### Proposal 4: Doctor Mode BDD Tests

Add Gherkin scenarios for doctor mode. Currently only tested via Go unit tests. BDD scenarios would make the expected behavior more visible and could serve as documentation.

### Proposal 5: Performance Benchmarking at Scale

Extend `make bench` to run against the full stress test corpus. Track per-book validation time, memory usage, startup overhead (JVM vs native Go), and batch throughput.

### Proposal 6: CI Integration for Stress Tests

Add a CI job that downloads a cached set of test EPUBs, runs epubverify, compares against cached epubcheck results, and fails if any new disagreements appear.

---

## COMPLETED

### Tier 1 RelaxNG Gap Analysis (Proposal 2, Phase 1)

- RelaxNG schema audit script (`scripts/relaxng-audit.py`) — parses epubcheck's 34 RelaxNG .rnc schemas, extracts 115 element definitions and content model rules, compares against epubverify implementation
- Initial audit identified 63 content model gaps → **reduced to 11** (0 high, 1 medium, 10 low priority)
- Implemented block-in-phrasing detection (`checkBlockInPhrasing`) — catches block elements (div, table, ul, etc.) inside phrasing-only parents (p, h1-h6, span, em, strong, etc.)
- Implemented restricted children validation (`checkRestrictedChildren`) — catches invalid children of ul/ol, dl, hgroup, tr, thead/tbody/tfoot, select, optgroup, colgroup, datalist
- Implemented void element children (`checkVoidElementChildren`) — catches child elements inside br, hr, img, input, etc.
- Implemented table content model (`checkTableContentModel`) — catches non-table children (p, div, span) directly inside table
- 18 new unit tests for content model checks, all passing
- 16 test fixtures in `testdata/fixtures/relaxng-gaps/xhtml/`
- JSON gap analysis report in `testdata/fixtures/relaxng-gaps/gap-analysis.json`
- 0 regressions: 901/902 BDD scenarios still passing

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
