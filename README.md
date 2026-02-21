# ⚠️ WARNING: Vibecoded Experiment

**This is an experimental project created as a vibecoded experiment. It is not production-ready and is not used anywhere yet. Use at your own risk.**

---

# epubverify

A Go-based EPUB validator that checks EPUB files for compliance with standards.

## Installation

### go install

```bash
go install github.com/adammathes/epubverify/cmd/epubverify@latest
```

This installs the `epubverify` binary to `$GOPATH/bin` (or `$HOME/go/bin`).

### Building from source

```bash
git clone https://github.com/adammathes/epubverify.git
cd epubverify
go build -o epubverify ./cmd/epubverify/
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

### Spec compliance tests

Spec tests run the validator against the full [epubverify-spec](../epubverify-spec) fixture suite and compare results against curated expected output.

```bash
# Point at the spec directory
export EPUBCHECK_SPEC_DIR=/path/to/epubverify-spec

# Build fixtures first (requires epubcheck installed)
cd $EPUBCHECK_SPEC_DIR && make build

# Run spec tests
make spec-test
```

### All make targets

```
make build       Build the binary
make test        Run unit tests (pkg/...)
make spec-test   Run spec compliance tests (requires EPUBCHECK_SPEC_DIR)
make compare     Run full parity comparison via spec scripts
make bench       Benchmark epubverify vs reference epubcheck
make clean       Remove built binary
```

## Project Structure

```
epubverify-go/
├── cmd/epubverify/    # CLI entry point
│   └── main.go
├── pkg/
│   ├── epub/          # EPUB file parsing and zip handling
│   ├── validate/      # Validation logic (OCF, OPF, HTML, CSS, nav, etc.)
│   └── report/        # Report generation (text and JSON output)
└── test/
    └── spec_test.go   # Spec compliance tests against epubverify-spec
```

## License

See LICENSE file for details.
