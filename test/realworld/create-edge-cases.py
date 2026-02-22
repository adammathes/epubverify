#!/usr/bin/env python3
"""
Generate synthetic EPUB 3 files designed to exercise edge cases in an EPUB validator.

Each EPUB is structurally valid per the EPUB 3 specification.
"""

import os
import struct
import zipfile
import uuid
from datetime import datetime, timezone

SAMPLES_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "synthetic", "samples")

MIMETYPE = "application/epub+zip"

CONTAINER_XML = """\
<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="{opf_path}" media-type="application/oebps-package+xml" />
  </rootfiles>
</container>"""

MODIFIED_TS = "2024-01-15T12:00:00Z"


def make_xhtml(title, body_html):
    """Create a minimal valid XHTML content document."""
    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <head>
    <title>{title}</title>
  </head>
  <body>
    {body_html}
  </body>
</html>"""


def make_xhtml_with_meta(title, body_html, head_extra=""):
    """Create an XHTML content document with extra head content."""
    return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <head>
    <title>{title}</title>
    {head_extra}
  </head>
  <body>
    {body_html}
  </body>
</html>"""


def create_epub(filename, opf_path, files):
    """
    Create an EPUB file from a dict of {archive_path: content_bytes_or_str}.

    Ensures mimetype is first entry and stored uncompressed.
    opf_path is the path to the OPF inside the archive (for container.xml).
    """
    filepath = os.path.join(SAMPLES_DIR, filename)
    os.makedirs(SAMPLES_DIR, exist_ok=True)

    with zipfile.ZipFile(filepath, "w", zipfile.ZIP_DEFLATED) as zf:
        # 1. mimetype must be first, uncompressed, no extra field
        mi = zipfile.ZipInfo("mimetype")
        mi.compress_type = zipfile.ZIP_STORED
        mi.external_attr = 0
        zf.writestr(mi, MIMETYPE)

        # 2. META-INF/container.xml
        container = CONTAINER_XML.format(opf_path=opf_path)
        zf.writestr("META-INF/container.xml", container)

        # 3. All other files
        for path, content in files.items():
            if isinstance(content, str):
                content = content.encode("utf-8")
            zf.writestr(path, content)

    print(f"  Created: {filepath}")
    return filepath


def create_epub_with_encryption(filename, opf_path, files, encryption_xml):
    """
    Create an EPUB file with a META-INF/encryption.xml.
    """
    filepath = os.path.join(SAMPLES_DIR, filename)
    os.makedirs(SAMPLES_DIR, exist_ok=True)

    with zipfile.ZipFile(filepath, "w", zipfile.ZIP_DEFLATED) as zf:
        # 1. mimetype
        mi = zipfile.ZipInfo("mimetype")
        mi.compress_type = zipfile.ZIP_STORED
        mi.external_attr = 0
        zf.writestr(mi, MIMETYPE)

        # 2. META-INF/container.xml
        container = CONTAINER_XML.format(opf_path=opf_path)
        zf.writestr("META-INF/container.xml", container)

        # 3. META-INF/encryption.xml
        zf.writestr("META-INF/encryption.xml", encryption_xml)

        # 4. All other files
        for path, content in files.items():
            if isinstance(content, str):
                content = content.encode("utf-8")
            zf.writestr(path, content)

    print(f"  Created: {filepath}")
    return filepath


def create_epub_raw(filename, opf_path, files, extra_meta_inf=None):
    """
    Create an EPUB with precise control over filenames (for percent-encoded test).
    Uses decoded filenames in the ZIP but percent-encoded references in OPF.
    """
    filepath = os.path.join(SAMPLES_DIR, filename)
    os.makedirs(SAMPLES_DIR, exist_ok=True)

    with zipfile.ZipFile(filepath, "w", zipfile.ZIP_DEFLATED) as zf:
        # 1. mimetype
        mi = zipfile.ZipInfo("mimetype")
        mi.compress_type = zipfile.ZIP_STORED
        mi.external_attr = 0
        zf.writestr(mi, MIMETYPE)

        # 2. META-INF/container.xml
        container = CONTAINER_XML.format(opf_path=opf_path)
        zf.writestr("META-INF/container.xml", container)

        # 3. Extra META-INF files
        if extra_meta_inf:
            for path, content in extra_meta_inf.items():
                if isinstance(content, str):
                    content = content.encode("utf-8")
                zf.writestr(path, content)

        # 4. All other files
        for path, content in files.items():
            if isinstance(content, str):
                content = content.encode("utf-8")
            zf.writestr(path, content)

    print(f"  Created: {filepath}")
    return filepath


# ---------------------------------------------------------------------------
# 1. edge-deep-fallback.epub
# ---------------------------------------------------------------------------
def create_deep_fallback():
    """
    EPUB 3 with a deep fallback chain (6 levels): A -> B -> C -> D -> E -> F.
    Items A-E are application/x-custom (foreign), F is XHTML.
    The spine references item A. Because the fallback chain ultimately
    reaches a Core Media Type (XHTML), this is valid per EPUB 3.
    """
    uid = f"urn:uuid:{uuid.uuid4()}"

    # The final fallback is a real XHTML document
    fallback_xhtml = make_xhtml("Deep Fallback Content", """\
    <h1>Deep Fallback Content</h1>
    <p>This content is reached after traversing a 5-level fallback chain.</p>
    <p>Item A (custom) -> B (custom) -> C (custom) -> D (custom) -> E (custom) -> F (this XHTML).</p>""")

    nav_xhtml = make_xhtml("Navigation", """\
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="fallback-a.xml">Deep Fallback Content</a></li>
      </ol>
    </nav>""")

    # Custom data files for the fallback chain items A-E
    custom_data = "<data>custom content level {}</data>"

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: Deep Fallback Chain</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="item-a" href="fallback-a.xml" media-type="application/xml" fallback="item-b" />
    <item id="item-b" href="fallback-b.xml" media-type="application/xml" fallback="item-c" />
    <item id="item-c" href="fallback-c.xml" media-type="application/xml" fallback="item-d" />
    <item id="item-d" href="fallback-d.xml" media-type="application/xml" fallback="item-e" />
    <item id="item-e" href="fallback-e.xml" media-type="application/xml" fallback="item-f" />
    <item id="item-f" href="fallback-f.xhtml" media-type="application/xhtml+xml" />
  </manifest>
  <spine>
    <itemref idref="item-a" />
  </spine>
</package>"""

    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/fallback-a.xml": custom_data.format("A"),
        "EPUB/fallback-b.xml": custom_data.format("B"),
        "EPUB/fallback-c.xml": custom_data.format("C"),
        "EPUB/fallback-d.xml": custom_data.format("D"),
        "EPUB/fallback-e.xml": custom_data.format("E"),
        "EPUB/fallback-f.xhtml": fallback_xhtml,
    }

    create_epub("edge-deep-fallback.epub", "EPUB/package.opf", files)


# ---------------------------------------------------------------------------
# 2. edge-fxl-mixed.epub
# ---------------------------------------------------------------------------
def create_fxl_mixed():
    """
    Fixed-layout EPUB 3 with per-spine-item rendition overrides.
    Global default is pre-paginated. Some items override to reflowable.
    Uses rendition:spread and rendition:orientation properties.
    """
    uid = f"urn:uuid:{uuid.uuid4()}"

    def fxl_page(title, body, width=1024, height=768):
        return f"""\
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <head>
    <title>{title}</title>
    <meta name="viewport" content="width={width}, height={height}" />
    <style>
      body {{ margin: 0; padding: 20px; font-family: serif; }}
      h1 {{ color: #333; }}
    </style>
  </head>
  <body>
    {body}
  </body>
</html>"""

    def reflow_page(title, body):
        return make_xhtml(title, body)

    cover = fxl_page("Cover", """\
    <div style="text-align:center;">
      <h1>Edge Case: FXL Mixed Layout</h1>
      <p>A fixed-layout EPUB with rendition overrides</p>
    </div>""")

    page1 = fxl_page("Page 1 - Fixed", """\
    <h1>Page 1 (Fixed Layout)</h1>
    <p>This page is pre-paginated with a viewport of 1024x768.</p>""")

    page2 = fxl_page("Page 2 - Fixed Landscape", """\
    <h1>Page 2 (Fixed Layout, Landscape)</h1>
    <p>This page forces landscape orientation.</p>""", 1200, 800)

    page3_reflow = reflow_page("Page 3 - Reflowable Override", """\
    <h1>Page 3 (Reflowable Override)</h1>
    <p>This page overrides the global pre-paginated setting to be reflowable.</p>
    <p>It should reflow text normally without a fixed viewport.</p>
    <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod
    tempor incididunt ut labore et dolore magna aliqua.</p>""")

    page4 = fxl_page("Page 4 - Fixed Again", """\
    <h1>Page 4 (Fixed Layout)</h1>
    <p>Back to fixed layout after the reflowable override.</p>""")

    nav_xhtml = make_xhtml("Navigation", """\
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="cover.xhtml">Cover</a></li>
        <li><a href="page1.xhtml">Page 1 - Fixed</a></li>
        <li><a href="page2.xhtml">Page 2 - Fixed Landscape</a></li>
        <li><a href="page3.xhtml">Page 3 - Reflowable</a></li>
        <li><a href="page4.xhtml">Page 4 - Fixed</a></li>
      </ol>
    </nav>""")

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf"
         prefix="rendition: http://www.idpf.org/vocab/rendition/#">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: FXL Mixed Layout</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
    <meta property="rendition:layout">pre-paginated</meta>
    <meta property="rendition:orientation">auto</meta>
    <meta property="rendition:spread">auto</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="cover" href="cover.xhtml" media-type="application/xhtml+xml" />
    <item id="page1" href="page1.xhtml" media-type="application/xhtml+xml" />
    <item id="page2" href="page2.xhtml" media-type="application/xhtml+xml" />
    <item id="page3" href="page3.xhtml" media-type="application/xhtml+xml" />
    <item id="page4" href="page4.xhtml" media-type="application/xhtml+xml" />
  </manifest>
  <spine>
    <itemref idref="cover" properties="rendition:layout-pre-paginated rendition:spread-none" />
    <itemref idref="page1" properties="rendition:layout-pre-paginated rendition:orientation-portrait rendition:spread-landscape" />
    <itemref idref="page2" properties="rendition:layout-pre-paginated rendition:orientation-landscape rendition:spread-none" />
    <itemref idref="page3" properties="rendition:layout-reflowable rendition:spread-auto" />
    <itemref idref="page4" properties="rendition:layout-pre-paginated rendition:orientation-auto rendition:spread-both" />
  </spine>
</package>"""

    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/cover.xhtml": cover,
        "EPUB/page1.xhtml": page1,
        "EPUB/page2.xhtml": page2,
        "EPUB/page3.xhtml": page3_reflow,
        "EPUB/page4.xhtml": page4,
    }

    create_epub("edge-fxl-mixed.epub", "EPUB/package.opf", files)


# ---------------------------------------------------------------------------
# 3. edge-multi-nav.epub
# ---------------------------------------------------------------------------
def create_multi_nav():
    """
    EPUB 3 navigation document with ALL nav types: toc, landmarks, and page-list.
    page-list references multiple locations; landmarks includes bodymatter, toc, cover.
    """
    uid = f"urn:uuid:{uuid.uuid4()}"

    cover_xhtml = make_xhtml("Cover", """\
    <section epub:type="cover">
      <h1>Edge Case: Multi-Nav</h1>
      <p>A book with all navigation types.</p>
    </section>""")

    ch1 = make_xhtml("Chapter 1", """\
    <section epub:type="bodymatter chapter">
      <h1 id="ch1">Chapter 1: Introduction</h1>
      <p id="pg1">This is the beginning of chapter 1.</p>
      <p id="pg2">This is another paragraph on page 2.</p>
      <p id="pg3">Continuing with page 3 content.</p>
    </section>""")

    ch2 = make_xhtml("Chapter 2", """\
    <section epub:type="bodymatter chapter">
      <h1 id="ch2">Chapter 2: Development</h1>
      <p id="pg4">Chapter 2 begins here on page 4.</p>
      <p id="pg5">More content on page 5.</p>
    </section>""")

    ch3 = make_xhtml("Chapter 3", """\
    <section epub:type="bodymatter chapter">
      <h1 id="ch3">Chapter 3: Conclusion</h1>
      <p id="pg6">The final chapter starts on page 6.</p>
      <p id="pg7">Page 7 wraps things up.</p>
      <p id="pg8">The end, on page 8.</p>
    </section>""")

    nav_xhtml = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <head>
    <title>Navigation</title>
  </head>
  <body>
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="cover.xhtml">Cover</a></li>
        <li><a href="chapter1.xhtml#ch1">Chapter 1: Introduction</a></li>
        <li><a href="chapter2.xhtml#ch2">Chapter 2: Development</a></li>
        <li><a href="chapter3.xhtml#ch3">Chapter 3: Conclusion</a></li>
      </ol>
    </nav>

    <nav epub:type="landmarks">
      <h2>Landmarks</h2>
      <ol>
        <li><a epub:type="cover" href="cover.xhtml">Cover</a></li>
        <li><a epub:type="bodymatter" href="chapter1.xhtml">Start of Content</a></li>
      </ol>
    </nav>

    <nav epub:type="page-list">
      <h2>Page List</h2>
      <ol>
        <li><a href="chapter1.xhtml#pg1">1</a></li>
        <li><a href="chapter1.xhtml#pg2">2</a></li>
        <li><a href="chapter1.xhtml#pg3">3</a></li>
        <li><a href="chapter2.xhtml#pg4">4</a></li>
        <li><a href="chapter2.xhtml#pg5">5</a></li>
        <li><a href="chapter3.xhtml#pg6">6</a></li>
        <li><a href="chapter3.xhtml#pg7">7</a></li>
        <li><a href="chapter3.xhtml#pg8">8</a></li>
      </ol>
    </nav>
  </body>
</html>"""

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: Multi-Nav Types</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="cover" href="cover.xhtml" media-type="application/xhtml+xml" />
    <item id="ch1" href="chapter1.xhtml" media-type="application/xhtml+xml" />
    <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml" />
    <item id="ch3" href="chapter3.xhtml" media-type="application/xhtml+xml" />
  </manifest>
  <spine>
    <itemref idref="cover" />
    <itemref idref="ch1" />
    <itemref idref="ch2" />
    <itemref idref="ch3" />
  </spine>
</package>"""

    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/cover.xhtml": cover_xhtml,
        "EPUB/chapter1.xhtml": ch1,
        "EPUB/chapter2.xhtml": ch2,
        "EPUB/chapter3.xhtml": ch3,
    }

    create_epub("edge-multi-nav.epub", "EPUB/package.opf", files)


# ---------------------------------------------------------------------------
# 4. edge-deep-paths.epub
# ---------------------------------------------------------------------------
def create_deep_paths():
    """
    EPUB 3 with deeply nested directory structure and cross-references
    using relative paths.
    """
    uid = f"urn:uuid:{uuid.uuid4()}"

    deep_content = make_xhtml("Deep Content", """\
    <h1>Deeply Nested Content</h1>
    <p>This file lives at EPUB/a/b/c/d/content.xhtml</p>
    <p>Here is a link to the <a href="../../../../x/y/z/other.xhtml">other document</a>
       which lives at EPUB/x/y/z/other.xhtml.</p>
    <p>And a link to <a href="../../../../m/n/middle.xhtml">middle document</a>.</p>""")

    other_content = make_xhtml("Other Content", """\
    <h1>Other Nested Content</h1>
    <p>This file lives at EPUB/x/y/z/other.xhtml</p>
    <p>Here is a link back to the <a href="../../../a/b/c/d/content.xhtml">deep content</a>.</p>
    <p>And a link to <a href="../../../m/n/middle.xhtml">middle document</a>.</p>""")

    middle_content = make_xhtml("Middle Content", """\
    <h1>Middle Nested Content</h1>
    <p>This file lives at EPUB/m/n/middle.xhtml</p>
    <p>Link to <a href="../../a/b/c/d/content.xhtml">deep content</a>
       (going up to EPUB then into a/b/c/d/).</p>
    <p>Link to <a href="../../x/y/z/other.xhtml">other content</a>.</p>""")

    nav_xhtml = make_xhtml("Navigation", """\
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="a/b/c/d/content.xhtml">Deep Content (a/b/c/d/)</a></li>
        <li><a href="x/y/z/other.xhtml">Other Content (x/y/z/)</a></li>
        <li><a href="m/n/middle.xhtml">Middle Content (m/n/)</a></li>
      </ol>
    </nav>""")

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: Deep Nested Paths</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="deep" href="a/b/c/d/content.xhtml" media-type="application/xhtml+xml" />
    <item id="other" href="x/y/z/other.xhtml" media-type="application/xhtml+xml" />
    <item id="middle" href="m/n/middle.xhtml" media-type="application/xhtml+xml" />
  </manifest>
  <spine>
    <itemref idref="deep" />
    <itemref idref="other" />
    <itemref idref="middle" />
  </spine>
</package>"""

    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/a/b/c/d/content.xhtml": deep_content,
        "EPUB/x/y/z/other.xhtml": other_content,
        "EPUB/m/n/middle.xhtml": middle_content,
    }

    create_epub("edge-deep-paths.epub", "EPUB/package.opf", files)


# ---------------------------------------------------------------------------
# 5. edge-font-obfuscation.epub
# ---------------------------------------------------------------------------
def create_font_obfuscation():
    """
    EPUB 3 with META-INF/encryption.xml declaring font obfuscation
    using the IDPF algorithm (http://www.idpf.org/2008/embedding).
    Includes a minimal TTF font file (doesn't need to be actually obfuscated,
    just needs valid encryption.xml structure).
    """
    uid_value = str(uuid.uuid4())
    uid = f"urn:uuid:{uid_value}"

    # Create a minimal but structurally valid TrueType font file.
    # This is the smallest valid TTF: just the required tables with minimal data.
    # For testing purposes, we create a tiny binary blob that has the TTF signature.
    # A real font would be much larger, but we just need a file the system
    # recognizes as a font resource.
    def make_minimal_ttf():
        """Create a minimal TTF file with the basic required structure."""
        # TrueType header: sfVersion(4) + numTables(2) + searchRange(2) +
        #                   entrySelector(2) + rangeShift(2) = 12 bytes
        # We'll include 4 required tables: cmap, glyf, head, hhea, hmtx, loca, maxp, name, post
        # For simplicity, just create a file with correct magic number
        # Real validators check the encryption.xml, not font validity
        num_tables = 9
        search_range = 128
        entry_selector = 3
        range_shift = num_tables * 16 - search_range

        header = struct.pack(">IHHHH",
            0x00010000,  # sfVersion - TrueType
            num_tables,
            search_range,
            entry_selector,
            range_shift
        )
        # Pad to make it look like a font file (512 bytes minimum)
        return header + b'\x00' * 500

    # Also create a minimal WOFF font
    def make_minimal_woff():
        """Create a minimal WOFF file."""
        # WOFF header signature
        signature = b'wOFF'
        # flavor (TrueType)
        flavor = struct.pack(">I", 0x00010000)
        # length - just the header for now
        length = struct.pack(">I", 44)
        # numTables
        num_tables = struct.pack(">H", 0)
        # reserved
        reserved = struct.pack(">H", 0)
        # totalSfntSize
        total_sfnt = struct.pack(">I", 0)
        # majorVersion, minorVersion
        version = struct.pack(">HH", 1, 0)
        # metaOffset, metaLength, metaOrigLength
        meta = struct.pack(">III", 0, 0, 0)
        # privOffset, privLength
        priv = struct.pack(">II", 0, 0)
        return signature + flavor + length + num_tables + reserved + total_sfnt + version + meta + priv

    ttf_data = make_minimal_ttf()
    woff_data = make_minimal_woff()

    content_xhtml = make_xhtml_with_meta("Font Obfuscation Test",
        """\
    <h1>Font Obfuscation Test</h1>
    <p class="custom-font">This text would use the obfuscated custom font.</p>
    <p class="woff-font">This text would use the obfuscated WOFF font.</p>
    <p>The font files are declared as obfuscated in META-INF/encryption.xml
       using the IDPF obfuscation algorithm.</p>""",
        '<link rel="stylesheet" href="styles.css" type="text/css" />')

    nav_xhtml = make_xhtml("Navigation", """\
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="content.xhtml">Font Obfuscation Test</a></li>
      </ol>
    </nav>""")

    css = """\
@font-face {
    font-family: "CustomFont";
    src: url("fonts/custom-regular.ttf");
    font-weight: normal;
    font-style: normal;
}

@font-face {
    font-family: "WoffFont";
    src: url("fonts/custom-bold.woff");
    font-weight: bold;
    font-style: normal;
}

.custom-font {
    font-family: "CustomFont", serif;
}

.woff-font {
    font-family: "WoffFont", sans-serif;
    font-weight: bold;
}"""

    encryption_xml = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <EncryptedData xmlns="http://www.w3.org/2001/04/xmlenc#">
        <EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/>
        <CipherData>
            <CipherReference URI="EPUB/fonts/custom-regular.ttf"/>
        </CipherData>
    </EncryptedData>
    <EncryptedData xmlns="http://www.w3.org/2001/04/xmlenc#">
        <EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/>
        <CipherData>
            <CipherReference URI="EPUB/fonts/custom-bold.woff"/>
        </CipherData>
    </EncryptedData>
</encryption>"""

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: Font Obfuscation</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="content" href="content.xhtml" media-type="application/xhtml+xml" />
    <item id="css" href="styles.css" media-type="text/css" />
    <item id="font-ttf" href="fonts/custom-regular.ttf" media-type="application/font-sfnt" />
    <item id="font-woff" href="fonts/custom-bold.woff" media-type="application/font-woff" />
  </manifest>
  <spine>
    <itemref idref="content" />
  </spine>
</package>"""

    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/content.xhtml": content_xhtml,
        "EPUB/styles.css": css,
        "EPUB/fonts/custom-regular.ttf": ttf_data,
        "EPUB/fonts/custom-bold.woff": woff_data,
    }

    create_epub_with_encryption("edge-font-obfuscation.epub", "EPUB/package.opf",
                                 files, encryption_xml)


# ---------------------------------------------------------------------------
# 6. edge-smil-overlay.epub
# ---------------------------------------------------------------------------
def create_smil_overlay():
    """
    EPUB 3 with SMIL media overlays. Includes a .smil file with par/seq structures,
    media:duration and media:active-class in OPF, and a small silent audio file.
    """
    uid = f"urn:uuid:{uuid.uuid4()}"

    # Create a minimal valid MP3 file (silence).
    # A valid MP3 frame: sync word (0xFFE0+), followed by frame header and padding.
    # Simplest approach: MPEG1 Layer3, 128kbps, 44100Hz, mono, no padding
    def make_silent_mp3():
        """Create a minimal silent MP3 file (one frame of silence)."""
        # MP3 frame header for MPEG1, Layer 3, 128kbps, 44100Hz, stereo
        # FF FB 90 00 = sync(12) + version(2:MPEG1) + layer(2:III) + protection(1:none)
        #              + bitrate(4:128k) + samplerate(2:44100) + padding(1:0) + private(1:0)
        #              + channelmode(2:stereo) + ...
        header = bytes([0xFF, 0xFB, 0x90, 0x00])
        # Frame size for 128kbps, 44100Hz = 417 bytes (including header)
        # Fill the rest with zeros (silence)
        frame_size = 417
        frame = header + b'\x00' * (frame_size - len(header))
        # Repeat a few frames to make a ~50ms audio clip
        return frame * 5

    audio_data = make_silent_mp3()

    content_xhtml = make_xhtml_with_meta("Media Overlay Chapter", """\
    <h1 id="heading1">Chapter with Media Overlay</h1>
    <p id="para1">This is the first paragraph that has an audio overlay.</p>
    <p id="para2">This is the second paragraph with synchronized audio.</p>
    <p id="para3">The third paragraph continues the narration.</p>
    <p id="para4">And this final paragraph completes the overlay test.</p>""",
        '<link rel="stylesheet" href="overlay.css" type="text/css" />')

    intro_xhtml = make_xhtml("Introduction", """\
    <h1>Introduction</h1>
    <p>This EPUB tests SMIL media overlays.</p>""")

    nav_xhtml = make_xhtml("Navigation", """\
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="intro.xhtml">Introduction</a></li>
        <li><a href="chapter.xhtml">Chapter with Media Overlay</a></li>
      </ol>
    </nav>""")

    smil = """\
<?xml version="1.0" encoding="UTF-8"?>
<smil xmlns="http://www.w3.org/ns/SMIL" xmlns:epub="http://www.idpf.org/2007/ops" version="3.0">
    <body>
        <seq id="seq1" epub:textref="chapter.xhtml" epub:type="bodymatter chapter">
            <par id="par-heading">
                <text src="chapter.xhtml#heading1"/>
                <audio src="audio/silence.mp3" clipBegin="0:00:00.000" clipEnd="0:00:02.000"/>
            </par>
            <par id="par-p1">
                <text src="chapter.xhtml#para1"/>
                <audio src="audio/silence.mp3" clipBegin="0:00:02.000" clipEnd="0:00:05.000"/>
            </par>
            <par id="par-p2">
                <text src="chapter.xhtml#para2"/>
                <audio src="audio/silence.mp3" clipBegin="0:00:05.000" clipEnd="0:00:08.000"/>
            </par>
            <seq id="seq-nested" epub:textref="chapter.xhtml">
                <par id="par-p3">
                    <text src="chapter.xhtml#para3"/>
                    <audio src="audio/silence.mp3" clipBegin="0:00:08.000" clipEnd="0:00:11.000"/>
                </par>
                <par id="par-p4">
                    <text src="chapter.xhtml#para4"/>
                    <audio src="audio/silence.mp3" clipBegin="0:00:11.000" clipEnd="0:00:14.000"/>
                </par>
            </seq>
        </seq>
    </body>
</smil>"""

    overlay_css = """\
.-epub-media-overlay-active {
    background-color: #ffff00;
    color: #000000;
}"""

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: SMIL Media Overlays</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
    <meta property="media:duration" refines="#chapter-overlay">0:00:14.000</meta>
    <meta property="media:duration">0:00:14.000</meta>
    <meta property="media:narrator">Synthetic Narrator</meta>
    <meta property="media:active-class">-epub-media-overlay-active</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="intro" href="intro.xhtml" media-type="application/xhtml+xml" />
    <item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml" media-overlay="chapter-overlay" />
    <item id="chapter-overlay" href="chapter-overlay.smil" media-type="application/smil+xml" />
    <item id="audio-silence" href="audio/silence.mp3" media-type="audio/mpeg" />
    <item id="overlay-css" href="overlay.css" media-type="text/css" />
  </manifest>
  <spine>
    <itemref idref="intro" />
    <itemref idref="chapter" />
  </spine>
</package>"""

    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/intro.xhtml": intro_xhtml,
        "EPUB/chapter.xhtml": content_xhtml,
        "EPUB/chapter-overlay.smil": smil,
        "EPUB/audio/silence.mp3": audio_data,
        "EPUB/overlay.css": overlay_css,
    }

    create_epub("edge-smil-overlay.epub", "EPUB/package.opf", files)


# ---------------------------------------------------------------------------
# 7. edge-complex-css.epub
# ---------------------------------------------------------------------------
def create_complex_css():
    """
    EPUB 3 with CSS containing @media queries, @font-face rules,
    CSS custom properties (variables), complex selectors, pseudo-elements,
    and attribute selectors.
    """
    uid = f"urn:uuid:{uuid.uuid4()}"

    css = """\
/* CSS Custom Properties (Variables) */
:root {
    --primary-color: #2c3e50;
    --secondary-color: #e74c3c;
    --accent-color: #3498db;
    --font-size-base: 16px;
    --font-size-large: calc(var(--font-size-base) * 1.5);
    --line-height: 1.6;
    --spacing-unit: 8px;
    --border-radius: 4px;
}

/* @font-face rules */
@font-face {
    font-family: "CustomSerif";
    src: local("Georgia"), local("Times New Roman");
    font-weight: normal;
    font-style: normal;
    font-display: swap;
}

@font-face {
    font-family: "CustomSerif";
    src: local("Georgia Bold"), local("Times New Roman Bold");
    font-weight: bold;
    font-style: normal;
    font-display: swap;
}

/* Base styles */
body {
    font-family: "CustomSerif", Georgia, "Times New Roman", serif;
    font-size: var(--font-size-base);
    line-height: var(--line-height);
    color: var(--primary-color);
    margin: calc(var(--spacing-unit) * 3);
    padding: 0;
}

/* Complex selectors - multiple selectors per line */
h1, h2, h3, h4, h5, h6 { font-family: "CustomSerif", serif; margin-top: calc(var(--spacing-unit) * 4); }

/* Attribute selectors */
a[href] { color: var(--accent-color); }
a[href^="http"] { text-decoration: underline; }
a[href$=".pdf"] { color: var(--secondary-color); }
a[href*="chapter"] { font-weight: bold; }
input[type="text"] { border: 1px solid var(--primary-color); }
[lang|="en"] { quotes: '"' '"' "'" "'"; }
[data-type~="important"] { background-color: #ffffcc; }

/* Pseudo-elements */
p::first-line {
    font-variant: small-caps;
    color: var(--secondary-color);
}

p::first-letter {
    font-size: var(--font-size-large);
    float: left;
    margin-right: calc(var(--spacing-unit) / 2);
    color: var(--accent-color);
    font-weight: bold;
}

blockquote::before {
    content: open-quote;
    font-size: 2em;
    color: var(--secondary-color);
}

blockquote::after {
    content: close-quote;
    font-size: 2em;
    color: var(--secondary-color);
}

/* Pseudo-classes */
p:first-child { margin-top: 0; }
p:last-child { margin-bottom: 0; }
p:nth-child(odd) { background-color: rgba(0, 0, 0, 0.02); }
p:nth-of-type(3n+1) { border-left: 3px solid var(--accent-color); padding-left: var(--spacing-unit); }
li:not(:last-child) { margin-bottom: calc(var(--spacing-unit) / 2); }
a:hover, a:focus { color: var(--secondary-color); outline: 2px solid var(--accent-color); }
a:visited { color: #7b4f8a; }

/* Complex combinators */
section > h1 + p { font-size: 1.1em; font-style: italic; }
section > h1 ~ p { text-indent: 1.5em; }
div.container > article > section p { margin: var(--spacing-unit) 0; }
nav ol > li > a { text-decoration: none; }

/* @media queries */
@media screen and (max-width: 600px) {
    body {
        font-size: 14px;
        margin: var(--spacing-unit);
    }

    h1 { font-size: 1.5em; }
    h2 { font-size: 1.3em; }
}

@media screen and (min-width: 601px) and (max-width: 1024px) {
    body {
        font-size: 16px;
        margin: calc(var(--spacing-unit) * 2);
    }
}

@media (prefers-color-scheme: dark) {
    :root {
        --primary-color: #ecf0f1;
        --secondary-color: #e74c3c;
        --accent-color: #5dade2;
    }

    body {
        background-color: #1a1a2e;
    }
}

@media (prefers-reduced-motion: reduce) {
    * {
        transition: none !important;
        animation: none !important;
    }
}

@media print {
    body { font-size: 12pt; color: #000; }
    a[href]::after { content: " (" attr(href) ")"; font-size: 0.8em; }
    nav, .no-print { display: none; }
}

/* CSS Grid and Flexbox */
.grid-layout {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: var(--spacing-unit);
}

.flex-container {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    align-items: center;
}

/* Specificity edge cases */
body > main > section.content > p.intro:first-of-type::first-line {
    font-weight: bold;
    letter-spacing: 0.05em;
}

/* Multiple background and complex values */
.decorated {
    background:
        linear-gradient(135deg, transparent 25%, rgba(0,0,0,0.05) 25%, rgba(0,0,0,0.05) 50%, transparent 50%);
    background-size: 20px 20px;
    border: 1px solid var(--primary-color);
    border-radius: var(--border-radius);
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1), 0 4px 8px rgba(0, 0, 0, 0.05);
}

/* Transitions */
.animated {
    transition: color 0.3s ease-in-out, background-color 0.3s ease-in-out, transform 0.2s ease;
}

.animated:hover {
    transform: translateY(-2px);
}"""

    content_xhtml = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
  <head>
    <title>Complex CSS Test</title>
    <link rel="stylesheet" href="styles/complex.css" type="text/css" />
  </head>
  <body>
    <main>
      <section class="content">
        <h1>Complex CSS Edge Case</h1>
        <p class="intro">This document exercises complex CSS features including custom properties,
        media queries, pseudo-elements, and attribute selectors.</p>
        <p>Second paragraph with <a href="http://example.com">external link</a> and
        <a href="chapter2.xhtml">internal chapter link</a>.</p>
        <p data-type="important">This paragraph has a data attribute selector match.</p>

        <blockquote>
          <p>This is a blockquote with ::before and ::after pseudo-elements
          adding quotation marks.</p>
        </blockquote>

        <div class="grid-layout">
          <div class="decorated">Grid item 1 with complex background.</div>
          <div class="decorated">Grid item 2 with box shadows.</div>
          <div class="decorated">Grid item 3 with border radius.</div>
        </div>

        <div class="flex-container">
          <span class="animated">Hover me for transition</span>
          <span class="animated">Another animated element</span>
        </div>

        <ol>
          <li>First list item</li>
          <li>Second list item (not last, has margin)</li>
          <li>Third list item</li>
        </ol>
      </section>
    </main>
  </body>
</html>"""

    ch2 = make_xhtml("Chapter 2", """\
    <h1>Chapter 2</h1>
    <p>A second chapter for link testing.</p>""")

    nav_xhtml = make_xhtml("Navigation", """\
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="content.xhtml">Complex CSS Test</a></li>
        <li><a href="chapter2.xhtml">Chapter 2</a></li>
      </ol>
    </nav>""")

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: Complex CSS</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="content" href="content.xhtml" media-type="application/xhtml+xml" />
    <item id="ch2" href="chapter2.xhtml" media-type="application/xhtml+xml" />
    <item id="css" href="styles/complex.css" media-type="text/css" />
  </manifest>
  <spine>
    <itemref idref="content" />
    <itemref idref="ch2" />
  </spine>
</package>"""

    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/content.xhtml": content_xhtml,
        "EPUB/chapter2.xhtml": ch2,
        "EPUB/styles/complex.css": css,
    }

    create_epub("edge-complex-css.epub", "EPUB/package.opf", files)


# ---------------------------------------------------------------------------
# 8. edge-percent-encoded.epub
# ---------------------------------------------------------------------------
def create_percent_encoded():
    """
    EPUB 3 where filenames in the manifest contain percent-encoded characters
    (spaces as %20, etc.) but the actual filenames in the ZIP use decoded forms.
    Per the EPUB/OCF spec, IRI references in the OPF are percent-encoded but
    the ZIP entry names use the decoded (literal) forms.
    """
    uid = f"urn:uuid:{uuid.uuid4()}"

    # Files with spaces and special chars in their names
    # The ZIP stores them decoded; the OPF references them percent-encoded
    ch1_content = make_xhtml("Chapter One", """\
    <h1>Chapter One</h1>
    <p>This file has spaces in its name: "chapter one.xhtml".</p>
    <p>The OPF manifest references it as "chapter%20one.xhtml".</p>""")

    ch2_content = make_xhtml("My Chapter (Two)", """\
    <h1>My Chapter (Two)</h1>
    <p>This file has parentheses: "my chapter (two).xhtml".</p>""")

    extra_content = make_xhtml("Extra Content", """\
    <h1>Extra + Content</h1>
    <p>This file has a plus sign in its directory: "content files/extra doc.xhtml".</p>""")

    nav_xhtml = make_xhtml("Navigation", """\
    <nav epub:type="toc">
      <h1>Table of Contents</h1>
      <ol>
        <li><a href="chapter%20one.xhtml">Chapter One</a></li>
        <li><a href="my%20chapter%20%28two%29.xhtml">My Chapter (Two)</a></li>
        <li><a href="content%20files/extra%20doc.xhtml">Extra Content</a></li>
      </ol>
    </nav>""")

    opf = f"""\
<?xml version="1.0" encoding="UTF-8"?>
<package version="3.0" unique-identifier="pub-id" xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="pub-id">{uid}</dc:identifier>
    <dc:title>Edge Case: Percent-Encoded Filenames</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">{MODIFIED_TS}</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav" />
    <item id="ch1" href="chapter%20one.xhtml" media-type="application/xhtml+xml" />
    <item id="ch2" href="my%20chapter%20%28two%29.xhtml" media-type="application/xhtml+xml" />
    <item id="extra" href="content%20files/extra%20doc.xhtml" media-type="application/xhtml+xml" />
  </manifest>
  <spine>
    <itemref idref="ch1" />
    <itemref idref="ch2" />
    <itemref idref="extra" />
  </spine>
</package>"""

    # NOTE: The ZIP entry names use the DECODED forms (with actual spaces/parens).
    # The OPF uses percent-encoded forms. This is the correct behavior per OCF spec.
    files = {
        "EPUB/package.opf": opf,
        "EPUB/nav.xhtml": nav_xhtml,
        "EPUB/chapter one.xhtml": ch1_content,
        "EPUB/my chapter (two).xhtml": ch2_content,
        "EPUB/content files/extra doc.xhtml": extra_content,
    }

    create_epub("edge-percent-encoded.epub", "EPUB/package.opf", files)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
def main():
    os.makedirs(SAMPLES_DIR, exist_ok=True)
    print("Generating edge-case EPUB files...")
    print()

    print("[1/8] edge-deep-fallback.epub - Deep fallback chain (6 levels)")
    create_deep_fallback()
    print()

    print("[2/8] edge-fxl-mixed.epub - Fixed-layout with per-spine rendition overrides")
    create_fxl_mixed()
    print()

    print("[3/8] edge-multi-nav.epub - All nav types (toc, landmarks, page-list)")
    create_multi_nav()
    print()

    print("[4/8] edge-deep-paths.epub - Deeply nested paths with cross-references")
    create_deep_paths()
    print()

    print("[5/8] edge-font-obfuscation.epub - Font obfuscation with encryption.xml")
    create_font_obfuscation()
    print()

    print("[6/8] edge-smil-overlay.epub - SMIL media overlays")
    create_smil_overlay()
    print()

    print("[7/8] edge-complex-css.epub - Complex CSS features")
    create_complex_css()
    print()

    print("[8/8] edge-percent-encoded.epub - Percent-encoded filenames")
    create_percent_encoded()
    print()

    print("=" * 60)
    print("All 8 edge-case EPUBs generated successfully.")
    print(f"Location: {SAMPLES_DIR}")
    print()

    # Verify all generated EPUBs
    print("Verifying ZIP integrity...")
    edge_files = [
        "edge-deep-fallback.epub",
        "edge-fxl-mixed.epub",
        "edge-multi-nav.epub",
        "edge-deep-paths.epub",
        "edge-font-obfuscation.epub",
        "edge-smil-overlay.epub",
        "edge-complex-css.epub",
        "edge-percent-encoded.epub",
    ]

    all_ok = True
    for fname in edge_files:
        fpath = os.path.join(SAMPLES_DIR, fname)
        try:
            with zipfile.ZipFile(fpath, "r") as zf:
                # Verify ZIP integrity
                bad = zf.testzip()
                if bad is not None:
                    print(f"  FAIL: {fname} - corrupt entry: {bad}")
                    all_ok = False
                    continue

                # Verify mimetype is first entry
                first = zf.infolist()[0]
                if first.filename != "mimetype":
                    print(f"  FAIL: {fname} - mimetype is not the first entry (found: {first.filename})")
                    all_ok = False
                    continue

                # Verify mimetype is uncompressed
                if first.compress_type != zipfile.ZIP_STORED:
                    print(f"  FAIL: {fname} - mimetype is compressed (should be STORED)")
                    all_ok = False
                    continue

                # Verify mimetype content
                mt = zf.read("mimetype").decode("ascii")
                if mt != "application/epub+zip":
                    print(f"  FAIL: {fname} - mimetype content wrong: {mt!r}")
                    all_ok = False
                    continue

                # Verify container.xml exists
                if "META-INF/container.xml" not in zf.namelist():
                    print(f"  FAIL: {fname} - missing META-INF/container.xml")
                    all_ok = False
                    continue

                # Count entries
                num_entries = len(zf.infolist())
                print(f"  OK: {fname} ({num_entries} entries)")

        except Exception as e:
            print(f"  FAIL: {fname} - {e}")
            all_ok = False

    print()
    if all_ok:
        print("All verifications passed!")
    else:
        print("Some verifications FAILED!")
        exit(1)


if __name__ == "__main__":
    main()
