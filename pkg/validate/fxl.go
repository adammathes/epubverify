package validate

import (
	"fmt"
	"strings"

	"github.com/adammathes/epubverify/pkg/epub"
	"github.com/adammathes/epubverify/pkg/report"
)

// checkFXL validates fixed-layout properties.
func checkFXL(ep *epub.EPUB, r *report.Report) {
	if ep.Package == nil || ep.Package.Version < "3.0" {
		return
	}

	// FXL-001: rendition:layout must be valid
	if ep.Package.RenditionLayout != "" {
		if ep.Package.RenditionLayout != "pre-paginated" && ep.Package.RenditionLayout != "reflowable" {
			r.Add(report.Error, "FXL-001",
				fmt.Sprintf("The value of property rendition:layout must be either 'pre-paginated' or 'reflowable', but was '%s'", ep.Package.RenditionLayout))
		}
	}

	// FXL-002: rendition:orientation must be valid
	if ep.Package.RenditionOrientation != "" {
		valid := map[string]bool{"auto": true, "landscape": true, "portrait": true}
		if !valid[ep.Package.RenditionOrientation] {
			r.Add(report.Error, "FXL-002",
				fmt.Sprintf("The value of property rendition:orientation must be either 'auto', 'landscape', or 'portrait', but was '%s'", ep.Package.RenditionOrientation))
		}
	}

	// FXL-003: rendition:spread must be valid
	if ep.Package.RenditionSpread != "" {
		valid := map[string]bool{"auto": true, "landscape": true, "both": true, "none": true}
		if !valid[ep.Package.RenditionSpread] {
			r.Add(report.Error, "FXL-003",
				fmt.Sprintf("The value of property rendition:spread must be either 'auto', 'landscape', 'both', or 'none', but was '%s'", ep.Package.RenditionSpread))
		}
	}

	// FXL-004/FXL-005: spine itemref properties must be valid
	for _, ref := range ep.Package.Spine {
		if ref.Properties == "" {
			continue
		}
		for _, prop := range strings.Fields(ref.Properties) {
			if !validSpineProperties[prop] {
				// Determine the appropriate check ID
				checkID := "FXL-004"
				if strings.HasPrefix(prop, "rendition:spread") {
					checkID = "FXL-005"
				}
				r.Add(report.Error, checkID,
					fmt.Sprintf("Undefined property '%s' on spine itemref", prop))
			}
		}
	}
}
