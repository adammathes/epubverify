package validate

import (
	"encoding/xml"
	"io"
	"strings"
)

// navDocHasToc checks whether the given XHTML document contains
// a <nav> element with epub:type="toc".
func navDocHasToc(data []byte) bool {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "nav" {
				for _, attr := range se.Attr {
					if attr.Name.Local == "type" && containsToken(attr.Value, "toc") {
						return true
					}
				}
			}
		}
	}
	return false
}

func containsToken(s, token string) bool {
	for _, t := range strings.Fields(s) {
		if t == token {
			return true
		}
	}
	return false
}
