# epubverify

A fast, native Go EPUB validator that checks EPUB 2.0.1 and 3.3 files for compliance with W3C/IDPF standards.

## Status

**Experimental.** This project was created by Adam Mathes with AI agents as an alternative non-Java EPUB validator. It is not affiliated with any standards body or vendor.

epubverify passes 901 of 902 BDD scenarios ported from the [w3c/epubcheck](https://github.com/w3c/epubcheck) test suite (100% of non-pending scenarios) and matches epubcheck's validity verdict on 77/77 independently-tested real-world EPUBs. When in doubt, trust epubcheck over epubverify.

See [ROADMAP.md](ROADMAP.md) for detailed status and confidence assessment.

## Installation

```bash
go install github.com/adammathes/epubverify@latest
```

Or build from source:

```bash
git clone https://github.com/adammathes/epubverify.git
cd epubverify
make build
```

## Usage

```bash
# Validate an EPUB
epubverify book.epub

# JSON output
epubverify book.epub --json -          # to stdout
epubverify book.epub --json out.json   # to file

# Doctor mode: auto-repair common errors (writes to new file)
epubverify book.epub --doctor
epubverify book.epub --doctor -o repaired.epub
```

### Exit codes

| Code | Meaning |
|------|---------|
| 0 | Valid — no errors |
| 1 | Invalid — errors found |
| 2 | Fatal error or invalid arguments |

### Doctor mode

Doctor mode automatically repairs 24 types of common EPUB validation errors across 4 tiers (ZIP structure, OPF metadata, XHTML content, CSS/encoding). It always writes to a new file and re-validates the output.

See [docs/epub-doctor-mode.md](docs/epub-doctor-mode.md) for details.

## Testing

```bash
make test        # Unit tests (pkg/...)
make godog-test  # BDD spec compliance tests (godog/Gherkin)
make test-all    # Both
make stress-test # Real-world EPUB comparison vs epubcheck (requires Java)
make bench       # Benchmark vs epubcheck
```

All tests are self-contained — no external dependencies or repos needed.

## Project Structure

```
epubverify/
├── main.go               # CLI entry point
├── pkg/
│   ├── epub/             # EPUB file parsing and zip handling
│   ├── validate/         # Validation logic (OCF, OPF, HTML, CSS, nav, etc.)
│   ├── doctor/           # Auto-repair (--doctor mode)
│   └── report/           # Report generation (text and JSON output)
├── cmd/
│   ├── epubcompare/      # Tool to compare epubverify vs epubcheck output
│   └── epubfuzz/         # Fuzzing tool for robustness testing
├── test/godog/           # Godog step definitions and test runner
├── testdata/
│   ├── features/         # Gherkin feature files (epub2/, epub3/)
│   ├── fixtures/         # EPUB test fixtures
│   └── synthetic/        # Synthetic edge-case EPUBs
├── stress-test/          # Real-world EPUB stress testing infrastructure
├── scripts/              # Audit and analysis scripts
└── docs/                 # Design docs and testing strategy
```

## License

See LICENSE file for details.
