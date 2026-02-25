#!/usr/bin/env python3
"""
relaxng-audit.py - Audit epubverify coverage against epubcheck's RelaxNG schemas.

This is Tier 1 of the systematic gap extraction from epubcheck's three
validation tiers (RelaxNG schemas, Schematron rules, Java code).

Epubcheck uses RelaxNG schemas (via Jing) to validate XML grammar — which
elements are allowed where, with what attributes, containing what content.
Violations are reported as RSC-005.  This script parses the .rnc schemas,
extracts the content model rules, and compares them against the checks
implemented in epubverify's Go code.

Usage:
    python3 scripts/relaxng-audit.py                  # full gap report
    python3 scripts/relaxng-audit.py --json           # machine-readable output
    python3 scripts/relaxng-audit.py --generate-tests # also generate test fixtures
    python3 scripts/relaxng-audit.py --skip-update    # use cached epubcheck

How it works:
    RelaxNG Compact (.rnc) files define element grammars using patterns like:
        p.elem = element p { p.inner & p.attrs }
        p.inner = ( common.inner.phrasing )
        common.elem.flow |= p.elem

    This script extracts:
    1. Element definitions and their content models (flow vs phrasing)
    2. Which content category each element belongs to (flow, phrasing, metadata)
    3. Attribute definitions on each element
    4. Specific content restrictions (empty elements, transparent models, etc.)

    It then compares these rules against the KNOWN_CHECKS manifest to find gaps.

Schema → Document Type mapping (from XMLValidators.java):
    epub-xhtml-30.rnc    → XHTML content documents
    epub-nav-30.rnc      → Navigation documents
    epub-svg-30.rnc      → SVG content documents
    package-30.rnc       → OPF package documents
    media-overlay-30.rnc → Media overlays
    ocf-container-30.rnc → OCF container
    ocf-encryption-30.rnc → Encryption
    ocf-metadata-30.rnc  → OCF metadata

    All schema violations → RSC-005

Output directories (under testdata/fixtures/relaxng-gaps/):
    xhtml/   - XHTML content model test fixtures
    opf/     - OPF schema test fixtures
    svg/     - SVG schema test fixtures
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
CACHE_DIR = ".cache/epubcheck"
SCHEMA_BASE = "src/main/resources/com/adobe/epubcheck/schema/30"

# Core EPUB 3 RelaxNG schema files to audit (XHTML content model focus)
HTML5_MODULES = [
    "mod/html5/common.rnc",
    "mod/html5/structural.rnc",
    "mod/html5/sectional.rnc",
    "mod/html5/block.rnc",
    "mod/html5/phrase.rnc",
    "mod/html5/embed.rnc",
    "mod/html5/media.rnc",
    "mod/html5/tables.rnc",
    "mod/html5/ruby.rnc",
    "mod/html5/revision.rnc",
    "mod/html5/data.rnc",
    "mod/html5/web-forms.rnc",
    "mod/html5/web-forms2.rnc",
    "mod/html5/meta.rnc",
    "mod/html5/applications.rnc",
    "mod/html5/core-scripting.rnc",
    "mod/html5/microdata.rnc",
    "mod/html5/rdfa.rnc",
    "mod/html5/web-components.rnc",
    "mod/html5/html5exclusions.rnc",
]

EPUB_MODULES = [
    "mod/epub-xhtml-inc.rnc",
    "mod/epub-xhtml-integration.rnc",
    "mod/epub-xhtml-svg-mathml.rnc",
    "mod/epub-type-attr.rnc",
    "mod/epub-ssml-attrs.rnc",
    "mod/epub-switch.rnc",
    "mod/epub-trigger.rnc",
    "mod/epub-prefix-attr.rnc",
]

OTHER_SCHEMAS = [
    "package-30.rnc",
    "epub-nav-30.rnc",
    "epub-svg-30.rnc",
    "media-overlay-30.rnc",
    "ocf-container-30.rnc",
    "ocf-encryption-30.rnc",
]


# ---------------------------------------------------------------------------
# Data model
# ---------------------------------------------------------------------------

@dataclass
class ElementDef:
    """A single HTML/EPUB element definition extracted from RelaxNG."""
    name: str
    source_file: str
    content_model: str          # "flow", "phrasing", "empty", "transparent", "special"
    content_category: str       # "flow", "phrasing", "metadata", "sectioning", "heading"
    attrs: list[str] = field(default_factory=list)  # specific attributes defined
    content_detail: str = ""    # raw RNC content model string
    is_void: bool = False       # empty content model (br, hr, img, etc.)


@dataclass
class ContentRule:
    """A content model rule: which elements can contain what."""
    parent: str
    allowed_content: str        # "flow", "phrasing", "empty", "special"
    source_file: str
    detail: str = ""
    priority: str = "medium"    # "high", "medium", "low"


@dataclass
class GapItem:
    """A single identified gap between the schema and epubverify."""
    category: str               # "content-model", "element-nesting", "attribute", "element-category"
    description: str
    priority: str               # "high", "medium", "low"
    schema_source: str          # source .rnc file
    affected_elements: list[str] = field(default_factory=list)
    epubverify_status: str = "missing"  # "missing", "partial", "implemented"
    go_file: str = ""           # if partially implemented, which Go file
    note: str = ""


# ---------------------------------------------------------------------------
# Known implemented checks in epubverify
#
# Maps rule categories to implementation status.
# ---------------------------------------------------------------------------

KNOWN_CHECKS: dict[str, dict] = {
    # --- Element validity ---
    "valid-html-elements": {
        "status": "implemented",
        "go": "content.go:checkInvalidHTMLElements",
        "note": "Validates against validHTMLElements map with 100+ elements",
    },
    "obsolete-elements": {
        "status": "implemented",
        "go": "content.go:checkNoObsoleteElements",
        "note": "center, font, basefont, big, blink, marquee, etc.",
    },
    "obsolete-attributes": {
        "status": "implemented",
        "go": "content.go:checkObsoleteAttrs",
        "note": "Deprecated presentation attributes per element",
    },

    # --- Content model: elements checked ---
    "p-phrasing-only": {
        "status": "implemented",
        "go": "content.go:checkBlockInPhrasing",
        "note": "<p> allows only phrasing content; block children flagged as RSC-005",
    },
    "h1-h6-phrasing-only": {
        "status": "implemented",
        "go": "content.go:checkBlockInPhrasing",
        "note": "Headings allow only phrasing content; block children flagged as RSC-005",
    },
    "span-phrasing-only": {
        "status": "implemented",
        "go": "content.go:checkBlockInPhrasing",
        "note": "<span> allows only phrasing content; block children flagged as RSC-005",
    },
    "pre-phrasing-only": {
        "status": "implemented",
        "go": "content.go:checkBlockInPhrasing",
        "note": "<pre> allows only phrasing content; block children flagged as RSC-005",
    },

    # --- Block-in-inline nesting ---
    "block-in-phrasing": {
        "status": "implemented",
        "go": "content.go:checkBlockInPhrasing",
        "note": "Block elements (div, p, table, ul, ol, etc.) inside phrasing-only parents",
    },

    # --- Nested element restrictions ---
    "nested-dfn": {
        "status": "implemented",
        "go": "content.go:checkNestedDFN",
    },
    "nested-a": {
        "status": "implemented",
        "go": "content.go:checkNestedAnchors",
    },
    "nested-time": {
        "status": "implemented",
        "go": "content.go:checkNestedTime",
    },

    # --- Empty/void elements ---
    "void-elements-no-children": {
        "status": "implemented",
        "go": "content.go:checkVoidElementChildren",
        "note": "Void elements (br, hr, img, input, etc.) cannot have child elements",
    },

    # --- Structural requirements ---
    "html-root": {
        "status": "implemented",
        "go": "content.go:checkHTMLRootElement",
    },
    "single-body": {
        "status": "implemented",
        "go": "content.go:checkSingleBody",
    },
    "head-required": {
        "status": "implemented",
        "go": "content.go:checkContentHasHead",
    },
    "title-required": {
        "status": "implemented",
        "go": "content.go:checkContentHasTitle",
    },

    # --- Table content model ---
    "table-structure": {
        "status": "implemented",
        "go": "content.go:checkTableContentModel",
        "note": "Table direct children validated: caption, colgroup, thead, tbody, tfoot, tr only",
    },
    "tr-children": {
        "status": "implemented",
        "go": "content.go:checkRestrictedChildren",
        "note": "<tr> can only contain <td> or <th>",
    },

    # --- List content model ---
    "ul-ol-children": {
        "status": "implemented",
        "go": "content.go:checkRestrictedChildren",
        "note": "<ul>/<ol> can only contain <li> (and script-supporting elements)",
    },
    "dl-structure": {
        "status": "implemented",
        "go": "content.go:checkRestrictedChildren",
        "note": "<dl> can only contain <dt>, <dd>, <div> as direct children",
    },

    # --- Form content model ---
    "select-children": {
        "status": "implemented",
        "go": "content.go:checkRestrictedChildren",
        "note": "<select> can only contain <option>, <optgroup>",
    },

    # --- Media content model ---
    "video-audio-source-or-src": {
        "status": "partial",
        "go": "content.go:checkForeignResourceFallbacks",
        "note": "Partially checks media source types but not full content model",
    },
    "picture-img-required": {
        "status": "implemented",
        "go": "content.go:checkPictureContentModel",
        "note": "<picture> validated: source* then img, no other children",
    },
    "figure-figcaption-position": {
        "status": "implemented",
        "go": "content.go:checkFigcaptionPosition",
        "note": "<figcaption> must be first or last child of <figure>",
    },

    # --- SVG content model ---
    "svg-content-model": {
        "status": "partial",
        "go": "content.go",
        "note": "SVG property checking and foreignObject done; full SVG content model not validated",
    },

    # --- MathML content model ---
    "mathml-content-model": {
        "status": "partial",
        "go": "content.go:checkMathMLContentOnly",
        "note": "Basic MathML child element checking implemented",
    },

    # --- Navigation document ---
    "nav-content-model": {
        "status": "implemented",
        "go": "content.go:checkNavContentModel",
        "note": "Full nav content model validated (most complete implementation)",
    },

    # --- OPF package ---
    "opf-schema": {
        "status": "implemented",
        "go": "opf.go",
        "note": "Full OPF schema validation via targeted Go checks",
    },

    # --- EPUB extensions ---
    "epub-switch-content": {
        "status": "implemented",
        "go": "content.go:checkEpubSwitchTrigger",
    },
    "epub-type-attr": {
        "status": "implemented",
        "go": "content.go:checkEpubTypeAttributes",
    },
    "ssml-attrs": {
        "status": "implemented",
        "go": "content.go",
    },

    # --- ID/IDREF ---
    "duplicate-ids": {
        "status": "implemented",
        "go": "content.go:checkDuplicateIDs",
    },
    "id-references": {
        "status": "implemented",
        "go": "content.go:checkIDReferences",
    },

    # --- Namespace ---
    "xhtml-namespace": {
        "status": "implemented",
        "go": "content.go:checkMissingNamespace",
    },

    # --- Encoding ---
    "encoding-declaration": {
        "status": "implemented",
        "go": "content.go:checkEncodingDecl",
    },

    # --- DOCTYPE ---
    "doctype-html5": {
        "status": "implemented",
        "go": "content.go:checkDoctypeHTML5",
    },

    # --- Media overlay ---
    "media-overlay-schema": {
        "status": "implemented",
        "go": "media.go",
    },

    # --- OCF container ---
    "ocf-container-schema": {
        "status": "implemented",
        "go": "ocf.go",
    },

    # --- Transparent content model ---
    "a-transparent-flow": {
        "status": "implemented",
        "go": "content.go:checkTransparentContentModel",
        "note": "<a> transparent content model inherits phrasing restriction from parent",
    },
    "ins-del-transparent": {
        "status": "implemented",
        "go": "content.go:checkTransparentContentModel",
        "note": "<ins>/<del> transparent content model inherits parent restriction",
    },
    "object-transparent": {
        "status": "implemented",
        "go": "content.go:checkTransparentContentModel",
        "note": "<object> transparent content model inherits parent restriction",
    },

    # --- Interactive content restrictions ---
    "interactive-in-interactive": {
        "status": "implemented",
        "go": "content.go:checkInteractiveNesting",
        "note": "All interactive elements (a, button, input, select, textarea, label, etc.) checked",
    },

    # --- Required attributes ---
    "img-alt": {
        "status": "partial",
        "go": "accessibility.go",
        "note": "Checked via accessibility checks, not schema validation",
    },
    "input-type": {
        "status": "missing",
        "note": "<input> type attribute constrains allowed attributes (schema validates this)",
    },
}


# ---------------------------------------------------------------------------
# RelaxNG Compact Syntax Parser (pragmatic regex-based)
# ---------------------------------------------------------------------------

def parse_rnc_files(schema_dir: Path, file_list: list[str]) -> tuple[dict, dict, dict]:
    """Parse .rnc files and extract element definitions, content models, and categories.

    Returns:
        elements: dict mapping element name -> ElementDef
        content_rules: dict mapping parent element -> ContentRule
        raw_patterns: dict mapping pattern name -> raw definition string
    """
    elements: dict[str, ElementDef] = {}
    content_rules: dict[str, ContentRule] = {}
    raw_patterns: dict[str, str] = {}

    # Patterns to extract from .rnc files
    # element X { inner & attrs }
    elem_pat = re.compile(
        r'(\w[\w.-]*\.elem(?:\.[\w.-]*)?)\s*=\s*element\s+([\w:.-]+)\s*\{([^}]+)\}',
        re.MULTILINE | re.DOTALL,
    )
    # X.inner = ( content-model )
    inner_pat = re.compile(
        r'(\w[\w.-]*\.inner(?:\.[\w.-]*)?)\s*=\s*\(([^)]+)\)',
        re.MULTILINE | re.DOTALL,
    )
    # common.elem.flow |= X.elem
    flow_add_pat = re.compile(
        r'common\.elem\.flow\s*\|=\s*(\w[\w.-]*\.elem(?:\.[\w.-]*)?)',
    )
    # common.elem.phrasing |= X.elem
    phrasing_add_pat = re.compile(
        r'common\.elem\.phrasing\s*\|=\s*(\w[\w.-]*\.elem(?:\.[\w.-]*)?)',
    )
    # common.elem.metadata |= X.elem
    metadata_add_pat = re.compile(
        r'common\.elem\.metadata\s*\|=\s*(\w[\w.-]*\.elem(?:\.[\w.-]*)?)',
    )

    flow_elems = set()
    phrasing_elems = set()
    metadata_elems = set()

    for fname in file_list:
        fpath = schema_dir / fname
        if not fpath.exists():
            continue
        content = fpath.read_text(encoding="utf-8")

        # Strip comments
        content_no_comments = re.sub(r'#[^\n]*', '', content)

        # Extract element definitions
        for match in elem_pat.finditer(content_no_comments):
            pattern_name = match.group(1)
            elem_name = match.group(2)
            body = match.group(3).strip()

            raw_patterns[pattern_name] = body

            # Determine content model from the body
            content_model = "special"
            if "empty" in body and body.strip().replace("(", "").replace(")", "").strip() == "empty":
                content_model = "empty"

            ed = ElementDef(
                name=elem_name,
                source_file=fname,
                content_model=content_model,
                content_category="unknown",
            )
            # Don't overwrite if we already have one (first definition wins)
            if elem_name not in elements:
                elements[elem_name] = ed

        # Extract inner definitions
        for match in inner_pat.finditer(content_no_comments):
            pattern_name = match.group(1)
            inner_content = match.group(2).strip()
            raw_patterns[pattern_name] = inner_content

        # Track content categories
        for match in flow_add_pat.finditer(content_no_comments):
            flow_elems.add(match.group(1))
        for match in phrasing_add_pat.finditer(content_no_comments):
            phrasing_elems.add(match.group(1))
        for match in metadata_add_pat.finditer(content_no_comments):
            metadata_elems.add(match.group(1))

    # Map pattern names back to element names and set categories
    pattern_to_elem = {}
    for ename, edef in elements.items():
        for pname, body in raw_patterns.items():
            if pname.startswith(ename.replace(":", "_").replace("-", "")) or \
               pname.split(".")[0] == ename:
                pattern_to_elem[pname] = ename

    # More robust mapping: extract element name from pattern name
    for pname in list(flow_elems) + list(phrasing_elems) + list(metadata_elems):
        # pattern like "p.elem", "h1.elem", "section.elem", "a.elem.phrasing"
        base = pname.split(".")[0]
        # Try to find matching element
        for ename in elements:
            if ename == base or ename == base.replace("_", ":"):
                pattern_to_elem[pname] = ename
                break

    # Set content categories
    for pname in flow_elems:
        ename = pattern_to_elem.get(pname)
        if ename and ename in elements:
            elements[ename].content_category = "flow"

    for pname in phrasing_elems:
        ename = pattern_to_elem.get(pname)
        if ename and ename in elements:
            if elements[ename].content_category == "flow":
                elements[ename].content_category = "phrasing+flow"  # both
            else:
                elements[ename].content_category = "phrasing"

    for pname in metadata_elems:
        ename = pattern_to_elem.get(pname)
        if ename and ename in elements:
            elements[ename].content_category = "metadata"

    # Resolve content models from inner patterns
    for ename, edef in elements.items():
        # Look for X.inner pattern
        for suffix in [".inner", ".inner.flow", ".inner.phrasing"]:
            key = ename + suffix
            if key in raw_patterns:
                inner = raw_patterns[key]
                if "common.inner.phrasing" in inner and "common.inner.flow" not in inner:
                    edef.content_model = "phrasing"
                elif "common.inner.flow" in inner:
                    edef.content_model = "flow"
                elif "common.inner.transparent.flow" in inner:
                    edef.content_model = "transparent"
                elif "empty" in inner:
                    edef.content_model = "empty"
                    edef.is_void = True
                edef.content_detail = inner.strip()
                break

    # Build content rules
    for ename, edef in elements.items():
        if edef.content_model in ("phrasing", "flow", "empty", "transparent"):
            content_rules[ename] = ContentRule(
                parent=ename,
                allowed_content=edef.content_model,
                source_file=edef.source_file,
                detail=edef.content_detail,
            )

    return elements, content_rules, raw_patterns


# ---------------------------------------------------------------------------
# HTML5 Content Model Knowledge Base
#
# These are the ground-truth rules from the HTML5 spec, extracted from the
# RelaxNG schemas.  We use these for gap analysis.
# ---------------------------------------------------------------------------

# Elements whose content model is "phrasing" (only inline content allowed)
PHRASING_CONTENT_PARENTS = {
    "p", "h1", "h2", "h3", "h4", "h5", "h6",
    "pre", "span", "em", "strong", "small", "mark", "abbr", "dfn",
    "i", "b", "s", "u", "code", "var", "samp", "kbd", "sup", "sub",
    "q", "cite", "bdo", "bdi", "dt", "label", "legend",
    "summary", "figcaption",
    # These have phrasing in certain contexts:
    "a",  # when href is present, phrasing only (in phrasing context)
    "time", "data", "output",
}

# Elements that are "flow content" (block-level, can appear in flow context)
FLOW_CONTENT_ELEMENTS = {
    "div", "p", "hr", "pre", "blockquote", "section", "nav", "article",
    "aside", "header", "footer", "main", "search", "address",
    "h1", "h2", "h3", "h4", "h5", "h6", "hgroup",
    "ul", "ol", "dl", "figure", "table", "form", "fieldset", "details",
    "dialog", "menu",
}

# Elements that are "phrasing content" (inline, can appear in phrasing context)
PHRASING_CONTENT_ELEMENTS = {
    "a", "em", "strong", "small", "mark", "abbr", "dfn",
    "i", "b", "s", "u", "code", "var", "samp", "kbd", "sup", "sub",
    "q", "cite", "span", "bdo", "bdi", "br", "wbr",
    "time", "data", "ruby", "img", "picture", "iframe", "embed",
    "object", "video", "audio", "map", "area", "input", "button",
    "select", "textarea", "output", "label", "meter", "progress",
    "math", "svg",
}

# Void (empty) elements — cannot have children
VOID_ELEMENTS = {
    "br", "hr", "img", "input", "link", "meta", "area", "base",
    "col", "embed", "param", "source", "track", "wbr",
}

# Elements with restricted children
RESTRICTED_CHILDREN = {
    "ul": {"li"},
    "ol": {"li"},
    "dl": {"dt", "dd", "div"},
    "table": {"caption", "colgroup", "thead", "tbody", "tfoot", "tr"},
    "thead": {"tr"},
    "tbody": {"tr"},
    "tfoot": {"tr"},
    "tr": {"td", "th"},
    "colgroup": {"col"},
    "select": {"option", "optgroup"},
    "optgroup": {"option"},
    "hgroup": {"h1", "h2", "h3", "h4", "h5", "h6", "p"},
    "ruby": {"rb", "rt", "rtc", "rp"},
    "picture": {"source", "img"},
    "datalist": {"option"},
}


# ---------------------------------------------------------------------------
# Gap Analysis
# ---------------------------------------------------------------------------

def analyze_gaps(
    elements: dict[str, ElementDef],
    content_rules: dict[str, ContentRule],
) -> list[GapItem]:
    """Compare schema rules against KNOWN_CHECKS to find gaps."""
    gaps: list[GapItem] = []

    # 1. Content model gaps: phrasing-only parents
    # The general "block-in-phrasing" check covers ALL phrasing-only parents
    general_check = KNOWN_CHECKS.get("block-in-phrasing", {})
    general_implemented = general_check.get("status") == "implemented"

    # Check each phrasing-content parent
    for elem in sorted(PHRASING_CONTENT_PARENTS):
        rule_id = f"{elem}-phrasing-only"
        known = KNOWN_CHECKS.get(rule_id)
        if (known and known["status"] == "implemented") or general_implemented:
            continue

        # This is a gap
        priority = "high" if elem in ("p", "h1", "h2", "h3", "h4", "h5", "h6", "span") else "medium"
        gaps.append(GapItem(
            category="content-model",
            description=f"<{elem}> allows only phrasing content (text and inline elements). "
                        f"Block elements like <div>, <p>, <table>, <ul> must not appear inside <{elem}>.",
            priority=priority,
            schema_source="mod/html5/block.rnc" if elem in ("p", "pre") else "mod/html5/phrase.rnc",
            affected_elements=[elem],
            epubverify_status=known["status"] if known else "missing",
            go_file=known.get("go", "") if known else "",
        ))

    # 2. Void elements cannot have children
    known = KNOWN_CHECKS.get("void-elements-no-children")
    if not known or known["status"] != "implemented":
        gaps.append(GapItem(
            category="content-model",
            description="Void elements cannot have children. Elements like <br>, <hr>, <img>, "
                        "<input>, <meta>, <link> must be empty.",
            priority="medium",
            schema_source="mod/html5/phrase.rnc",
            affected_elements=sorted(VOID_ELEMENTS),
            epubverify_status="missing",
        ))

    # 3. Restricted children
    # checkRestrictedChildren and checkTableContentModel cover these elements.
    # Check if the general restricted-children checks are implemented.
    restricted_children_implemented = (
        KNOWN_CHECKS.get("ul-ol-children", {}).get("status") == "implemented"
        and KNOWN_CHECKS.get("tr-children", {}).get("status") == "implemented"
        and KNOWN_CHECKS.get("dl-structure", {}).get("status") == "implemented"
    )

    for parent, allowed in sorted(RESTRICTED_CHILDREN.items()):
        # Check if this specific parent's children are validated
        is_checked = restricted_children_implemented
        if not is_checked:
            for k, v in KNOWN_CHECKS.items():
                if parent in k and v["status"] == "implemented":
                    is_checked = True
                    break

        if not is_checked:
            priority = "high" if parent in ("ul", "ol", "table", "tr", "select") else "medium"
            gaps.append(GapItem(
                category="element-nesting",
                description=f"<{parent}> can only contain: {', '.join(f'<{c}>' for c in sorted(allowed))}. "
                            f"Other elements are not allowed as direct children.",
                priority=priority,
                schema_source="mod/html5/block.rnc" if parent in ("ul", "ol", "dl") else "mod/html5/tables.rnc",
                affected_elements=[parent] + sorted(allowed),
                epubverify_status="missing",
            ))

    # 4. Transparent content model
    # checkTransparentContentModel covers all transparent elements
    transparent_implemented = KNOWN_CHECKS.get("a-transparent-flow", {}).get("status") == "implemented"
    for elem in ["a", "ins", "del", "object", "video", "audio", "map"]:
        if transparent_implemented:
            continue
        known_t = None
        for k, v in KNOWN_CHECKS.items():
            if elem in k and "transparent" in k:
                known_t = v
                break
        if not known_t or known_t["status"] != "implemented":
            gaps.append(GapItem(
                category="content-model",
                description=f"<{elem}> has a transparent content model — it inherits the "
                            f"content model of its parent. If parent allows only phrasing, "
                            f"<{elem}> must also contain only phrasing content.",
                priority="low",
                schema_source="mod/html5/phrase.rnc" if elem == "a" else "mod/html5/embed.rnc",
                affected_elements=[elem],
                epubverify_status="missing",
            ))

    # 5. Interactive content restrictions (beyond nested <a>)
    known_i = KNOWN_CHECKS.get("interactive-in-interactive")
    if not known_i or known_i["status"] != "implemented":
        gaps.append(GapItem(
            category="element-nesting",
            description="Interactive elements (<a>, <button>, <input>, <select>, <textarea>, "
                        "<label>, <embed>, <iframe>) cannot be nested inside other interactive elements.",
            priority="medium",
            schema_source="mod/html5/phrase.rnc",
            affected_elements=["a", "button", "input", "select", "textarea", "label"],
            epubverify_status="partial" if known_i else "missing",
            go_file="content.go:checkNestedAnchors",
            note="Only nested <a> is currently checked",
        ))

    # 6. Specific attribute restrictions from schema
    known_input = KNOWN_CHECKS.get("input-type")
    if not known_input or known_input["status"] != "implemented":
        gaps.append(GapItem(
            category="attribute",
            description="<input> type attribute constrains which other attributes are allowed. "
                        "For example, 'checked' is only valid on type='checkbox' and type='radio'. "
                        "The schema defines 13+ input type variants with distinct attribute sets.",
            priority="low",
            schema_source="mod/html5/web-forms.rnc",
            affected_elements=["input"],
            epubverify_status="missing",
        ))

    # 7. Picture element content model
    known_pic = KNOWN_CHECKS.get("picture-img-required")
    if not known_pic or known_pic["status"] != "implemented":
        gaps.append(GapItem(
            category="content-model",
            description="<picture> must contain <source> elements followed by exactly one <img>. "
                        "The <img> must be the last child.",
            priority="low",
            schema_source="mod/html5/embed.rnc",
            affected_elements=["picture", "source", "img"],
            epubverify_status="missing",
        ))

    # 8. Figure/figcaption position
    known_fig = KNOWN_CHECKS.get("figure-figcaption-position")
    if not known_fig or known_fig["status"] != "implemented":
        gaps.append(GapItem(
            category="content-model",
            description="<figcaption> must be the first or last child of <figure>. "
                        "It cannot appear in the middle.",
            priority="low",
            schema_source="mod/html5/media.rnc",
            affected_elements=["figure", "figcaption"],
            epubverify_status="missing",
        ))

    return gaps


# ---------------------------------------------------------------------------
# Report formatting
# ---------------------------------------------------------------------------

def print_text_report(
    elements: dict[str, ElementDef],
    content_rules: dict[str, ContentRule],
    gaps: list[GapItem],
):
    """Print a human-readable gap report."""
    print("=" * 70)
    print("RELAXNG SCHEMA AUDIT REPORT (Tier 1)")
    print("epubcheck RelaxNG schemas vs. epubverify implementation")
    print("=" * 70)
    print()

    # Summary stats
    total_elements = len(elements)
    flow_count = sum(1 for e in elements.values() if "flow" in e.content_category)
    phrasing_count = sum(1 for e in elements.values() if "phrasing" in e.content_category)
    void_count = sum(1 for e in elements.values() if e.is_void)

    print(f"Schema elements parsed:    {total_elements}")
    print(f"  Flow content elements:   {flow_count}")
    print(f"  Phrasing elements:       {phrasing_count}")
    print(f"  Void elements:           {void_count}")
    print()

    implemented = sum(1 for v in KNOWN_CHECKS.values() if v["status"] == "implemented")
    partial = sum(1 for v in KNOWN_CHECKS.values() if v["status"] == "partial")
    missing = sum(1 for v in KNOWN_CHECKS.values() if v["status"] == "missing")

    print(f"Known check categories:    {len(KNOWN_CHECKS)}")
    print(f"  Implemented:             {implemented}")
    print(f"  Partial:                 {partial}")
    print(f"  Missing:                 {missing}")
    print()

    high_gaps = [g for g in gaps if g.priority == "high"]
    med_gaps = [g for g in gaps if g.priority == "medium"]
    low_gaps = [g for g in gaps if g.priority == "low"]

    print(f"Gaps identified:           {len(gaps)}")
    print(f"  High priority:           {len(high_gaps)}")
    print(f"  Medium priority:         {len(med_gaps)}")
    print(f"  Low priority:            {len(low_gaps)}")
    print()

    # Content model summary
    print("-" * 70)
    print("ELEMENT CONTENT MODELS (from RelaxNG schemas)")
    print("-" * 70)
    print()
    print("Elements that allow ONLY PHRASING content (no block elements):")
    general_ok = KNOWN_CHECKS.get("block-in-phrasing", {}).get("status") == "implemented"
    for elem in sorted(PHRASING_CONTENT_PARENTS):
        elem_ok = KNOWN_CHECKS.get(f"{elem}-phrasing-only", {}).get("status") == "implemented"
        marker = "  [OK]" if (elem_ok or general_ok) else "  [GAP]"
        print(f"  {marker} <{elem}>")
    print()

    print("Void elements (must be empty — no children):")
    for elem in sorted(VOID_ELEMENTS):
        print(f"    <{elem}>")
    known_void = KNOWN_CHECKS.get("void-elements-no-children", {})
    print(f"  Status: {known_void.get('status', 'missing')}")
    print()

    print("Elements with restricted children:")
    for parent, children in sorted(RESTRICTED_CHILDREN.items()):
        kids = ", ".join(f"<{c}>" for c in sorted(children))
        print(f"    <{parent}> → {kids}")
    print()

    # Gaps by priority
    for priority, label in [("high", "HIGH PRIORITY"), ("medium", "MEDIUM PRIORITY"), ("low", "LOW PRIORITY")]:
        priority_gaps = [g for g in gaps if g.priority == priority]
        if not priority_gaps:
            continue

        print("-" * 70)
        print(f"{label} GAPS")
        print("-" * 70)
        print()
        for i, gap in enumerate(priority_gaps, 1):
            print(f"  {i}. [{gap.category}] {gap.description}")
            print(f"     Status: {gap.epubverify_status}")
            if gap.go_file:
                print(f"     Go file: {gap.go_file}")
            if gap.note:
                print(f"     Note: {gap.note}")
            print(f"     Elements: {', '.join(gap.affected_elements[:10])}")
            print(f"     Schema: {gap.schema_source}")
            print()

    # Implementation recommendations
    print("=" * 70)
    print("IMPLEMENTATION RECOMMENDATIONS")
    print("=" * 70)
    print()
    print("Phase 1 — Block-in-Inline Detection (highest impact):")
    print("  Implement content category tracking (flow vs phrasing) in content.go.")
    print("  When inside a phrasing-only parent (p, h1-h6, span, etc.), flag any")
    print("  flow-only child elements (div, p, table, ul, ol, dl, etc.) as RSC-005.")
    print("  This catches the most common real-world schema violations.")
    print()
    print("Phase 2 — Restricted Children Validation:")
    print("  Validate that list elements (ul/ol) only contain li,")
    print("  table elements follow proper structure, and select/optgroup")
    print("  only contain option elements.")
    print()
    print("Phase 3 — Void Element Children:")
    print("  Flag content inside void elements (br, hr, img, input, etc.)")
    print()
    print("Phase 4 — Interactive Nesting:")
    print("  Extend nested-anchor check to cover all interactive elements.")
    print()
    print("Phase 5 — Transparent Content Models & Edge Cases:")
    print("  Implement transparent content model inheritance for a, ins, del, etc.")
    print()


def print_json_report(
    elements: dict[str, ElementDef],
    content_rules: dict[str, ContentRule],
    gaps: list[GapItem],
):
    """Print machine-readable JSON report."""
    output = {
        "summary": {
            "total_elements": len(elements),
            "flow_elements": sum(1 for e in elements.values() if "flow" in e.content_category),
            "phrasing_elements": sum(1 for e in elements.values() if "phrasing" in e.content_category),
            "void_elements": sum(1 for e in elements.values() if e.is_void),
            "known_checks": len(KNOWN_CHECKS),
            "implemented": sum(1 for v in KNOWN_CHECKS.values() if v["status"] == "implemented"),
            "partial": sum(1 for v in KNOWN_CHECKS.values() if v["status"] == "partial"),
            "missing": sum(1 for v in KNOWN_CHECKS.values() if v["status"] == "missing"),
            "gaps": len(gaps),
            "high_priority_gaps": sum(1 for g in gaps if g.priority == "high"),
            "medium_priority_gaps": sum(1 for g in gaps if g.priority == "medium"),
            "low_priority_gaps": sum(1 for g in gaps if g.priority == "low"),
        },
        "content_model": {
            "phrasing_only_parents": sorted(PHRASING_CONTENT_PARENTS),
            "flow_elements": sorted(FLOW_CONTENT_ELEMENTS),
            "phrasing_elements": sorted(PHRASING_CONTENT_ELEMENTS),
            "void_elements": sorted(VOID_ELEMENTS),
            "restricted_children": {
                k: sorted(v) for k, v in sorted(RESTRICTED_CHILDREN.items())
            },
        },
        "gaps": [asdict(g) for g in gaps],
        "known_checks": {
            k: v for k, v in sorted(KNOWN_CHECKS.items())
        },
    }
    print(json.dumps(output, indent=2))


# ---------------------------------------------------------------------------
# Test fixture generation
# ---------------------------------------------------------------------------

XHTML_TEMPLATE = """\
<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"
      xml:lang="en" lang="en">
<head><title>Test: {test_id}</title></head>
<body>
{body}
</body>
</html>"""

# Fixtures for block-in-inline violations (phrasing-only parents)
BLOCK_IN_INLINE_FIXTURES = {
    "p-contains-div": (
        "p",
        '  <p>Text before <div>block inside paragraph</div> text after</p>',
        "<p> must not contain <div> (block in inline)",
    ),
    "p-contains-ul": (
        "p",
        '  <p>Text before <ul><li>list inside paragraph</li></ul></p>',
        "<p> must not contain <ul> (block in inline)",
    ),
    "p-contains-table": (
        "p",
        '  <p>Text before <table><tr><td>table inside paragraph</td></tr></table></p>',
        "<p> must not contain <table> (block in inline)",
    ),
    "h1-contains-div": (
        "h1",
        '  <h1><div>block inside heading</div></h1>',
        "<h1> must not contain <div> (block in heading)",
    ),
    "span-contains-div": (
        "span",
        '  <p><span><div>block inside span</div></span></p>',
        "<span> must not contain <div> (block in inline)",
    ),
    "span-contains-p": (
        "span",
        '  <p><span><p>paragraph inside span</p></span></p>',
        "<span> must not contain <p> (block in inline)",
    ),
    "em-contains-div": (
        "em",
        '  <p><em><div>block inside emphasis</div></em></p>',
        "<em> must not contain <div> (block in inline)",
    ),
    "strong-contains-div": (
        "strong",
        '  <p><strong><div>block inside strong</div></strong></p>',
        "<strong> must not contain <div> (block in inline)",
    ),
}

# Fixtures for restricted children violations
RESTRICTED_CHILDREN_FIXTURES = {
    "ul-contains-div": (
        "ul",
        '  <ul><div>div inside ul instead of li</div></ul>',
        "<ul> can only contain <li> elements",
    ),
    "ol-contains-p": (
        "ol",
        '  <ol><p>paragraph inside ol instead of li</p></ol>',
        "<ol> can only contain <li> elements",
    ),
    "tr-contains-div": (
        "tr",
        '  <table><tr><div>div inside tr instead of td/th</div></tr></table>',
        "<tr> can only contain <td> or <th>",
    ),
    "select-contains-div": (
        "select",
        '  <form><select><div>div inside select</div></select></form>',
        "<select> can only contain <option> or <optgroup>",
    ),
    "table-contains-p": (
        "table",
        '  <table><p>paragraph directly in table</p></table>',
        "<table> can only contain caption, colgroup, thead, tbody, tfoot, tr",
    ),
}

# Fixtures for void element violations
VOID_ELEMENT_FIXTURES = {
    "br-with-content": (
        "br",
        '  <p>text <br>content inside br</br> more text</p>',
        "<br> is a void element and cannot have children",
    ),
    "hr-with-content": (
        "hr",
        '  <hr>content inside hr</hr>',
        "<hr> is a void element and cannot have children",
    ),
    "img-with-content": (
        "img",
        '  <p><img src="test.png" alt="test">content inside img</img></p>',
        "<img> is a void element and cannot have children",
    ),
}


def generate_test_fixtures(gaps: list[GapItem], gaps_dir: Path) -> list[Path]:
    """Generate test fixture files for identified gaps."""
    xhtml_dir = gaps_dir / "xhtml"
    xhtml_dir.mkdir(parents=True, exist_ok=True)

    generated = []

    # Generate block-in-inline fixtures
    for test_id, (parent, body, desc) in BLOCK_IN_INLINE_FIXTURES.items():
        content = XHTML_TEMPLATE.format(test_id=test_id, body=body)
        filepath = xhtml_dir / f"rng-{test_id}.xhtml"
        filepath.write_text(content, encoding="utf-8")
        generated.append(filepath)

    # Generate restricted children fixtures
    for test_id, (parent, body, desc) in RESTRICTED_CHILDREN_FIXTURES.items():
        content = XHTML_TEMPLATE.format(test_id=test_id, body=body)
        filepath = xhtml_dir / f"rng-{test_id}.xhtml"
        filepath.write_text(content, encoding="utf-8")
        generated.append(filepath)

    # Generate void element fixtures
    for test_id, (parent, body, desc) in VOID_ELEMENT_FIXTURES.items():
        content = XHTML_TEMPLATE.format(test_id=test_id, body=body)
        filepath = xhtml_dir / f"rng-{test_id}.xhtml"
        filepath.write_text(content, encoding="utf-8")
        generated.append(filepath)

    return generated


def generate_feature_snippet(gaps: list[GapItem]) -> str:
    """Generate Gherkin scenario outlines for the gap fixtures."""
    lines = [
        "# Auto-generated scenarios for RelaxNG schema gaps not yet implemented.",
        "# These test the HTML5 content model rules enforced by epubcheck's",
        "# RelaxNG schemas (Tier 1 validation).",
        "# Generated by: python3 scripts/relaxng-audit.py --generate-tests",
        "",
        "# --- Block-in-Inline Violations (Phrasing-Only Parents) ---",
        "",
    ]

    for test_id, (parent, body, desc) in BLOCK_IN_INLINE_FIXTURES.items():
        lines.append(f"  @relaxng @pending")
        lines.append(f"  Scenario: [{test_id}] {desc}")
        lines.append(f"    When checking document 'rng-{test_id}.xhtml'")
        lines.append(f"    Then error RSC-005 is reported")
        lines.append(f"")

    lines.append("# --- Restricted Children Violations ---")
    lines.append("")

    for test_id, (parent, body, desc) in RESTRICTED_CHILDREN_FIXTURES.items():
        lines.append(f"  @relaxng @pending")
        lines.append(f"  Scenario: [{test_id}] {desc}")
        lines.append(f"    When checking document 'rng-{test_id}.xhtml'")
        lines.append(f"    Then error RSC-005 is reported")
        lines.append(f"")

    lines.append("# --- Void Element Violations ---")
    lines.append("")

    for test_id, (parent, body, desc) in VOID_ELEMENT_FIXTURES.items():
        lines.append(f"  @relaxng @pending")
        lines.append(f"  Scenario: [{test_id}] {desc}")
        lines.append(f"    When checking document 'rng-{test_id}.xhtml'")
        lines.append(f"    Then error RSC-005 is reported")
        lines.append(f"")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Epubcheck repo management
# ---------------------------------------------------------------------------

def ensure_epubcheck(repo_root: Path) -> Path:
    """Clone or update the epubcheck repo; return path to schema dir."""
    cache = repo_root / CACHE_DIR
    if cache.exists():
        print(f"Using cached epubcheck in {cache}", file=sys.stderr)
        result = subprocess.run(
            ["git", "log", "-1", "--format=%H %s"],
            cwd=cache, capture_output=True, text=True,
        )
        print(f"epubcheck commit: {result.stdout.strip()}", file=sys.stderr)
    else:
        print(f"Cloning epubcheck into {cache} ...", file=sys.stderr)
        cache.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run(
            ["git", "clone", "--depth", "1", "-b", EPUBCHECK_BRANCH,
             EPUBCHECK_REPO, str(cache)],
            capture_output=True, check=True,
        )
    schema_dir = cache / SCHEMA_BASE
    if not schema_dir.exists():
        print(f"ERROR: schema directory not found at {schema_dir}", file=sys.stderr)
        sys.exit(1)
    return schema_dir


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Audit epubverify coverage against epubcheck RelaxNG schemas (Tier 1)",
    )
    parser.add_argument(
        "--generate-tests",
        action="store_true",
        help="Generate XHTML test fixtures for identified gaps",
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
        help="Directory for generated test fixtures (default: testdata/fixtures/relaxng-gaps/)",
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
        schema_dir = repo_root / CACHE_DIR / SCHEMA_BASE
        if not schema_dir.exists():
            print("ERROR: no cached epubcheck. Run without --skip-update first.", file=sys.stderr)
            sys.exit(1)
    else:
        schema_dir = ensure_epubcheck(repo_root)

    # Parse RelaxNG schema files
    all_files = HTML5_MODULES + EPUB_MODULES + OTHER_SCHEMAS
    print(f"Parsing {len(all_files)} RelaxNG schema files from {schema_dir.relative_to(repo_root)} ...",
          file=sys.stderr)

    elements, content_rules, raw_patterns = parse_rnc_files(schema_dir, all_files)
    print(f"Found {len(elements)} element definitions, {len(content_rules)} content rules, "
          f"{len(raw_patterns)} patterns", file=sys.stderr)

    # Run gap analysis
    gaps = analyze_gaps(elements, content_rules)
    gaps.sort(key=lambda g: {"high": 0, "medium": 1, "low": 2}[g.priority])

    # Output report
    if args.json:
        print_json_report(elements, content_rules, gaps)
    else:
        print_text_report(elements, content_rules, gaps)

    # Generate test fixtures if requested
    if args.generate_tests:
        gaps_dir = args.output_dir or (
            repo_root / "testdata" / "fixtures" / "relaxng-gaps"
        )
        print(f"\nGenerating test fixtures in {gaps_dir.relative_to(repo_root)}/ ...", file=sys.stderr)
        generated = generate_test_fixtures(gaps, gaps_dir)
        print(f"Generated {len(generated)} test fixture files:", file=sys.stderr)
        for p in generated:
            print(f"  {p.relative_to(repo_root)}", file=sys.stderr)

        # Generate feature file snippet
        snippet = generate_feature_snippet(gaps)
        snippet_path = gaps_dir / "rng-generated-scenarios.feature.txt"
        snippet_path.write_text(snippet, encoding="utf-8")
        print(f"  {snippet_path.relative_to(repo_root)} (feature file snippet)", file=sys.stderr)

    # Exit with non-zero if there are high-priority gaps
    high_gaps = [g for g in gaps if g.priority == "high"]
    if high_gaps:
        sys.exit(2)


if __name__ == "__main__":
    main()
