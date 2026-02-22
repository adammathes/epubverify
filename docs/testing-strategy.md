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

# 2. Download sample EPUBs (25 public domain books)
./test/realworld/download-samples.sh

# 3. Run the Go integration tests
make realworld-test

# 4. (Optional) Compare side-by-side with epubcheck (requires Java + epubcheck JAR)
EPUBCHECK_JAR=/path/to/epubcheck.jar make realworld-compare
```

## Sample Corpus

The corpus consists of 25 EPUBs from two sources: Project Gutenberg and
Feedbooks. 21 are valid, 4 are known-invalid (both tools agree).

### Valid Samples (21 — epubcheck and epubverify both report 0 errors)

| File | Title | Why included |
|------|-------|--------------|
| `pg11-alice.epub` | Alice in Wonderland | Small, simple structure |
| `pg84-frankenstein.epub` | Frankenstein | Multiple authors in metadata |
| `pg1342-pride-and-prejudice.epub` | Pride and Prejudice | Large (24 MB), heavy CSS, `epub:type="normal"` |
| `pg1661-sherlock.epub` | Sherlock Holmes | Multiple chapters |
| `pg2701-moby-dick.epub` | Moby Dick | Complex TOC |
| `pg74-twain-tom-sawyer.epub` | Tom Sawyer | Standard structure |
| `pg98-dickens-two-cities.epub` | A Tale of Two Cities | Standard structure |
| `pg345-dracula.epub` | Dracula | Standard structure |
| `pg1080-dante-inferno.epub` | Dante's Inferno | Translated work |
| `pg4300-joyce-ulysses.epub` | Ulysses | Large, complex |
| `pg2600-war-and-peace.epub` | War and Peace | Multiple `dc:contributor` elements |
| `pg1041-shakespeare-sonnets.epub` | Shakespeare's Sonnets | Poetry |
| `pg1524-hamlet.epub` | Hamlet | Drama |
| `pg996-don-quixote-es.epub` | Don Quixote (Spanish) | Non-English, large (44 MB) |
| `pg2000-don-quixote-es.epub` | Don Quixote (English) | Translation |
| `pg17989-les-miserables-fr.epub` | Les Miserables | French |
| `pg7000-grimm-de.epub` | Grimm's Fairy Tales | German, `dc:contributor` with ID |
| `pg25328-tao-te-ching-zh.epub` | Tao Te Ching | Chinese text |
| `pg1982-siddhartha-jp.epub` | Siddhartha | Multilingual |
| `pg5200-kafka-metamorphosis.epub` | Metamorphosis | Translator as `dc:contributor` |
| `pg46-christmas-carol-epub2.epub` | A Christmas Carol | **EPUB 2**, nested `navPoint` elements |

### Known-Invalid Samples (4 — both tools report errors)

| File | Title | Errors |
|------|-------|--------|
| `fb-sherlock-study.epub` | A Study in Scarlet (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |
| `fb-art-of-war.epub` | Art of War (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch, bad date |
| `fb-odyssey.epub` | The Odyssey (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |
| `fb-republic.epub` | The Republic (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |

All samples are public domain and freely downloadable. The download script
(`download-samples.sh`) is polite: it fetches a fixed set of URLs with a
1-second delay between requests.

Sample `.epub` files are git-ignored — they must be downloaded locally.

## Test Layers

### 1. Go Integration Test (`test/realworld/realworld_test.go`)

Two test functions:

- **`TestRealWorldSamples`** — validates all samples; valid samples must have
  0 errors and 0 warnings; known-invalid samples must have errors.
- **`TestKnownInvalidExpectedErrors`** — verifies known-invalid samples
  produce specific expected check IDs (OCF-003, E2-010).

Run with:
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
fb-art-of-war                            | INVALID E:2 W:6 | INVALID E:3 W:0 | YES
pg11-alice                               | VALID E:0 W:0 | VALID E:0 W:0 | YES
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

5. **If the sample is genuinely invalid** (epubcheck also reports errors),
   add it to the `knownInvalid` map in `realworld_test.go`.

### Good Sources for Samples

- **[Project Gutenberg](https://www.gutenberg.org/)** — Public domain,
  EPUB 3 with images. Append `.epub3.images` to the ebook URL.
  For EPUB 2, use `.epub.noimages`.
- **[Feedbooks](https://www.feedbooks.com/)** — Public domain, EPUB 2.
  URL pattern: `https://www.feedbooks.com/book/{id}.epub`.
- **[Standard Ebooks](https://standardebooks.org/)** — High-quality
  EPUB 3. Note: programmatic downloads are currently blocked.
- **[EPUB test suite](https://github.com/w3c/epub-tests)** — W3C's
  official EPUB 3 test documents (requires building from source).
- **[Open Textbook Library](https://open.umn.edu/opentextbooks/)** —
  CC-licensed textbooks with complex structure.

### Guidelines

- Only use freely available, legally distributable EPUBs.
- Don't bulk-download or scrape sites. Add specific URLs one at a time.
- Aim for diversity: different publishers, structures, EPUB versions,
  content types (novels, textbooks, poetry, comics), languages.
- Prefer samples that exercise different validation paths (CSS, images,
  audio, fixed layout, navigation, metadata).

## Bugs Found and Fixed

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

## Future Work

- **Add fixed-layout samples** — need EPUB 3 FXL content (comics, picture
  books). The IDPF epub3-samples repo has sources but requires building.
- **Add Standard Ebooks samples** — currently blocked by their anti-bot
  measures. Could build from their GitHub source repos.
- **Add audio/video EPUB samples** — rare in public domain; may need to
  construct synthetic test EPUBs.
- **Structured diff** — extend compare.sh to diff individual messages,
  not just validity verdicts, for more granular parity tracking.
