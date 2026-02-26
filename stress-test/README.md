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
- **epubcheck JAR** — set `EPUBCHECK_JAR` environment variable, or place at
  `$HOME/tools/epubcheck-5.1.0/epubcheck.jar` (the default)
- **curl** (for downloading EPUBs)

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

## Scripts

| Script | Purpose |
|--------|---------|
| `download-epubs.sh` | Download EPUBs from public domain sources |
| `run-comparison.sh` | Run both validators and save JSON results |
| `analyze-results.sh` | Compare results and report discrepancies |
| `epub-sources.txt` | Catalog of EPUB URLs and sources |

### Download options

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
