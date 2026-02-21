package validate

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// UTF-16 BOM markers
var utf16LEBOM = []byte{0xff, 0xfe}
var utf16BEBOM = []byte{0xfe, 0xff}

// checkEncoding validates encoding of content documents.
// Returns a set of full paths that have encoding errors (should be skipped by content checks).
func checkEncoding(ep *epub.EPUB, r *report.Report) map[string]bool {
	badEncoding := make(map[string]bool)
	if ep.Package == nil {
		return badEncoding
	}

	xmlEncodingRe := regexp.MustCompile(`<\?xml[^?]*encoding=["']([^"']+)["']`)

	for _, item := range ep.Package.Manifest {
		if item.MediaType != "application/xhtml+xml" {
			continue
		}
		if item.Href == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		data, err := ep.ReadFile(fullPath)
		if err != nil {
			continue
		}

		// ENC-002: check for UTF-16 BOM
		if bytes.HasPrefix(data, utf16LEBOM) || bytes.HasPrefix(data, utf16BEBOM) {
			r.AddWithLocation(report.Error, "ENC-002",
				fmt.Sprintf("Content document '%s' must be encoded in UTF-8, but appears to be UTF-16", item.Href),
				fullPath)
			badEncoding[fullPath] = true
			continue
		}

		// ENC-001: check XML encoding declaration
		// Look for encoding attribute in XML declaration
		header := string(data[:min(200, len(data))])
		if matches := xmlEncodingRe.FindStringSubmatch(header); len(matches) > 1 {
			enc := strings.ToUpper(matches[1])
			if enc != "UTF-8" {
				r.AddWithLocation(report.Error, "ENC-001",
					fmt.Sprintf("Content document '%s' must be encoded in UTF-8, but declares encoding '%s'", item.Href, matches[1]),
					fullPath)
				badEncoding[fullPath] = true
			}
		}
	}
	return badEncoding
}
