package validate

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// EPUB 3 core media types
var coreMediaTypes = map[string]bool{
	"image/gif":                    true,
	"image/jpeg":                   true,
	"image/png":                    true,
	"image/svg+xml":                true,
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
	"video/mp4":                    true,
	"video/h264":                   true,
	"application/smil+xml":         true,
	"application/pls+xml":          true,
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

	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" || item.MediaType == "\x00MISSING" {
			continue
		}

		fullPath := ep.ResolveHref(item.Href)
		if _, exists := ep.Files[fullPath]; !exists {
			continue
		}

		// MED-001: image media type must match actual content
		// MED-003: image must not be corrupted
		if strings.HasPrefix(item.MediaType, "image/") && item.MediaType != "image/svg+xml" {
			mismatch := checkImageMediaType(ep, item, fullPath, r)
			// Only check for corruption if media type matches (mismatch is a different problem)
			if !mismatch {
				checkImageNotCorrupted(ep, item, fullPath, r)
			}
		}

		// MED-004/MED-005: foreign resources must have fallback
		// Skip image/webp - epubcheck 5.3.0 does not flag it
		if ep.Package.Version >= "3.0" && !coreMediaTypes[item.MediaType] && item.MediaType != "image/webp" && item.Fallback == "" {
			r.Add(report.Error, foreignResourceCheckID(item.MediaType),
				fmt.Sprintf("Fallback must be provided for foreign resources: '%s' has media type '%s'", item.Href, item.MediaType))
		}
	}
}

// MED-001: verify image file type matches declared media type
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
		r.Add(report.Error, "MED-001",
			fmt.Sprintf("The file '%s' does not appear to match the media type '%s'", item.Href, item.MediaType))
		return true
	}
	return false
}

// MED-003: verify image is not corrupted
func checkImageNotCorrupted(ep *epub.EPUB, item epub.ManifestItem, fullPath string, r *report.Report) {
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return
	}

	if len(data) < 8 {
		r.Add(report.Error, "MED-003",
			fmt.Sprintf("Corrupted image file '%s': file too small", item.Href))
		return
	}

	// Check for valid magic bytes
	switch item.MediaType {
	case "image/png":
		if !bytes.HasPrefix(data, pngMagic) {
			r.Add(report.Error, "MED-003",
				fmt.Sprintf("Corrupted image file '%s': invalid PNG header", item.Href))
		}
	case "image/jpeg":
		if !bytes.HasPrefix(data, jpegMagic) {
			r.Add(report.Error, "MED-003",
				fmt.Sprintf("Corrupted image file '%s': invalid JPEG header", item.Href))
		}
	case "image/gif":
		if !bytes.HasPrefix(data, gifMagic) {
			r.Add(report.Error, "MED-003",
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
