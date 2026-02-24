# Runbook: Updating epubverify with the Testdata Workflow

This document describes how to work with the self-contained `testdata/`
test suite and how to add, port, or debug test scenarios.

The test suite uses [Godog](https://github.com/cucumber/godog) (Go + Gherkin/Cucumber BDD).
Feature files and EPUB fixtures live in `testdata/` alongside the repo.
No external dependencies or separate spec repos are required.

---

## Quick Start

```bash
# Run all godog BDD tests
go test ./test/godog/...

# Run unit tests
go test ./pkg/...

# Run everything
go test ./...
```

---

## Understanding the Test Suite

### Structure

```
testdata/
├── features/           ← Gherkin .feature files (test scenarios)
│   ├── epub3/          ←   EPUB 3 checks, organized by spec section
│   │   ├── 03-resources/resources.feature
│   │   ├── 04-ocf/
│   │   │   ├── container.feature
│   │   │   ├── filename-checker.feature
│   │   │   └── ...
│   │   └── ...
│   └── epub2/          ←   EPUB 2 / legacy checks
└── fixtures/           ← EPUB fixture files (test inputs)
    ├── epub3/
    │   └── 03-resources/
    │       ├── resources-cmt-font-truetype-valid/   ← directory = unpacked EPUB
    │       ├── resources-core-media-types-not-preferred-valid.opf  ← bare .opf
    │       └── ...
    └── epub2/
        └── ...

test/godog/
└── epubcheck_test.go   ← step definitions (connects Gherkin to Go code)
```

### How It Works

1. Gherkin `.feature` files describe scenarios in plain English
2. Step definitions in `epubcheck_test.go` match each sentence to Go code
3. The Go code calls the validator and checks the result

### Fixture Formats

- **Directory fixtures**: an unpacked EPUB (like `my-epub-valid/`) with
  `META-INF/container.xml`, `EPUB/`, `mimetype`, etc.
- **File fixtures**: a `.opf`, `.xhtml`, `.svg`, or `.smil` file for
  single-file validation

---

## Checking the Current State

```bash
# Pass/fail summary
go test ./test/godog/... 2>&1 | tail -5

# Full verbose output — shows which scenarios pass/fail
go test -v ./test/godog/... 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|FAIL)"

# Run a single feature file
cd test/godog && go test -godog.format=pretty \
  --godog.feature=../../testdata/features/epub3/03-resources/resources.feature .

# Run scenarios matching a keyword
cd test/godog && go test -godog.format=pretty \
  --godog.feature=../../testdata/features/epub3/03-resources/resources.feature \
  --godog.tags=@spec .
```

---

## Scenario 1: Adding a New Check

When you implement a new validation rule, add a corresponding scenario to
confirm it works.

### Step 1 — Create or find the feature file

Feature files are organized by spec section, mirroring epubcheck's test
structure. Find the right file (e.g., OPF checks → `epub3/03-resources/resources.feature`)
or create a new one.

```gherkin
# testdata/features/epub3/03-resources/resources.feature

  @spec @xref:sec-manifest-elem
  Scenario: Report items using non-preferred core media types
    Given the reporting level is set to USAGE
    When checking EPUB 'resources-cmt-font-truetype-valid'
    Then usage OPF-090 is reported 2 times
    But no errors or warnings are reported
```

### Step 2 — Create the fixture

Put a minimal EPUB (or bare OPF) in the matching `testdata/fixtures/` directory:

```
testdata/fixtures/epub3/03-resources/resources-cmt-font-truetype-valid/
├── META-INF/container.xml
├── EPUB/
│   ├── package.opf
│   ├── nav.xhtml
│   └── ...
└── mimetype
```

Keep fixtures minimal — only include files that are relevant to the check
being tested. The goal is to make it obvious what the fixture is testing.

### Step 3 — Implement the check in Go

Find the right validate file:

| Category | File |
|----------|------|
| PKG (container/zip) | `pkg/validate/ocf.go` |
| OPF (package doc) | `pkg/validate/opf.go` |
| RSC (references) | `pkg/validate/references.go` |
| HTM (content docs) | `pkg/validate/content.go` |
| NAV (navigation) | `pkg/validate/nav.go` |
| NCX (EPUB 2 NCX) | `pkg/validate/epub2.go` |
| CSS | `pkg/validate/css.go` |
| FXL (fixed-layout) | `pkg/validate/fxl.go` |
| MED (media overlays) | `pkg/validate/media.go` |
| ENC (encoding) | `pkg/validate/encoding.go` |

Every check is a small function called from the master phase function:

```go
// OPF-090: manifest items with non-preferred but valid core media types.
func checkNonPreferredMediaTypes(pkg *epub.Package, r *report.Report) {
    for _, item := range pkg.Manifest {
        if preferred, ok := nonPreferredMediaTypes[item.MediaType]; ok {
            r.Add(report.Usage, "OPF-090",
                fmt.Sprintf("... '%s' uses non-preferred media type ...", item.Href))
        }
    }
}
```

Then call it from `checkOPF`:

```go
// OPF-090: manifest items with non-preferred but valid core media types
checkNonPreferredMediaTypes(pkg, r)
```

### Step 4 — Run and iterate

```bash
go test ./test/godog/... 2>&1 | tail -5
```

Iterate until the scenario passes. Then run all tests to confirm no regressions:

```bash
go test ./...
```

---

## Scenario 2: Porting Scenarios from Upstream epubcheck

When a new version of epubcheck adds checks, port the corresponding scenarios.

### Step 1 — Find the upstream feature file

Epubcheck's feature files live at:
`https://github.com/w3c/epubcheck/tree/main/src/test/resources/com/adobe/epubcheck/test`

Or browse them locally if you have the repo.

### Step 2 — Copy feature file and fixtures

1. Copy the relevant `.feature` file to `testdata/features/epub3/<section>/`
2. Copy the EPUB fixture directories to `testdata/fixtures/epub3/<section>/`
3. Remove or adapt any scenarios that test epubcheck-internal behavior
   (e.g., Java stack traces, epubcheck-specific severity labels)

### Step 3 — Check for missing step definitions

Run the tests. Any scenario that prints `godog.ErrPending` or
`Step implementation is missing` needs a new step definition:

```bash
go test -v ./test/godog/... 2>&1 | grep -A2 "pending\|Pending\|undefined"
```

Add the step to `test/godog/epubcheck_test.go`:

```go
ctx.Step(`^checking file name '([^']*)'$`, func(name string) error {
    s.result = validate.ValidateFilenameString(name, s.epubVersion == "2")
    return nil
})
```

### Step 4 — Adapt message matching if needed

The step `Then error OPF-035 is reported` checks that a message with
CheckID `"OPF-035"` exists. If the message text doesn't matter, this is
sufficient. If you need to match text, use:

```gherkin
Then error OPF-035 is reported
And message contains 'some expected text'
```

---

## Scenario 3: Debugging a Failing Scenario

### Run a single scenario

```bash
cd test/godog && go test -v -godog.format=pretty \
  --godog.feature=../../testdata/features/epub3/03-resources/resources.feature .
```

### Add temporary logging

In `epubcheck_test.go`, the `s.result` field holds the `*report.Report`.
Print all messages to understand what's being emitted:

```go
// In a step definition, temporarily:
for _, m := range s.result.Messages {
    fmt.Printf("  [%s] %s: %s\n", m.Severity, m.CheckID, m.Message)
}
```

### Validate the fixture manually

```bash
go run ./cmd/epubverify/main.go testdata/fixtures/epub3/03-resources/resources-cmt-font-truetype-valid
```

### Check the fixture structure

The validator loads fixtures from `testdata/fixtures/` relative to the
test binary's working directory. If a fixture path seems wrong, check:

```go
// In epubcheck_test.go:
fmt.Println(s.lastFixturePath) // debug the resolved path
```

---

## Scenario 4: Adding a New Severity Level or Step

The step definitions in `epubcheck_test.go` handle the most common patterns:

```gherkin
Then error CODE is reported
Then error CODE is reported N times
Then warning CODE is reported
Then usage CODE is reported
Then no errors or warnings are reported
Then no other errors or warnings are reported
Then info CODE is reported N times
```

If a scenario uses a step pattern that doesn't exist, add it:

```go
ctx.Step(`^my new step pattern '([^']*)'$`, func(value string) error {
    // implement the step
    return nil
})
```

Use `godog.ErrPending` to mark steps that aren't ready yet:

```go
ctx.Step(`^parsing viewport (.+)$`, func(vp string) error {
    return godog.ErrPending
})
```

---

## Report API Reference

```go
// Add a message (most common)
r.Add(report.Error, "OPF-035", "human-readable message")

// Add with file location
r.AddWithLocation(report.Warning, "CSS-003", "message", "OEBPS/style.css")

// Severity constants
report.Fatal    // stops validation pipeline
report.Error    // EPUB is invalid
report.Warning  // suspicious but not invalid
report.Usage    // informational; mirrors epubcheck 'usage' level
report.Info     // suppressed in normal output
```

---

## EPUB Struct Reference

```go
ep.Files                        // map[string][]byte — all zip entries by path
ep.RootfilePath                 // path to the OPF file (e.g., "EPUB/package.opf")
ep.Package                      // parsed OPF package document
ep.Package.Manifest             // []ManifestItem
ep.Package.Spine                // []SpineItemref
ep.Package.Metadata             // title, language, identifier, etc.
ep.ResolveHref(item.Href)       // resolve manifest href to container path
ep.ReadFile("EPUB/content.opf") // read a file from the container
```

---

## Verifying Your Work

After any changes:

```bash
# All tests
go test ./...

# Godog only (shows BDD results)
go test ./test/godog/... 2>&1 | tail -3
```

A clean result looks like:

```
ok      github.com/adammathes/epubverify/test/godog   1.35s
```
