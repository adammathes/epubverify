package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
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
	XMLName   xml.Name     `xml:"container"`
	RootFiles rootFilesXML `xml:"rootfiles"`
}

type rootFilesXML struct {
	RootFile []rootFileXML `xml:"rootfile"`
}

type rootFileXML struct {
	FullPath  string `xml:"full-path,attr"`
	MediaType string `xml:"media-type,attr"`
}

// ParseContainer parses META-INF/container.xml and sets RootfilePath.
func (ep *EPUB) ParseContainer() error {
	data, err := ep.ReadFile("META-INF/container.xml")
	if err != nil {
		return err
	}

	var c containerXML
	if err := xml.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("parsing container.xml: %w", err)
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

	p := &Package{
		UniqueIdentifier: structInfo.uniqueIdentifier,
		Version:          structInfo.version,
		SpineToc:         structInfo.spineToc,
	}

	// Parse metadata if present
	if structInfo.hasMetadata {
		p.Metadata = parseMetadata(data)
	}

	// Parse rendition:layout from metadata metas
	for _, m := range structInfo.metas {
		if m.property == "dcterms:modified" {
			p.Metadata.Modified = m.value
		}
		if m.property == "rendition:layout" {
			p.RenditionLayout = m.value
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

	ep.Package = p
	return nil
}

type opfStructInfo struct {
	version          string
	uniqueIdentifier string
	hasMetadata      bool
	hasManifest      bool
	hasSpine         bool
	spineToc         string
	spineItems       []SpineItemref
	metas            []metaInfo
}

type metaInfo struct {
	property string
	value    string
}

// scanOPFStructure does a raw XML scan of the OPF to detect structural elements.
func scanOPFStructure(data []byte) (*opfStructInfo, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	info := &opfStructInfo{}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch se.Name.Local {
		case "package":
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "version":
					info.version = attr.Value
				case "unique-identifier":
					info.uniqueIdentifier = attr.Value
				}
			}
		case "metadata":
			info.hasMetadata = true
		case "manifest":
			info.hasManifest = true
		case "spine":
			info.hasSpine = true
			for _, attr := range se.Attr {
				if attr.Name.Local == "toc" {
					info.spineToc = attr.Value
				}
			}
		case "itemref":
			var idref string
			for _, attr := range se.Attr {
				if attr.Name.Local == "idref" {
					idref = attr.Value
				}
			}
			info.spineItems = append(info.spineItems, SpineItemref{IDRef: idref})
		case "meta":
			var prop, val string
			for _, attr := range se.Attr {
				if attr.Name.Local == "property" {
					prop = attr.Value
				}
			}
			if prop != "" {
				// Read the text content
				inner, _ := decoder.Token()
				if cd, ok := inner.(xml.CharData); ok {
					val = strings.TrimSpace(string(cd))
				}
				info.metas = append(info.metas, metaInfo{property: prop, value: val})
			}
		}
	}

	return info, nil
}

// parseMetadata parses dc: metadata elements from raw XML.
func parseMetadata(data []byte) Metadata {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var md Metadata
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
			switch t.Name.Local {
			case "title":
				if text := readElementText(decoder); text != "" {
					md.Titles = append(md.Titles, text)
				}
			case "identifier":
				id := ""
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
					}
				}
				val := readElementText(decoder)
				md.Identifiers = append(md.Identifiers, DCIdentifier{ID: id, Value: val})
			case "language":
				if text := readElementText(decoder); text != "" {
					md.Languages = append(md.Languages, text)
				}
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
func (ep *EPUB) ResolveHref(href string) string {
	dir := ep.OPFDir()
	if dir == "." {
		return href
	}
	return dir + "/" + href
}
