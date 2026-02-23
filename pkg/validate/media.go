package validate

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// EPUB 3 core media types
var coreMediaTypes = map[string]bool{
	"image/gif":                    true,
	"image/jpeg":                   true,
	"image/png":                    true,
	"image/svg+xml":                true,
	"image/webp":                   true,
	"application/xhtml+xml":        true,
	"application/x-dtbncx+xml":     true,
	"text/css":                     true,
	"application/javascript":       true,
	"text/javascript":              true,
	"font/woff":                    true,
	"font/woff2":                   true,
	"font/otf":                     true,
	"font/ttf":                     true,
	"application/font-woff":        true,
	"application/font-sfnt":        true,
	"application/vnd.ms-opentype":  true,
	"audio/mpeg":                   true,
	"audio/mp4":                    true,
	"audio/ogg":                    true,
	"video/mp4":                    true,
	"video/h264":                   true,
	"application/smil+xml":         true,
	"application/pls+xml":          true,
}

// isFontMediaType returns true if the media type is a font type (core or foreign)
func isFontMediaType(mt string) bool {
	return strings.HasPrefix(mt, "font/") ||
		mt == "application/font-woff" ||
		mt == "application/font-sfnt" ||
		mt == "application/vnd.ms-opentype" ||
		mt == "application/x-font-woff" ||
		mt == "application/x-font-ttf" ||
		mt == "application/x-font-truetype" ||
		mt == "application/x-font-opentype" ||
		strings.Contains(mt, "font")
}

// acceptedFontTypeAliases are non-core font media types that are widely accepted
// aliases for standard font formats (e.g. application/x-font-ttf for TrueType).
// These should NOT trigger CSS-007 because they are effectively equivalent to
// core font types, just using older/alternate naming conventions.
var acceptedFontTypeAliases = map[string]bool{
	"font/otf":                    true,
	"font/ttf":                    true,
	"font/woff":                   true,
	"font/woff2":                  true,
	"application/font-woff":       true,
	"application/font-woff2":      true,
	"application/vnd.ms-opentype": true,
	"application/font-sfnt":       true,
	"application/x-font-ttf":      true,
	"application/x-font-opentype": true,
	"application/x-font-truetype": true,
}

// Core image media types per EPUB spec
var coreImageTypes = map[string]bool{
	"image/gif":     true,
	"image/jpeg":    true,
	"image/png":     true,
	"image/svg+xml": true,
}

// Core video media types per EPUB spec
var coreVideoTypes = map[string]bool{
	"video/mp4":  true,
	"video/h264": true,
	"video/webm": true,
}

// Image magic bytes for type detection
var pngMagic = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
var jpegMagic = []byte{0xff, 0xd8, 0xff}
var gifMagic = []byte{0x47, 0x49, 0x46, 0x38}

// checkMedia validates media files.
func checkMedia(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil {
		return
	}

	// Build set of spine item IDs for MED-004 check
	spineItemIDs := make(map[string]bool)
	for _, ref := range ep.Package.Spine {
		spineItemIDs[ref.IDRef] = true
	}

	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.MediaType == "\x00MISSING" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)
		if _, exists := ep.Files[fullPath]; !exists {
			continue
		}

		// For non-core image types (foreign resources), skip all image checks.
		// Foreign image resources are valid per EPUB spec when they have fallbacks
		// (either via manifest fallback attribute or HTML fallback like <picture>/<object>).
		isNonCoreImage := strings.HasPrefix(item.MediaType, "image/") && !coreImageTypes[item.MediaType]

		// MED-002: images should use core media types
		// Only warn for spine items — non-spine foreign images are expected
		// to be handled via fallback mechanisms (manifest fallback,
		// <picture>/<object> elements).
		if isNonCoreImage && spineItemIDs[item.ID] && item.Fallback == "" {
			r.Add(report.Warning, "MED-002",
				fmt.Sprintf("Image '%s' uses non-core media type '%s'; prefer JPEG, PNG, GIF, or SVG", item.Href, item.MediaType))
		}

		// MED-012: video resources should use core media types
		if strings.HasPrefix(item.MediaType, "video/") && !coreVideoTypes[item.MediaType] {
			r.Add(report.Warning, "MED-012",
				fmt.Sprintf("Video '%s' uses non-core media type '%s'; prefer MP4 or WebM", item.Href, item.MediaType))
		}

		// OPF-029: image media type must match actual content
		// MED-004: image must not be corrupted
		// PKG-021: image file must not be empty
		// Only check core image types — foreign image resources may have
		// arbitrary binary formats that don't match known magic bytes.
		if strings.HasPrefix(item.MediaType, "image/") && item.MediaType != "image/svg+xml" && !isNonCoreImage {
			mismatch := checkImageMediaType(ep, item, fullPath, r)
			// Only check for corruption if media type matches (mismatch is a different problem)
			if !mismatch {
				checkImageNotCorrupted(ep, item, fullPath, r)
			}
		}

		// Foreign resources check: per EPUB 3 spec, foreign resources
		// referenced from content documents need fallbacks. However, many types
		// are exempt: fonts, video, tracks, linked resources, and unreferenced items.
		// We skip the broad manifest-level MED-004/MED-005 here because the
		// content-level RSC-032 check handles this more precisely.
		// Only flag truly non-exempt foreign resources that have no manifest fallback.

		// MED-006 through MED-011: media overlay SMIL checks
		if item.MediaType == "application/smil+xml" && ep.Package.Version >= "3.0" {
			checkMediaOverlay(ep, item, fullPath, r)
		}
	}

	// MED-009: media overlay items must have duration metadata
	if ep.Package.Version >= "3.0" {
		checkMediaOverlayDuration(ep, r)
	}

	// MED-013: media-overlay property must reference valid SMIL
	if ep.Package.Version >= "3.0" {
		checkMediaOverlayProperty(ep, r)
	}
}

// OPF-029: verify image file type matches declared media type in manifest.
// Returns true if a mismatch was detected.
func checkImageMediaType(ep *epub.EPUB, item epub.ManifestItem, fullPath string, r *report.Report) bool {
	data, err := ep.ReadFile(fullPath)
	if err != nil || len(data) < 8 {
		return false
	}

	detected := detectImageType(data)
	if detected == "" {
		return false
	}

	if detected != item.MediaType {
		r.Add(report.Error, "OPF-029",
			fmt.Sprintf("The file '%s' does not appear to match the declared media type '%s'", item.Href, item.MediaType))
		return true
	}
	return false
}

// MED-004: verify image is not corrupted; PKG-021: image file must not be empty.
func checkImageNotCorrupted(ep *epub.EPUB, item epub.ManifestItem, fullPath string, r *report.Report) {
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return
	}

	if len(data) == 0 {
		r.Add(report.Error, "PKG-021",
			fmt.Sprintf("The image file '%s' is empty", item.Href))
		r.Add(report.Error, "MED-004",
			fmt.Sprintf("Corrupted image file '%s': the file is empty", item.Href))
		return
	}

	if len(data) < 8 {
		r.Add(report.Error, "MED-004",
			fmt.Sprintf("Corrupted image file '%s': file too small", item.Href))
		return
	}

	// Check for valid magic bytes
	switch item.MediaType {
	case "image/png":
		if !bytes.HasPrefix(data, pngMagic) {
			r.Add(report.Error, "MED-004",
				fmt.Sprintf("Corrupted image file '%s': invalid PNG header", item.Href))
		}
	case "image/jpeg":
		if !bytes.HasPrefix(data, jpegMagic) {
			r.Add(report.Error, "MED-004",
				fmt.Sprintf("Corrupted image file '%s': invalid JPEG header", item.Href))
		}
	case "image/gif":
		if !bytes.HasPrefix(data, gifMagic) {
			r.Add(report.Error, "MED-004",
				fmt.Sprintf("Corrupted image file '%s': invalid GIF header", item.Href))
		}
	}
}

func detectImageType(data []byte) string {
	if bytes.HasPrefix(data, pngMagic) {
		return "image/png"
	}
	if bytes.HasPrefix(data, jpegMagic) {
		return "image/jpeg"
	}
	if bytes.HasPrefix(data, gifMagic) {
		return "image/gif"
	}
	return ""
}

func foreignResourceCheckID(mediaType string) string {
	if strings.HasPrefix(mediaType, "audio/") {
		return "MED-005"
	}
	return "MED-004"
}

// checkMediaOverlay validates a SMIL media overlay document.
func checkMediaOverlay(ep *epub.EPUB, item epub.ManifestItem, fullPath string, r *report.Report) {
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return
	}

	// MED-006: SMIL must be well-formed XML
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	var tokens []xml.Token
	wellFormed := true
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			r.AddWithLocation(report.Fatal, "MED-006",
				fmt.Sprintf("Media overlay document is not well-formed: element must be followed by either attribute specifications or end-tag (%s)", err.Error()),
				fullPath)
			r.AddWithLocation(report.Error, "MED-006",
				fmt.Sprintf("Media overlay validation aborted due to XML error in '%s'", fullPath),
				fullPath)
			wellFormed = false
			break
		}
		tokens = append(tokens, xml.CopyToken(tok))
	}

	if !wellFormed {
		return
	}

	smilDir := path.Dir(fullPath)

	// Parse SMIL structure
	var inBody, inPar, inSeq bool
	hasBody := false
	for _, tok := range tokens {
		se, ok := tok.(xml.StartElement)
		if !ok {
			if ee, ok := tok.(xml.EndElement); ok {
				if ee.Name.Local == "body" {
					inBody = false
				}
				if ee.Name.Local == "par" {
					inPar = false
				}
				if ee.Name.Local == "seq" {
					inSeq = false
				}
			}
			continue
		}

		switch se.Name.Local {
		case "body":
			hasBody = true
			inBody = true
		case "seq":
			inSeq = true
		case "par":
			inPar = true
		case "audio":
			if inBody {
				checkSMILAudio(ep, se, smilDir, fullPath, r)
			}
			// MED-011: audio must be inside par, not directly in body/seq
			if inBody && !inPar {
				r.AddWithLocation(report.Error, "MED-011",
					"Element 'audio' not allowed here; expected element inside 'par'",
					fullPath)
			}
		case "text":
			if inBody {
				checkSMILText(ep, se, smilDir, fullPath, r)
			}
			// MED-011: text must be inside par, not directly in body/seq
			if inBody && !inPar {
				r.AddWithLocation(report.Error, "MED-011",
					"Element 'text' not allowed here; expected element inside 'par'",
					fullPath)
			}
		default:
			// MED-011: unexpected elements in body
			if inBody && !inPar && !inSeq && se.Name.Local != "smil" {
				r.AddWithLocation(report.Error, "MED-011",
					fmt.Sprintf("Element '%s' not allowed here; expected element 'par' or 'seq'", se.Name.Local),
					fullPath)
			}
		}
	}

	_ = hasBody

	// OPF-014: Check if media overlay has remote resources and content doc needs remote-resources property
	hasRemoteInOverlay := false
	for _, tok := range tokens {
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "audio" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "src" {
					if isRemoteURL(attr.Value) {
						hasRemoteInOverlay = true
					}
				}
			}
		}
	}
	if hasRemoteInOverlay {
		// Find the content document that has media-overlay pointing to this overlay
		for _, mItem := range ep.Package.Manifest {
			if mItem.MediaOverlay == item.ID {
				if !hasProperty(mItem.Properties, "remote-resources") {
					contentPath := ep.ResolveHref(mItem.Href)
					r.AddWithLocation(report.Error, "OPF-014",
						"Property 'remote-resources' should be declared in the manifest for content with remote resources",
						contentPath)
				}
			}
		}
	}
}

// MED-007: audio src must exist in container
func checkSMILAudio(ep *epub.EPUB, se xml.StartElement, smilDir string, location string, r *report.Report) {
	for _, attr := range se.Attr {
		if attr.Name.Local == "src" && attr.Value != "" {
			u, err := url.Parse(attr.Value)
			if err != nil || u.Scheme != "" {
				continue
			}
			target := resolvePath(smilDir, u.Path)
			if _, exists := ep.Files[target]; !exists {
				r.AddWithLocation(report.Error, "MED-007",
					fmt.Sprintf("Referenced resource '%s' could not be found in the container", attr.Value),
					location)
			}
		}
		// MED-010: clipBegin/clipEnd must be valid SMIL clock values
		if attr.Name.Local == "clipBegin" || attr.Name.Local == "clipEnd" {
			if !isValidSMILClockValue(attr.Value) {
				r.AddWithLocation(report.Error, "MED-010",
					fmt.Sprintf("The value of attribute '%s' is invalid: '%s'", attr.Name.Local, attr.Value),
					location)
			}
		}
	}
}

// MED-008: text src must reference an existing fragment
func checkSMILText(ep *epub.EPUB, se xml.StartElement, smilDir string, location string, r *report.Report) {
	for _, attr := range se.Attr {
		if attr.Name.Local == "src" && attr.Value != "" {
			u, err := url.Parse(attr.Value)
			if err != nil || u.Scheme != "" {
				continue
			}
			target := resolvePath(smilDir, u.Path)
			if _, exists := ep.Files[target]; !exists {
				r.AddWithLocation(report.Error, "MED-008",
					fmt.Sprintf("Fragment identifier is not defined in '%s'", attr.Value),
					location)
			} else if u.Fragment != "" {
				// Check that the fragment ID actually exists in the target document
				targetData, err := ep.ReadFile(target)
				if err == nil {
					ids := collectIDs(targetData)
					if !ids[u.Fragment] {
						r.AddWithLocation(report.Error, "MED-008",
							fmt.Sprintf("Fragment identifier is not defined in '%s'", attr.Value),
							location)
					}
				}
			}
		}
	}
}

// SMIL clock value patterns
var smilClockRe = regexp.MustCompile(`^(\d+:)?(\d{1,2}):(\d{2})(\.\d+)?$|^\d+(\.\d+)?(h|min|s|ms)?$`)

func isValidSMILClockValue(val string) bool {
	if val == "" {
		return false
	}
	return smilClockRe.MatchString(val)
}

// MED-009: media overlay items must have media:duration meta elements
func checkMediaOverlayDuration(ep *epub.EPUB, r *report.Report) {
	hasSMIL := false
	for _, item := range ep.Package.Manifest {
		if item.MediaType == "application/smil+xml" {
			hasSMIL = true
			break
		}
	}
	if !hasSMIL {
		return
	}

	// Check for media:duration or duration metadata
	data, err := ep.ReadFile(ep.RootfilePath)
	if err != nil {
		return
	}
	content := string(data)
	if !strings.Contains(content, "media:duration") {
		r.Add(report.Error, "MED-009",
			"The global media:duration meta element not set on the publication")
	}
}

// MED-013: content documents with media-overlay must reference valid SMIL
func checkMediaOverlayProperty(ep *epub.EPUB, r *report.Report) {
	smilIDs := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.MediaType == "application/smil+xml" {
			smilIDs[item.ID] = true
		}
	}

	for _, item := range ep.Package.Manifest {
		if item.MediaOverlay == "" {
			continue
		}
		if !smilIDs[item.MediaOverlay] {
			r.Add(report.Error, "MED-013",
				fmt.Sprintf("Media Overlay Document referenced by '%s' could not be found: '%s'", item.Href, item.MediaOverlay))
		}
	}
}
