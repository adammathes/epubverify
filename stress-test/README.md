# Stress Test Infrastructure

Tools for stress-testing epubverify against the reference epubcheck Java
validator using real-world EPUBs.

## Quick Start

```bash
# 1. Build epubverify
make build

# 2. Download test EPUBs (one-time, respectful rate limiting)
bash stress-test/download-epubs.sh

# 3. Run both validators on all EPUBs
bash stress-test/run-comparison.sh

# 4. Analyze results
bash stress-test/analyze-results.sh
```

## Prerequisites

- **Go 1.24+** (to build epubverify)
- **Java 11+** (to run epubcheck)
- **epubcheck 5.3.0 JAR** — see [Installing epubcheck](#installing-epubcheck) below
- **curl** (for downloading EPUBs)
- **python3** (for the crawl pipeline scripts)

### Installing epubcheck

The crawl validation pipeline compares epubverify against the reference
[epubcheck](https://www.w3.org/publishing/epubcheck/) Java validator. Without
it, `crawl-validate.sh` still runs but can only test epubverify in isolation
(no false-positive/false-negative detection).

**Quick install** (downloads to the default location):

```bash
bash scripts/install-epubcheck.sh
```

**Manual install**:

```bash
EPUBCHECK_VERSION="5.3.0"
mkdir -p ~/tools
curl -sL -o /tmp/epubcheck.zip \
  "https://github.com/w3c/epubcheck/releases/download/v${EPUBCHECK_VERSION}/epubcheck-${EPUBCHECK_VERSION}.zip"
unzip -q -o /tmp/epubcheck.zip -d ~/tools/
rm /tmp/epubcheck.zip

# Verify it works
java -jar ~/tools/epubcheck-${EPUBCHECK_VERSION}/epubcheck.jar --version
```

The default path is `$HOME/tools/epubcheck-5.3.0/epubcheck.jar`. To use a
different location, set the `EPUBCHECK_JAR` environment variable:

```bash
export EPUBCHECK_JAR=/path/to/epubcheck.jar
```

## Corpus (200+ EPUBs from 5 sources)

| Source | Count | What it tests |
|--------|-------|---------------|
| **Project Gutenberg** | ~105 | Ebookmaker EPUB3 output, diverse content |
| **IDPF/W3C Samples** | 18 | FXL, MathML, SVG, media overlays, CJK, RTL |
| **Standard Ebooks** | 30 | High-quality EPUB3, rich accessibility metadata, se:* vocabulary |
| **Feedbooks** | 20 | Calibre-generated output patterns |
| **EPUB2 Variants** | 17 | Legacy EPUB 2.0.1 validation paths |

### Diversity dimensions

- **Non-English**: French, German, Italian, Spanish, Russian, Chinese, Japanese, Korean, Persian, Portuguese, Greek, Esperanto, Hindi/Sanskrit
- **Large**: Bible, Complete Shakespeare, Encyclopaedia Britannica, multi-volume histories
- **Legacy**: 17 EPUB2 variants for NCX/OPF 2.0/OPS coverage
- **Tool output**: Gutenberg Ebookmaker, Calibre/Feedbooks, Standard Ebooks toolchain

## Crawl Workflow (Recommended)

The crawl pipeline automatically discovers, downloads, and validates EPUBs from
public sources, then compares epubverify against epubcheck to find bugs.

```bash
# 1. Build epubverify
make build

# 2. Install epubcheck (one-time)
bash scripts/install-epubcheck.sh

# 3. Crawl new EPUBs from public sources
bash scripts/epub-crawler.sh --limit 10

# 4. Validate with both epubverify and epubcheck
bash scripts/crawl-validate.sh

# 5. Generate discrepancy report
bash scripts/crawl-report.sh
```

Crawled EPUBs are deduplicated by SHA-256 hash and tracked in
`stress-test/crawl-manifest.json`. The manifest accumulates across runs so
the corpus grows over time.

### Crawl options

```bash
# Target a specific source
bash scripts/epub-crawler.sh --source gutenberg --limit 20
bash scripts/epub-crawler.sh --source standardebooks --limit 10
bash scripts/epub-crawler.sh --source feedbooks --limit 10
bash scripts/epub-crawler.sh --source oapen --limit 5

# Preview without downloading
bash scripts/epub-crawler.sh --dry-run

# File GitHub issues for any discrepancies found
bash scripts/crawl-report.sh --file-issues
```

### Crawl sources

| Source | What it tests |
|--------|---------------|
| **Project Gutenberg** | Ebookmaker EPUB3, diverse public domain content |
| **Standard Ebooks** | High-quality EPUB3, accessibility metadata |
| **Feedbooks** | Calibre-generated EPUB2 with urn:uuid: identifiers |
| **OAPEN** | Scholarly open-access EPUBs with footnotes and citations |
| **Internet Archive** | Huge variety (requires archive.org network access) |

## Scripts

| Script | Purpose |
|--------|---------|
| `scripts/epub-crawler.sh` | Discover and download EPUBs from public sources |
| `scripts/crawl-validate.sh` | Validate crawled EPUBs with epubverify + epubcheck |
| `scripts/crawl-report.sh` | Generate discrepancy report from validation results |
| `scripts/install-epubcheck.sh` | Download and install epubcheck 5.3.0 |
| `stress-test/download-epubs.sh` | Download static EPUB corpus (legacy) |
| `stress-test/run-comparison.sh` | Run both validators on static corpus (legacy) |
| `stress-test/analyze-results.sh` | Compare static corpus results (legacy) |
| `stress-test/epub-sources.txt` | Catalog of EPUB URLs for static corpus |

### Static corpus download options (legacy)

```bash
bash stress-test/download-epubs.sh --all            # All sources (default)
bash stress-test/download-epubs.sh --gutenberg       # Project Gutenberg only
bash stress-test/download-epubs.sh --idpf            # IDPF/W3C samples only
bash stress-test/download-epubs.sh --standardebooks  # Standard Ebooks only
bash stress-test/download-epubs.sh --feedbooks       # Feedbooks only
bash stress-test/download-epubs.sh --epub2           # EPUB2 variants only
```

## Directory Layout

```
stress-test/
  epubs/           # Downloaded EPUB files (gitignored)
  results/         # Validation output JSON (gitignored)
    epubverify/    # epubverify JSON + stderr per book
    epubcheck/     # epubcheck JSON + stderr per book
  download-epubs.sh
  run-comparison.sh
  analyze-results.sh
  epub-sources.txt
  README.md
```

## Adding New EPUBs

1. Add the URL and description to `epub-sources.txt`
2. Add the download command to `download-epubs.sh`
3. Run `go test ./test/stress/` to verify source count and diversity requirements
4. Run the comparison and analysis scripts
5. If new discrepancies are found, document in `docs/testing-strategy.md`

## Notes

- All EPUBs are from public domain sources (Project Gutenberg, IDPF, Standard Ebooks, Feedbooks)
- Downloads use 2-second delays between requests to be respectful
- EPUBs and results are gitignored (only scripts and catalogs are committed)
- The analysis script produces a summary showing agreement/disagreement counts,
  false positives, false negatives, and per-check-ID frequency tables
- Source diversity is validated by Go tests in `test/stress/sources_test.go`
