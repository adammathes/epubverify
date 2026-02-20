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
	XMLName   xml.Name   `xml:"container"`
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

// OPF XML types

type packageXML struct {
	XMLName          xml.Name    `xml:"package"`
	UniqueIdentifier string      `xml:"unique-identifier,attr"`
	Version          string      `xml:"version,attr"`
	Metadata         metadataXML `xml:"metadata"`
	Manifest         manifestXML `xml:"manifest"`
	Spine            spineXML    `xml:"spine"`
}

type metadataXML struct {
	Titles      []string          `xml:"title"`
	Identifiers []dcIdentifierXML `xml:"identifier"`
	Languages   []string          `xml:"language"`
	Metas       []metaXML         `xml:"meta"`
}

type dcIdentifierXML struct {
	ID    string `xml:"id,attr"`
	Value string `xml:",chardata"`
}

type metaXML struct {
	Property string `xml:"property,attr"`
	Value    string `xml:",chardata"`
}

type manifestXML struct {
	Items []itemXML `xml:"item"`
}

type itemXML struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type spineXML struct {
	ItemRefs []itemrefXML `xml:"itemref"`
}

type itemrefXML struct {
	IDRef string `xml:"idref,attr"`
}

// ParseOPF parses the OPF package document and populates ep.Package.
func (ep *EPUB) ParseOPF() error {
	if ep.RootfilePath == "" {
		return fmt.Errorf("no rootfile path set")
	}

	data, err := ep.ReadFile(ep.RootfilePath)
	if err != nil {
		return err
	}

	var pkg packageXML
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("parsing OPF: %w", err)
	}

	p := &Package{
		UniqueIdentifier: pkg.UniqueIdentifier,
		Version:          pkg.Version,
	}

	// Metadata
	p.Metadata.Titles = pkg.Metadata.Titles
	for _, id := range pkg.Metadata.Identifiers {
		p.Metadata.Identifiers = append(p.Metadata.Identifiers, DCIdentifier{
			ID:    id.ID,
			Value: id.Value,
		})
	}
	p.Metadata.Languages = pkg.Metadata.Languages

	for _, m := range pkg.Metadata.Metas {
		if m.Property == "dcterms:modified" {
			p.Metadata.Modified = m.Value
		}
	}

	// Manifest - parse raw XML to detect missing attributes
	rawItems, err := parseManifestRaw(data)
	if err != nil {
		return err
	}
	p.Manifest = rawItems

	// Spine
	for _, ref := range pkg.Spine.ItemRefs {
		p.Spine = append(p.Spine, SpineItemref{IDRef: ref.IDRef})
	}

	ep.Package = p
	return nil
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
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "id":
						item.ID = attr.Value
					case "href":
						item.Href = attr.Value
						hasHref = true
					case "media-type":
						item.MediaType = attr.Value
						hasMediaType = true
					case "properties":
						item.Properties = attr.Value
					}
				}
				// Use sentinel values to indicate missing attributes
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
