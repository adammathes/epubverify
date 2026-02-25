# Agent Guidelines for epubverify

## Development Style: Red/Green TDD

Always develop in **red/green TDD style**:

1. **Red**: Run the failing tests first to understand what needs to change.
2. **Green**: Write the minimal code to make that test pass.
3. **Refactor**: Clean up if needed, then verify tests still pass.
4. **Repeat**: Move to the next failing test.

Never write implementation code without a failing test to guide you.

## Running Tests

```bash
make test        # Unit tests — must always pass, do not regress
make godog-test  # BDD spec compliance tests (godog/Gherkin)
make test-all    # Both
make stress-test # Real-world EPUB comparison vs epubcheck (requires Java)
```

**Current state (February 2026):** 901/902 godog scenarios passing (1 pending), all unit tests passing, 77/77 stress test EPUBs match epubcheck.

## Epubcheck Reference

**epubcheck** is the reference implementation. When there is a disagreement between what
epubcheck reports and what epubverify reports, **assume epubcheck is correct**.

- epubcheck source: https://github.com/w3c/epubcheck
- The project uses epubcheck 5.3.0 feature files and fixtures in `testdata/`
- Error codes (OPF-xxx, HTM-xxx, RSC-xxx, etc.) match epubcheck's error catalog

**When you find a divergence**:
1. Check the epubcheck source to understand what it does
2. Check the EPUB spec to understand what the spec requires
3. If epubcheck and the spec agree → fix epubverify
4. If they disagree → still match epubcheck behavior, but note the divergence

## EPUB Specifications

- **EPUB 3.3**: https://www.w3.org/TR/epub-33/
- **EPUB 3.3 Reading Systems**: https://www.w3.org/TR/epub-rs-33/
- **EPUB 2.0.1** (legacy): https://idpf.org/epub/201

The `testdata/features/` directory contains the actual epubcheck Gherkin scenarios.
These are the ground truth for what epubverify must implement.

## Committing

Commit frequently. Good commit cadence:
- After making a failing test pass
- After a group of related fixes
- Before starting a new category of fixes

Commit message format: describe what was fixed and how many scenarios improved.
Example: `Fix OPF-012 nav property check: 41 → 38 failing scenarios`

## After Making Changes

1. Run `make test` — all unit tests must pass
2. Run `make godog-test` — note the new pass/fail counts
3. Update `ROADMAP.md` with the new counts and what was fixed
4. Commit

## Working with the Test Suite

Failing scenarios are in `testdata/features/` (Gherkin `.feature` files).
Fixtures (test EPUB files) are in `testdata/fixtures/`.

To investigate a specific failing test:
1. Find the feature file for that scenario
2. Look at the fixture it references
3. Run just that scenario: `go test ./test/godog/ -v -run TestFeatures/scenario_name`
4. Understand what epubcheck expects vs what epubverify produces

Step definitions: `test/godog/epubcheck_test.go`
Validation logic: `pkg/validate/`

## Code Organization

```
pkg/validate/
├── validator.go      # Main entry point, Options, Validate()
├── ocf.go            # OCF/container validation
├── opf.go            # OPF/package document validation
├── content.go        # XHTML/HTML content document checks
├── nav.go            # Navigation document checks
├── css.go            # CSS validation
├── media.go          # Media overlay (SMIL) checks
├── references.go     # Cross-reference checks (RSC codes)
├── encoding.go       # Encoding detection
├── epub2.go          # EPUB 2-specific checks
├── fxl.go            # Fixed-layout checks
├── viewport.go       # Viewport meta tag parsing
└── accessibility.go  # Accessibility checks
```

When the context window is getting long, **stop and commit your work** before the context
gets truncated. It is better to commit partial progress than to lose work.
