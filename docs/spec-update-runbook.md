# Runbook: Syncing epubverify with epubverify-spec

This document describes how to update the Go implementation when the
[epubverify-spec](https://github.com/adammathes/epubverify-spec) test suite changes.

---

## Setup (One-Time)

```bash
# Clone both repos side by side
git clone https://github.com/adammathes/epubverify-spec
git clone https://github.com/adammathes/epubverify

# Build spec fixtures (requires Java + epubcheck installed; run bootstrap.sh first)
cd epubverify-spec
./bootstrap.sh
make build

# The Makefile auto-detects ../epubverify-spec, or set manually:
# export EPUBCHECK_SPEC_DIR=/path/to/epubverify-spec
```

---

## Check Current Parity

Before making any changes, see where things stand:

```bash
cd epubverify

# Quick: just pass/fail per spec test
make spec-test

# Full parity report against every check in checks.json
make compare
# Output: PASS/FAIL/SKIP per check with a summary percentage
```

---

## Scenario 1: Spec Added New Checks

The most common update — new checks were added to `checks.json` and new
fixtures added to `expected/`. The Go implementation needs to handle them.

### Step 1 — Pull the latest spec

```bash
cd epubverify-spec
git pull
make build    # rebuild fixtures if any were added
```

### Step 2 — Find what's failing

```bash
cd epubverify
make spec-test 2>&1 | grep FAIL
```

Each failing test names a check ID. Look it up in the spec:

```bash
# Show everything about a specific check
jq '.checks[] | select(.id == "OPF-035")' $EPUBCHECK_SPEC_DIR/checks.json

# Or list all checks not currently passing (after make compare)
make compare 2>&1 | grep FAIL
```

The check entry tells you:
- `description` — what rule it enforces
- `spec_ref` — link to the EPUB spec section
- `category` — which validate/*.go file owns it (OPF → opf.go, PKG → ocf.go, etc.)
- `severity` — ERROR, FATAL, or WARNING
- `fixture_invalid` — the EPUB that should trigger this error
- `epubcheck_message_id` — what epubcheck reports (useful for understanding intent)

Look at the expected output for the fixture:

```bash
cat $EPUBCHECK_SPEC_DIR/expected/invalid/<fixture-name>.json
```

The `message_pattern` field is the regex your implementation's message text must match.

### Step 3 — Implement the check

Find the right file. Categories map to files like this:

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
| MED (media) | `pkg/validate/media.go` |
| ENC (encoding) | `pkg/validate/encoding.go` |

Follow the pattern already in the file. Every check is a small function:

```go
func checkMyNewRule(ep *epub.EPUB, r *report.Report) {
    // inspect ep.Package, ep.Files, etc.
    if /* condition */ {
        r.Add(report.Error, "OPF-035", "message text that matches the expected pattern")
    }
}
```

Then call it from the master phase function (e.g., `checkOPF`):

```go
func checkOPF(ep *epub.EPUB, r *report.Report, opts Options) bool {
    // ... existing calls ...
    checkMyNewRule(ep, r)   // ← add here
    // ...
}
```

**Key rules:**
- The second argument to `r.Add()` is the check ID from `checks.json` (e.g., `"OPF-035"`)
- The message text must be a string that matches `message_pattern` from the expected file (case-insensitive regex)
- Use `r.Add(report.Fatal, ...)` for FATAL severity checks — these halt the pipeline
- Use `r.AddWithLocation(severity, id, message, path)` when you have a specific file to blame

### Step 4 — Run the spec tests

```bash
make spec-test
```

Iterate until the new check passes. Then run the full suite to confirm nothing regressed:

```bash
make spec-test && make compare
```

---

## Scenario 2: Spec Updated an Expected File

Sometimes `expected/` files are updated (message pattern changed, severity
adjusted, count corrected). These show up as test failures even though the
underlying check hasn't changed.

```bash
cd epubverify-spec && git pull
cd epubverify && make spec-test
```

If tests that previously passed now fail, read the new expected file carefully:

```bash
cat $EPUBCHECK_SPEC_DIR/expected/invalid/<fixture-name>.json
```

Likely fixes:
- **`message_pattern` changed** — update your message text in the validate/*.go function
- **`severity` changed** — update the `r.Add(report.Error, ...)` call
- **`error_count_min` added** — means epubcheck now reports cascading errors; your count may need adjusting

---

## Scenario 3: Spec Upgraded epubcheck Version

When the spec bumps its reference epubcheck (e.g., 5.3.0 → 5.4.0), rebuild fixtures
and re-run spec tests. Some expected files may have changed (see Scenario 2).

```bash
cd epubverify-spec
git pull
make build       # rebuild all fixture EPUBs
cd epubverify
make spec-test
```

The Go implementation itself doesn't depend on epubcheck directly, so usually
no Go code changes are needed — only message text adjustments if patterns changed.

---

## Scenario 4: Implementing a Full Level

When the spec ships a new level (e.g., Level 4 accessibility checks), the
approach is the same as Scenario 1 but in bulk.

```bash
# See all checks at a given level
jq '.checks[] | select(.level == 4) | .id' $EPUBCHECK_SPEC_DIR/checks.json

# Run spec tests and collect all failures
make spec-test 2>&1 | grep FAIL | tee /tmp/failing.txt

# Parity report sorted by category — helps batch the work
make compare 2>&1 | grep FAIL | sort
```

Work category by category (all OPF checks, then all NAV, etc.) since they
each live in one file.

---

## Verifying Your Work

After implementing changes, run all three levels of verification:

```bash
# 1. Unit tests (fast, no spec dir needed)
make test

# 2. Spec compliance (requires EPUBCHECK_SPEC_DIR)
make spec-test

# 3. Full parity report (requires EPUBCHECK_SPEC_DIR + built binary)
make compare
```

A healthy implementation looks like:
```
Passed: 118/123 (95.9%)
Failed: 5
Skipped: 0
```

---

## Implementation Reference

### Report API

```go
// Add a message (most common)
r.Add(report.Error, "OPF-035", "human-readable message")

// Add with file location
r.AddWithLocation(report.Warning, "CSS-003", "message", "OEBPS/style.css")

// Severity constants
report.Fatal    // stops validation pipeline
report.Error    // EPUB is invalid
report.Warning  // suspicious but not invalid
```

### EPUB struct fields (pkg/epub/types.go)

```go
ep.Files          // map[string][]byte — all zip entries by path
ep.Rootfile       // path to the OPF file
ep.Package        // parsed OPF package document
ep.Package.Manifest.Items   // []ManifestItem
ep.Package.Spine.Itemrefs   // []SpineItemref
ep.Package.Metadata         // title, language, identifier, etc.
```

### Reading a file from the EPUB

```go
data, err := ep.ReadFile("OEBPS/content.opf")
```

### Checking if a file exists

```go
_, exists := ep.Files["mimetype"]
```
