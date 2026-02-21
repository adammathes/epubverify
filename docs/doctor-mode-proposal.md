# EPUB Doctor Mode — Proposal & Feasibility Report

## Summary

**Verdict: This is a promising direction.** The implementation covers 16 fix types across Tier 1, Tier 2, and Tier 3. Tier 1 handles 8 ZIP/OPF/content structural issues; Tier 2 adds 5 more (obsolete elements, bad dates, orphan files, empty hrefs, deprecated guide); Tier 3 handles CSS @import inlining and encoding transcoding (ISO-8859-1, Windows-1252, UTF-16). All tiers take broken EPUBs to 0 errors in integration tests.

## How It Works

```
epubverify book.epub --doctor [-o output.epub]
```

1. Opens the EPUB and validates it with the standard checker
2. Reads all files into memory
3. Applies safe, mechanical fixes for known issues
4. Writes a new EPUB (the writer itself fixes ZIP-structural issues by construction)
5. Re-validates the output to confirm fixes worked
6. Reports what changed: before/after error counts and each fix applied

Output goes to `<input>.fixed.epub` by default, or a custom path with `-o`.

## Tier 1 Fixes (Implemented)

These are **safe, deterministic, content-preserving** fixes where the correct action is unambiguous:

| Check ID | Problem | Fix | Risk |
|----------|---------|-----|------|
| OCF-001 | Missing mimetype file | Add with correct content | None |
| OCF-002 | mimetype not first ZIP entry | Writer always puts it first | None |
| OCF-003 | Wrong mimetype content | Write `application/epub+zip` | None |
| OCF-004 | Extra field in mimetype header | Writer omits extra field | None |
| OCF-005 | mimetype compressed | Writer uses Store method | None |
| OPF-004 | Missing `dcterms:modified` | Add `<meta>` with current UTC time | Low — timestamp is synthetic |
| OPF-024 / MED-001 | Media-type mismatch | Correct based on file magic bytes | Low — magic bytes are reliable |
| HTM-005/006/007 | Missing manifest properties | Add `scripted`/`svg`/`mathml` | None — detected from content |
| HTM-010/011 | Non-HTML5 DOCTYPE | Replace with `<!DOCTYPE html>` | Low — EPUB 3 requires HTML5 |

## Integration Test Results

A test EPUB with 6 simultaneous errors:

```
Before: 6 errors, 0 warnings
  ERROR(OCF-002): The mimetype file must be the first entry in the zip archive
  ERROR(OCF-003): The mimetype file must contain exactly 'application/epub+zip'
  ERROR(OPF-004): Package metadata is missing required element dcterms:modified
  ERROR(HTM-010): Irregular DOCTYPE: EPUB 3 content must use <!DOCTYPE html>
  ERROR(HTM-005): Property 'scripted' should be declared in the manifest
  ERROR(MED-001): The file 'cover.jpg' does not appear to match media type 'image/png'

Applied 6 fixes:
  [OCF-003] Fixed mimetype content
  [OCF-002] Reordered mimetype as first ZIP entry
  [OPF-004] Added dcterms:modified
  [OPF-024] Fixed media-type for 'cover.jpg' from 'image/png' to 'image/jpeg'
  [HTM-005] Added 'scripted' property to manifest item 'ch1'
  [HTM-010] Replaced non-HTML5 DOCTYPE with <!DOCTYPE html>

After: 0 errors, 0 warnings
```

## Architecture

```
pkg/doctor/
  doctor.go       — orchestrator: validate → fix → write → re-validate
  fixes.go        — individual fix functions + helpers (Tier 1 + Tier 2 + Tier 3)
  writer.go       — EPUB ZIP writer (correct mimetype handling by construction)
  doctor_test.go  — unit tests (22 tests: all tiers, encoding transcoding, date parsing, round-trip)
  integration_test.go — multi-problem integration tests (Tier 1 + Tier 2 + Tier 3)
```

Key design decisions:
- **Non-destructive**: Always writes to a new file, never modifies the original
- **Verify after**: Re-validates the output so you can confirm improvements
- **Fix by construction**: ZIP-structural issues (OCF-002/004/005) are handled by the writer always producing correct structure, rather than trying to patch the ZIP
- **OPF manipulation uses regex**: For the prototype, OPF edits use targeted regex matching on `<item>` elements. This works well for attribute changes but a future version might benefit from a proper XML round-trip (Go's `encoding/xml` doesn't preserve formatting/comments well)

## Tier 2 Fixes (Implemented)

These required more care but are still safe mechanical fixes:

| Check ID | Problem | Fix | Risk |
|----------|---------|-----|------|
| OPF-039 | `<guide>` in EPUB 3 | Remove entire `<guide>` element | None — deprecated, no functional impact |
| OPF-036 | Bad `dc:date` format | Parse common formats and reformat to W3CDTF | Low — best-effort date parsing |
| RSC-002 | File in container not in manifest | Add `<item>` with guessed media-type and generated unique ID | Low — extension-based media-type guessing |
| HTM-003 | Empty `href=""` on `<a>` | Remove the href attribute, keep element | None — empty href has no target |
| HTM-004 | Obsolete elements (`<center>`, `<big>`, `<strike>`, `<tt>`, `<acronym>`, `<dir>`) | Replace with styled modern equivalents | Low — equivalent CSS styles |

### Date format parsing (OPF-036)

The date reformatter handles these common non-W3CDTF patterns found in real EPUBs:
- "January 15, 2024" / "Jan 1, 2024" → `2024-01-15` / `2024-01-01`
- "15 January 2024" → `2024-01-15`
- "2024/01/15" or "2024.01.15" → `2024-01-15`
- "01/15/2024" (US format) → `2024-01-15`

### Obsolete element replacements (HTM-004)

| Obsolete | Replacement | Style |
|----------|-------------|-------|
| `<center>` | `<div>` | `text-align: center;` |
| `<big>` | `<span>` | `font-size: larger;` |
| `<strike>` | `<span>` | `text-decoration: line-through;` |
| `<tt>` | `<span>` | `font-family: monospace;` |
| `<acronym>` | `<abbr>` | (direct semantic replacement) |
| `<dir>` | `<ul>` | (direct semantic replacement) |

## Tier 2 Integration Test Results

A test EPUB with 5 simultaneous Tier 2 issues:

```
Before: 2 errors, 4 warnings
  WARNING(OPF-036): Date value 'March 15, 2024' does not follow recommended syntax of W3CDTF
  WARNING(OPF-039): The guide element is deprecated in EPUB 3 and should not be used
  WARNING(RSC-002): File 'OEBPS/orphan.css' in container is not declared in the OPF manifest
  WARNING(HTM-003): Hyperlink href attribute must not be empty
  ERROR(HTM-004): Element 'center' is not allowed in EPUB content documents
  ERROR(HTM-004): Element 'big' is not allowed in EPUB content documents

Applied 5 fixes:
  [OPF-039] Removed deprecated <guide> element from EPUB 3 package document
  [OPF-036] Reformatted dc:date from 'March 15, 2024' to '2024-03-15'
  [RSC-002] Added 'OEBPS/orphan.css' to manifest (id='orphan', media-type='text/css')
  [HTM-003] Removed 1 empty href attribute(s) from <a> elements
  [HTM-004] Replaced obsolete element(s) big, center with modern equivalents

After: 0 errors, 0 warnings
```

## Tier 3 Fixes (Implemented)

These are higher-complexity fixes requiring encoding awareness and cross-file operations:

| Check ID | Problem | Fix | Risk |
|----------|---------|-----|------|
| CSS-005 | `@import` rules in CSS | Inline imported CSS file contents | Low — file must exist in container |
| ENC-001 | Non-UTF-8 encoding declaration | Transcode from declared encoding to UTF-8; supports ISO-8859-1, Windows-1252 | Low — well-defined mappings |
| ENC-002 | UTF-16 encoded content | Transcode UTF-16 (LE/BE with BOM) to UTF-8 | Low — standard library Unicode handling |

### CSS @import inlining (CSS-005)

Handles all common `@import` syntax variants:
- `@import url("style.css");`
- `@import url('style.css');`
- `@import "style.css";`
- `@import 'style.css';`

The imported file's contents replace the `@import` rule inline. Relative paths are resolved correctly. Remote URLs are left unchanged. Safety limit of 10 inlines per file.

### Encoding transcoding (ENC-001/ENC-002)

**ENC-001** (non-UTF-8 declaration): Two paths:
1. If the file declares a non-UTF-8 encoding and the content is actually valid UTF-8 → just fix the declaration
2. If the file contains genuine non-UTF-8 bytes → transcode using the declared encoding:
   - **ISO-8859-1/Latin-1**: Direct 1:1 byte-to-codepoint mapping
   - **Windows-1252**: Same as Latin-1 except bytes 0x80-0x9F (smart quotes, euro sign, etc.) map to specific Unicode codepoints

**ENC-002** (UTF-16): Detects BOM (FF FE for LE, FE FF for BE), decodes UTF-16 code units including surrogate pairs, and re-encodes as UTF-8. No external dependencies — uses pure Go standard library.

## Tier 3 Integration Test Results

A test EPUB with both CSS @import and encoding issues:

```
Before: 1 errors, 1 warnings
  ERROR(ENC-001): Content document 'chapter1.xhtml' must be encoded in UTF-8, but declares encoding 'iso-8859-1'
  WARNING(CSS-005): @import rules should not be used in EPUB CSS stylesheets

Applied 2 fixes:
  [CSS-005] Inlined 1 @import rule(s)
  [ENC-001] Transcoded from iso-8859-1 to UTF-8

After: 0 errors, 0 warnings
```

## What Won't Work in Doctor Mode

Some issues are fundamentally unfixable automatically:

- **HTM-001/OPF-011**: Malformed XML — requires understanding author intent
- **RSC-001**: Missing referenced files — the content doesn't exist
- **OPF-009**: Spine references nonexistent manifest items — structural confusion
- **RSC-003**: Broken fragment identifiers — could be typos, need human judgment
- **HTM-017**: HTML entities in XHTML — need to know correct character
- **OPF-022**: Circular fallback chains — ambiguous how to break the cycle

## Recommendation

Ship this as an experimental `--doctor` flag. The architecture is clean, the fixes are safe, and the test coverage is comprehensive (26 tests: 22 unit + 4 integration). Tiers 1-3 together handle the most common "my EPUB won't pass validation" problems — covering 16 distinct fix types across ZIP structure, OPF metadata, XHTML content, CSS, and encoding.

The regex-based approach works well for all current fix types. For future enhancements requiring more complex XML structural changes, consider:
1. A proper XML serializer that preserves formatting
2. Or, byte-level splicing using the parsed positions from `encoding/xml`
