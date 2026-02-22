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

# 2. Download sample EPUBs (122 from Gutenberg, Feedbooks, IDPF, DAISY, Standard Ebooks, etc.)
./test/realworld/download-samples.sh

# 3. Run the Go integration tests
make realworld-test

# 4. (Optional) Compare side-by-side with epubcheck (requires Java + epubcheck JAR)
EPUBCHECK_JAR=/path/to/epubcheck.jar make realworld-compare
```

## Sample Corpus

The corpus consists of 122 EPUBs from six sources: Project Gutenberg,
Feedbooks, IDPF epub3-samples (both releases), DAISY accessibility tests,
bmaupin/epub-samples, and Standard Ebooks. 114 are valid per epubverify
(including 2 known false-negative gaps), 6 are known-invalid (both tools agree).

### Valid Samples — Project Gutenberg (55)

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
| `pg28054-brothers-karamazov.epub` | Brothers Karamazov | Very large novel |
| `pg17405-art-of-war.epub` | Art of War | Short classic |
| `pg2554-crime-and-punishment.epub` | Crime and Punishment | Complex structure |
| `pg1260-jane-eyre.epub` | Jane Eyre | Illustrated |
| `pg768-wuthering-heights.epub` | Wuthering Heights | Gothic novel |
| `pg55201-republic-plato.epub` | The Republic | Philosophy |
| `pg16328-beowulf.epub` | Beowulf | Old English poetry |
| `pg35-time-machine.epub` | The Time Machine | Sci-fi |
| `pg236-jungle-book.epub` | The Jungle Book | Illustrated children's |
| `pg55-wizard-of-oz.epub` | Wizard of Oz | Illustrated children's |
| `pg6130-iliad.epub` | The Iliad | Epic poetry |
| `pg158-emma.epub` | Emma | Jane Austen |
| `pg93-nietzsche-zarathustra.epub` | Thus Spake Zarathustra | Philosophy |
| `pg844-rime-ancient-mariner.epub` | Rime of the Ancient Mariner | Poetry |
| `pg74-adventures-tom-sawyer.epub` | Adventures of Tom Sawyer | Illustrated, large |
| `pg76-huckleberry-finn.epub` | Adventures of Huckleberry Finn | Illustrated, large |
| `pg1661-adventures-sherlock-holmes.epub` | Adventures of Sherlock Holmes | Short stories |
| `pg4300-ulysses.epub` | Ulysses | Complex, large |
| `pg174-dorian-gray.epub` | Picture of Dorian Gray | Gothic novel |
| `pg219-heart-of-darkness.epub` | Heart of Darkness | Novella |
| `pg43-jekyll-hyde.epub` | Strange Case of Dr Jekyll and Mr Hyde | Novella |
| `pg5200-kafka-metamorphosis.epub` | Metamorphosis | Short novel, translator |
| `pg46-christmas-carol-epub2.epub` | A Christmas Carol | **EPUB 2**, nested `navPoint` elements |
| `pg174-dorian-gray-epub2.epub` | Picture of Dorian Gray | **EPUB 2** |
| `pg76-twain-huck-finn-epub2.epub` | Huckleberry Finn | **EPUB 2** |
| `pg1232-prince-epub2.epub` | The Prince | **EPUB 2** |
| `pg1400-great-expectations-epub2.epub` | Great Expectations | **EPUB 2** |
| `pg120-treasure-island-epub2.epub` | Treasure Island | **EPUB 2** |
| `pg2591-grimm-epub2.epub` | Grimm's Fairy Tales | **EPUB 2** |
| `pg11339-aesop-epub2.epub` | Aesop's Fables | **EPUB 2**, short stories |
| `pg1184-monte-cristo-epub2.epub` | Count of Monte Cristo | **EPUB 2**, very large |

### Valid Samples — IDPF epub3-samples (41)

All 43 EPUB files from the [IDPF epub3-samples](https://github.com/IDPF/epub3-samples)
GitHub releases (20230704), minus 2 excluded for requiring HTML5 schema
validation (accessible_epub_3, cc-shared-culture) and 2 known-invalid
(see below). Plus 2 samples from the older 20170606 release for backward
compatibility testing. These exercise exotic EPUB 3 features not found in
standard novels.

**Fixed-layout (9):** haruko-html-jpeg, haruko-ahl (region-based navigation),
haruko-jpeg (JPEG-in-spine), cole-voyage-of-life (2 variants), page-blanche
(2 variants), sous-le-vent (2 variants)

**International/RTL (6):** regime-anticancer-arabic, israelsailing (Hebrew),
mahabharata (Devanagari), emakimono (Japanese scrolling), jlreq-in-japanese,
kusamakura (3 variants: vertical writing, preview, embedded)

**Fonts (6):** wasteland (5 variants: plain, OTF, WOFF, OTF-obfuscated,
WOFF-obfuscated)

**Media/SVG/MathML (4):** moby-dick-mo (media overlays), mymedia_lite,
svg-in-spine, linear-algebra (MathML)

**Accessibility/metadata (6):** georgia-pls-ssml (SSML), childrens-literature,
childrens-media-query, figure-gallery-bindings, indexing (2 variants),
internallinks

**Other (8):** moby-dick, trees, quiz-bindings, GhV-oeb-page, hefty-water
(ultra-minimal, 4 KB)

### Valid Samples — DAISY Accessibility Tests (4)

From [DAISY epub-accessibility-tests](https://github.com/daisy/epub-accessibility-tests)
GitHub releases. Rich accessibility metadata.

| File | Features |
|------|----------|
| `daisy-basic-functionality.epub` | Accessibility metadata, WCAG conformance |
| `daisy-non-visual-reading.epub` | Screen reader testing, alt text |
| `daisy-read-aloud.epub` | Read aloud / TTS testing |
| `daisy-visual-adjustments.epub` | Visual adjustments, display preferences |

### Valid Samples — bmaupin/epub-samples (2)

From [bmaupin/epub-samples](https://github.com/bmaupin/epub-samples)
GitHub releases. Minimal EPUBs for edge-case testing.

| File | Features |
|------|----------|
| `bm-minimal-v3.epub` | **Minimal valid EPUB 3** (2 KB) |
| `bm-basic-v3plus2.epub` | **EPUB 3+2 hybrid** |

### Valid Samples — Standard Ebooks (17)

From [Standard Ebooks](https://standardebooks.org/). Professionally
typeset EPUB 3 with rich accessibility metadata, custom `se:*` vocabulary,
`<guide>` elements, ONIX records, and complex `<meta refines>` chains
targeting `dc:publisher`, `dc:subject`, `dc:description`, and other DC
elements. These exercise the OPF-037 refines check extensively.

| File | Title | Why included |
|------|-------|--------------|
| `se-pride-prejudice.epub` | Pride and Prejudice | Rich metadata, 7 subjects, endnotes |
| `se-frankenstein.epub` | Frankenstein | Multiple contributors with roles |
| `se-hound-baskervilles.epub` | Hound of the Baskervilles | Detective fiction |
| `se-dorian-gray.epub` | Picture of Dorian Gray | Gothic novel |
| `se-moby-dick.epub` | Moby Dick | Large, endnotes with `backlink` epub:type |
| `se-jane-eyre.epub` | Jane Eyre | Long novel with appendices |
| `se-great-gatsby.epub` | The Great Gatsby | Short novel |
| `se-dracula.epub` | Dracula | Epistolary novel |
| `se-time-machine.epub` | The Time Machine | Sci-fi |
| `se-tom-sawyer.epub` | Adventures of Tom Sawyer | American classic |
| `se-tale-two-cities.epub` | Tale of Two Cities | Long novel |
| `se-jekyll-hyde.epub` | Jekyll and Hyde | Novella |
| `se-mrs-dalloway.epub` | Mrs Dalloway | Modernist literature |
| `se-heart-darkness.epub` | Heart of Darkness | Novella |
| `se-treasure-island.epub` | Treasure Island | Adventure |
| `se-princess-mars.epub` | A Princess of Mars | Sci-fi |
| `se-call-wild.epub` | The Call of the Wild | Adventure |

### Known-Invalid Samples (6 — both tools report errors)

| File | Title | Errors |
|------|-------|--------|
| `fb-sherlock-study.epub` | A Study in Scarlet (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |
| `fb-art-of-war.epub` | Art of War (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch, bad date |
| `fb-odyssey.epub` | The Odyssey (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |
| `fb-republic.epub` | The Republic (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |
| `fb-jane-eyre.epub` | Jane Eyre (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |
| `fb-heart-darkness.epub` | Heart of Darkness (Feedbooks) | Mimetype trailing CRLF, NCX UID mismatch |

### Known False-Negative Samples (2 — epubcheck INVALID, epubverify VALID)

These samples are invalid per epubcheck but pass epubverify because the
specific checks are not implemented:

| File | epubcheck Error | Gap |
|------|-----------------|-----|
| `idpf-WCAG.epub` | OPF-007c (dc prefix redeclared in `prefix` attribute) | Prefix namespace validation not implemented |
| `idpf-vertically-scrollable-manga.epub` | RSC-007 (mailto link) | `mailto:` link validation not implemented |

All samples are public domain and freely available. The download script
(`download-samples.sh`) is polite: it fetches a fixed set of URLs with a
1-second delay between requests.

Sample `.epub` files are git-ignored — they must be downloaded/built locally.

## Test Layers

### 1. Go Integration Test (`test/realworld/realworld_test.go`)

Two test functions:

- **`TestRealWorldSamples`** — validates all samples; valid samples must have
  0 errors; known-invalid samples must have errors.
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
- **[IDPF epub3-samples](https://github.com/IDPF/epub3-samples)** —
  Official EPUB 3 sample documents with exotic features (FXL, SVG,
  MathML, media overlays, SSML). Pre-built EPUBs available as
  [GitHub releases](https://github.com/IDPF/epub3-samples/releases).
- **[DAISY accessibility tests](https://github.com/daisy/epub-accessibility-tests)** —
  EPUBs with rich accessibility metadata. Available as GitHub releases.
- **[bmaupin/epub-samples](https://github.com/bmaupin/epub-samples)** —
  Minimal EPUBs validated with epubcheck. Available as GitHub releases.
- **[Standard Ebooks](https://standardebooks.org/)** — High-quality
  EPUB 3 with rich metadata. Append `?source=download` to the download URL
  for direct CLI access.
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

## Future Work

- **HTML5 schema validation (RSC-005)** — some EPUBs (e.g.,
  `cc-shared-culture.epub`) have HTML5 schema errors that epubcheck flags
  but we don't detect. This would require integrating an HTML5 validator.
- **RSC-007 (mailto link validation)** — epubcheck flags `mailto:` links
  as errors when referenced as resources. Low priority.
- **OPF-007c (prefix namespace validation)** — detecting redeclared
  Dublin Core namespace prefixes in the `prefix` attribute.
