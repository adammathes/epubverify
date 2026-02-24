package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Open opens an EPUB file and parses its structure.
// The caller must call Close() when done.
func Open(filepath string) (*EPUB, error) {
	zr, err := zip.OpenReader(filepath)
	if err != nil {
		return nil, fmt.Errorf("opening epub: %w", err)
	}

	ep := &EPUB{
		Path:    filepath,
		ZipFile: zr,
		Files:   make(map[string]*zip.File),
	}

	for _, f := range zr.File {
		ep.Files[f.Name] = f
	}

	return ep, nil
}

// Close releases the underlying zip reader.
func (ep *EPUB) Close() error {
	if ep.ZipFile != nil {
		return ep.ZipFile.Close()
	}
	return nil
}

// ReadFile reads the contents of a file within the EPUB.
func (ep *EPUB) ReadFile(name string) ([]byte, error) {
	f, ok := ep.Files[name]
	if !ok {
		return nil, fmt.Errorf("file not found in epub: %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", name, err)
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Container XML types

type containerXML struct {
	XMLName   xml.Name       `xml:"container"`
	RootFiles rootFilesXML   `xml:"rootfiles"`
	Links     containerLinks `xml:"links"`
}

type rootFilesXML struct {
	RootFile []rootFileXML `xml:"rootfile"`
}

type rootFileXML struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

type containerLinks struct {
	Link []containerLink `xml:"link"`
}

type containerLink struct {
	Href      string `xml:"href,attr"`
	Rel       string `xml:"rel,attr"`
	MediaType string `xml:"media-type,attr"`
}

// ParseContainer parses META-INF/container.xml and sets RootfilePath.
func (ep *EPUB) ParseContainer() error {
	data, err := ep.ReadFile("META-INF/container.xml")
	if err != nil {
		return err
	}
	ep.ContainerData = data

	var c containerXML
	if err := xml.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("parsing container.xml: %w", err)
	}

	// Store all rootfiles
	for _, rf := range c.RootFiles.RootFile {
		ep.AllRootfiles = append(ep.AllRootfiles, Rootfile{
			FullPath:  rf.FullPath,
			MediaType: rf.MediaType,
		})
	}

	// Store container-level links (e.g., mapping documents for multiple renditions)
	for _, link := range c.Links.Link {
		if link.Href != "" {
			ep.ContainerLinks = append(ep.ContainerLinks, link.Href)
		}
	}

	for _, rf := range c.RootFiles.RootFile {
		if rf.MediaType == "application/oebps-package+xml" || rf.MediaType == "" {
			ep.RootfilePath = rf.FullPath
			return nil
		}
	}

	// No rootfile with correct media-type found but there may be one without
	if len(c.RootFiles.RootFile) > 0 {
		ep.RootfilePath = c.RootFiles.RootFile[0].FullPath
	}

	return nil
}

// ParseOPF parses the OPF package document and populates ep.Package.
// It uses raw XML scanning to detect structural issues like missing elements.
func (ep *EPUB) ParseOPF() error {
	if ep.RootfilePath == "" {
		return fmt.Errorf("no rootfile path set")
	}

	data, err := ep.ReadFile(ep.RootfilePath)
	if err != nil {
		return err
	}

	// First: detect structural elements via raw scan
	structInfo, err := scanOPFStructure(data)
	if err != nil {
		ep.OPFParseError = err
		return err
	}

	ep.HasMetadata = structInfo.hasMetadata
	ep.HasManifest = structInfo.hasManifest
	ep.HasSpine = structInfo.hasSpine
	ep.IsLegacyOEBPS12 = structInfo.isLegacyOEBPS12
	ep.PackageXMLLang = structInfo.xmlLang

	p := &Package{
		UniqueIdentifier:         structInfo.uniqueIdentifier,
		Version:                  structInfo.version,
		Dir:                      structInfo.dir,
		Prefix:                   structInfo.prefix,
		SpineToc:                 structInfo.spineToc,
		SpinePageMap:             structInfo.spinePageMap,
		PageProgressionDirection: structInfo.pageProgressionDirection,
		HasGuide:                 structInfo.hasGuide,
		MetaRefines:              structInfo.metaRefines,
		MetaIDs:                  structInfo.metaIDs,
		ElementOrder:             structInfo.elementOrder,
	}

	// Parse metadata if present
	if structInfo.hasMetadata {
		p.Metadata = parseMetadata(data)
	}

	// Parse rendition properties from metadata metas
	modifiedCount := 0
	for _, m := range structInfo.metas {
		switch m.property {
		case "dcterms:modified":
			p.Metadata.Modified = m.value
			modifiedCount++
		case "rendition:layout":
			p.RenditionLayout = m.value
		case "rendition:orientation":
			p.RenditionOrientation = m.value
		case "rendition:spread":
			p.RenditionSpread = m.value
		case "rendition:flow":
			p.RenditionFlow = m.value
		case "media:active-class", "media:playback-active-class":
			p.HasMediaActiveClass = true
		}
	}
	p.ModifiedCount = modifiedCount
	p.MetadataLinks = structInfo.metadataLinks
	p.MetaSchemes = structInfo.metaSchemes
	p.AllXMLLangs = structInfo.allXMLLangs
	p.MetaEmptyProps = structInfo.metaEmptyProps
	p.MetaListProps = structInfo.metaListProps
	p.MetaEmptyValues = structInfo.metaEmptyValues

	// Map meta element IDs to their property names for refines validation
	if p.Metadata.IDToElement == nil {
		p.Metadata.IDToElement = make(map[string]string)
	}
	for id, prop := range structInfo.metaIDToProperty {
		p.Metadata.IDToElement[id] = prop
	}

	// Build primary metas (non-refining meta elements)
	for _, m := range structInfo.metas {
		if m.refines == "" {
			p.PrimaryMetas = append(p.PrimaryMetas, MetaPrimary{Property: m.property, Value: m.value})
		}
	}

	// Parse manifest items
	rawItems, err := parseManifestRaw(data)
	if err != nil {
		return err
	}
	p.Manifest = rawItems

	// Parse spine
	p.Spine = structInfo.spineItems

	// Parse guide (EPUB 2)
	p.Guide = structInfo.guideRefs

	// XML-level fields
	p.HasBindings = structInfo.hasBindings
	p.UnknownElements = structInfo.unknownElements
	p.XMLIDCounts = structInfo.xmlIDCounts
	p.PackageNamespace = structInfo.packageNamespace

	ep.Package = p
	return nil
}

type opfStructInfo struct {
	isLegacyOEBPS12          bool
	version                  string
	uniqueIdentifier         string
	dir                      string
	prefix                   string
	xmlLang                  string
	hasMetadata              bool
	hasManifest              bool
	hasSpine                 bool
	hasGuide                 bool
	spineToc                 string
	spinePageMap             string
	pageProgressionDirection string
	spineItems               []SpineItemref
	metas                    []metaInfo
	metaRefines              []MetaRefines
	metaIDs                  []string
	guideRefs                []GuideReference
	elementOrder             []string
	metadataLinks            []MetadataLink
	allXMLLangs              []string // all xml:lang attribute values found in the OPF
	metaSchemes              []MetaScheme // scheme attributes on meta elements
	metaIDToProperty         map[string]string // meta element ID → property name
	metaEmptyProps           int      // count of meta elements with empty property attribute
	metaListProps            []string // meta property attributes containing spaces
	metaEmptyValues          int      // count of meta elements with empty text content
	hasBindings              bool
	unknownElements          []string
	xmlIDCounts              map[string]int
	packageNamespace         string
}

type metaInfo struct {
	property string
	value    string
	refines  string
}

// scanOPFStructure does a raw XML scan of the OPF to detect structural elements.
func scanOPFStructure(data []byte) (*opfStructInfo, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	info := &opfStructInfo{
		metaIDToProperty: make(map[string]string),
		xmlIDCounts:      make(map[string]int),
	}

	depth := 0 // track nesting to detect direct children of <package>
	knownPackageChildren := map[string]bool{
		"metadata": true, "manifest": true, "spine": true, "guide": true,
		"bindings": true, "collection": true,
	}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch tok.(type) {
		case xml.EndElement:
			depth--
			continue
		default:
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		// Collect xml:lang attributes from any element
		for _, attr := range se.Attr {
			if attr.Name.Local == "lang" && (attr.Name.Space == "http://www.w3.org/XML/1998/namespace" || attr.Name.Space == "xml") {
				info.allXMLLangs = append(info.allXMLLangs, attr.Value)
			}
		}

		// Collect all id attributes for XML-level duplicate detection (with whitespace normalization)
		for _, attr := range se.Attr {
			if attr.Name.Local == "id" && attr.Value != "" {
				normalized := strings.TrimSpace(attr.Value)
				if normalized != "" {
					info.xmlIDCounts[normalized]++
				}
			}
		}

		switch se.Name.Local {
		case "package":
			info.packageNamespace = se.Name.Space
			// Detect OEBPS 1.2 namespace
			if se.Name.Space == "http://openebook.org/namespaces/oeb-package/1.0/" {
				info.isLegacyOEBPS12 = true
			}
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "version":
					info.version = attr.Value
				case "unique-identifier":
					info.uniqueIdentifier = attr.Value
				case "dir":
					info.dir = attr.Value
				case "prefix":
					info.prefix = attr.Value
				case "lang":
					// xml:lang on the package element
					if attr.Name.Space == "http://www.w3.org/XML/1998/namespace" || attr.Name.Space == "xml" {
						info.xmlLang = attr.Value
					}
				}
			}
		case "metadata":
			info.hasMetadata = true
			info.elementOrder = append(info.elementOrder, "metadata")
		case "manifest":
			info.hasManifest = true
			info.elementOrder = append(info.elementOrder, "manifest")
		case "spine":
			info.hasSpine = true
			info.elementOrder = append(info.elementOrder, "spine")
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "toc":
					info.spineToc = attr.Value
				case "page-progression-direction":
					info.pageProgressionDirection = attr.Value
				case "page-map":
					info.spinePageMap = attr.Value
				}
			}
		case "guide":
			info.hasGuide = true
			info.elementOrder = append(info.elementOrder, "guide")
		case "bindings":
			info.hasBindings = true
		case "itemref":
			var idref, props, linear string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "idref":
					idref = attr.Value
				case "properties":
					props = attr.Value
				case "linear":
					linear = attr.Value
				}
			}
			info.spineItems = append(info.spineItems, SpineItemref{IDRef: idref, Properties: props, Linear: linear})
		case "reference":
			var refType, refTitle, refHref string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "type":
					refType = attr.Value
				case "title":
					refTitle = attr.Value
				case "href":
					refHref = attr.Value
				}
			}
			info.guideRefs = append(info.guideRefs, GuideReference{
				Type: refType, Title: refTitle, Href: refHref,
			})
		case "meta":
			var prop, refines, val, metaID, scheme string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "property":
					prop = attr.Value
				case "refines":
					refines = attr.Value
				case "id":
					metaID = attr.Value
				case "scheme":
					scheme = attr.Value
				}
			}
			if scheme != "" {
				info.metaSchemes = append(info.metaSchemes, MetaScheme{Scheme: scheme, Property: prop})
			}
			// Track empty/invalid property attributes
			trimmedProp := strings.TrimSpace(prop)
			if trimmedProp == "" {
				info.metaEmptyProps++
			} else if strings.Contains(trimmedProp, " ") {
				info.metaListProps = append(info.metaListProps, prop)
			}
			if trimmedProp != "" {
				// Read the text content
				inner, _ := decoder.Token()
				if cd, ok := inner.(xml.CharData); ok {
					val = strings.TrimSpace(string(cd))
				}
				if val == "" {
					info.metaEmptyValues++
				}
				info.metas = append(info.metas, metaInfo{property: prop, value: val, refines: refines})
				if metaID != "" {
					info.metaIDs = append(info.metaIDs, metaID)
					info.metaIDToProperty[metaID] = prop
				}
				if refines != "" {
					info.metaRefines = append(info.metaRefines, MetaRefines{
						ID:       metaID,
						Refines:  refines,
						Property: prop,
						Value:    val,
					})
				}
			}
		case "link":
			var href, rel, mediaType, hreflang, linkRefines, linkProps string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "href":
					href = attr.Value
				case "rel":
					rel = attr.Value
				case "media-type":
					mediaType = attr.Value
				case "hreflang":
					hreflang = attr.Value
				case "refines":
					linkRefines = attr.Value
				case "properties":
					linkProps = attr.Value
				}
			}
			if href != "" || rel != "" {
				info.metadataLinks = append(info.metadataLinks, MetadataLink{
					Href:       href,
					Rel:        rel,
					MediaType:  mediaType,
					Hreflang:   hreflang,
					Refines:    linkRefines,
					Properties: linkProps,
				})
			}
		default:
			// Track unknown direct children of <package>
			if depth == 1 && !knownPackageChildren[se.Name.Local] {
				info.unknownElements = append(info.unknownElements, se.Name.Local)
			}
		}
		depth++
	}

	return info, nil
}

// parseMetadata parses dc: metadata elements from raw XML.
func parseMetadata(data []byte) Metadata {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var md Metadata
	md.IDToElement = make(map[string]string)
	inMetadata := false

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "metadata" {
				inMetadata = true
				continue
			}
			if !inMetadata {
				continue
			}

			// Capture id attribute from any DC element (for OPF-037 refines targets)
			dcID := ""
			for _, attr := range t.Attr {
				if attr.Name.Local == "id" {
					dcID = attr.Value
					break
				}
			}
			if dcID != "" {
				md.DCElementIDs = append(md.DCElementIDs, dcID)
				md.IDToElement[dcID] = t.Name.Local
			}

			switch t.Name.Local {
			case "title":
				id := dcID
				text := readElementText(decoder)
				md.Titles = append(md.Titles, DCTitle{ID: id, Value: text})
			case "identifier":
				id := dcID
				scheme := ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "scheme" {
						scheme = attr.Value
					}
				}
				val := readElementText(decoder)
				md.Identifiers = append(md.Identifiers, DCIdentifier{ID: id, Value: val, Scheme: scheme})
			case "language":
				text := readElementText(decoder)
				md.Languages = append(md.Languages, text)
			case "date":
				text := readElementText(decoder)
				md.Dates = append(md.Dates, text)
			case "source":
				if text := readElementText(decoder); text != "" {
					md.Sources = append(md.Sources, text)
				}
			case "creator":
				role := ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "role" {
						role = attr.Value
					}
				}
				val := readElementText(decoder)
				md.Creators = append(md.Creators, DCCreator{ID: dcID, Value: val, Role: role})
			case "contributor":
				role := ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "role" {
						role = attr.Value
					}
				}
				val := readElementText(decoder)
				md.Contributors = append(md.Contributors, DCCreator{ID: dcID, Value: val, Role: role})
			}
		case xml.EndElement:
			if t.Name.Local == "metadata" {
				inMetadata = false
			}
		}
	}

	return md
}

func readElementText(decoder *xml.Decoder) string {
	var text string
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.CharData:
			text += string(t)
		case xml.EndElement:
			return strings.TrimSpace(text)
		}
	}
	return strings.TrimSpace(text)
}

// parseManifestRaw parses manifest items from raw XML to detect missing attributes.
func parseManifestRaw(data []byte) ([]ManifestItem, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var items []ManifestItem
	inManifest := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "manifest" {
				inManifest = true
			}
			if inManifest && t.Name.Local == "item" {
				item := ManifestItem{}
				hasHref := false
				hasMediaType := false
				hasID := false
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "id":
						item.ID = attr.Value
						hasID = true
					case "href":
						item.Href = attr.Value
						hasHref = true
					case "media-type":
						item.MediaType = attr.Value
						hasMediaType = true
					case "properties":
						item.Properties = attr.Value
					case "fallback":
						item.Fallback = attr.Value
					case "fallback-style":
						item.FallbackStyle = attr.Value
					case "media-overlay":
						item.MediaOverlay = attr.Value
					}
				}
				item.HasID = hasID
				if !hasHref {
					item.Href = "\x00MISSING"
				}
				if !hasMediaType {
					item.MediaType = "\x00MISSING"
				}
				items = append(items, item)
			}
		case xml.EndElement:
			if t.Name.Local == "manifest" {
				inManifest = false
			}
		}
	}

	return items, nil
}

// OPFDir returns the directory containing the OPF file.
func (ep *EPUB) OPFDir() string {
	return path.Dir(ep.RootfilePath)
}

// ResolveHref resolves a relative href from the OPF file to a full path within the EPUB.
// Manifest hrefs are IRI-encoded (e.g. spaces as %20), but ZIP entry names use
// decoded forms, so we percent-decode before joining. Path is cleaned to normalize
// any ".." segments. Unicode characters are NFC-normalized so that NFD-encoded
// hrefs (e.g. u+combining-diaeresis) match NFC ZIP entry names.
func (ep *EPUB) ResolveHref(href string) string {
	decoded, err := url.PathUnescape(href)
	if err != nil {
		decoded = href // fall back to raw href if decoding fails
	}
	// Normalize to NFC so decomposed (NFD) hrefs match composed (NFC) ZIP entry names
	decoded = norm.NFC.String(decoded)
	dir := ep.OPFDir()
	if dir == "." {
		return path.Clean(decoded)
	}
	return path.Clean(dir + "/" + decoded)
}
