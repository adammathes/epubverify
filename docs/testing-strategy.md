# Real-World Testing Strategy

This document describes epubverify's strategy for testing against real-world
EPUBs and comparing results with the reference [epubcheck](https://github.com/w3c/epubcheck)
implementation.

## Goals

1. **Catch false positives** — epubverify should not flag valid EPUBs that
   epubcheck accepts.
2. **Catch false negatives** — epubverify should flag issues that epubcheck
   reports (where our checks overlap).
3. **Repeatable process** — anyone can reproduce the comparison from scratch.
4. **Grow the corpus over time** without breaking the existing tests.

## Quick Start

```bash
# 1. Build epubverify
make build

# 2. Download sample EPUBs (5 public domain books from Project Gutenberg)
./test/realworld/download-samples.sh

# 3. Run the Go integration tests
make realworld-test

# 4. (Optional) Compare side-by-side with epubcheck (requires Java + epubcheck JAR)
EPUBCHECK_JAR=/path/to/epubcheck.jar make realworld-compare
```

## Sample Corpus

The initial corpus consists of 5 Project Gutenberg EPUBs (EPUB 3, with images):

| File | Title | Why included |
|------|-------|--------------|
| `pg11-alice.epub` | Alice's Adventures in Wonderland | Small, simple structure |
| `pg84-frankenstein.epub` | Frankenstein | Multiple authors in metadata |
| `pg1342-pride-and-prejudice.epub` | Pride and Prejudice | Large (24 MB), heavy CSS, uses `epub:type="normal"` |
| `pg1661-sherlock.epub` | Adventures of Sherlock Holmes | Multiple chapters, standard structure |
| `pg2701-moby-dick.epub` | Moby Dick | Large, complex table of contents |

All samples are public domain and freely downloadable. The download script
(`download-samples.sh`) is polite: it fetches a fixed set of URLs with a
1-second delay between requests.

Sample `.epub` files are git-ignored — they must be downloaded locally.

## Test Layers

### 1. Go Integration Test (`test/realworld/realworld_test.go`)

Validates each sample EPUB and asserts:
- `valid == true` (no errors or warnings)
- `error_count == 0`
- `warning_count == 0`

This catches false positives. Run with:
```bash
go test ./test/realworld/ -v
```

Skips gracefully if no samples are downloaded.

### 2. Comparison Script (`test/realworld/compare.sh`)

Runs both epubverify and epubcheck against all samples and produces a
side-by-side table:

```
SAMPLE                                   | EPUBVERIFY   | EPUBCHECK    | MATCH
-----------------------------------------+--------------+--------------+------
pg11-alice                               | VALID E:0 W:0 | VALID E:0 W:0 | YES
pg1342-pride-and-prejudice               | VALID E:0 W:0 | VALID E:0 W:0 | YES
...
```

Exits with code 0 if all validity verdicts match, code 1 if any differ.
JSON results are saved to `test/realworld/results/` for manual inspection.

### 3. Makefile Targets

| Target | Description |
|--------|-------------|
| `make realworld-test` | Run Go integration tests against samples |
| `make realworld-compare` | Run full epubverify vs epubcheck comparison |

## Adding More Samples

To expand the corpus:

1. **Add the URL to `download-samples.sh`** in the `SAMPLES` array:
   ```bash
   SAMPLES=(
     ...existing entries...
     "newfile.epub|https://example.com/book.epub|Description"
   )
   ```

2. **Run the download script**: `./test/realworld/download-samples.sh`

3. **Run the tests**: `make realworld-test`

4. **If tests fail**, the failures indicate bugs to investigate and fix.

### Good Sources for Samples

- **[Project Gutenberg](https://www.gutenberg.org/)** — Public domain,
  EPUB 3 with images. Append `.epub3.images` to the ebook URL.
- **[Standard Ebooks](https://standardebooks.org/)** — High-quality
  EPUB 3 with careful metadata. Download via their catalog.
- **[EPUB test suite](https://github.com/w3c/epub-tests)** — W3C's
  official EPUB 3 test documents.
- **[Open Textbook Library](https://open.umn.edu/opentextbooks/)** —
  CC-licensed textbooks with complex structure.

### Guidelines

- Only use freely available, legally distributable EPUBs.
- Don't bulk-download or scrape sites. Add specific URLs one at a time.
- Aim for diversity: different publishers, structures, EPUB versions,
  content types (novels, textbooks, poetry, comics).
- Prefer samples that exercise different validation paths (CSS, images,
  audio, fixed layout, navigation).

## Bugs Found and Fixed

### Round 1 (Initial Corpus, 2026-02-22)

All 5 Gutenberg EPUBs passed epubcheck with 0 errors and 0 warnings.
epubverify reported false positives on all 5. Four bugs were identified
and fixed:

| Bug | Check ID | Severity | Description | Fix |
|-----|----------|----------|-------------|-----|
| 1 | OPF-037 | ERROR | `refines` target IDs on `dc:creator` elements not tracked | Added `ID` field to `DCCreator` type; parser now captures `id` attribute; validator includes creator IDs in valid targets map |
| 2 | CSS-002 | WARNING | CSS selectors with pseudo-classes (e.g., `a:link`) matched as property names | Rewrote property extraction to only match lines inside rule blocks (`{...}`) |
| 3 | HTM-015 | WARNING | Unknown `epub:type` values flagged as warnings | Downgraded to INFO — EPUB 3 structural semantics vocabulary is extensible per spec |
| 4 | NAV-010 | WARNING | Unknown landmark `epub:type` values flagged as warnings | Downgraded to INFO — same rationale as HTM-015 |

## Future Work

- **Expand to 20+ samples** covering EPUB 2, fixed-layout, audio/video,
  and non-English content.
- **Add Standard Ebooks samples** (their EPUBs use sophisticated
  `epub:type` semantics and advanced CSS).
- **Test invalid EPUBs** — find known-bad EPUBs and verify epubverify
  flags the same errors as epubcheck.
- **CI integration** — cache downloaded samples in CI and run the
  comparison as part of the test suite.
- **Structured diff** — extend the comparison script to diff individual
  messages, not just validity verdicts.
