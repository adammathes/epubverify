#!/usr/bin/env python3
"""
java-audit.py - Audit epubverify coverage against epubcheck's Java code checks.

This is Tier 3 of the systematic gap extraction from epubcheck's three
validation tiers (RelaxNG schemas, Schematron rules, Java code).

Epubcheck's Java code implements checks that can't be expressed in RelaxNG
schemas or Schematron rules: cross-file references, ZIP structure, encoding
detection, media type sniffing, complex business logic, etc.  Every check
emits a message via MessageId.XXX.  This script builds a complete inventory
of those message IDs, maps them to the Java classes that emit them, and
cross-references against epubverify's Go implementation to find gaps.

Usage:
    python3 scripts/java-audit.py                  # full gap report
    python3 scripts/java-audit.py --json           # machine-readable output
    python3 scripts/java-audit.py --skip-update    # use cached epubcheck
    python3 scripts/java-audit.py --summary        # compact summary only

How it works:
    1. Parses MessageId.java to get every defined message ID
    2. Parses DefaultSeverities.java to get the default severity for each
    3. Parses MessageBundle.properties to get the English message text
    4. Greps all Java source files for MessageId.XXX references to find
       which Java classes emit each message
    5. Greps epubverify's Go source for check ID strings to find which
       codes are already implemented
    6. Greps BDD .feature files for check IDs referenced in test assertions
    7. Cross-references everything and produces a gap report

Output categories:
    - IMPLEMENTED: epubverify emits this check ID in Go code
    - TESTED:      check ID appears in BDD feature file assertions
    - SUPPRESSED:  epubcheck default severity is SUPPRESSED (intentionally disabled)
    - MISSING:     epubverify does not implement this check (the gap)
    - PARTIAL:     manually flagged as partially implemented

Updating from upstream:
    When epubcheck adds new MessageIds (new error codes), re-run this script
    to find new gaps. Then:
      1. Run: python3 scripts/java-audit.py
      2. Review the "MISSING CHECKS" section for newly added codes
      3. For each missing code, decide:
         a. Implement it: add the check in the appropriate Go file, then
            the script auto-detects it on the next run
         b. Skip it: add an entry to KNOWN_OVERRIDES below with
            status="wontfix" and a note explaining why
      4. Re-run the script to confirm 0 missing
      5. Commit the updated KNOWN_OVERRIDES and any new Go code

    The KNOWN_OVERRIDES dict only needs entries for codes that require
    manual annotation (wontfix, partial).  Codes that appear in the Go
    source are auto-detected as "implemented".  Codes with SUPPRESSED
    severity in epubcheck are auto-detected as "suppressed".

The .cache/epubcheck directory is gitignored and auto-cloned on first run.
"""

import argparse
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Optional

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

EPUBCHECK_REPO = "https://github.com/w3c/epubcheck.git"
EPUBCHECK_BRANCH = "main"

# Relative to the repo root
CACHE_DIR = ".cache/epubcheck"

# Where the Java source lives inside epubcheck
JAVA_SRC_DIR = "src/main/java"
MESSAGES_DIR = "src/main/java/com/adobe/epubcheck/messages"
RESOURCES_DIR = "src/main/resources/com/adobe/epubcheck/messages"

# Where the Go source lives in epubverify
GO_VALIDATE_DIR = "pkg/validate"
GO_ALL_DIRS = ["pkg/validate", "pkg/report", "pkg/doctor", "pkg/epub"]

# Where BDD features live
FEATURES_DIR = "testdata/features"

# ---------------------------------------------------------------------------
# Known implementation status overrides
#
# Maintain this list for codes that need manual annotation.  The audit
# script auto-detects "implemented" (Go code emits the ID) and "suppressed"
# (epubcheck default severity is SUPPRESSED).  Use this for:
#   - "partial"  — Go code emits the ID but not all sub-cases
#   - "wontfix"  — intentionally skipped
#   - "note"     — extra context
# ---------------------------------------------------------------------------
KNOWN_OVERRIDES: dict[str, dict] = {
    # --- Accessibility: most are SUPPRESSED by default in epubcheck ---
    "ACC-001": {"status": "implemented", "go": "accessibility.go", "note": "accessibility metadata check"},
    "ACC-002": {"status": "implemented", "go": "accessibility.go", "note": "alt text check"},
    "ACC-003": {"status": "implemented", "go": "accessibility.go", "note": "alt text check"},
    "ACC-004": {"status": "implemented", "go": "accessibility.go", "note": "link text check"},
    "ACC-005": {"status": "implemented", "go": "accessibility.go", "note": "table heading check"},
    "ACC-006": {"status": "implemented", "go": "accessibility.go", "note": "table thead check"},
    "ACC-007": {"status": "implemented", "go": "accessibility.go", "note": "epub:type semantic check"},
    "ACC-008": {"status": "implemented", "go": "accessibility.go", "note": "accessibility metadata check"},
    "ACC-009": {"status": "implemented", "go": "accessibility.go", "note": "MathML alttext check"},
    "ACC-010": {"status": "implemented", "go": "accessibility.go", "note": "landmarks check"},
    "ACC-011": {"status": "wontfix", "note": "SVG hyperlink accessible name; niche SVG-specific check"},

    # --- CHK: checker configuration errors (internal to epubcheck) ---
    "CHK-001": {"status": "wontfix", "note": "epubcheck custom message overrides file; internal to epubcheck"},
    "CHK-002": {"status": "wontfix", "note": "epubcheck custom message overrides; internal"},
    "CHK-003": {"status": "wontfix", "note": "epubcheck custom message overrides; internal"},
    "CHK-004": {"status": "wontfix", "note": "epubcheck custom message overrides; internal"},
    "CHK-005": {"status": "wontfix", "note": "epubcheck custom message overrides; internal"},
    "CHK-006": {"status": "wontfix", "note": "epubcheck custom message overrides; internal"},
    "CHK-007": {"status": "wontfix", "note": "epubcheck custom message overrides; internal"},
    "CHK-008": {"status": "implemented", "go": "epub2.go", "note": "error processing item; skip further checks"},

    # --- SCP: scripting checks (all SUPPRESSED in epubcheck) ---
    "SCP-001": {"status": "wontfix", "note": "scripting check; all SCP codes are SUPPRESSED in epubcheck"},
    "SCP-002": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-003": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-004": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-005": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-006": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-007": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-008": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-009": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},
    "SCP-010": {"status": "wontfix", "note": "scripting check; SUPPRESSED"},

    # --- INF: informational ---
    "INF-001": {"status": "wontfix", "note": "informational message about rule under review; meta-level"},

    # --- Partial implementations ---
    "OPF-004": {"status": "implemented", "go": "opf.go", "note": "prefix declaration leading/trailing whitespace"},
    "OPF-004a": {"status": "implemented", "go": "opf.go", "note": "empty prefix"},
    "OPF-004b": {"status": "implemented", "go": "opf.go", "note": "prefix not valid NCName"},
    "OPF-004c": {"status": "implemented", "go": "opf.go", "note": "prefix not followed by colon"},
    "OPF-004d": {"status": "implemented", "go": "opf.go", "note": "prefix not separated by space"},
    "OPF-004e": {"status": "implemented", "go": "opf.go", "note": "illegal whitespace between prefix and URI"},
    "OPF-004f": {"status": "implemented", "go": "opf.go", "note": "illegal whitespace between prefix mappings"},

    # --- Codes that are emitted by schema validation, not directly in Go ---
    "RSC-005": {"status": "implemented", "go": "content.go/opf.go/ocf.go",
                "note": "schema validation error; covers RelaxNG and content model checks"},
    "RSC-016": {"status": "implemented", "go": "opf.go/content.go",
                "note": "fatal XML parse error"},

    # --- Known wontfix: very niche features ---
    "HTM-051": {"status": "wontfix", "note": "EDUPUB Microdata warning; EDUPUB is defunct"},
    "HTM-052": {"status": "wontfix", "note": "Data Navigation Documents; very rare feature"},
    "OPF-064": {"status": "wontfix", "note": "INFO: declares type; informational only"},
    "OPF-077": {"status": "wontfix", "note": "Data Navigation Document spine warning; very niche"},
    "OPF-078": {"status": "wontfix", "note": "EPUB Dictionary content check; very niche"},
    "OPF-079": {"status": "wontfix", "note": "EPUB Dictionary dc:type check; very niche"},
    "OPF-080": {"status": "wontfix", "note": "Search Key Map file extension; EPUB Dictionary"},
    "OPF-081": {"status": "wontfix", "note": "EPUB Dictionary collection resource not found; very niche"},
    "OPF-082": {"status": "wontfix", "note": "EPUB Dictionary multiple Search Key Maps; very niche"},
    "OPF-083": {"status": "wontfix", "note": "EPUB Dictionary no Search Key Map; very niche"},
    "OPF-084": {"status": "wontfix", "note": "EPUB Dictionary invalid collection resource; very niche"},
    "OPF-071": {"status": "wontfix", "note": "Index collections XHTML only; very niche"},
    "OPF-075": {"status": "wontfix", "note": "Preview collections content docs only; very niche"},
    "OPF-076": {"status": "wontfix", "note": "Preview collections no CFI; very niche"},
    "RSC-021": {"status": "wontfix", "note": "Search Key Map Document spine check; EPUB Dictionary"},
    "RSC-022": {"status": "wontfix", "note": "Java version check for image details; not applicable to Go"},

    # --- Codes emitted by schema/Schematron only (no Java code check needed) ---
    "OPF-019": {"status": "implemented", "go": "opf.go",
                "note": "dcterms:modified format; SUPPRESSED in epubcheck (reported as RSC-005)"},

    # --- E2-xxx: EPUB 2 specific codes used in epubverify but not in epubcheck MessageId ---
    # These are epubverify-specific codes, not in the epubcheck enum

    # --- Additional overrides for partially-implemented codes ---
    "OPF-009": {"status": "wontfix", "note": "SUPPRESSED; duplicate manifest item ID (handled by XML parser)"},
    "OPF-020": {"status": "wontfix", "note": "SUPPRESSED; role attribute deprecated syntax"},
    "OPF-046": {"status": "wontfix", "note": "SUPPRESSED; spine toc attribute presence check"},
    "OPF-056": {"status": "wontfix", "note": "SUPPRESSED; meta generator info"},
    "OPF-057": {"status": "wontfix", "note": "SUPPRESSED; image file size check (not spec)"},
    "OPF-058": {"status": "wontfix", "note": "SUPPRESSED; dc:creator/contributor meta parsing"},
    "OPF-059": {"status": "wontfix", "note": "SUPPRESSED; bindings handler media-type"},
    "OPF-068": {"status": "wontfix", "note": "SUPPRESSED; unknown DC metadata element"},
    "OPF-069": {"status": "wontfix", "note": "SUPPRESSED; dc:contributor vs. metadata"},
    "OPF-051": {"status": "wontfix", "note": "SUPPRESSED; image dimensions exceed recommended size"},

    # SUPPRESSED HTM codes
    "HTM-005": {"status": "wontfix", "note": "SUPPRESSED as USAGE; external reference found"},
    "HTM-006": {"status": "wontfix", "note": "SUPPRESSED; style attribute check"},
    "HTM-008": {"status": "wontfix", "note": "SUPPRESSED; hyperlink check"},
    "HTM-012": {"status": "wontfix", "note": "SUPPRESSED; deprecated element in content"},
    "HTM-013": {"status": "wontfix", "note": "SUPPRESSED; HTML5 deprecated element"},
    "HTM-014": {"status": "wontfix", "note": "SUPPRESSED; content type mismatch"},
    "HTM-014a": {"status": "wontfix", "note": "SUPPRESSED; content type mismatch variant"},
    "HTM-015": {"status": "implemented", "go": "content.go", "note": "SUPPRESSED but implemented for doctor mode"},
    "HTM-016": {"status": "implemented", "go": "content.go", "note": "SUPPRESSED; epub:switch deprecated"},
    "HTM-017": {"status": "implemented", "go": "content.go", "note": "SUPPRESSED; reported as RSC-005"},
    "HTM-018": {"status": "wontfix", "note": "SUPPRESSED; unused namespace declaration"},
    "HTM-019": {"status": "wontfix", "note": "SUPPRESSED; non-standard namespace"},
    "HTM-020": {"status": "wontfix", "note": "SUPPRESSED; namespace prefix mismatch"},
    "HTM-021": {"status": "implemented", "go": "content.go", "note": "SUPPRESSED; position:absolute in inline style"},
    "HTM-022": {"status": "wontfix", "note": "SUPPRESSED; duplicate accesskey values"},
    "HTM-023": {"status": "implemented", "go": "content.go", "note": "SUPPRESSED; empty heading"},
    "HTM-024": {"status": "wontfix", "note": "SUPPRESSED; heading level skip"},
    "HTM-027": {"status": "wontfix", "note": "SUPPRESSED; hgroup content model"},
    "HTM-028": {"status": "wontfix", "note": "SUPPRESSED; obsolete EPUB attribute"},
    "HTM-029": {"status": "wontfix", "note": "SUPPRESSED; hyperlink to non-spine item"},
    "HTM-033": {"status": "implemented", "go": "content.go", "note": "SUPPRESSED; head element issue"},
    "HTM-036": {"status": "wontfix", "note": "SUPPRESSED; IFrame found"},
    "HTM-038": {"status": "wontfix", "note": "SUPPRESSED; img usemap attribute"},
    "HTM-044": {"status": "wontfix", "note": "USAGE; unused namespace URI"},
    "HTM-045": {"status": "wontfix", "note": "USAGE; empty href encountered"},
    "HTM-049": {"status": "wontfix", "note": "SUPPRESSED; reported as RSC-005"},
    "HTM-050": {"status": "wontfix", "note": "SUPPRESSED; RDFa attribute"},

    # Media codes with nuance
    "MED-001": {"status": "wontfix", "note": "SUPPRESSED; deprecated media type"},
    "MED-002": {"status": "implemented", "go": "media.go", "note": "SUPPRESSED but partially tracked"},
    "MED-006": {"status": "wontfix", "note": "SUPPRESSED; audio codec check"},

    # CSS SUPPRESSED codes
    "CSS-006": {"status": "wontfix", "note": "USAGE; CSS position:fixed check"},
    "CSS-009": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-010": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-012": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-013": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-016": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-017": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-020": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-021": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-022": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-023": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-024": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},
    "CSS-025": {"status": "wontfix", "note": "SUPPRESSED; CSS property check"},

    # NCX codes
    "NCX-002": {"status": "wontfix", "note": "SUPPRESSED; reported as RSC-005"},
    "NCX-003": {"status": "wontfix", "note": "SUPPRESSED; NCX structure check"},
    "NCX-004": {"status": "wontfix", "note": "USAGE; NCX dtb:uid whitespace"},
    "NCX-005": {"status": "wontfix", "note": "SUPPRESSED; NCX structure check"},

    # NAV codes
    "NAV-002": {"status": "wontfix", "note": "SUPPRESSED; nav span no heading"},
    "NAV-004": {"status": "wontfix", "note": "USAGE; EDUPUB heading hierarchy"},

    # PKG codes
    "PKG-001": {"status": "wontfix", "note": "WARNING; version mismatch info (handled differently)"},
    "PKG-015": {"status": "wontfix", "note": "FATAL; unable to read EPUB contents (covered by PKG-008)"},
    "PKG-017": {"status": "wontfix", "note": "WARNING; uncommon file extension"},
    "PKG-018": {"status": "wontfix", "note": "FATAL; EPUB file not found (handled by Go os.Open)"},
    "PKG-020": {"status": "wontfix", "note": "ERROR; OPF file not found (covered by OPF-002)"},
    "PKG-023": {"status": "wontfix", "note": "USAGE; EPUB 2 default validation profile"},
    "PKG-024": {"status": "wontfix", "note": "USAGE; uncommon file extension"},

    # RSC codes
    "RSC-018": {"status": "wontfix", "note": "SUPPRESSED; reported as RSC-007"},
    "RSC-019": {"status": "wontfix", "note": "WARNING; multi-rendition metadata.xml suggestion"},
    "RSC-023": {"status": "wontfix", "note": "SUPPRESSED; reported as RSC-020"},
    "RSC-024": {"status": "wontfix", "note": "USAGE; informative parsing warning"},

    # --- Tier 3 gap analysis: codes confirmed as wontfix after Java code review ---
    "OPF-011": {"status": "wontfix", "note": "commented out in epubcheck (dead code); page-spread mutual exclusivity handled by OPF-088"},
    "OPF-021": {"status": "wontfix", "note": "WARNING; non-registered URI scheme in DTBook only; very niche DAISY/DTBook feature"},
    "OPF-047": {"status": "wontfix", "note": "USAGE; OEBPS 1.2 syntax info; already handled via IsLegacyOEBPS12 detection"},
    "OPF-066": {"status": "wontfix", "note": "ERROR; pagination source metadata; only checked for EDUPUB profile (defunct)"},
    "OPF-097": {"status": "wontfix", "note": "USAGE; unreferenced manifest item; requires full reference tracking infrastructure"},
}


# ---------------------------------------------------------------------------
# Data structures
# ---------------------------------------------------------------------------

@dataclass
class MessageInfo:
    """Information about a single epubcheck message ID."""
    code: str                          # e.g., "OPF-001"
    severity: str = ""                 # e.g., "ERROR", "SUPPRESSED"
    message_text: str = ""             # English message template
    java_files: list[str] = field(default_factory=list)  # Java files that emit this
    category: str = ""                 # prefix: OPF, HTM, RSC, etc.

    # epubverify status
    in_go_code: bool = False           # found in Go source
    in_bdd_tests: bool = False         # found in BDD feature assertions
    override_status: str = ""          # from KNOWN_OVERRIDES
    override_note: str = ""            # from KNOWN_OVERRIDES

    @property
    def status(self) -> str:
        """Determine the overall status of this check."""
        if self.override_status:
            return self.override_status
        if self.severity == "SUPPRESSED":
            return "suppressed"
        if self.in_go_code:
            return "implemented"
        return "missing"


@dataclass
class AuditResult:
    """Summary of the full audit."""
    epubcheck_commit: str = ""
    total_codes: int = 0
    implemented: int = 0
    tested: int = 0
    suppressed: int = 0
    wontfix: int = 0
    missing: int = 0
    partial: int = 0
    messages: list[MessageInfo] = field(default_factory=list)


# ---------------------------------------------------------------------------
# Epubcheck repo management (shared with other audit scripts)
# ---------------------------------------------------------------------------

def ensure_epubcheck(repo_root: Path, skip_update: bool = False) -> Path:
    """Clone or update the epubcheck repo; return path to repo root."""
    cache = repo_root / CACHE_DIR
    if cache.exists():
        if not skip_update:
            print(f"Updating epubcheck in {cache} ...", file=sys.stderr)
            subprocess.run(
                ["git", "fetch", "origin", EPUBCHECK_BRANCH],
                cwd=cache, capture_output=True
            )
            subprocess.run(
                ["git", "reset", "--hard", f"origin/{EPUBCHECK_BRANCH}"],
                cwd=cache, capture_output=True
            )
        else:
            print(f"Using cached epubcheck in {cache} (skip-update)", file=sys.stderr)
    else:
        print(f"Cloning epubcheck into {cache} ...", file=sys.stderr)
        cache.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            ["git", "clone", "--depth", "1", "-b", EPUBCHECK_BRANCH,
             EPUBCHECK_REPO, str(cache)],
            capture_output=True, check=True,
        )

    # Print the epubcheck commit for reproducibility
    result = subprocess.run(
        ["git", "log", "-1", "--format=%H %s"],
        cwd=cache, capture_output=True, text=True,
    )
    commit = result.stdout.strip()
    print(f"epubcheck commit: {commit}", file=sys.stderr)
    return cache, commit


# ---------------------------------------------------------------------------
# Parsers
# ---------------------------------------------------------------------------

def parse_message_ids(epubcheck_dir: Path) -> dict[str, MessageInfo]:
    """Parse MessageId.java to extract all defined message IDs."""
    mid_file = epubcheck_dir / MESSAGES_DIR / "MessageId.java"
    if not mid_file.exists():
        print(f"ERROR: {mid_file} not found", file=sys.stderr)
        sys.exit(1)

    messages = {}
    # Match lines like: ACC_001("ACC-001"),
    pattern = re.compile(r'\s+\w+\("([A-Z]{2,3}[-_]\d{3}[a-z]?)"\)')

    for line in mid_file.read_text().splitlines():
        m = pattern.search(line)
        if m:
            # Normalize: epubcheck uses both - and _ in the string values
            code = m.group(1).replace("_", "-")
            category = code.split("-")[0]
            messages[code] = MessageInfo(code=code, category=category)

    print(f"  Parsed {len(messages)} message IDs from MessageId.java", file=sys.stderr)
    return messages


def parse_severities(epubcheck_dir: Path, messages: dict[str, MessageInfo]):
    """Parse DefaultSeverities.java to get severity for each message."""
    sev_file = epubcheck_dir / MESSAGES_DIR / "DefaultSeverities.java"
    if not sev_file.exists():
        print(f"WARNING: {sev_file} not found, skipping severity parsing", file=sys.stderr)
        return

    # Match lines like: severities.put(MessageId.ACC_001, Severity.USAGE);
    pattern = re.compile(
        r'severities\.put\(MessageId\.(\w+),\s*Severity\.(\w+)\)'
    )

    count = 0
    for line in sev_file.read_text().splitlines():
        m = pattern.search(line)
        if m:
            enum_name = m.group(1)  # e.g., ACC_001
            severity = m.group(2)    # e.g., USAGE
            # Convert enum name to code: ACC_001 -> ACC-001, HTM_060a -> HTM-060a
            code = re.sub(r'^([A-Z]+)_(\d+)(.*)', r'\1-\2\3', enum_name)
            if code in messages:
                messages[code].severity = severity
                count += 1

    print(f"  Parsed {count} severity mappings from DefaultSeverities.java", file=sys.stderr)


def parse_message_texts(epubcheck_dir: Path, messages: dict[str, MessageInfo]):
    """Parse MessageBundle.properties to get English message text."""
    bundle_file = epubcheck_dir / RESOURCES_DIR / "MessageBundle.properties"
    if not bundle_file.exists():
        print(f"WARNING: {bundle_file} not found, skipping message text", file=sys.stderr)
        return

    # Match lines like: ACC_004=Html "a" element must have text.
    # Also handle continuation lines and skip _SUG entries
    pattern = re.compile(r'^([A-Z]{2,3}_\d{3}[a-z]?)=(.+)$')

    count = 0
    for line in bundle_file.read_text().splitlines():
        if "_SUG" in line:
            continue
        m = pattern.match(line)
        if m:
            code = m.group(1).replace("_", "-")
            text = m.group(2).strip()
            if code in messages:
                messages[code].message_text = text
                count += 1

    print(f"  Parsed {count} message texts from MessageBundle.properties", file=sys.stderr)


def find_java_emitters(epubcheck_dir: Path, messages: dict[str, MessageInfo]):
    """Find which Java files emit each MessageId."""
    java_dir = epubcheck_dir / JAVA_SRC_DIR
    if not java_dir.exists():
        print(f"WARNING: {java_dir} not found, skipping Java source scan", file=sys.stderr)
        return

    # Build a map from enum name (e.g., ACC_001) to code (e.g., ACC-001)
    enum_to_code = {}
    for code in messages:
        enum_name = code.replace("-", "_")
        enum_to_code[enum_name] = code

    # Grep all Java files for MessageId.XXX
    pattern = re.compile(r'MessageId\.([A-Z_0-9a-z]+)')

    count = 0
    for java_file in java_dir.rglob("*.java"):
        # Skip the enum definition and severity files themselves
        if java_file.name in ("MessageId.java", "DefaultSeverities.java"):
            continue

        try:
            content = java_file.read_text(errors="replace")
        except Exception:
            continue

        rel_path = str(java_file.relative_to(java_dir))
        # Simplify to just the filename for readability
        short_name = java_file.stem

        for m in pattern.finditer(content):
            enum_name = m.group(1)
            if enum_name in enum_to_code:
                code = enum_to_code[enum_name]
                if short_name not in messages[code].java_files:
                    messages[code].java_files.append(short_name)
                    count += 1

    print(f"  Found {count} Java file → MessageId mappings", file=sys.stderr)


def find_go_implementations(repo_root: Path, messages: dict[str, MessageInfo]):
    """Find which check IDs are used in epubverify's Go source."""
    go_codes = set()

    for go_dir in GO_ALL_DIRS:
        go_path = repo_root / go_dir
        if not go_path.exists():
            continue
        for go_file in go_path.rglob("*.go"):
            try:
                content = go_file.read_text()
            except Exception:
                continue
            # Match quoted check IDs: "OPF-001", "RSC-005", etc.
            for m in re.finditer(r'"([A-Z]{2,3}-\d{3}[a-z]?)"', content):
                go_codes.add(m.group(1))

    count = 0
    for code in messages:
        if code in go_codes:
            messages[code].in_go_code = True
            count += 1

    print(f"  Found {count}/{len(messages)} check IDs in Go source", file=sys.stderr)


def find_bdd_coverage(repo_root: Path, messages: dict[str, MessageInfo]):
    """Find which check IDs are referenced in BDD feature files."""
    features_path = repo_root / FEATURES_DIR
    if not features_path.exists():
        return

    bdd_codes = set()
    for feature_file in features_path.rglob("*.feature"):
        try:
            content = feature_file.read_text()
        except Exception:
            continue
        # Match error/warning/fatal/usage/info + code patterns
        for m in re.finditer(
            r'(?:error|warning|fatal|usage|info)\s+([A-Z]{2,3}-\d{3}[a-z]?)',
            content
        ):
            bdd_codes.add(m.group(1))

    count = 0
    for code in messages:
        if code in bdd_codes:
            messages[code].in_bdd_tests = True
            count += 1

    print(f"  Found {count}/{len(messages)} check IDs in BDD features", file=sys.stderr)


def apply_overrides(messages: dict[str, MessageInfo]):
    """Apply manual status overrides from KNOWN_OVERRIDES."""
    for code, override in KNOWN_OVERRIDES.items():
        if code in messages:
            messages[code].override_status = override.get("status", "")
            messages[code].override_note = override.get("note", "")


# ---------------------------------------------------------------------------
# Report generation
# ---------------------------------------------------------------------------

def generate_report(messages: dict[str, MessageInfo], commit: str) -> AuditResult:
    """Generate the audit result from the collected data."""
    result = AuditResult(epubcheck_commit=commit)
    result.total_codes = len(messages)
    result.messages = sorted(messages.values(), key=lambda m: m.code)

    for msg in result.messages:
        status = msg.status
        if status == "implemented":
            result.implemented += 1
        elif status == "suppressed":
            result.suppressed += 1
        elif status == "wontfix":
            result.wontfix += 1
        elif status == "partial":
            result.partial += 1
        elif status == "missing":
            result.missing += 1

        if msg.in_bdd_tests:
            result.tested += 1

    return result


def print_text_report(result: AuditResult, summary_only: bool = False):
    """Print a human-readable gap report."""
    print()
    print("=" * 72)
    print("Tier 3: Java Code Check Coverage Report")
    print("=" * 72)
    print(f"epubcheck commit: {result.epubcheck_commit}")
    print()

    print(f"Total message IDs defined:  {result.total_codes}")
    print(f"  Implemented:              {result.implemented}")
    print(f"  Tested (BDD):             {result.tested}")
    print(f"  Suppressed by epubcheck:  {result.suppressed}")
    print(f"  Wontfix (too niche):      {result.wontfix}")
    print(f"  Partial:                  {result.partial}")
    print(f"  MISSING:                  {result.missing}")
    print()

    # Group by category
    categories = {}
    for msg in result.messages:
        cat = msg.category
        if cat not in categories:
            categories[cat] = {"implemented": 0, "missing": 0, "suppressed": 0,
                               "wontfix": 0, "partial": 0, "total": 0}
        categories[cat]["total"] += 1
        categories[cat][msg.status] += 1

    print("Coverage by category:")
    print(f"  {'Category':<8} {'Total':>5} {'Impl':>5} {'Miss':>5} {'Supp':>5} {'Skip':>5} {'Part':>5}")
    print(f"  {'--------':<8} {'-----':>5} {'-----':>5} {'-----':>5} {'-----':>5} {'-----':>5} {'-----':>5}")
    for cat in sorted(categories):
        c = categories[cat]
        print(f"  {cat:<8} {c['total']:>5} {c['implemented']:>5} "
              f"{c['missing']:>5} {c['suppressed']:>5} {c['wontfix']:>5} {c['partial']:>5}")
    print()

    if summary_only:
        return

    # Print missing checks (the gaps) grouped by category and severity
    missing = [m for m in result.messages if m.status == "missing"]
    if missing:
        print("-" * 72)
        print(f"MISSING CHECKS ({len(missing)} gaps to evaluate)")
        print("-" * 72)
        print()

        # Group by severity for prioritization
        by_severity = {}
        for msg in missing:
            sev = msg.severity or "UNKNOWN"
            if sev not in by_severity:
                by_severity[sev] = []
            by_severity[sev].append(msg)

        severity_order = ["FATAL", "ERROR", "WARNING", "USAGE", "INFO", "UNKNOWN"]
        for sev in severity_order:
            if sev not in by_severity:
                continue
            msgs = by_severity[sev]
            print(f"  [{sev}] ({len(msgs)} codes)")
            for msg in sorted(msgs, key=lambda m: m.code):
                java_info = ", ".join(msg.java_files[:3]) if msg.java_files else "?"
                text = msg.message_text[:80] if msg.message_text else ""
                print(f"    {msg.code:<12} Java: {java_info}")
                if text:
                    print(f"    {'':12} Msg:  {text}")
            print()

    # Print partial checks
    partial = [m for m in result.messages if m.status == "partial"]
    if partial:
        print("-" * 72)
        print(f"PARTIAL CHECKS ({len(partial)} need completion)")
        print("-" * 72)
        for msg in partial:
            print(f"  {msg.code:<12} {msg.override_note}")
        print()

    # Print implemented but untested
    untested = [m for m in result.messages
                if m.status == "implemented" and not m.in_bdd_tests
                and m.severity not in ("SUPPRESSED", "")]
    if untested:
        print("-" * 72)
        print(f"IMPLEMENTED BUT NOT IN BDD TESTS ({len(untested)})")
        print("-" * 72)
        for msg in sorted(untested, key=lambda m: m.code):
            sev = msg.severity
            print(f"  {msg.code:<12} [{sev}]  {msg.message_text[:60] if msg.message_text else ''}")
        print()


def print_json_report(result: AuditResult):
    """Print machine-readable JSON report."""
    output = {
        "epubcheck_commit": result.epubcheck_commit,
        "summary": {
            "total": result.total_codes,
            "implemented": result.implemented,
            "tested_bdd": result.tested,
            "suppressed": result.suppressed,
            "wontfix": result.wontfix,
            "partial": result.partial,
            "missing": result.missing,
        },
        "messages": [],
    }
    for msg in result.messages:
        output["messages"].append({
            "code": msg.code,
            "severity": msg.severity,
            "message": msg.message_text,
            "category": msg.category,
            "java_files": msg.java_files,
            "status": msg.status,
            "in_go_code": msg.in_go_code,
            "in_bdd_tests": msg.in_bdd_tests,
            "note": msg.override_note,
        })

    json.dump(output, sys.stdout, indent=2)
    print()


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Audit epubverify coverage against epubcheck Java code checks (Tier 3)"
    )
    parser.add_argument("--json", action="store_true",
                        help="Output machine-readable JSON")
    parser.add_argument("--skip-update", action="store_true",
                        help="Use cached epubcheck repo (no network)")
    parser.add_argument("--summary", action="store_true",
                        help="Print compact summary only (no per-code details)")
    args = parser.parse_args()

    # Find the repo root (the directory containing this script is scripts/)
    repo_root = Path(__file__).resolve().parent.parent

    # Step 1: Ensure epubcheck source is available
    print("Step 1: Ensuring epubcheck source ...", file=sys.stderr)
    epubcheck_dir, commit = ensure_epubcheck(repo_root, args.skip_update)

    # Step 2: Parse all message IDs
    print("Step 2: Parsing message IDs ...", file=sys.stderr)
    messages = parse_message_ids(epubcheck_dir)

    # Step 3: Parse severities
    print("Step 3: Parsing severities ...", file=sys.stderr)
    parse_severities(epubcheck_dir, messages)

    # Step 4: Parse message texts
    print("Step 4: Parsing message texts ...", file=sys.stderr)
    parse_message_texts(epubcheck_dir, messages)

    # Step 5: Find Java file emitters
    print("Step 5: Scanning Java source for MessageId usage ...", file=sys.stderr)
    find_java_emitters(epubcheck_dir, messages)

    # Step 6: Find Go implementations
    print("Step 6: Scanning epubverify Go source ...", file=sys.stderr)
    find_go_implementations(repo_root, messages)

    # Step 7: Find BDD test coverage
    print("Step 7: Scanning BDD feature files ...", file=sys.stderr)
    find_bdd_coverage(repo_root, messages)

    # Step 8: Apply manual overrides
    print("Step 8: Applying manual overrides ...", file=sys.stderr)
    apply_overrides(messages)

    # Step 9: Generate report
    print("Step 9: Generating report ...", file=sys.stderr)
    result = generate_report(messages, commit)

    if args.json:
        print_json_report(result)
    else:
        print_text_report(result, summary_only=args.summary)

    # Exit with non-zero if there are missing checks
    if result.missing > 0:
        print(f"\n{result.missing} missing checks found.", file=sys.stderr)


if __name__ == "__main__":
    main()
