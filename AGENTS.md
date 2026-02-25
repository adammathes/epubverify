# Agent Guidelines for epubverify

## Development Style: Red/Green TDD

Always develop in **red/green TDD style**:

1. **Red**: Run the failing tests first to understand what needs to change. Identify the specific failing scenario.
2. **Green**: Write the minimal code to make that test pass. No more, no less.
3. **Refactor**: Clean up if needed, then verify tests still pass.
4. **Repeat**: Move to the next failing test.

Never write implementation code without a failing test to guide you. Never write tests that already pass.

## Running Tests

```bash
# Unit tests (must always pass — do not regress these)
make test
# or: go test ./pkg/...

# BDD/godog spec compliance tests
make godog-test
# or: go test ./test/godog/ -v -count=1

# All tests
make test-all
```

**Critical**: All unit tests (`pkg/...`) must pass before committing. The godog tests are
the in-progress spec compliance suite — failing godog scenarios are expected while work is
in progress, but **do not regress previously-passing scenarios**.

## Epubcheck Reference

**epubcheck** is the reference implementation. When there is a disagreement between what
epubcheck reports and what epubverify reports, **assume epubcheck is correct**.

- epubcheck source: https://github.com/w3c/epubcheck
- epubcheck releases: https://github.com/w3c/epubcheck/releases
- The project uses epubcheck 5.3.0 feature files and fixtures in `testdata/`
- Error codes (OPF-xxx, HTM-xxx, RSC-xxx, etc.) match epubcheck's error catalog

**When you find a divergence**:
1. Check the epubcheck source to understand what it does
2. Check the EPUB spec (see below) to understand what the spec requires
3. If epubcheck and the spec agree → fix epubverify
4. If epubcheck and the spec disagree → still match epubcheck behavior, but note the
   divergence with a comment in the code and in ROADMAP.md

## EPUB Specifications

The primary specs to consult:

- **EPUB 3.3**: https://www.w3.org/TR/epub-33/
- **EPUB 3.3 Reading Systems**: https://www.w3.org/TR/epub-rs-33/
- **EPUB Packages 3.3** (OPF): https://www.w3.org/TR/epub-33/#sec-package-documents
- **EPUB Content Documents 3.3**: https://www.w3.org/TR/epub-33/#sec-content-docs
- **EPUB Open Container Format (OCF) 3.3**: https://www.w3.org/TR/epub-33/#sec-ocf
- **EPUB 2.0.1** (legacy): https://idpf.org/epub/201

The `testdata/features/` directory contains the actual epubcheck Gherkin scenarios that
define the expected behavior. These are the ground truth for what epubverify must implement.

## Committing Regularly

Commit frequently as you make progress. A good commit cadence:

- After making a failing test pass: commit immediately 🎉
- After a group of related fixes: commit with a descriptive message
- Before starting a new category of fixes: commit any in-progress work

**Celebrate when tests pass!** In your thoughts/discussion, use emoji when you fix
scenarios: 🟢 for a passing test, 🎉 for a batch of fixes, 🔴 for a failing test you're
working on.

Commit message format: describe what was fixed and how many scenarios improved.
Example: `Fix OPF-012 nav property check: 41 → 38 failing scenarios`

## After Making Changes

1. Run `make test` — all unit tests must pass
2. Run `make godog-test` — note the new pass/fail counts
3. Update `ROADMAP.md` with the new counts and what was fixed
4. Commit

## ROADMAP.md

The `ROADMAP.md` file at the repo root tracks:
- Current pass/fail counts for godog scenarios
- The breakdown of remaining failures by category
- The history of what has been completed

**Always update ROADMAP.md** after a session of fixes. Keep it accurate.

## Working with the Test Suite

Failing scenarios are in `testdata/features/` (Gherkin `.feature` files).
Fixtures (test EPUB files) are in `testdata/fixtures/`.

To investigate a specific failing test:
1. Find the feature file for that scenario
2. Look at the fixture it references
3. Run just that scenario: `go test ./test/godog/ -v -run TestFeatures/scenario_name`
4. Understand what epubcheck expects vs what epubverify produces

The step definitions are in `test/godog/epubcheck_test.go`.
The validation logic is in `pkg/validate/`.

## Code Organization

```
pkg/validate/
├── validator.go    # Main entry point, Options, Validate()
├── ocf.go          # OCF/container validation
├── opf.go          # OPF/package document validation
├── content.go      # XHTML/HTML content document checks
├── nav.go          # Navigation document checks
├── css.go          # CSS validation
├── media.go        # Media overlay (SMIL) checks
├── references.go   # Cross-reference checks (RSC codes)
├── encoding.go     # Encoding detection
├── epub2.go        # EPUB 2-specific checks
├── fxl.go          # Fixed-layout checks
└── accessibility.go # Accessibility checks
```

When the context window is getting long, **stop and commit your work** before the context
gets truncated. It is better to commit partial progress than to lose work.
