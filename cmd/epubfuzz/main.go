// Command epubfuzz generates randomized synthetic EPUB files with potential
// validation failures for testing epubverify against epubcheck.
package main

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Fault describes a single mutation applied to a generated EPUB.
type Fault struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// EPUBSpec describes the parameters used to generate an EPUB.
type EPUBSpec struct {
	ID        int     `json:"id"`
	Version   string  `json:"version"` // "2.0" or "3.0"
	Faults    []Fault `json:"faults"`
	Filename  string  `json:"filename"`
	NumChapters int   `json:"num_chapters"`
}

// faultFunc is a function that mutates an EPUB builder to inject a fault.
type faultFunc struct {
	name        string
	description string
	apply       func(b *epubBuilder, rng *rand.Rand)
	weight      int // relative probability weight
}

var allFaults []faultFunc

func init() {
	allFaults = []faultFunc{
		// === OCF / Container faults ===
		{
			name:        "missing_mimetype",
			description: "Omit the mimetype file entirely",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitMimetype = true
			},
		},
		{
			name:        "wrong_mimetype_content",
			description: "Use wrong content in mimetype file",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{
					"application/epub",
					"application/zip",
					"application/epub+zip\n",
					"application/epub+zip ",
					" application/epub+zip",
					"APPLICATION/EPUB+ZIP",
					"text/plain",
				}
				b.mimetypeContent = options[rng.Intn(len(options))]
			},
		},
		{
			name:        "mimetype_compressed",
			description: "Store mimetype with compression instead of Store method",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.compressMimetype = true
			},
		},
		{
			name:        "mimetype_not_first",
			description: "Put mimetype file after other entries in the ZIP",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.mimetypeNotFirst = true
			},
		},
		{
			name:        "missing_container_xml",
			description: "Omit META-INF/container.xml",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitContainerXML = true
			},
		},
		{
			name:        "malformed_container_xml",
			description: "Use malformed XML in container.xml",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{
					`<container><rootfiles><rootfile full-path="EPUB/package.opf"`,
					`not xml at all`,
					`<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"></container>`,
					`<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles></rootfiles></container>`,
				}
				b.containerXMLOverride = options[rng.Intn(len(options))]
			},
		},
		{
			name:        "wrong_rootfile_path",
			description: "Point container.xml rootfile to non-existent OPF",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.rootfilePath = "EPUB/nonexistent.opf"
			},
		},
		// === OPF Metadata faults ===
		{
			name:        "missing_dc_title",
			description: "Omit dc:title from metadata",
			weight:      4,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitTitle = true
			},
		},
		{
			name:        "empty_dc_title",
			description: "Use empty dc:title",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.title = ""
				b.emptyTitle = true
			},
		},
		{
			name:        "missing_dc_identifier",
			description: "Omit dc:identifier from metadata",
			weight:      4,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitIdentifier = true
			},
		},
		{
			name:        "empty_dc_identifier",
			description: "Use empty dc:identifier",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.identifier = ""
				b.emptyIdentifier = true
			},
		},
		{
			name:        "missing_dc_language",
			description: "Omit dc:language from metadata",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitLanguage = true
			},
		},
		{
			name:        "invalid_dc_language",
			description: "Use invalid BCP 47 language tag",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{"xx", "123", "english", "e", "zz-ZZ-ZZZZ", ""}
				b.language = options[rng.Intn(len(options))]
			},
		},
		{
			name:        "missing_dcterms_modified",
			description: "Omit dcterms:modified (EPUB 3)",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitModified = true
			},
		},
		{
			name:        "invalid_dcterms_modified",
			description: "Use invalid format for dcterms:modified",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{
					"2024-01-15",
					"2024-01-15T10:30:00",
					"Jan 15, 2024",
					"not a date",
					"2024-13-45T99:99:99Z",
				}
				b.modified = options[rng.Intn(len(options))]
			},
		},
		{
			name:        "duplicate_dcterms_modified",
			description: "Include dcterms:modified twice",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.duplicateModified = true
			},
		},
		{
			name:        "missing_unique_identifier_attr",
			description: "Omit unique-identifier attribute from package element",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitUniqueIdentifier = true
			},
		},
		{
			name:        "dangling_unique_identifier",
			description: "unique-identifier references non-existent id",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.uniqueIdentifierRef = "nonexistent-id"
			},
		},
		{
			name:        "invalid_package_version",
			description: "Use invalid version in package element",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{"1.0", "4.0", "3.1", "abc", ""}
				b.versionOverride = options[rng.Intn(len(options))]
			},
		},
		// === Manifest faults ===
		{
			name:        "duplicate_manifest_ids",
			description: "Use duplicate IDs in manifest items",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.duplicateManifestIDs = true
			},
		},
		{
			name:        "manifest_missing_href",
			description: "Omit href from a manifest item",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.manifestMissingHref = true
			},
		},
		{
			name:        "manifest_missing_media_type",
			description: "Omit media-type from a manifest item",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.manifestMissingMediaType = true
			},
		},
		{
			name:        "manifest_missing_id",
			description: "Omit id from a manifest item",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.manifestMissingID = true
			},
		},
		{
			name:        "manifest_wrong_media_type",
			description: "Use wrong media-type for manifest item",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{"text/html", "text/plain", "application/xml", "image/png"}
				b.manifestWrongMediaType = options[rng.Intn(len(options))]
			},
		},
		{
			name:        "manifest_href_fragment",
			description: "Include fragment (#) in manifest item href",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.manifestHrefFragment = true
			},
		},
		{
			name:        "manifest_dangling_href",
			description: "Manifest item href points to non-existent file",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.manifestDanglingHref = true
			},
		},
		{
			name:        "duplicate_manifest_hrefs",
			description: "Use duplicate hrefs in manifest items",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.duplicateManifestHrefs = true
			},
		},
		// === Spine faults ===
		{
			name:        "empty_spine",
			description: "Spine has no itemrefs",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.emptySpine = true
			},
		},
		{
			name:        "spine_dangling_idref",
			description: "Spine itemref references non-existent manifest ID",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.spineDanglingIDRef = true
			},
		},
		{
			name:        "spine_duplicate_idref",
			description: "Spine has duplicate itemrefs for same manifest item",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.spineDuplicateIDRef = true
			},
		},
		// === Navigation faults (EPUB 3) ===
		{
			name:        "missing_nav_document",
			description: "Omit nav document (EPUB 3 only)",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.omitNav = true
			},
		},
		{
			name:        "nav_missing_toc",
			description: "Nav document has no toc nav element",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.navMissingToc = true
			},
		},
		{
			name:        "nav_broken_links",
			description: "Nav document has links to non-existent files",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.navBrokenLinks = true
			},
		},
		// === Content faults ===
		{
			name:        "malformed_xhtml",
			description: "Content document has malformed XML",
			weight:      4,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{
					"not xml at all <><>",
					`<html><body><p>unclosed paragraph<p>another</body></html>`,
					`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body><div><p>mismatched</div></p></body></html>`,
					`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body>&invalid;</body></html>`,
				}
				b.malformedXHTML = options[rng.Intn(len(options))]
			},
		},
		{
			name:        "xhtml_missing_title",
			description: "XHTML content missing <title> element",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.xhtmlMissingTitle = true
			},
		},
		{
			name:        "xhtml_empty_href",
			description: "XHTML content has empty href attribute",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.xhtmlEmptyHref = true
			},
		},
		{
			name:        "xhtml_obsolete_elements",
			description: "Use obsolete HTML elements like <center>, <font>",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.xhtmlObsoleteElements = true
			},
		},
		{
			name:        "xhtml_duplicate_ids",
			description: "Use duplicate IDs in XHTML content",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.xhtmlDuplicateIDs = true
			},
		},
		// === CSS faults ===
		{
			name:        "malformed_css",
			description: "Include malformed CSS stylesheet",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.includeMalformedCSS = true
			},
		},
		// === EPUB 2 faults ===
		{
			name:        "epub2_missing_ncx",
			description: "EPUB 2 without NCX file",
			weight:      3,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.epub2MissingNCX = true
			},
		},
		{
			name:        "epub2_malformed_ncx",
			description: "EPUB 2 with malformed NCX",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.epub2MalformedNCX = true
			},
		},
		{
			name:        "epub2_spine_no_toc_attr",
			description: "EPUB 2 spine missing toc attribute",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.epub2SpineNoToc = true
			},
		},
		// === Multi-chapter / complex structure ===
		{
			name:        "extra_unreferenced_file",
			description: "Include a file in ZIP not referenced in manifest",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.extraUnreferencedFile = true
			},
		},
		{
			name:        "file_in_meta_inf",
			description: "Put a publication resource in META-INF/",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.fileInMetaInf = true
			},
		},
		{
			name:        "invalid_date_format",
			description: "Use invalid dc:date format",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.invalidDateFormat = true
			},
		},
		{
			name:        "circular_fallback",
			description: "Create a circular fallback chain in manifest",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.circularFallback = true
			},
		},
		{
			name:        "opf_guide_in_epub3",
			description: "Include deprecated guide element in EPUB 3",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.guideInEPUB3 = true
			},
		},
		{
			name:        "special_chars_in_filename",
			description: "Use special characters in content filenames",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.specialCharsFilename = true
			},
		},
		{
			name:        "manifest_empty_href",
			description: "Use empty href attribute in manifest item",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.manifestEmptyHref = true
			},
		},
		{
			name:        "invalid_manifest_properties",
			description: "Use invalid properties value on manifest item",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.invalidManifestProperties = true
			},
		},
		{
			name:        "multiple_nav_documents",
			description: "Multiple manifest items with nav property",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.multipleNavDocs = true
			},
		},
		{
			name:        "invalid_spine_ppd",
			description: "Invalid page-progression-direction on spine",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				options := []string{"left-to-right", "up", "invalid", ""}
				b.invalidSpinePPD = options[rng.Intn(len(options))]
			},
		},
		{
			name:        "malformed_opf_xml",
			description: "Use malformed XML in the OPF file",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.malformedOPF = true
			},
		},
		{
			name:        "missing_opf_file",
			description: "Container.xml points to OPF but file missing from ZIP",
			weight:      2,
			apply: func(b *epubBuilder, rng *rand.Rand) {
				b.missingOPFFile = true
			},
		},
	}
}

// epubBuilder accumulates parameters for building an EPUB.
type epubBuilder struct {
	version string // "2.0" or "3.0"
	numChapters int

	// Mimetype/OCF faults
	omitMimetype       bool
	mimetypeContent    string
	compressMimetype   bool
	mimetypeNotFirst   bool
	omitContainerXML   bool
	containerXMLOverride string
	rootfilePath       string

	// Metadata faults
	title                string
	omitTitle            bool
	emptyTitle           bool
	identifier           string
	omitIdentifier       bool
	emptyIdentifier      bool
	language             string
	omitLanguage         bool
	modified             string
	omitModified         bool
	duplicateModified    bool
	omitUniqueIdentifier bool
	uniqueIdentifierRef  string
	versionOverride      string
	invalidDateFormat    bool

	// Manifest faults
	duplicateManifestIDs     bool
	manifestMissingHref      bool
	manifestMissingMediaType bool
	manifestMissingID        bool
	manifestWrongMediaType   string
	manifestHrefFragment     bool
	manifestDanglingHref     bool
	duplicateManifestHrefs   bool
	manifestEmptyHref        bool
	invalidManifestProperties bool
	circularFallback         bool

	// Spine faults
	emptySpine        bool
	spineDanglingIDRef bool
	spineDuplicateIDRef bool
	invalidSpinePPD    string

	// Navigation faults
	omitNav        bool
	navMissingToc  bool
	navBrokenLinks bool
	multipleNavDocs bool

	// Content faults
	malformedXHTML        string
	xhtmlMissingTitle     bool
	xhtmlEmptyHref        bool
	xhtmlObsoleteElements bool
	xhtmlDuplicateIDs     bool
	includeMalformedCSS   bool

	// EPUB 2 faults
	epub2MissingNCX   bool
	epub2MalformedNCX bool
	epub2SpineNoToc   bool

	// Misc faults
	extraUnreferencedFile bool
	fileInMetaInf         bool
	guideInEPUB3          bool
	specialCharsFilename  bool
	malformedOPF          bool
	missingOPFFile        bool
}

func newBuilder(version string, numChapters int) *epubBuilder {
	return &epubBuilder{
		version:     version,
		numChapters: numChapters,
		title:       "Test Book",
		identifier:  "urn:uuid:12345678-1234-1234-1234-123456789abc",
		language:    "en",
		modified:    "2024-01-15T10:30:00Z",
	}
}

type entry struct {
	name       string
	content    []byte
	method     uint16
	isMimetype bool // write without extra field
}

// writeZip creates a ZIP archive from entries. For mimetype entries, it writes
// raw local file headers without extra fields to comply with EPUB OCF spec.
func writeZip(entries []entry) ([]byte, error) {
	var buf bytes.Buffer

	type centralEntry struct {
		name       string
		offset     uint32
		method     uint16
		crc        uint32
		compSize   uint32
		uncompSize uint32
	}
	var central []centralEntry

	for _, e := range entries {
		offset := uint32(buf.Len())
		content := e.content
		crc := crc32.ChecksumIEEE(content)
		uncompSize := uint32(len(content))

		var compressed []byte
		method := e.method
		if method == uint16(zip.Deflate) {
			var cbuf bytes.Buffer
			fw, _ := flate.NewWriter(&cbuf, flate.DefaultCompression)
			fw.Write(content)
			fw.Close()
			compressed = cbuf.Bytes()
		} else {
			compressed = content
		}
		compSize := uint32(len(compressed))

		if e.isMimetype && method == uint16(zip.Store) {
			// Write raw local file header without extra field
			nameBytes := []byte(e.name)
			// Local file header
			binary.Write(&buf, binary.LittleEndian, uint32(0x04034b50)) // signature
			binary.Write(&buf, binary.LittleEndian, uint16(20))        // version needed
			binary.Write(&buf, binary.LittleEndian, uint16(0))         // flags
			binary.Write(&buf, binary.LittleEndian, method)            // compression
			binary.Write(&buf, binary.LittleEndian, uint16(0))         // mod time
			binary.Write(&buf, binary.LittleEndian, uint16(0))         // mod date
			binary.Write(&buf, binary.LittleEndian, crc)               // crc32
			binary.Write(&buf, binary.LittleEndian, compSize)          // compressed size
			binary.Write(&buf, binary.LittleEndian, uncompSize)        // uncompressed size
			binary.Write(&buf, binary.LittleEndian, uint16(len(nameBytes))) // name length
			binary.Write(&buf, binary.LittleEndian, uint16(0))              // extra length = 0
			buf.Write(nameBytes)
			buf.Write(compressed)
		} else {
			// Write normal local file header (with timestamp but no extra field)
			nameBytes := []byte(e.name)
			binary.Write(&buf, binary.LittleEndian, uint32(0x04034b50))
			binary.Write(&buf, binary.LittleEndian, uint16(20))
			binary.Write(&buf, binary.LittleEndian, uint16(0))
			binary.Write(&buf, binary.LittleEndian, method)
			binary.Write(&buf, binary.LittleEndian, uint16(0)) // mod time
			binary.Write(&buf, binary.LittleEndian, uint16(0)) // mod date
			binary.Write(&buf, binary.LittleEndian, crc)
			binary.Write(&buf, binary.LittleEndian, compSize)
			binary.Write(&buf, binary.LittleEndian, uncompSize)
			binary.Write(&buf, binary.LittleEndian, uint16(len(nameBytes)))
			binary.Write(&buf, binary.LittleEndian, uint16(0)) // extra length = 0
			buf.Write(nameBytes)
			buf.Write(compressed)
		}

		central = append(central, centralEntry{
			name:       e.name,
			offset:     offset,
			method:     method,
			crc:        crc,
			compSize:   compSize,
			uncompSize: uncompSize,
		})
	}

	// Central directory
	cdOffset := uint32(buf.Len())
	for _, ce := range central {
		nameBytes := []byte(ce.name)
		binary.Write(&buf, binary.LittleEndian, uint32(0x02014b50))          // signature
		binary.Write(&buf, binary.LittleEndian, uint16(20))                  // version made by
		binary.Write(&buf, binary.LittleEndian, uint16(20))                  // version needed
		binary.Write(&buf, binary.LittleEndian, uint16(0))                   // flags
		binary.Write(&buf, binary.LittleEndian, ce.method)                   // compression
		binary.Write(&buf, binary.LittleEndian, uint16(0))                   // mod time
		binary.Write(&buf, binary.LittleEndian, uint16(0))                   // mod date
		binary.Write(&buf, binary.LittleEndian, ce.crc)                      // crc32
		binary.Write(&buf, binary.LittleEndian, ce.compSize)                 // compressed size
		binary.Write(&buf, binary.LittleEndian, ce.uncompSize)               // uncompressed size
		binary.Write(&buf, binary.LittleEndian, uint16(len(nameBytes)))      // name length
		binary.Write(&buf, binary.LittleEndian, uint16(0))                   // extra length
		binary.Write(&buf, binary.LittleEndian, uint16(0))                   // comment length
		binary.Write(&buf, binary.LittleEndian, uint16(0))                   // disk number
		binary.Write(&buf, binary.LittleEndian, uint16(0))                   // internal attrs
		binary.Write(&buf, binary.LittleEndian, uint32(0))                   // external attrs
		binary.Write(&buf, binary.LittleEndian, ce.offset)                   // local header offset
		buf.Write(nameBytes)
	}
	cdSize := uint32(buf.Len()) - cdOffset

	// End of central directory
	binary.Write(&buf, binary.LittleEndian, uint32(0x06054b50)) // signature
	binary.Write(&buf, binary.LittleEndian, uint16(0))          // disk number
	binary.Write(&buf, binary.LittleEndian, uint16(0))          // disk with CD
	binary.Write(&buf, binary.LittleEndian, uint16(len(central)))
	binary.Write(&buf, binary.LittleEndian, uint16(len(central)))
	binary.Write(&buf, binary.LittleEndian, cdSize)
	binary.Write(&buf, binary.LittleEndian, cdOffset)
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // comment length

	return buf.Bytes(), nil
}

func (b *epubBuilder) build() ([]byte, error) {
	var entries []entry

	// 1. Mimetype
	if !b.omitMimetype {
		mt := "application/epub+zip"
		if b.mimetypeContent != "" {
			mt = b.mimetypeContent
		}
		method := uint16(zip.Store)
		if b.compressMimetype {
			method = uint16(zip.Deflate)
		}
		if !b.mimetypeNotFirst {
			entries = append(entries, entry{"mimetype", []byte(mt), method, true})
		}
	}

	// 2. Container.xml
	opfPath := "EPUB/package.opf"
	if b.rootfilePath != "" {
		opfPath = b.rootfilePath
	}
	if !b.omitContainerXML {
		containerXML := b.containerXMLOverride
		if containerXML == "" {
			containerXML = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="%s" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`, opfPath)
		}
		entries = append(entries, entry{"META-INF/container.xml", []byte(containerXML), uint16(zip.Deflate), false})
	}

	// Add mimetype late if mimetypeNotFirst
	if b.mimetypeNotFirst && !b.omitMimetype {
		mt := "application/epub+zip"
		if b.mimetypeContent != "" {
			mt = b.mimetypeContent
		}
		entries = append(entries, entry{"mimetype", []byte(mt), uint16(zip.Store), true})
	}

	// 3. File in META-INF (fault)
	if b.fileInMetaInf {
		entries = append(entries, entry{"META-INF/extra.xhtml", []byte(makeXHTML("Extra", "<p>Should not be here</p>", b.version)), uint16(zip.Deflate), false})
	}

	// 4. OPF Package Document
	if !b.missingOPFFile {
		actualOPFPath := "EPUB/package.opf"
		if b.rootfilePath != "" && b.rootfilePath != "EPUB/nonexistent.opf" {
			actualOPFPath = b.rootfilePath
		}
		if b.malformedOPF {
			entries = append(entries, entry{actualOPFPath, []byte(`<?xml version="1.0"?><package><metadata><unclosed>`), uint16(zip.Deflate), false})
		} else {
			opfContent := b.generateOPF()
			entries = append(entries, entry{actualOPFPath, []byte(opfContent), uint16(zip.Deflate), false})
		}
	}

	// 5. Content documents
	if !b.missingOPFFile && !b.malformedOPF {
		// Nav document for EPUB 3
		if b.version == "3.0" && !b.omitNav {
			navContent := b.generateNav()
			entries = append(entries, entry{"EPUB/nav.xhtml", []byte(navContent), uint16(zip.Deflate), false})
			if b.multipleNavDocs {
				entries = append(entries, entry{"EPUB/nav2.xhtml", []byte(navContent), uint16(zip.Deflate), false})
			}
		}

		// NCX for EPUB 2
		if b.version == "2.0" && !b.epub2MissingNCX {
			ncxContent := b.generateNCX()
			entries = append(entries, entry{"EPUB/toc.ncx", []byte(ncxContent), uint16(zip.Deflate), false})
		}

		// Chapter files
		for i := 1; i <= b.numChapters; i++ {
			fname := fmt.Sprintf("EPUB/chapter%d.xhtml", i)
			if i == 1 && b.specialCharsFilename {
				fname = "EPUB/chapter 1.xhtml"
			}
			var content string
			if i == 1 && b.malformedXHTML != "" {
				content = b.malformedXHTML
			} else {
				body := fmt.Sprintf("<h1>Chapter %d</h1><p>Content of chapter %d.</p>", i, i)
				if i == 1 && b.xhtmlEmptyHref {
					body += `<a href="">empty link</a>`
				}
				if i == 1 && b.xhtmlObsoleteElements {
					body += `<center><font color="red">obsolete</font></center>`
				}
				if i == 1 && b.xhtmlDuplicateIDs {
					body += `<div id="dup">first</div><div id="dup">second</div>`
				}
				if b.xhtmlMissingTitle {
					content = makeXHTMLNoTitle(body, b.version)
				} else {
					content = makeXHTML(fmt.Sprintf("Chapter %d", i), body, b.version)
				}
			}
			entries = append(entries, entry{fname, []byte(content), uint16(zip.Deflate), false})
		}

		// Manifest dangling href: don't create the file
		// (the manifest will reference "EPUB/phantom.xhtml" but it won't exist)

		// CSS
		if b.includeMalformedCSS {
			entries = append(entries, entry{"EPUB/style.css", []byte("body { color: ; font-size: px; } .broken { }}}"), uint16(zip.Deflate), false})
		}
	}

	// 6. Extra unreferenced file
	if b.extraUnreferencedFile {
		entries = append(entries, entry{"EPUB/unreferenced.xhtml", []byte(makeXHTML("Unreferenced", "<p>Not in manifest</p>", b.version)), uint16(zip.Deflate), false})
	}

	// We need to write the ZIP manually to avoid Go's zip library adding
	// extra fields to the mimetype entry (PKG-005 violation).
	return writeZip(entries)
}

func (b *epubBuilder) generateOPF() string {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")

	// Package element
	version := b.version
	if b.versionOverride != "" {
		version = b.versionOverride
	}
	if b.omitUniqueIdentifier {
		sb.WriteString(fmt.Sprintf(`<package xmlns="http://www.idpf.org/2007/opf" version="%s">`, version))
	} else {
		uidRef := "pub-id"
		if b.uniqueIdentifierRef != "" {
			uidRef = b.uniqueIdentifierRef
		}
		sb.WriteString(fmt.Sprintf(`<package xmlns="http://www.idpf.org/2007/opf" version="%s" unique-identifier="%s"`, version, uidRef))
		if b.version == "3.0" {
			sb.WriteString(` xmlns:dc="http://purl.org/dc/elements/1.1/"`)
		}
		sb.WriteString(`>`)
	}
	sb.WriteString("\n")

	// Metadata
	sb.WriteString(`  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"`)
	if b.version == "3.0" {
		sb.WriteString(` xmlns:opf="http://www.idpf.org/2007/opf"`)
	}
	sb.WriteString(">\n")

	if !b.omitTitle {
		if b.emptyTitle {
			sb.WriteString("    <dc:title></dc:title>\n")
		} else {
			sb.WriteString(fmt.Sprintf("    <dc:title>%s</dc:title>\n", b.title))
		}
	}
	if !b.omitIdentifier {
		if b.emptyIdentifier {
			sb.WriteString(`    <dc:identifier id="pub-id"></dc:identifier>` + "\n")
		} else {
			sb.WriteString(fmt.Sprintf(`    <dc:identifier id="pub-id">%s</dc:identifier>`+"\n", b.identifier))
		}
	}
	if !b.omitLanguage {
		sb.WriteString(fmt.Sprintf("    <dc:language>%s</dc:language>\n", b.language))
	}

	if b.version == "3.0" {
		if !b.omitModified {
			sb.WriteString(fmt.Sprintf(`    <meta property="dcterms:modified">%s</meta>`+"\n", b.modified))
			if b.duplicateModified {
				sb.WriteString(fmt.Sprintf(`    <meta property="dcterms:modified">%s</meta>`+"\n", b.modified))
			}
		}
	}

	if b.invalidDateFormat {
		sb.WriteString("    <dc:date>not-a-date</dc:date>\n")
	}

	sb.WriteString("  </metadata>\n")

	// Manifest
	sb.WriteString("  <manifest>\n")

	// Nav or NCX
	if b.version == "3.0" && !b.omitNav {
		navProps := `properties="nav"`
		sb.WriteString(fmt.Sprintf(`    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" %s/>`+"\n", navProps))
		if b.multipleNavDocs {
			sb.WriteString(`    <item id="nav2" href="nav2.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")
		}
	}
	if b.version == "2.0" && !b.epub2MissingNCX {
		sb.WriteString(`    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>` + "\n")
	}

	// Chapter items
	for i := 1; i <= b.numChapters; i++ {
		id := fmt.Sprintf("ch%d", i)
		href := fmt.Sprintf("chapter%d.xhtml", i)
		if i == 1 && b.specialCharsFilename {
			href = "chapter%201.xhtml"
		}
		mediaType := "application/xhtml+xml"

		if i == 1 && b.duplicateManifestIDs && b.numChapters > 1 {
			id = "ch2" // Will duplicate
		}
		if i == 1 && b.manifestMissingHref {
			sb.WriteString(fmt.Sprintf(`    <item id="%s" media-type="%s"/>`+"\n", id, mediaType))
			continue
		}
		if i == 1 && b.manifestMissingMediaType {
			sb.WriteString(fmt.Sprintf(`    <item id="%s" href="%s"/>`+"\n", id, href))
			continue
		}
		if i == 1 && b.manifestMissingID {
			sb.WriteString(fmt.Sprintf(`    <item href="%s" media-type="%s"/>`+"\n", href, mediaType))
			continue
		}
		if i == 1 && b.manifestWrongMediaType != "" {
			mediaType = b.manifestWrongMediaType
		}
		if i == 1 && b.manifestHrefFragment {
			href += "#fragment"
		}
		if i == 1 && b.manifestEmptyHref {
			href = ""
		}

		props := ""
		if i == 1 && b.invalidManifestProperties {
			props = ` properties="invalid-property"`
		}

		sb.WriteString(fmt.Sprintf(`    <item id="%s" href="%s" media-type="%s"%s/>`, id, href, mediaType, props))
		sb.WriteString("\n")
	}

	// Dangling href item
	if b.manifestDanglingHref {
		sb.WriteString(`    <item id="phantom" href="phantom.xhtml" media-type="application/xhtml+xml"/>` + "\n")
	}

	// Duplicate href item
	if b.duplicateManifestHrefs && b.numChapters > 0 {
		sb.WriteString(`    <item id="dup-href" href="chapter1.xhtml" media-type="application/xhtml+xml"/>` + "\n")
	}

	// Circular fallback
	if b.circularFallback {
		sb.WriteString(`    <item id="fb1" href="fb1.xml" media-type="application/xml" fallback="fb2"/>` + "\n")
		sb.WriteString(`    <item id="fb2" href="fb2.xml" media-type="application/xml" fallback="fb1"/>` + "\n")
	}

	// CSS in manifest
	if b.includeMalformedCSS {
		sb.WriteString(`    <item id="css" href="style.css" media-type="text/css"/>` + "\n")
	}

	sb.WriteString("  </manifest>\n")

	// Spine
	sb.WriteString("  <spine")
	if b.version == "2.0" && !b.epub2SpineNoToc && !b.epub2MissingNCX {
		sb.WriteString(` toc="ncx"`)
	}
	if b.invalidSpinePPD != "" {
		sb.WriteString(fmt.Sprintf(` page-progression-direction="%s"`, b.invalidSpinePPD))
	}
	sb.WriteString(">\n")

	if !b.emptySpine {
		for i := 1; i <= b.numChapters; i++ {
			idref := fmt.Sprintf("ch%d", i)
			if i == 1 && b.spineDanglingIDRef {
				idref = "nonexistent"
			}
			sb.WriteString(fmt.Sprintf(`    <itemref idref="%s"/>`, idref))
			sb.WriteString("\n")
		}
		if b.spineDuplicateIDRef && b.numChapters > 0 {
			sb.WriteString(`    <itemref idref="ch1"/>` + "\n")
		}
		if b.manifestDanglingHref {
			sb.WriteString(`    <itemref idref="phantom"/>` + "\n")
		}
	}

	sb.WriteString("  </spine>\n")

	// Guide (EPUB 3 deprecation test)
	if b.guideInEPUB3 && b.version == "3.0" {
		sb.WriteString("  <guide>\n")
		sb.WriteString(`    <reference type="toc" title="Table of Contents" href="nav.xhtml"/>` + "\n")
		sb.WriteString("  </guide>\n")
	}

	sb.WriteString("</package>\n")
	return sb.String()
}

func (b *epubBuilder) generateNav() string {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<!DOCTYPE html>` + "\n")
	sb.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">` + "\n")
	sb.WriteString("<head><title>Navigation</title></head>\n")
	sb.WriteString("<body>\n")

	if !b.navMissingToc {
		sb.WriteString(`<nav epub:type="toc" id="toc">` + "\n")
		sb.WriteString("<h1>Table of Contents</h1>\n")
		sb.WriteString("<ol>\n")
		for i := 1; i <= b.numChapters; i++ {
			href := fmt.Sprintf("chapter%d.xhtml", i)
			if i == 1 && b.navBrokenLinks {
				href = "nonexistent.xhtml"
			}
			if i == 1 && b.specialCharsFilename {
				href = "chapter%201.xhtml"
			}
			sb.WriteString(fmt.Sprintf(`  <li><a href="%s">Chapter %d</a></li>`+"\n", href, i))
		}
		sb.WriteString("</ol>\n")
		sb.WriteString("</nav>\n")
	}

	sb.WriteString("</body>\n</html>\n")
	return sb.String()
}

func (b *epubBuilder) generateNCX() string {
	if b.epub2MalformedNCX {
		return `<?xml version="1.0"?><ncx><not-closed>`
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sb.WriteString(`<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">` + "\n")
	sb.WriteString("<head>\n")
	sb.WriteString(fmt.Sprintf(`  <meta name="dtb:uid" content="%s"/>`, b.identifier) + "\n")
	sb.WriteString(`  <meta name="dtb:depth" content="1"/>` + "\n")
	sb.WriteString(`  <meta name="dtb:totalPageCount" content="0"/>` + "\n")
	sb.WriteString(`  <meta name="dtb:maxPageNumber" content="0"/>` + "\n")
	sb.WriteString("</head>\n")
	sb.WriteString(fmt.Sprintf("<docTitle><text>%s</text></docTitle>\n", b.title))
	sb.WriteString("<navMap>\n")
	for i := 1; i <= b.numChapters; i++ {
		sb.WriteString(fmt.Sprintf(`  <navPoint id="np%d" playOrder="%d">`+"\n", i, i))
		sb.WriteString(fmt.Sprintf("    <navLabel><text>Chapter %d</text></navLabel>\n", i))
		sb.WriteString(fmt.Sprintf(`    <content src="chapter%d.xhtml"/>`+"\n", i))
		sb.WriteString("  </navPoint>\n")
	}
	sb.WriteString("</navMap>\n")
	sb.WriteString("</ncx>\n")
	return sb.String()
}

func makeXHTML(title, body, version string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	if version == "3.0" {
		sb.WriteString("<!DOCTYPE html>\n")
	}
	sb.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml">` + "\n")
	sb.WriteString(fmt.Sprintf("<head><title>%s</title></head>\n", title))
	sb.WriteString(fmt.Sprintf("<body>%s</body>\n", body))
	sb.WriteString("</html>\n")
	return sb.String()
}

func makeXHTMLNoTitle(body, version string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	if version == "3.0" {
		sb.WriteString("<!DOCTYPE html>\n")
	}
	sb.WriteString(`<html xmlns="http://www.w3.org/1999/xhtml">` + "\n")
	sb.WriteString("<head></head>\n")
	sb.WriteString(fmt.Sprintf("<body>%s</body>\n", body))
	sb.WriteString("</html>\n")
	return sb.String()
}

// fixedModTime is unused but kept for reference.
var _ = time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

func generateEPUB(id int, rng *rand.Rand) (*EPUBSpec, []byte, error) {
	// Pick version: 60% EPUB 3, 40% EPUB 2
	version := "3.0"
	if rng.Float64() < 0.4 {
		version = "2.0"
	}

	numChapters := 1 + rng.Intn(4) // 1-4 chapters

	b := newBuilder(version, numChapters)

	// Decide how many faults to inject: 0-4
	// 15% valid (0 faults), 35% 1 fault, 30% 2 faults, 15% 3 faults, 5% 4 faults
	r := rng.Float64()
	var numFaults int
	switch {
	case r < 0.15:
		numFaults = 0
	case r < 0.50:
		numFaults = 1
	case r < 0.80:
		numFaults = 2
	case r < 0.95:
		numFaults = 3
	default:
		numFaults = 4
	}

	// Filter faults by version compatibility
	var applicable []faultFunc
	for _, f := range allFaults {
		// Skip EPUB 2 faults for EPUB 3 and vice versa
		if version == "3.0" && (f.name == "epub2_missing_ncx" || f.name == "epub2_malformed_ncx" || f.name == "epub2_spine_no_toc_attr") {
			continue
		}
		if version == "2.0" && (f.name == "missing_nav_document" || f.name == "nav_missing_toc" || f.name == "nav_broken_links" || f.name == "missing_dcterms_modified" || f.name == "invalid_dcterms_modified" || f.name == "duplicate_dcterms_modified" || f.name == "opf_guide_in_epub3" || f.name == "multiple_nav_documents" || f.name == "invalid_manifest_properties") {
			continue
		}
		applicable = append(applicable, f)
	}

	// Weighted random selection of faults
	spec := &EPUBSpec{
		ID:          id,
		Version:     version,
		NumChapters: numChapters,
	}

	usedFaults := map[string]bool{}
	for i := 0; i < numFaults && len(applicable) > 0; i++ {
		// Build weight sum
		totalWeight := 0
		for _, f := range applicable {
			if !usedFaults[f.name] {
				totalWeight += f.weight
			}
		}
		if totalWeight == 0 {
			break
		}

		pick := rng.Intn(totalWeight)
		cumulative := 0
		for _, f := range applicable {
			if usedFaults[f.name] {
				continue
			}
			cumulative += f.weight
			if pick < cumulative {
				usedFaults[f.name] = true
				f.apply(b, rng)
				spec.Faults = append(spec.Faults, Fault{Name: f.name, Description: f.description})

				// Some faults are mutually exclusive with others
				switch f.name {
				case "missing_mimetype":
					usedFaults["wrong_mimetype_content"] = true
					usedFaults["mimetype_compressed"] = true
					usedFaults["mimetype_not_first"] = true
				case "wrong_mimetype_content":
					usedFaults["missing_mimetype"] = true
				case "malformed_container_xml":
					usedFaults["missing_container_xml"] = true
					usedFaults["wrong_rootfile_path"] = true
				case "missing_container_xml":
					usedFaults["malformed_container_xml"] = true
					usedFaults["wrong_rootfile_path"] = true
				case "malformed_opf_xml":
					// Can't have OPF-level faults with malformed OPF
					usedFaults["missing_dc_title"] = true
					usedFaults["missing_dc_identifier"] = true
					usedFaults["missing_dc_language"] = true
					usedFaults["empty_spine"] = true
				case "missing_opf_file":
					usedFaults["malformed_opf_xml"] = true
					usedFaults["missing_dc_title"] = true
				case "omit_title":
					usedFaults["empty_dc_title"] = true
				case "empty_dc_title":
					usedFaults["missing_dc_title"] = true
				case "missing_dc_identifier":
					usedFaults["empty_dc_identifier"] = true
				case "empty_dc_identifier":
					usedFaults["missing_dc_identifier"] = true
				}
				break
			}
		}
	}

	data, err := b.build()
	if err != nil {
		return nil, nil, err
	}

	spec.Filename = fmt.Sprintf("synth_%03d.epub", id)
	return spec, data, nil
}

func main() {
	count := 100
	outDir := "testdata/synthetic"
	seed := int64(42)

	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", outDir, err)
		os.Exit(1)
	}

	rng := rand.New(rand.NewSource(seed))

	var specs []EPUBSpec

	for i := 1; i <= count; i++ {
		spec, data, err := generateEPUB(i, rng)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate %d: %v\n", i, err)
			os.Exit(1)
		}

		path := filepath.Join(outDir, spec.Filename)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}

		specs = append(specs, *spec)

		faultNames := make([]string, len(spec.Faults))
		for j, f := range spec.Faults {
			faultNames[j] = f.Name
		}
		faultStr := "valid (no faults)"
		if len(faultNames) > 0 {
			faultStr = strings.Join(faultNames, ", ")
		}
		fmt.Printf("[%3d] %s v%s %dch: %s\n", i, spec.Filename, spec.Version, spec.NumChapters, faultStr)
	}

	// Write manifest
	manifestPath := filepath.Join(outDir, "manifest.json")
	manifestData, _ := json.MarshalIndent(specs, "", "  ")
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nGenerated %d EPUBs in %s\n", count, outDir)
	fmt.Printf("Manifest: %s\n", manifestPath)
}
