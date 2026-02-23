# epubverify

A Go-based EPUB validator that checks EPUB files for compliance with standards.

## ⚠️ WARNING: Vibe-coded Experiment, WIP

**This is an experimental project created with AI agents by Adam Mathes to make an additional non-Java epub validator.**

**It is not production-ready and is not affiliated with any standards body or vendor.  It does pass the language independent [epubverify test suite](https://github.com/adammathes/epubverify-spec), but that was also vibe-coded. It definitely flags books wrongly right now -- if you get different output trust epubcheck, not this. Use at your own risk!**

## Installation

### go install

```bash
go install github.com/adammathes/epubverify@latest
```

This installs the `epubverify` binary to `$GOPATH/bin` (or `$HOME/go/bin`).

### Building from source

```bash
git clone https://github.com/adammathes/epubverify.git
cd epubverify
go build -o epubverify .
```

The compiled binary will be created as `epubverify` in the current directory.

### Verify installation

```bash
./epubverify --version
```

## Usage

### Basic validation

```bash
./epubverify path/to/book.epub
```

### JSON output

```bash
./epubverify path/to/book.epub --json -          # to stdout
./epubverify path/to/book.epub --json out.json   # to file
```

### Doctor mode (experimental)

Doctor mode automatically repairs common EPUB validation errors. It applies safe, mechanical fixes — things like missing mimetype files, wrong media types, bad date formats, obsolete HTML elements, encoding issues, and more (24 fix types total across 4 tiers).

```bash
# Repair an EPUB (writes to book.epub.fixed.epub)
./epubverify book.epub --doctor

# Specify output path
./epubverify book.epub --doctor -o repaired.epub
```

Doctor mode always writes to a new file — it never modifies the original. After applying fixes, it re-validates the output and reports before/after error counts.

```
Applied 3 fixes:
  [OCF-003] Fixed mimetype content
  [OPF-004] Added dcterms:modified
  [HTM-010] Replaced non-HTML5 DOCTYPE with <!DOCTYPE html>

Before: 3 errors, 0 warnings
After:  0 errors, 0 warnings
Output: book.epub.fixed.epub
```

See [docs/epub-doctor-mode.md](docs/epub-doctor-mode.md) for the full list of supported fixes, architecture details, and known limitations.

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Valid — no errors |
| 1 | Invalid — errors found |
| 2 | Fatal error or invalid arguments |

## Testing

### Unit tests

```bash
go test ./pkg/...
# or
make test
```

### Spec compliance tests (godog/Gherkin)

Spec compliance tests use [godog](https://github.com/cucumber/godog) to run Gherkin feature files against the validator. Feature files and EPUB fixtures live in `testdata/` within this repo — no external dependencies needed.

```bash
make godog-test
```

### All make targets

```
make build       Build the binary
make test        Run unit tests (pkg/...)
make godog-test  Run Gherkin/godog spec compliance tests
make test-all    Run all tests (unit + godog)
make bench       Benchmark epubverify vs reference epubcheck
make clean       Remove built binary
```

## Project Structure

```
epubverify/
├── main.go               # CLI entry point
├── pkg/
│   ├── epub/          # EPUB file parsing and zip handling
│   ├── validate/      # Validation logic (OCF, OPF, HTML, CSS, nav, etc.)
│   ├── doctor/        # Experimental auto-repair (--doctor mode)
│   └── report/        # Report generation (text and JSON output)
├── test/
│   └── godog/         # Godog step definitions and test runner
└── testdata/
    ├── features/      # Gherkin feature files (epub2/, epub3/)
    └── fixtures/      # EPUB test fixtures
```

## License

See LICENSE file for details.
