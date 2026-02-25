# Test Fixtures

## Directory Layout

### `epub3/` and `epub2/`

Hand-crafted test fixtures organized by EPUB spec section. These are referenced
by `.feature` files and used directly in validation tests.

### `schematron-gaps/`

Auto-generated fixtures produced by `scripts/schematron-audit.py --generate-tests`.
These represent epubcheck Schematron rules that epubverify does not yet cover.

Subdirectories mirror the document type:

- `xhtml/` — XHTML content document fixtures
- `opf/` — OPF package document fixtures
- `ocf/` — OCF container/metadata fixtures

The `sch-generated-scenarios.feature.txt` file contains Gherkin scenario
snippets that can be moved into the appropriate `.feature` file once a check
is implemented.

**Do not hand-edit files here.** Re-run the audit script to regenerate:

```
python3 scripts/schematron-audit.py --generate-tests
```
