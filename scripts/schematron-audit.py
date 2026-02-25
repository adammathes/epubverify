#!/usr/bin/env python3
"""
schematron-audit.py - Audit epubverify coverage against epubcheck's Schematron rules.

Downloads (or updates) the epubcheck repository, parses all Schematron (.sch)
files, and compares against a manifest of known-implemented checks in epubverify.
Produces a gap report and can optionally generate test fixture files for
missing checks.

Usage:
    python3 scripts/schematron-audit.py                  # gap report only
    python3 scripts/schematron-audit.py --generate-tests # also generate test fixtures
    python3 scripts/schematron-audit.py --json           # machine-readable output
    python3 scripts/schematron-audit.py --skip-update    # use cached epubcheck (no network)

How it works:
    epubcheck uses a 3-layer validation architecture:
      1. RELAX NG (.rnc/.rng) - structural grammar validation via Jing
      2. Schematron (.sch)    - semantic/XPath-based rules within a single document
      3. Java code            - cross-document and stateful checks

    This script audits layer 2 (Schematron) by parsing all .sch files and
    comparing rule pattern IDs against the KNOWN_CHECKS manifest below.

Updating from upstream:
    When epubcheck adds new Schematron rules, re-run this script to find new
    gaps. Then:
      1. Run: python3 scripts/schematron-audit.py
      2. Review the "MISSING CHECKS" section for newly added patterns
      3. Implement the check in the appropriate Go file (content.go, opf.go, etc.)
      4. Add an entry to KNOWN_CHECKS in this file with status="implemented"
      5. Run with --generate-tests to create test fixtures for remaining gaps
      6. Move generated scenarios from sch-generated-scenarios.feature.txt
         into the appropriate .feature file

    The .cache/epubcheck directory is gitignored and auto-cloned on first run.

Output directories (all under testdata/fixtures/schematron-gaps/):
    XHTML fixtures  -> schematron-gaps/xhtml/
    OPF fixtures    -> schematron-gaps/opf/
    OCF fixtures    -> schematron-gaps/ocf/
"""

import argparse
import json
import os
import subprocess
import sys
import xml.etree.ElementTree as ET
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
SCHEMA_SUBDIR = "src/main/resources/com/adobe/epubcheck/schema/30"

# Core schema files to audit (skip edupub/dict/idx/preview extensions)
CORE_SCHEMAS = [
    "package-30.sch",
    "epub-xhtml-30.sch",
    "epub-nav-30.sch",
    "media-overlay-30.sch",
    "ocf-encryption-30.sch",
    "ocf-metadata-30.sch",
    "collection-do-30.sch",
    "collection-manifest-30.sch",
    "multiple-renditions/container.sch",
    "mod/id-unique.sch",
]

SCH_NS = {"sch": "http://purl.oclc.org/dsdl/schematron"}

# ---------------------------------------------------------------------------
# Known-implemented checks in epubverify (pattern ID -> status)
#
# Maintain this list as checks are implemented.  The audit script will flag
# any Schematron pattern whose ID is NOT in this map.
#
# Status values:
#   "implemented" - fully implemented in epubverify
#   "partial"     - partially implemented (note explains gap)
#   "wontfix"     - intentionally skipped (too niche, deprecated, etc.)
# ---------------------------------------------------------------------------
KNOWN_CHECKS: dict[str, dict] = {
    # --- package-30.sch ---
    "opf.uid":                              {"status": "implemented", "go": "opf.go:checkUniqueIdentifierResolves"},
    "opf.dcterms.modified":                 {"status": "implemented", "go": "opf.go:checkDCTermsModifiedExactlyOnce"},
    "opf.dcterms.modified.syntax":          {"status": "implemented", "go": "opf.go:checkDCTermsModifiedFormat"},
    "opf.link.record":                      {"status": "implemented", "go": "opf.go:checkLinkRelation"},
    "opf.link.voicing":                     {"status": "implemented", "go": "opf.go:checkLinkRelation"},
    "opf.refines.relative":                 {"status": "implemented", "go": "opf.go:checkMetaRefinesTarget"},
    "opf.refines.by-fragment":              {"status": "partial",     "go": "opf.go:checkMetaRefinesTarget", "note": "warns for file-path refines but doesn't suggest fragment IDs"},
    "opf.refines.fragment-exists":          {"status": "implemented", "go": "opf.go:checkMetaRefinesTarget"},
    "opf.dc.subject.authority-term":        {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.authority":                   {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.belongs-to-collection":       {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.collection-type":             {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.display-seq":                 {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.file-as":                     {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.group-position":              {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.identifier-type":             {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.role":                        {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.source-of":                   {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.term":                        {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.meta.title-type":                  {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.itemref":                          {"status": "implemented", "go": "opf.go:checkSpineIdrefResolves"},
    "opf.toc.ncx":                          {"status": "implemented", "go": "opf.go:checkNCXIdentification"},
    "opf.toc.ncx.2":                        {"status": "implemented", "go": "opf.go:checkNCXIdentification"},
    "opf.nav.prop":                         {"status": "implemented", "go": "opf.go:checkNavProperty"},
    "opf.nav.type":                         {"status": "implemented", "go": "opf.go:checkNavProperty"},
    "opf.datanav.prop":                     {"status": "partial",     "go": "opf.go", "note": "data-nav accepted but cardinality <=1 not enforced"},
    "opf.cover-image":                      {"status": "implemented", "go": "opf.go:checkCoverImageUnique"},
    "opf.rendition.globals":                {"status": "implemented", "go": "opf.go:checkRenditionProperties"},
    "opf.rendition.overrides":              {"status": "implemented", "go": "opf.go:checkRenditionProperties"},
    "opf.collection.refines-restriction":   {"status": "partial",     "go": "opf.go:checkCollections", "note": "collection roles checked but @refines scoping not validated"},
    "opf_guideReferenceUnique":             {"status": "implemented", "go": "opf.go:checkGuideDuplicates"},
    "opf.duration.metadata.item":           {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.media.overlay":                    {"status": "implemented", "go": "opf.go:checkMediaOverlayType"},
    "opf.media.overlay.metadata.global":    {"status": "implemented", "go": "opf.go:checkMediaOverlayDurationMeta"},
    "opf.media.overlay.metadata.item":      {"status": "implemented", "go": "opf.go:checkMediaOverlayDurationMeta"},
    "opf.media.overlay.metadata.active-class":          {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.media.overlay.metadata.playback-active-class": {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},
    "opf.spine.duplicate.refs":             {"status": "implemented", "go": "opf.go:checkSpineUniqueIdrefs"},
    "opf.bindings.deprecated":              {"status": "implemented", "go": "opf.go"},
    "opf.meta.meta-auth.deprecated":        {"status": "implemented", "go": "opf.go:checkMetaRefinesPropertyRules"},

    # --- epub-xhtml-30.sch ---
    "encoding.decl.state":                  {"status": "implemented", "go": "content.go:checkEncodingDecl"},
    "title.present":                        {"status": "implemented", "go": "content.go:checkContentHasTitle"},
    "title.non-empty":                      {"status": "implemented", "go": "content.go:checkContentHasTitle"},
    "epub.switch.deprecated":               {"status": "implemented", "go": "content.go:checkEpubSwitchTrigger"},
    "epub.trigger.deprecated":              {"status": "implemented", "go": "content.go:checkEpubSwitchTrigger"},
    "lang-xmllang":                         {"status": "implemented", "go": "content.go:checkLangXMLLangMatch"},
    "id-unique":                            {"status": "implemented", "go": "content.go:checkUniqueIDs"},
    "dpub-aria.doc-endnote.deprecated":     {"status": "implemented", "go": "content.go:checkDeprecatedDPUBARIA"},
    "md-a-area":                            {"status": "implemented", "go": "content.go:checkMicrodataAttrs"},
    "md-iframe-embed-object":               {"status": "partial",     "go": "content.go:checkMicrodataAttrs", "note": "object checked, iframe/embed may be incomplete"},
    "map.id":                               {"status": "implemented", "go": "content.go:checkImageMapValid"},
    "descendant-dfn-dfn":                   {"status": "implemented", "go": "content.go:checkNestedDFN"},

    # --- epub-xhtml-30.sch: disallowed-descendants (Tier 2) ---
    "descendant-a-interactive":             {"status": "implemented", "go": "content.go:checkInteractiveNesting"},
    "descendant-button-interactive":        {"status": "implemented", "go": "content.go:checkInteractiveNesting"},
    "descendant-audio-audio":               {"status": "implemented", "go": "content.go:checkInteractiveNesting"},
    "descendant-audio-video":               {"status": "implemented", "go": "content.go:checkInteractiveNesting"},
    "descendant-video-video":               {"status": "implemented", "go": "content.go:checkInteractiveNesting"},
    "descendant-video-audio":               {"status": "implemented", "go": "content.go:checkInteractiveNesting"},
    "descendant-address-address":           {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-address-header":            {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-address-footer":            {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-form-form":                 {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-progress-progress":         {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-meter-meter":               {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-caption-table":             {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-header-header":             {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-header-footer":             {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-footer-footer":             {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-footer-header":             {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},
    "descendant-label-label":               {"status": "implemented", "go": "content.go:checkDisallowedDescendants"},

    # --- epub-xhtml-30.sch: required-ancestor (Tier 2) ---
    "ancestor-area-map":                    {"status": "implemented", "go": "content.go:checkRequiredAncestor"},
    "ancestor-imgismap-ahref":              {"status": "implemented", "go": "content.go:checkRequiredAncestor"},

    # --- epub-xhtml-30.sch: required-attr (Tier 2) ---
    "bdo-dir":                              {"status": "implemented", "go": "content.go:checkBdoDir"},

    # --- epub-xhtml-30.sch: IDREF/IDREFS validation (Tier 2) ---
    "idref-aria-activedescendant":          {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idref-label-for":                      {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idref-input-list":                     {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idref-forms-form":                     {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idrefs-headers":                       {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idrefs-aria-describedby":              {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idrefs-output-for":                    {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idrefs-aria-flowto":                   {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idrefs-aria-labelledby":               {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idrefs-aria-owns":                     {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idrefs-aria-controls":                 {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idref-trigger-observer":               {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idref-trigger-ref":                    {"status": "implemented", "go": "content.go:checkIDReferences"},
    "idref-mathml-xref":                    {"status": "partial",     "go": "content.go:checkIDReferences", "note": "MathML xref not tracked by checkIDReferences (only XHTML namespace)"},
    "idref-mathml-indenttarget":            {"status": "partial",     "go": "content.go:checkIDReferences", "note": "MathML indenttarget not tracked by checkIDReferences (only XHTML namespace)"},

    # --- epub-xhtml-30.sch: concrete patterns (Tier 2) ---
    "ssml-ph":                              {"status": "implemented", "go": "content.go:checkSSMLPhNesting"},
    "map.name":                             {"status": "implemented", "go": "content.go:checkDuplicateMapName"},
    "select-multiple":                      {"status": "implemented", "go": "content.go:checkSelectMultiple"},
    "meta-charset":                         {"status": "implemented", "go": "content.go:checkMetaCharset"},
    "link-sizes":                           {"status": "implemented", "go": "content.go:checkLinkSizes"},
    "track":                                {"status": "partial",     "go": "", "note": "track label uniqueness not yet implemented"},
    "md-media":                             {"status": "partial",     "go": "content.go:checkMicrodataAttrs", "note": "microdata media element checks not yet complete"},

    # --- epub-nav-30.sch ---
    "nav-ocurrence":                        {"status": "implemented", "go": "content.go:checkNavContentModel"},
    "span-no-sublist":                      {"status": "partial",     "go": "content.go:checkNavContentModel", "note": "checks li+span must have ol child, but not the reverse"},
    "landmarks":                            {"status": "implemented", "go": "content.go:checkNavContentModel"},
    "link-labels":                          {"status": "implemented", "go": "content.go:checkNavContentModel"},
    "span-labels":                          {"status": "implemented", "go": "content.go:checkNavContentModel"},
    "req-heading":                          {"status": "implemented", "go": "content.go:checkNavContentModel"},
    "heading-content":                      {"status": "implemented", "go": "content.go:checkNavContentModel"},
    "flat-nav":                             {"status": "implemented", "go": "content.go:checkNavContentModel"},

    # --- ocf-encryption-30.sch ---
    "ocf-enc.id-unique":                    {"status": "implemented", "go": "ocf.go:checkEncryptionXMLFull"},

    # --- ocf-metadata-30.sch (multi-rendition metadata.xml) ---
    # These apply to META-INF/metadata.xml in multi-rendition EPUBs.
    # The equivalent OPF-level checks are fully implemented in opf.go.
    # Multi-rendition EPUBs are extremely rare in practice.
    "ocf.uid":                              {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.dcterms.modified":                 {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.dcterms.modified.syntax":          {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.refines.relative":                 {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.meta.source-of":                   {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.link.record":                      {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.meta.belongs-to-collection":       {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.meta.collection-type":             {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},
    "ocf.rendition.globals":                {"status": "wontfix", "note": "multi-rendition metadata.xml; OPF equivalent in opf.go"},

    # --- collection-do-30.sch (distributable-object collections) ---
    "do.collection":                        {"status": "wontfix", "note": "distributable-object collection structure; very rare in practice"},

    # --- collection-manifest-30.sch ---
    "manifest.collection":                  {"status": "partial",  "go": "opf.go:checkCollections", "note": "top-level check done; child content/link count checks not yet implemented"},

    # --- multiple-renditions/container.sch ---
    "selection.accessModes":                {"status": "wontfix", "note": "multi-rendition container.xml; very rare"},
    "selection.mandatingOneSelectionAttribute": {"status": "wontfix", "note": "multi-rendition container.xml; very rare"},
    "mapping.atMostOne":                    {"status": "wontfix", "note": "multi-rendition container.xml; very rare"},
    "mapping.mediaType":                    {"status": "wontfix", "note": "multi-rendition container.xml; very rare"},
}


# ---------------------------------------------------------------------------
# Data model
# ---------------------------------------------------------------------------

@dataclass
class SchematronCheck:
    file: str
    pattern_id: str
    context: str
    check_type: str  # "assert" or "report"
    test_xpath: str
    message: str
    is_abstract_instance: bool = False
    abstract_pattern: str = ""
    params: dict = field(default_factory=dict)


@dataclass
class AuditResult:
    total_patterns: int = 0
    total_checks: int = 0
    implemented: int = 0
    partial: int = 0
    missing: int = 0
    missing_checks: list = field(default_factory=list)
    partial_checks: list = field(default_factory=list)


# ---------------------------------------------------------------------------
# Epubcheck repo management
# ---------------------------------------------------------------------------

def ensure_epubcheck(repo_root: Path) -> Path:
    """Clone or update the epubcheck repo; return path to schema dir."""
    cache = repo_root / CACHE_DIR
    if cache.exists():
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
        print(f"Cloning epubcheck into {cache} ...", file=sys.stderr)
        cache.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            ["git", "clone", "--depth", "1", "-b", EPUBCHECK_BRANCH,
             EPUBCHECK_REPO, str(cache)],
            capture_output=True, check=True,
        )
    schema_dir = cache / SCHEMA_SUBDIR
    if not schema_dir.exists():
        print(f"ERROR: schema directory not found at {schema_dir}", file=sys.stderr)
        sys.exit(1)

    # Print the epubcheck commit for reproducibility
    result = subprocess.run(
        ["git", "log", "-1", "--format=%H %s"],
        cwd=cache, capture_output=True, text=True,
    )
    print(f"epubcheck commit: {result.stdout.strip()}", file=sys.stderr)
    return schema_dir


# ---------------------------------------------------------------------------
# Schematron parser
# ---------------------------------------------------------------------------

def parse_schematron(schema_dir: Path) -> list[SchematronCheck]:
    """Parse all core .sch files and return a flat list of checks."""
    checks = []

    for fname in CORE_SCHEMAS:
        fpath = schema_dir / fname
        if not fpath.exists():
            print(f"  WARNING: {fname} not found, skipping", file=sys.stderr)
            continue

        tree = ET.parse(fpath)
        root = tree.getroot()

        # Collect abstract patterns for lookup
        abstracts = {}
        for pat in root.findall(".//sch:pattern[@abstract='true']", SCH_NS):
            pid = pat.get("id", "")
            abstracts[pid] = pat

        # Collect concrete patterns
        for pat in root.findall(".//sch:pattern", SCH_NS):
            if pat.get("abstract") == "true":
                continue

            pat_id = pat.get("id", "")
            is_a = pat.get("is-a", "")

            if is_a:
                # Abstract pattern instance
                params = {}
                for p in pat.findall("sch:param", SCH_NS):
                    params[p.get("name", "")] = p.get("value", "")

                checks.append(SchematronCheck(
                    file=fname,
                    pattern_id=pat_id,
                    context="(abstract instance)",
                    check_type="instance",
                    test_xpath="",
                    message=f"Instance of '{is_a}' with {params}",
                    is_abstract_instance=True,
                    abstract_pattern=is_a,
                    params=params,
                ))
                continue

            for rule in pat.findall(".//sch:rule", SCH_NS):
                ctx = rule.get("context", "")
                for assert_el in rule.findall("sch:assert", SCH_NS):
                    msg = " ".join("".join(assert_el.itertext()).split())
                    checks.append(SchematronCheck(
                        file=fname,
                        pattern_id=pat_id,
                        context=ctx,
                        check_type="assert",
                        test_xpath=assert_el.get("test", ""),
                        message=msg[:200],
                    ))
                for report_el in rule.findall("sch:report", SCH_NS):
                    msg = " ".join("".join(report_el.itertext()).split())
                    checks.append(SchematronCheck(
                        file=fname,
                        pattern_id=pat_id,
                        context=ctx,
                        check_type="report",
                        test_xpath=report_el.get("test", ""),
                        message=msg[:200],
                    ))

    return checks


# ---------------------------------------------------------------------------
# Audit logic
# ---------------------------------------------------------------------------

def audit(checks: list[SchematronCheck]) -> AuditResult:
    """Compare parsed checks against KNOWN_CHECKS."""
    result = AuditResult()

    # Deduplicate by pattern ID (a pattern can have multiple rules)
    seen_patterns: set[str] = set()

    for c in checks:
        pid = c.pattern_id
        if pid not in seen_patterns:
            seen_patterns.add(pid)
            result.total_patterns += 1

        result.total_checks += 1

        known = KNOWN_CHECKS.get(pid)
        if known is None:
            result.missing += 1
            result.missing_checks.append(c)
        elif known["status"] == "partial":
            result.partial += 1
            result.partial_checks.append(c)
        else:
            result.implemented += 1

    return result


# ---------------------------------------------------------------------------
# Test fixture generation
# ---------------------------------------------------------------------------

XHTML_TEMPLATE = """\
<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"
      xmlns:ssml="http://www.w3.org/2001/10/synthesis" xml:lang="en" lang="en">
<head><title>Test: {pattern_id}</title></head>
<body>
{body}
</body>
</html>"""


def _strip_ns(name: str) -> str:
    """Strip 'h:' or 'epub:' namespace prefix from element name."""
    for prefix in ("h:", "epub:", "math:", "svg:"):
        if name.startswith(prefix):
            return name[len(prefix):]
    return name


# ----- Abstract pattern generators -----

def _gen_disallowed_descendants(params: dict, pat_id: str) -> Optional[tuple[str, str]]:
    ancestor = _strip_ns(params.get("ancestor", ""))
    descendant = _strip_ns(params.get("descendant", ""))
    if not ancestor or not descendant:
        return None

    inner = f"<p>nested {descendant} inside {ancestor}</p>" if descendant not in ("audio", "video", "table") else ""
    body = f"  <{ancestor}>\n    <{descendant}>{inner}</{descendant}>\n  </{ancestor}>"
    desc = f"{descendant} must not appear inside {ancestor}"
    return body, desc


def _gen_no_interactive_content(params: dict, pat_id: str) -> Optional[tuple[str, str]]:
    ancestor = _strip_ns(params.get("ancestor", ""))
    if not ancestor:
        return None
    if ancestor == "a":
        body = '  <a href="#x"><button type="button">nested interactive</button></a>'
    else:
        body = f'  <{ancestor}><a href="#x">nested link inside {ancestor}</a></{ancestor}>'
    desc = f"interactive content must not appear inside {ancestor}"
    return body, desc


def _gen_required_ancestor(params: dict, pat_id: str) -> Optional[tuple[str, str]]:
    descendant = _strip_ns(params.get("descendant", "").split("[")[0])
    ancestor = _strip_ns(params.get("ancestor", "").split("[")[0])
    if not descendant or not ancestor:
        return None
    body = f"  <p><{descendant}>orphaned {descendant} without {ancestor} ancestor</{descendant}></p>"
    desc = f"{descendant} requires {ancestor} ancestor"
    return body, desc


def _gen_required_attr(params: dict, pat_id: str) -> Optional[tuple[str, str]]:
    elem = _strip_ns(params.get("elem", ""))
    attr = params.get("attr", "")
    if not elem or not attr:
        return None
    body = f"  <p><{elem}>missing {attr} attribute</{elem}></p>"
    desc = f"{elem} requires {attr} attribute"
    return body, desc


def _gen_idrefs_any(params: dict, pat_id: str) -> Optional[tuple[str, str]]:
    element = _strip_ns(params.get("element", "*"))
    attr = params.get("idrefs-attr-name", "")
    if not attr:
        return None
    # Handle wildcard and namespaced elements
    if element in ("*", ""):
        tag = "span"
    elif element.endswith("*"):
        # e.g. math:* → use a concrete MathML element
        tag = None  # handled by concrete fixtures
        return None
    else:
        tag = element
    body = f'  <p><{tag} {attr}="nonexistent-id">dangling {attr} reference</{tag}></p>'
    desc = f"{attr} must reference existing IDs"
    return body, desc


def _gen_idref_any(params: dict, pat_id: str) -> Optional[tuple[str, str]]:
    element = params.get("element", "*")
    attr = params.get("idref-attr-name", "")
    if not attr:
        return None
    # Defer namespaced elements (e.g. math:*, epub:trigger) to concrete fixtures
    if ":" in element:
        return None
    if element in ("*", ""):
        tag = "span"
    else:
        tag = element
    body = f'  <p><{tag} {attr}="nonexistent-id">dangling {attr} reference</{tag}></p>'
    desc = f"{attr} must reference existing IDs"
    return body, desc


def _gen_idref_named(params: dict, pat_id: str) -> Optional[tuple[str, str]]:
    element = _strip_ns(params.get("element", "*"))
    attr = params.get("idref-attr-name", "")
    target = _strip_ns(params.get("target-name", ""))
    if not attr:
        return None
    tag = element if element != "*" else "span"
    body = f'  <p><{tag} {attr}="wrong-target">must reference a {target} element</{tag}></p>'
    desc = f"{attr} on {tag} must reference a {target} element"
    return body, desc


ABSTRACT_GENERATORS = {
    "disallowed-descendants": _gen_disallowed_descendants,
    "no-interactive-content-descendants": _gen_no_interactive_content,
    "required-ancestor": _gen_required_ancestor,
    "required-attr": _gen_required_attr,
    "idrefs-any": _gen_idrefs_any,
    "idref-any": _gen_idref_any,
    "idref-named": _gen_idref_named,
}


# ----- Concrete (non-abstract) pattern fixture generators -----
# Keyed by pattern_id. Returns XHTML body content string or None.

CONCRETE_FIXTURES: dict[str, str] = {
    # --- epub-xhtml-30.sch: concrete patterns ---

    "idref-aria-activedescendant": """\
  <div aria-activedescendant="nonexistent-child">
    <span id="real-child">This child exists but the reference is wrong</span>
  </div>""",

    "idref-label-for": """\
  <label for="no-such-element">Label pointing to non-existent form control</label>
  <p id="not-a-form-control">This is not a form control</p>""",

    "idrefs-headers": """\
  <table>
    <tr>
      <th id="h1">Header</th>
      <td headers="nonexistent-header">Cell with dangling headers ref</td>
    </tr>
  </table>""",

    "map.name": """\
  <map name="duplicate-map"><area href="#a" alt="area1"/></map>
  <map name="duplicate-map"><area href="#b" alt="area2"/></map>""",

    "select-multiple": """\
  <form>
    <select>
      <option selected="selected">Option A</option>
      <option selected="selected">Option B - second selected without @multiple</option>
    </select>
  </form>""",

    "track": """\
  <video src="video.mp4">
    <track kind="subtitles" label="" srclang="en"/>
    <track kind="captions" default="default" srclang="en" label="caps1"/>
    <track kind="descriptions" default="default" srclang="en" label="desc1"/>
  </video>""",

    "ssml-ph": """\
  <p ssml:ph="outer">
    <span ssml:ph="inner">Nested ssml:ph is not allowed</span>
  </p>""",

    "link-sizes": """\
  <p>See head for the error</p>""",

    "meta-charset": """\
  <p>Two meta charset declarations in head</p>""",

    "md-media": """\
  <video itemprop="video">No src attribute</video>
  <audio itemprop="audio">No src attribute</audio>""",

    # --- MathML IDREF patterns ---

    "idref-mathml-xref": """\
  <p>
    <math xmlns="http://www.w3.org/1998/Math/MathML">
      <mi xref="nonexistent-id">x</mi>
    </math>
  </p>""",

    "idref-mathml-indenttarget": """\
  <p>
    <math xmlns="http://www.w3.org/1998/Math/MathML">
      <mo indenttarget="nonexistent-id">=</mo>
    </math>
  </p>""",

    # --- epub:trigger patterns (deprecated but still checked) ---

    "idref-trigger-observer": """\
  <div id="observed-elem">Content</div>
  <epub:trigger xmlns:epub="http://www.idpf.org/2007/ops"
      xmlns:ev="http://www.w3.org/2001/xml-events"
      action="show" ref="observed-elem" ev:observer="nonexistent-observer"
      ev:event="click"/>""",

    "idref-trigger-ref": """\
  <div id="some-observer">Content</div>
  <epub:trigger xmlns:epub="http://www.idpf.org/2007/ops"
      xmlns:ev="http://www.w3.org/2001/xml-events"
      action="show" ref="nonexistent-ref" ev:observer="some-observer"
      ev:event="click"/>""",
}

# link-sizes and meta-charset need custom head sections
CUSTOM_TEMPLATES: dict[str, str] = {
    "link-sizes": """\
<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
<head>
  <title>Test: link-sizes</title>
  <link rel="stylesheet" sizes="16x16" href="style.css"/>
</head>
<body>
  <p>link @sizes must only appear on link[rel="icon"]</p>
</body>
</html>""",

    "meta-charset": """\
<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en" lang="en">
<head>
  <meta charset="utf-8"/>
  <meta charset="utf-8"/>
  <title>Test: meta-charset</title>
</head>
<body>
  <p>Only one meta charset element allowed per document</p>
</body>
</html>""",
}


OPF_TEMPLATE = """\
<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid"
    xmlns:dc="http://purl.org/dc/elements/1.1/">
    <metadata>
        <dc:title>Test: {pattern_id}</dc:title>
        <dc:language>en</dc:language>
        <dc:identifier id="uid">urn:uuid:test-{pattern_id}</dc:identifier>
        <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>
    </metadata>
    <manifest>
        <item id="nav" href="nav.xhtml" properties="nav" media-type="application/xhtml+xml"/>
    </manifest>
    <spine>
        <itemref idref="nav"/>
    </spine>
{extra}
</package>"""

CONTAINER_TEMPLATE = """\
<?xml version="1.0" encoding="UTF-8"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"
    xmlns:rendition="http://www.idpf.org/2013/rendition">
    <rootfiles>
        <rootfile full-path="EPUB/package.opf" media-type="application/oebps-package+xml"/>
{extra_rootfiles}
    </rootfiles>
{extra}
</container>"""

OCF_METADATA_TEMPLATE = """\
<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://www.idpf.org/2013/metadata"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    unique-identifier="{uid_ref}">
    {metadata_body}
</metadata>"""


# ----- OPF fixtures for collection rules -----

OPF_FIXTURES: dict[str, str] = {
    # distributable-object collection missing metadata
    "do.collection": """\
    <collection role="distributable-object">
        <link href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    </collection>""",

    # manifest collection at top level (must be child of another collection)
    "manifest.collection": """\
    <collection role="manifest">
        <link href="chapter1.xhtml" media-type="application/xhtml+xml"/>
    </collection>""",
}

# ----- OCF metadata fixtures -----

OCF_METADATA_FIXTURES: dict[str, tuple[str, str]] = {
    # (uid_ref, metadata_body)
    "ocf.uid": ("nonexistent-uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="actual-uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>"""),

    "ocf.dcterms.modified": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>"""),

    "ocf.dcterms.modified.syntax": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">not-a-valid-date</meta>"""),

    "ocf.refines.relative": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>
    <meta property="title-type" refines="#nonexistent-id">main</meta>"""),

    "ocf.meta.source-of": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>
    <meta property="source-of">invalid-value</meta>"""),

    "ocf.link.record": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>
    <link rel="record" href="record.json"/>"""),

    "ocf.meta.belongs-to-collection": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>
    <meta id="creator1" property="role" refines="#uid">aut</meta>
    <meta property="belongs-to-collection" refines="#creator1">My Collection</meta>"""),

    "ocf.meta.collection-type": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>
    <meta property="collection-type" refines="#uid">series</meta>"""),

    "ocf.rendition.globals": ("uid", """\
<dc:title>Test</dc:title>
    <dc:language>en</dc:language>
    <dc:identifier id="uid">urn:uuid:test</dc:identifier>
    <meta property="dcterms:modified">2024-01-01T12:00:00Z</meta>
    <meta property="rendition:layout">reflowable</meta>
    <meta property="rendition:layout">pre-paginated</meta>"""),
}

# ----- Container fixtures for multi-rendition rules -----

CONTAINER_FIXTURES: dict[str, tuple[str, str]] = {
    # (extra_rootfiles, extra_elements)
    "selection.accessModes": ("""\
        <rootfile full-path="EPUB/alt.opf" media-type="application/oebps-package+xml"
            rendition:accessMode="textual textual"/>""", ""),

    "selection.mandatingOneSelectionAttribute": ("""\
        <rootfile full-path="EPUB/alt.opf" media-type="application/oebps-package+xml"/>""", ""),

    "mapping.atMostOne": ("", """\
    <links>
        <link href="mapping1.xhtml" rel="mapping" media-type="application/xhtml+xml"/>
        <link href="mapping2.xhtml" rel="mapping" media-type="application/xhtml+xml"/>
    </links>"""),

    "mapping.mediaType": ("", """\
    <links>
        <link href="mapping.json" rel="mapping" media-type="application/json"/>
    </links>"""),
}


def generate_test_fixtures(
    missing: list[SchematronCheck], repo_root: Path, gaps_dir: Path
) -> list[Path]:
    """Generate test fixture files for all missing checks.

    All fixtures go under gaps_dir with subdirectories by type:
      gaps_dir/xhtml/  - XHTML content document fixtures
      gaps_dir/opf/    - OPF package document fixtures
      gaps_dir/ocf/    - OCF container and metadata fixtures
    """
    xhtml_dir = gaps_dir / "xhtml"
    opf_dir = gaps_dir / "opf"
    ocf_dir = gaps_dir / "ocf"
    for d in (xhtml_dir, opf_dir, ocf_dir):
        d.mkdir(parents=True, exist_ok=True)

    generated = []
    seen_patterns: set[str] = set()

    for check in missing:
        pid = check.pattern_id
        if pid in seen_patterns:
            continue
        seen_patterns.add(pid)

        content = None
        out_dir = xhtml_dir
        ext = ".xhtml"

        # 1. Try abstract pattern generators (XHTML)
        if check.is_abstract_instance:
            gen = ABSTRACT_GENERATORS.get(check.abstract_pattern)
            if gen is not None:
                result = gen(check.params, pid)
                if result is not None:
                    body, _ = result
                    content = XHTML_TEMPLATE.format(pattern_id=pid, body=body)

        # 2. Try custom full-document templates (XHTML)
        if content is None and pid in CUSTOM_TEMPLATES:
            content = CUSTOM_TEMPLATES[pid]

        # 3. Try concrete body fixtures (XHTML)
        if content is None and pid in CONCRETE_FIXTURES:
            body = CONCRETE_FIXTURES[pid]
            content = XHTML_TEMPLATE.format(pattern_id=pid, body=body)

        # 4. Try OPF fixtures (collection rules)
        if content is None and pid in OPF_FIXTURES:
            extra = OPF_FIXTURES[pid]
            content = OPF_TEMPLATE.format(pattern_id=pid, extra=extra)
            out_dir = opf_dir
            ext = ".opf"

        # 5. Try OCF metadata fixtures
        if content is None and pid in OCF_METADATA_FIXTURES:
            uid_ref, metadata_body = OCF_METADATA_FIXTURES[pid]
            content = OCF_METADATA_TEMPLATE.format(uid_ref=uid_ref, metadata_body=metadata_body)
            out_dir = ocf_dir
            ext = ".xml"

        # 6. Try container fixtures (multi-rendition)
        if content is None and pid in CONTAINER_FIXTURES:
            extra_rootfiles, extra = CONTAINER_FIXTURES[pid]
            content = CONTAINER_TEMPLATE.format(extra_rootfiles=extra_rootfiles, extra=extra)
            out_dir = ocf_dir
            ext = ".xml"

        if content is None:
            continue

        filename = f"sch-{pid}-error{ext}"
        filepath = out_dir / filename
        filepath.write_text(content, encoding="utf-8")
        generated.append(filepath)

    return generated


# ---------------------------------------------------------------------------
# Report formatting
# ---------------------------------------------------------------------------

def print_text_report(checks: list[SchematronCheck], result: AuditResult):
    """Print a human-readable gap report."""
    print("=" * 70)
    print("SCHEMATRON AUDIT REPORT")
    print("epubcheck Schematron rules vs. epubverify implementation")
    print("=" * 70)
    print()
    print(f"Total patterns scanned:  {result.total_patterns}")
    print(f"Total individual checks: {result.total_checks}")
    print()
    print(f"  Implemented: {result.implemented}")
    print(f"  Partial:     {result.partial}")
    print(f"  Missing:     {result.missing}")
    print()

    if result.missing_checks:
        print("-" * 70)
        print("MISSING CHECKS")
        print("-" * 70)
        by_file: dict[str, list] = {}
        for c in result.missing_checks:
            by_file.setdefault(c.file, []).append(c)

        for fname, file_checks in sorted(by_file.items()):
            print(f"\n  [{fname}]")
            seen = set()
            for c in file_checks:
                key = (c.pattern_id, c.message[:80])
                if key in seen:
                    continue
                seen.add(key)
                label = c.pattern_id
                if c.is_abstract_instance:
                    label += f" (is-a {c.abstract_pattern})"
                print(f"    {label}")
                print(f"      {c.message[:100]}")

    if result.partial_checks:
        print()
        print("-" * 70)
        print("PARTIAL CHECKS (implemented but incomplete)")
        print("-" * 70)
        seen = set()
        for c in result.partial_checks:
            if c.pattern_id in seen:
                continue
            seen.add(c.pattern_id)
            known = KNOWN_CHECKS.get(c.pattern_id, {})
            print(f"  {c.pattern_id}: {known.get('note', '')}")


def print_json_report(checks: list[SchematronCheck], result: AuditResult):
    """Print machine-readable JSON report."""
    output = {
        "summary": {
            "total_patterns": result.total_patterns,
            "total_checks": result.total_checks,
            "implemented": result.implemented,
            "partial": result.partial,
            "missing": result.missing,
        },
        "missing": [
            {
                "file": c.file,
                "pattern_id": c.pattern_id,
                "abstract_pattern": c.abstract_pattern,
                "params": c.params,
                "message": c.message,
            }
            for c in result.missing_checks
        ],
        "partial": [
            {
                "pattern_id": c.pattern_id,
                "note": KNOWN_CHECKS.get(c.pattern_id, {}).get("note", ""),
            }
            for c in result.partial_checks
        ],
    }
    # Deduplicate
    seen = set()
    deduped = []
    for item in output["missing"]:
        if item["pattern_id"] not in seen:
            seen.add(item["pattern_id"])
            deduped.append(item)
    output["missing"] = deduped

    seen = set()
    deduped = []
    for item in output["partial"]:
        if item["pattern_id"] not in seen:
            seen.add(item["pattern_id"])
            deduped.append(item)
    output["partial"] = deduped

    print(json.dumps(output, indent=2))


# ---------------------------------------------------------------------------
# Feature file generation for missing checks
# ---------------------------------------------------------------------------

def generate_feature_snippet(missing: list[SchematronCheck]) -> str:
    """Generate Gherkin scenario outlines for missing checks."""
    lines = [
        "# Auto-generated scenarios for Schematron rules not yet implemented.",
        "# Add these to the appropriate .feature file after implementing the checks.",
        "# Generated by: python3 scripts/schematron-audit.py --generate-tests",
        "",
    ]

    seen = set()

    # Group by schema source file
    by_file: dict[str, list] = {}
    for c in missing:
        if c.pattern_id in seen:
            continue
        seen.add(c.pattern_id)
        by_file.setdefault(c.file, []).append(c)

    for fname, file_checks in sorted(by_file.items()):
        lines.append(f"# --- {fname} ---")
        lines.append("")

        for c in file_checks:
            pid = c.pattern_id
            # Determine fixture filename and expected error code
            if fname.startswith("epub-xhtml") or fname.startswith("epub-nav"):
                fixture = f"sch-{pid}-error.xhtml"
                error_code = "RSC-005"
            elif fname.startswith("collection") or fname.startswith("package"):
                fixture = f"sch-{pid}-error.opf"
                error_code = "RSC-005"
            elif fname.startswith("ocf-metadata"):
                fixture = f"sch-{pid}-error.xml"
                error_code = "RSC-005"
            elif fname.startswith("multiple-renditions"):
                fixture = f"sch-{pid}-error.xml"
                error_code = "RSC-005"
            else:
                fixture = f"sch-{pid}-error.xhtml"
                error_code = "RSC-005"

            desc = c.message[:80] if not c.is_abstract_instance else f"{pid}"
            lines.append(f"  @schematron @pending")
            lines.append(f"  Scenario: [{pid}] {desc}")
            lines.append(f"    When checking document '{fixture}'")
            lines.append(f"    Then error {error_code} is reported")
            lines.append(f"    And no other errors or warnings are reported")
            lines.append("")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Audit epubverify coverage against epubcheck Schematron rules",
    )
    parser.add_argument(
        "--generate-tests",
        action="store_true",
        help="Generate XHTML test fixtures for missing checks",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output machine-readable JSON instead of text",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=None,
        help="Directory for generated test fixtures (default: testdata/fixtures/schematron-gaps/)",
    )
    parser.add_argument(
        "--skip-update",
        action="store_true",
        help="Skip cloning/updating epubcheck (use existing cache)",
    )
    args = parser.parse_args()

    # Find repo root
    repo_root = Path(__file__).resolve().parent.parent
    if not (repo_root / "go.mod").exists():
        print("ERROR: cannot find repo root (no go.mod)", file=sys.stderr)
        sys.exit(1)

    # Get epubcheck schemas
    if args.skip_update:
        schema_dir = repo_root / CACHE_DIR / SCHEMA_SUBDIR
        if not schema_dir.exists():
            print("ERROR: no cached epubcheck. Run without --skip-update first.", file=sys.stderr)
            sys.exit(1)
    else:
        schema_dir = ensure_epubcheck(repo_root)

    # Parse all Schematron files
    print(f"Parsing Schematron files from {schema_dir.relative_to(repo_root)} ...", file=sys.stderr)
    checks = parse_schematron(schema_dir)
    print(f"Found {len(checks)} checks across {len(CORE_SCHEMAS)} schema files", file=sys.stderr)

    # Run audit
    result = audit(checks)

    # Output report
    if args.json:
        print_json_report(checks, result)
    else:
        print_text_report(checks, result)

    # Generate test fixtures if requested
    if args.generate_tests:
        gaps_dir = args.output_dir or (
            repo_root / "testdata" / "fixtures" / "schematron-gaps"
        )
        print(f"\nGenerating test fixtures in {gaps_dir.relative_to(repo_root)}/ ...", file=sys.stderr)
        generated = generate_test_fixtures(result.missing_checks, repo_root, gaps_dir)
        print(f"Generated {len(generated)} test fixture files:", file=sys.stderr)
        for p in generated:
            print(f"  {p.relative_to(repo_root)}", file=sys.stderr)

        # Also generate feature file snippet
        snippet = generate_feature_snippet(result.missing_checks)
        snippet_path = gaps_dir / "sch-generated-scenarios.feature.txt"
        snippet_path.write_text(snippet, encoding="utf-8")
        print(f"  {snippet_path.relative_to(repo_root)} (feature file snippet)", file=sys.stderr)

    # Exit with non-zero if there are gaps
    if result.missing > 0:
        sys.exit(2)


if __name__ == "__main__":
    main()
