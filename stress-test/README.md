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

## Scripts

| Script | Purpose |
|--------|---------|
| `download-epubs.sh` | Download EPUBs from public domain sources |
| `run-comparison.sh` | Run both validators and save JSON results |
| `analyze-results.sh` | Compare results and report discrepancies |
| `epub-sources.txt` | Catalog of EPUB URLs and sources |

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
3. Run the comparison and analysis scripts
4. If new discrepancies are found, document in `docs/testing-strategy.md`

## Notes

- All EPUBs are from public domain sources (Project Gutenberg, IDPF samples)
- Downloads use 2-second delays between requests to be respectful
- EPUBs and results are gitignored (only scripts and catalogs are committed)
- The analysis script produces a summary showing agreement/disagreement counts,
  false positives, false negatives, and per-check-ID frequency tables
