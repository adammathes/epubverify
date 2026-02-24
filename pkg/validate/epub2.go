package validate

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// checkEPUB2 runs EPUB 2 specific checks.
func checkEPUB2(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil || ep.Package.Version >= "3.0" {
		return
	}
	// Skip EPUB 2 checks for OEBPS 1.2 publications (OPF-001 was already emitted)
	if ep.IsLegacyOEBPS12 {
		return
	}

	// E2-004: spine toc attribute now checked in checkOPF (runs in both modes)

	// E2-005: EPUB 2 must not have nav property
	checkEPUB2NoNavProperty(ep, r)

	// E2-006: EPUB 2 must not have dcterms:modified
	checkEPUB2NoDCTermsModified(ep, r)

	// OPF-031: guide references must resolve
	checkEPUB2GuideRefs(ep, r)

	// OPF-050: spine toc attribute must point to NCX document
	// This also emits CHK-008 if toc references a non-NCX item, and returns false
	tocIsNCX := checkSpineTocToNCX(ep, r)

	// E2-001: NCX must be present
	// If toc attribute pointed to a non-NCX resource, skip NCX validation
	if ep.Package.SpineToc != "" && !tocIsNCX {
		return
	}
	ncxPath := findNCXPath(ep)
	if ncxPath == "" {
		return // Can't check further
	}

	ncxFullPath := ep.ResolveHref(ncxPath)
	if _, exists := ep.Files[ncxFullPath]; !exists {
		r.Add(report.Error, "E2-001",
			fmt.Sprintf("Referenced resource '%s' could not be found in the container", ncxPath))
		return
	}

	data, err := ep.ReadFile(ncxFullPath)
	if err != nil {
		return
	}

	// E2-002: NCX must be well-formed XML
	if !checkNCXWellFormed(data, r) {
		return
	}

	// E2-003: NCX must have navMap
	checkNCXHasNavMap(data, r)

	// E2-007: navPoint must have content element
	checkNCXNavPointContent(data, r)

	// E2-008: navPoint content src must resolve
	checkNCXContentSrcResolves(ep, data, ncxFullPath, r)

	// RSC-012: NCX fragment identifiers must exist in target documents
	checkNCXFragmentIdentifiers(ep, data, ncxFullPath, r)

	// E2-010: NCX uid must match OPF uid
	checkNCXUIDMatchesOPF(ep, data, r)

	// RSC-005: NCX IDs must be unique
	checkNCXUniqueIDs(data, r)

	// RSC-005: NCX id attribute values must be syntactically valid
	checkNCXIDSyntax(data, r)

	// RSC-005: NCX pageTarget type attribute must be valid
	checkNCXPageTargetType(data, r)

	// NCX-006: NCX text labels must not be empty (usage)
	checkNCXEmptyTextLabels(data, r)

	// RSC-010: NCX content src must point to an OPS document (not a foreign resource)
	checkNCXLinkToOPS(ep, data, ncxFullPath, r)

	// OPF-032: guide references to non-OPS resources
	checkEPUB2GuideToNonOPS(ep, r)

	// OPF-035: XHTML OPS document declared as text/html
	checkManifestItemXHTMLAsHTML(ep, r)

	// E2-012: guide reference type must be valid
	checkEPUB2GuideTypeValid(ep, r)

	// E2-013: dc:creator opf:role must be valid MARC relator
	checkEPUB2CreatorRoleValid(ep, r)

	// E2-014: OPF elements must appear in correct order
	checkEPUB2OPFElementOrder(ep, r)

	// E2-015: NCX depth metadata must match actual depth
	checkNCXDepthValid(data, r)
}

// RSC-005: EPUB 2 spine must have toc attribute (schema validation error)
func checkEPUB2SpineToc(ep *epub.EPUB, r *report.Report) {
	if !ep.HasSpine {
		return // Don't check toc attribute when spine element is missing
	}
	if ep.Package.SpineToc == "" {
		r.Add(report.Error, "RSC-005",
			`missing required attribute "toc"`)
	}
}

// E2-005: EPUB 2 must not have properties attribute on manifest items
func checkEPUB2NoNavProperty(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Properties != "" {
			r.Add(report.Error, "E2-005",
				fmt.Sprintf("Manifest item properties attribute '%s' is not allowed in EPUB 2", item.Properties))
		}
	}
}

// E2-006: EPUB 2 must not have dcterms:modified meta with property attribute
func checkEPUB2NoDCTermsModified(ep *epub.EPUB, r *report.Report) {
	if ep.Package.ModifiedCount > 0 {
		r.Add(report.Error, "E2-006",
			"The 'property' attribute on meta element is not allowed in EPUB 2")
	}
}

// OPF-031: guide references must resolve
func checkEPUB2GuideRefs(ep *epub.EPUB, r *report.Report) {
	manifestHrefs := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestHrefs[ep.ResolveHref(item.Href)] = true
		}
	}

	for _, ref := range ep.Package.Guide {
		if ref.Href == "" {
			continue
		}
		u, err := url.Parse(ref.Href)
		if err != nil {
			continue
		}
		target := ep.ResolveHref(u.Path)
		if !manifestHrefs[target] {
			r.Add(report.Error, "OPF-031",
				fmt.Sprintf("Guide reference '%s' is not declared in OPF manifest", ref.Href))
		}
		// Also check if the file exists in the container
		if _, exists := ep.Files[target]; !exists {
			r.Add(report.Error, "RSC-007",
				fmt.Sprintf("Referenced resource '%s' could not be found in the container", ref.Href))
		}
	}
}


// OPF-032: guide references must not point to non-OPS resources (e.g., images)
var guideOPSTypes = map[string]bool{
	"application/xhtml+xml": true,
	"application/x-dtbook+xml": true,
}

func checkEPUB2GuideToNonOPS(ep *epub.EPUB, r *report.Report) {
	// Build map of manifest items by resolved path and their media types
	manifestByPath := make(map[string]string) // resolved path → media-type
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" && item.Href != "" {
			manifestByPath[ep.ResolveHref(item.Href)] = item.MediaType
		}
	}

	for _, ref := range ep.Package.Guide {
		if ref.Href == "" {
			continue
		}
		u, err := url.Parse(ref.Href)
		if err != nil {
			continue
		}
		target := ep.ResolveHref(u.Path)
		mediaType, exists := manifestByPath[target]
		if !exists {
			continue // already reported by OPF-031
		}
		if !guideOPSTypes[mediaType] {
			r.Add(report.Error, "OPF-032",
				fmt.Sprintf("Guide reference '%s' points to a non-OPS resource of type '%s'", ref.Href, mediaType))
		}
	}
}

// OPF-035: EPUB 2 manifest item with media-type text/html should be application/xhtml+xml
func checkManifestItemXHTMLAsHTML(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.MediaType == "text/html" {
			r.Add(report.Warning, "OPF-035",
				fmt.Sprintf("Manifest item '%s' is declared as text/html but EPUB 2 OPS documents must use application/xhtml+xml", item.Href))
		}
	}
}

// OPF-050: EPUB 2 spine toc attribute must point to an NCX document.
// Returns true if toc points to a valid NCX document (or is absent).
func checkSpineTocToNCX(ep *epub.EPUB, r *report.Report) bool {
	if ep.Package.SpineToc == "" {
		return false // RSC-005 already covers missing toc attribute
	}
	// Find the manifest item referenced by the toc attribute
	for _, item := range ep.Package.Manifest {
		if item.ID == ep.Package.SpineToc {
			if item.MediaType != "application/x-dtbncx+xml" {
				r.Add(report.Error, "OPF-050",
					fmt.Sprintf("The 'toc' attribute value '%s' must reference the NCX document", ep.Package.SpineToc))
				r.Add(report.Error, "CHK-008",
					fmt.Sprintf("Skipping checks for item '%s' (not a valid NCX document)", ep.Package.SpineToc))
				return false // Not NCX, skip NCX validation
			}
			return true // Valid NCX reference
		}
	}
	// Not found in manifest - already caught by other checks
	return false
}

// findNCXPath finds the NCX file path from the manifest.
func findNCXPath(ep *epub.EPUB) string {
	// First try the spine toc attribute
	if ep.Package.SpineToc != "" {
		for _, item := range ep.Package.Manifest {
			if item.ID == ep.Package.SpineToc {
				return item.Href
			}
		}
	}

	// Fallback: find by media-type
	for _, item := range ep.Package.Manifest {
		if item.MediaType == "application/x-dtbncx+xml" {
			return item.Href
		}
	}

	return ""
}

// E2-002: NCX must be well-formed XML
func checkNCXWellFormed(data []byte, r *report.Report) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			r.Add(report.Fatal, "E2-002",
				"NCX document is not well-formed: XML document structures must start and end within the same entity")
			return false
		}
	}
	return true
}

// E2-003: NCX must contain navMap
func checkNCXHasNavMap(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	hasNavMap := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "navMap" {
				hasNavMap = true
				break
			}
		}
	}

	if !hasNavMap {
		r.Add(report.Error, "E2-003",
			"NCX document is missing required element 'navMap'")
	}
}

// E2-007: navPoint elements must have a content child element.
// Uses a stack to correctly handle nested navPoint elements.
func checkNCXNavPointContent(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	// Stack tracks whether each navPoint level has seen a <content> child
	var hasContentStack []bool

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "navPoint" {
				hasContentStack = append(hasContentStack, false)
			}
			if t.Name.Local == "content" && len(hasContentStack) > 0 {
				hasContentStack[len(hasContentStack)-1] = true
			}
		case xml.EndElement:
			if t.Name.Local == "navPoint" && len(hasContentStack) > 0 {
				if !hasContentStack[len(hasContentStack)-1] {
					r.Add(report.Error, "E2-007",
						"NCX navPoint element is incomplete: missing required child element 'content'")
				}
				hasContentStack = hasContentStack[:len(hasContentStack)-1]
			}
		}
	}
}


// RSC-012: NCX content src fragment identifiers must exist in target documents
func checkNCXFragmentIdentifiers(ep *epub.EPUB, data []byte, ncxFullPath string, r *report.Report) {
	ncxDir := path.Dir(ncxFullPath)
	decoder := xml.NewDecoder(strings.NewReader(string(data)))

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "content" {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local != "src" || attr.Value == "" {
				continue
			}
			u, err := url.Parse(attr.Value)
			if err != nil || u.Scheme != "" || u.Fragment == "" {
				continue
			}
			// We have a fragment reference - check if the target file has that ID
			targetPath := resolvePath(ncxDir, u.Path)
			if u.Path == "" {
				targetPath = ncxFullPath
			}
			targetData, readErr := ep.ReadFile(targetPath)
			if readErr != nil {
				continue // File missing handled by RSC-007
			}
			if !documentHasID(targetData, u.Fragment) {
				r.Add(report.Error, "RSC-012",
					fmt.Sprintf("Fragment identifier is not defined in '%s'", attr.Value))
			}
		}
	}
}

// documentHasID returns true if the XML/HTML document has an element with the given id attribute.
func documentHasID(data []byte, id string) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "id" && attr.Value == id {
				return true
			}
		}
	}
	return false
}

// RSC-007: navPoint content src must point to an existing resource
func checkNCXContentSrcResolves(ep *epub.EPUB, data []byte, ncxFullPath string, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	ncxDir := path.Dir(ncxFullPath)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "content" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" && attr.Value != "" {
					u, err := url.Parse(attr.Value)
					if err != nil || u.Scheme != "" {
						continue
					}
					target := resolvePath(ncxDir, u.Path)
					if _, exists := ep.Files[target]; !exists {
						r.Add(report.Error, "RSC-007",
							fmt.Sprintf("Referenced resource '%s' could not be found in the container", attr.Value))
					}
				}
			}
		}
	}
}

// NCX-001: NCX dtb:uid must match the OPF dc:identifier
func checkNCXUIDMatchesOPF(ep *epub.EPUB, data []byte, r *report.Report) {
	// Find OPF unique-identifier value
	opfUID := ""
	if ep.Package.UniqueIdentifier != "" {
		for _, id := range ep.Package.Metadata.Identifiers {
			if id.ID == ep.Package.UniqueIdentifier {
				opfUID = id.Value
				break
			}
		}
	}
	if opfUID == "" {
		return
	}

	// Find NCX dtb:uid
	ncxUID := ""
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "meta" {
			var name, content string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "name":
					name = attr.Value
				case "content":
					content = attr.Value
				}
			}
			if name == "dtb:uid" {
				ncxUID = content
				break
			}
		}
	}

	if ncxUID != "" && strings.TrimSpace(ncxUID) != strings.TrimSpace(opfUID) {
		r.Add(report.Error, "NCX-001",
			fmt.Sprintf("NCX identifier '%s' does not match OPF identifier '%s'", ncxUID, opfUID))
	}
}

// RSC-005: NCX id attributes must be unique (schema validation)
func checkNCXUniqueIDs(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	idCount := make(map[string]int)

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "id" {
				idCount[attr.Value]++
			}
		}
	}
	// Report all occurrences of duplicated IDs
	for _, count := range idCount {
		if count > 1 {
			for i := 0; i < count; i++ {
				r.Add(report.Error, "RSC-005",
					`The "id" attribute does not have a unique value`)
			}
		}
	}
}

// isValidNCXID checks whether an NCX id attribute value is syntactically valid.
// NCX ids must be valid XML NCNames (no colons, starts with letter or underscore).
func isValidNCXID(id string) bool {
	if id == "" {
		return false
	}
	for i, c := range id {
		if i == 0 {
			if !(c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c > 0x7F) {
				return false
			}
		} else {
			if !(c == '_' || c == '-' || c == '.' || (c >= '0' && c <= '9') ||
				(c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c > 0x7F) {
				return false
			}
		}
	}
	return true
}

// RSC-005: NCX id attribute values must be syntactically valid XML NCNames
func checkNCXIDSyntax(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "id" && !isValidNCXID(attr.Value) {
				r.Add(report.Error, "RSC-005",
					`value of attribute "id" is invalid`)
				break
			}
		}
	}
}

// RSC-005: NCX pageTarget type attribute must be "front", "normal", or "special"
func checkNCXPageTargetType(data []byte, r *report.Report) {
	validTypes := map[string]bool{"front": true, "normal": true, "special": true}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "pageTarget" {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "type" && attr.Value != "" && !validTypes[attr.Value] {
				r.Add(report.Error, "RSC-005",
					`value of attribute "type" is invalid`)
			}
		}
	}
}

// OPS content document media types
var opsMediaTypes = map[string]bool{
	"application/xhtml+xml": true,
	"application/x-dtbncx+xml": true,
}

// RSC-010: NCX navPoint content src must point to an OPS content document, not a foreign resource
func checkNCXLinkToOPS(ep *epub.EPUB, data []byte, ncxFullPath string, r *report.Report) {
	// Build map of OPS document paths from manifest
	opsPaths := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.MediaType == "application/xhtml+xml" || item.MediaType == "application/x-dtbook+xml" {
			opsPaths[ep.ResolveHref(item.Href)] = true
		}
	}

	ncxDir := path.Dir(ncxFullPath)
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "content" {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "src" && attr.Value != "" {
				u, err := url.Parse(attr.Value)
				if err != nil || u.Scheme != "" {
					continue
				}
				target := resolvePath(ncxDir, u.Path)
				// Only check if target exists (missing resources caught by RSC-007)
				if _, exists := ep.Files[target]; !exists {
					continue
				}
				// Check if the target is an OPS document
				if !opsPaths[target] {
					r.Add(report.Error, "RSC-010",
						fmt.Sprintf("The media type of '%s' is not an OPS Core Media Type", attr.Value))
				}
			}
		}
	}
}

// Valid EPUB 2 guide reference types
var validGuideTypes = map[string]bool{
	"cover":                true,
	"title-page":          true,
	"toc":                 true,
	"index":               true,
	"glossary":            true,
	"acknowledgements":    true,
	"bibliography":        true,
	"colophon":            true,
	"copyright-page":      true,
	"dedication":          true,
	"epigraph":            true,
	"foreword":            true,
	"loi":                 true,
	"lot":                 true,
	"notes":               true,
	"preface":             true,
	"text":                true,
}

// E2-012: EPUB 2 guide reference type must be valid
func checkEPUB2GuideTypeValid(ep *epub.EPUB, r *report.Report) {
	for _, ref := range ep.Package.Guide {
		if ref.Type == "" {
			continue
		}
		// Allow "other." prefix for custom types
		if strings.HasPrefix(ref.Type, "other.") {
			continue
		}
		if !validGuideTypes[ref.Type] {
			r.Add(report.Warning, "E2-012",
				fmt.Sprintf("Guide reference type '%s' is not a recognized value", ref.Type))
		}
	}
}

// Valid MARC relator codes (common subset)
var validMARCRelators = map[string]bool{
	"act": true, "adp": true, "anl": true, "ann": true, "ant": true, "app": true,
	"arc": true, "arr": true, "art": true, "asg": true, "asn": true, "att": true,
	"auc": true, "aud": true, "aui": true, "aus": true, "aut": true, "bdd": true,
	"bjd": true, "bkd": true, "bkp": true, "blw": true, "bnd": true, "bpd": true,
	"bsl": true, "ccp": true, "chr": true, "cli": true, "cll": true, "clr": true,
	"clt": true, "cmm": true, "cmp": true, "cmt": true, "cnd": true, "cng": true,
	"cns": true, "coe": true, "col": true, "com": true, "cos": true, "cot": true,
	"cov": true, "cpc": true, "cpe": true, "cph": true, "cpl": true, "cpt": true,
	"cre": true, "crp": true, "crr": true, "csl": true, "csp": true, "cst": true,
	"ctb": true, "cte": true, "ctg": true, "ctr": true, "cts": true, "ctt": true,
	"cur": true, "cwt": true, "dfd": true, "dfe": true, "dft": true, "dgg": true,
	"dis": true, "dln": true, "dnc": true, "dnr": true, "dpc": true, "dpt": true,
	"drm": true, "drt": true, "dsr": true, "dst": true, "dtc": true, "dte": true,
	"dtm": true, "dto": true, "dub": true, "edt": true, "egr": true, "elg": true,
	"elt": true, "eng": true, "etr": true, "evp": true, "exp": true, "fac": true,
	"fld": true, "flm": true, "fmo": true, "fnd": true, "fpy": true, "frg": true,
	"gis": true, "grt": true, "hnr": true, "hst": true, "ill": true, "ilu": true,
	"ins": true, "inv": true, "itr": true, "ive": true, "ivr": true, "lbr": true,
	"lbt": true, "lee": true, "lel": true, "len": true, "let": true, "lgd": true,
	"lie": true, "lil": true, "lit": true, "lsa": true, "lse": true, "lso": true,
	"ltg": true, "lyr": true, "mcp": true, "mdc": true, "mfp": true, "mfr": true,
	"mod": true, "mon": true, "mrb": true, "mrk": true, "msd": true, "mte": true,
	"mus": true, "nrt": true, "opn": true, "org": true, "orm": true, "oth": true,
	"own": true, "pat": true, "pbd": true, "pbl": true, "pdr": true, "pfr": true,
	"pht": true, "plt": true, "pma": true, "pmn": true, "pop": true, "ppm": true,
	"ppt": true, "prc": true, "prd": true, "prf": true, "prg": true, "prm": true,
	"pro": true, "prt": true, "pta": true, "pte": true, "ptf": true, "pth": true,
	"ptt": true, "rbr": true, "rce": true, "rcp": true, "red": true, "ren": true,
	"res": true, "rev": true, "rps": true, "rpt": true, "rpy": true, "rse": true,
	"rsg": true, "rsp": true, "rst": true, "rth": true, "rtm": true, "sad": true,
	"sce": true, "scl": true, "scr": true, "sds": true, "sec": true, "sgn": true,
	"sht": true, "sng": true, "spk": true, "spn": true, "spy": true, "srv": true,
	"std": true, "stl": true, "stm": true, "stn": true, "str": true, "tcd": true,
	"tch": true, "ths": true, "tld": true, "tlp": true, "trc": true, "trl": true,
	"tyd": true, "tyg": true, "uvp": true, "vac": true, "vdg": true, "wac": true,
	"wal": true, "wam": true, "wat": true, "wdc": true, "wde": true, "win": true,
	"wit": true, "wpr": true, "wst": true,
}

// E2-013: dc:creator opf:role must be a valid MARC relator code
func checkEPUB2CreatorRoleValid(ep *epub.EPUB, r *report.Report) {
	for _, creator := range ep.Package.Metadata.Creators {
		if creator.Role == "" {
			continue
		}
		if !validMARCRelators[creator.Role] {
			r.Add(report.Error, "E2-013",
				fmt.Sprintf("Role value '%s' is not valid", creator.Role))
		}
	}
}

// E2-014: EPUB 2 OPF elements must appear in correct order (metadata, manifest, spine, guide)
func checkEPUB2OPFElementOrder(ep *epub.EPUB, r *report.Report) {
	expectedOrder := []string{"metadata", "manifest", "spine", "guide"}
	orderIdx := make(map[string]int)
	for i, name := range expectedOrder {
		orderIdx[name] = i
	}

	lastIdx := -1
	for _, elem := range ep.Package.ElementOrder {
		idx, ok := orderIdx[elem]
		if !ok {
			continue
		}
		if idx < lastIdx {
			r.Add(report.Error, "E2-014",
				fmt.Sprintf("Element '%s' not allowed yet; missing required element in correct order", elem))
		}
		lastIdx = idx
	}
}

// E2-015: NCX dtb:depth metadata must match actual navigation depth
func checkNCXDepthValid(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	declaredDepth := ""
	actualDepth := 0
	currentDepth := 0

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "meta" {
				var name, content string
				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "name":
						name = attr.Value
					case "content":
						content = attr.Value
					}
				}
				if name == "dtb:depth" {
					declaredDepth = content
				}
			}
			if t.Name.Local == "navPoint" {
				currentDepth++
				if currentDepth > actualDepth {
					actualDepth = currentDepth
				}
			}
		case xml.EndElement:
			if t.Name.Local == "navPoint" {
				currentDepth--
			}
		}
	}

	if declaredDepth != "" && declaredDepth != fmt.Sprintf("%d", actualDepth) {
		r.Add(report.Warning, "E2-015",
			fmt.Sprintf("NCX declared depth '%s' does not match actual navigation depth '%d'", declaredDepth, actualDepth))
	}
}

// checkLegacyNCXForAll runs NCX fragment validation for any EPUB (2 or 3) that has an NCX.
// This is separate from checkEPUB2 which skips for EPUB 3 publications.
func checkLegacyNCXForAll(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil {
		return
	}
	// Skip EPUB 2 - already handled by checkEPUB2
	if ep.Package.Version < "3.0" {
		return
	}
	// Find the NCX document in the manifest
	ncxPath := findNCXPath(ep)
	if ncxPath == "" {
		return
	}
	ncxFullPath := ep.ResolveHref(ncxPath)
	if _, exists := ep.Files[ncxFullPath]; !exists {
		return
	}
	data, err := ep.ReadFile(ncxFullPath)
	if err != nil {
		return
	}
	// RSC-012: NCX fragment identifiers must exist in target documents
	checkNCXFragmentIdentifiers(ep, data, ncxFullPath, r)
}

// NCX-006: NCX text labels (docTitle/navLabel) must not be empty (usage)
func checkNCXEmptyTextLabels(data []byte, r *report.Report) {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	inTextLabel := false // inside docTitle or navLabel
	inText := false
	textHasContent := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			if local == "docTitle" || local == "navLabel" {
				inTextLabel = true
			}
			if inTextLabel && local == "text" {
				inText = true
				textHasContent = false
			}
		case xml.EndElement:
			local := t.Name.Local
			if local == "text" && inText {
				if !textHasContent {
					r.Add(report.Usage, "NCX-006", "NCX text label is empty")
				}
				inText = false
			}
			if local == "docTitle" || local == "navLabel" {
				inTextLabel = false
				inText = false
			}
		case xml.CharData:
			if inText && strings.TrimSpace(string(t)) != "" {
				textHasContent = true
			}
		}
	}
}

