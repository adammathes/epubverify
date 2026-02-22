# EPUB Doctor Mode

## Overview

Doctor mode (`--doctor`) is an experimental feature that automatically repairs common EPUB validation errors. It applies safe, mechanical fixes for known issues — the kind of problems where the correct action is unambiguous and the fix preserves content.

The approach: validate, fix what's fixable, write a new file, re-validate to confirm.

## Current Status

**Experimental.** The feature works and has good test coverage (30+ unit tests, 5 integration tests across all tiers), but it has not been tested against a wide variety of real-world EPUBs. Use it on copies of your files.

## What It Fixes

Doctor mode handles 24 fix types across four tiers, organized by complexity and risk.

### Tier 1 — Safe structural fixes

These are deterministic, content-preserving fixes where the correct action is unambiguous.

| Check ID | Problem | Fix |
|----------|---------|-----|
| OCF-001 | Missing mimetype file | Add with correct content |
| OCF-002 | mimetype not first ZIP entry | Writer always puts it first |
| OCF-003 | Wrong mimetype content | Write `application/epub+zip` |
| OCF-004 | Extra field in mimetype header | Writer omits extra field |
| OCF-005 | mimetype compressed | Writer uses Store method |
| OPF-004 | Missing `dcterms:modified` | Add `<meta>` with current UTC time |
| OPF-024 / MED-001 | Media-type mismatch | Correct based on file magic bytes |
| HTM-005/006/007 | Missing manifest properties | Add `scripted`/`svg`/`mathml` |
| HTM-010/011 | Non-HTML5 DOCTYPE | Replace with `<!DOCTYPE html>` |

### Tier 2 — Low-risk content fixes

These require more care but are still safe mechanical fixes.

| Check ID | Problem | Fix |
|----------|---------|-----|
| OPF-039 | `<guide>` in EPUB 3 | Remove entire `<guide>` element |
| OPF-036 | Bad `dc:date` format | Parse common formats, reformat to W3CDTF |
| RSC-002 | File in container not in manifest | Add `<item>` with guessed media-type |
| HTM-003 | Empty `href=""` on `<a>` | Remove the href attribute |
| HTM-004 | Obsolete elements (`<center>`, `<big>`, etc.) | Replace with styled modern equivalents |

### Tier 3 — Encoding and CSS fixes

Higher complexity, requiring encoding awareness and cross-file operations.

| Check ID | Problem | Fix |
|----------|---------|-----|
| CSS-005 | `@import` rules in CSS | Inline imported file contents |
| ENC-001 | Non-UTF-8 encoding declaration | Transcode from declared encoding to UTF-8 |
| ENC-002 | UTF-16 encoded content | Transcode to UTF-8 |

Supported encodings for transcoding: ISO-8859-1/Latin-1, Windows-1252, UTF-16 LE/BE.

### Tier 4 — Cleanup and consistency

Lower severity issues that are still mechanically fixable.

| Check ID | Problem | Fix |
|----------|---------|-----|
| OPF-028 | Multiple `dcterms:modified` | Remove duplicates, keep first |
| OPF-033 | Fragment in manifest href | Strip `#fragment` from href |
| OPF-017 | Duplicate spine `itemref` | Remove subsequent duplicates |
| OPF-038 | Invalid `linear` attribute value | Normalize `true`->`yes`, `false`->`no` |
| HTM-009 | `<base>` element in content | Remove element |
| HTM-020 | Processing instructions (e.g., `<?oxygen?>`) | Remove non-XML PIs |
| HTM-026 | `lang`/`xml:lang` mismatch | Sync `lang` to match `xml:lang` |
| HTM-002 | Missing `<title>` element | Add `<title>Untitled</title>` |

## What It Won't Fix

Some issues are fundamentally unfixable automatically:

- **Malformed XML** (HTM-001/OPF-011) — requires understanding author intent
- **Missing referenced files** (RSC-001) — the content doesn't exist
- **Broken fragment identifiers** (RSC-003) — could be typos, need human judgment
- **HTML entities in XHTML** (HTM-017) — need to know the correct character
- **Circular fallback chains** (OPF-022) — ambiguous how to break the cycle
- **Spine references to nonexistent items** (OPF-009) — structural confusion

## Architecture

```
pkg/doctor/
  doctor.go           — orchestrator: validate -> fix -> write -> re-validate
  fixes.go            — individual fix functions + helpers (Tiers 1-4)
  writer.go           — EPUB ZIP writer (correct mimetype handling by construction)
  doctor_test.go      — unit tests for each fix type
  integration_test.go — multi-problem integration tests per tier
```

Key design decisions:

- **Non-destructive**: Always writes to a new file, never modifies the original
- **Verify after**: Re-validates the output so you can confirm improvements
- **Fix by construction**: ZIP-structural issues (OCF-002/004/005) are handled by the writer always producing correct structure, rather than patching the ZIP
- **OPF manipulation uses regex**: Targeted regex matching on `<item>` elements avoids the formatting/comment loss that Go's `encoding/xml` causes on round-trip

## Known Limitations

1. **Cross-tier fix interactions**: Some fixes can interact. For example, RSC-002 (adding files to manifest) and OPF-033 (stripping fragments from hrefs) can create a duplicate manifest entry if a file is referenced both with and without a fragment. This is a known edge case documented in the Tier 4 integration tests.

2. **Regex-based OPF editing**: Works well for all current fix types but could hit edge cases with unusual whitespace or attribute ordering in hand-crafted OPF files. A proper XML round-trip serializer would be more robust but Go's `encoding/xml` doesn't preserve formatting or comments.

3. **Synthetic values**: Some fixes insert synthetic values — `dcterms:modified` uses the current UTC time, missing `<title>` gets "Untitled". These are technically correct but not meaningful.

4. **Date parsing**: The OPF-036 date reformatter handles common patterns (US dates, European dates, month names) but won't parse every possible format.

## Testing Against an Implementation-Independent Spec

The [epubverify-spec](https://github.com/adammathes/epubverify-spec) repository contains implementation-independent validation test fixtures (EPUB files + expected JSON outcomes). The doctor tests are currently **not** in that repo — they live as Go unit and integration tests in `pkg/doctor/`.

### Why keep doctor tests internal for now

- Doctor mode is experimental and the fix behavior may change
- Repair tests are inherently more opinionated than validation tests (there's often more than one reasonable way to fix something)
- The current tests construct EPUBs programmatically, which is faster to iterate on than maintaining binary fixture files
- Once the feature stabilizes, adding doctor fixtures to epubverify-spec would make sense — with expectations that assert "these check IDs should be absent after repair" rather than dictating how the fix was applied

### What shared doctor test fixtures could look like

If/when doctor tests move to the spec repo, a practical format would be:

- `fixtures/epub/doctor/` — broken EPUB files (one per fix type, plus multi-issue combos)
- `expected/doctor/` — JSON files specifying which check IDs should be present *before* repair and absent *after* repair
- No structural assertions (like "the `<base>` tag was removed") — those belong in implementation-specific unit tests

This gives other implementers the "what" without dictating the "how."

## Research: Existing EPUB Test Suites and Repair Tools

### W3C/IDPF test suites

There are three W3C/IDPF test suite efforts, none of which target validators or repair tools:

- **[IDPF/epub-testsuite](https://github.com/IDPF/epub-testsuite)** — Reading system conformance tests (human-evaluated pass/fail for rendering)
- **[w3c/epub-tests](https://github.com/w3c/epub-tests)** — Newer W3C reading system tests for EPUB 3.3
- **[w3c/epub-structural-tests](https://github.com/w3c/epub-structural-tests)** — Maps EPUBCheck's Gherkin tests to the EPUB 3.3/3.4 spec (coverage analysis, not a test suite)

**No official shared validator test suite exists.** EPUBCheck's Gherkin/Cucumber tests are the closest thing, but they're tightly coupled to EPUBCheck's Java API.

### EPUBCheck's test approach

EPUBCheck uses Gherkin `.feature` files executed with Cucumber. The scenarios are human-readable ("When checking EPUB 'minimal', Then no errors or warnings are reported") but the step definitions, type registry, and test runner are all Java-specific. Another implementation can't reuse the `.feature` files without reimplementing all the glue code.

### Other EPUB repair tools

- **[antiwong/epubrepairtool](https://github.com/antiwong/epubrepairtool)** — Python toolkit, own tests, no shared format
- **Commercial services** (Ebookifi, Epubfixnow, Filestar) — proprietary, no shared test infrastructure
- **FlightDeck** by Firebrand Technologies — proprietary checks beyond EPUBCheck

**No shared EPUB repair test suite exists anywhere.** The epubverify doctor tests are, as far as we can find, the most comprehensive open-source EPUB repair test suite.
