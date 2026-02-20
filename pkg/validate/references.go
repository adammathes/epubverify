package validate

import (
	"fmt"
	"strings"

	"github.com/adammathes/epubcheck-go/pkg/epub"
	"github.com/adammathes/epubcheck-go/pkg/report"
)

// checkReferences validates cross-references between manifest and zip contents.
func checkReferences(ep *epub.EPUB, r *report.Report) {
	pkg := ep.Package
	if pkg == nil {
		return
	}

	// RSC-001: every manifest href must exist in the zip
	checkManifestFilesExist(ep, r)

	// RSC-002: every content file in zip should be in manifest
	// Note: epubcheck 5.3.0 does not report this at ERROR/WARNING level,
	// so we skip this check to match expected behavior.

	// NAV-001: exactly one manifest item with properties="nav"
	checkNavDeclared(ep, r)

	// NAV-002: nav document must have epub:type="toc"
	checkNavHasToc(ep, r)
}

// RSC-001
func checkManifestFilesExist(ep *epub.EPUB, r *report.Report) {
	for _, item := range ep.Package.Manifest {
		if item.Href == "\x00MISSING" {
			continue
		}
		fullPath := ep.ResolveHref(item.Href)
		if _, exists := ep.Files[fullPath]; !exists {
			r.Add(report.Error, "RSC-001",
				fmt.Sprintf("Referenced resource '%s' was not found in the container", item.Href))
		}
	}
}

// RSC-002
func checkZipEntriesInManifest(ep *epub.EPUB, r *report.Report) {
	// Build set of manifest hrefs (resolved to full paths)
	manifestPaths := make(map[string]bool)
	for _, item := range ep.Package.Manifest {
		if item.Href != "\x00MISSING" {
			manifestPaths[ep.ResolveHref(item.Href)] = true
		}
	}

	opfDir := ep.OPFDir()

	for name := range ep.Files {
		// Skip META-INF/ and mimetype — these aren't content files
		if name == "mimetype" || strings.HasPrefix(name, "META-INF/") {
			continue
		}
		// Skip the OPF file itself
		if name == ep.RootfilePath {
			continue
		}
		// Skip directories
		if strings.HasSuffix(name, "/") {
			continue
		}
		// Check if the file is under the OPF directory or at the root
		_ = opfDir
		if !manifestPaths[name] {
			r.Add(report.Error, "RSC-002",
				fmt.Sprintf("File '%s' exists in the container but is not declared in the manifest", name))
		}
	}
}

// NAV-001
func checkNavDeclared(ep *epub.EPUB, r *report.Report) {
	if ep.Package.Version < "3.0" {
		return
	}
	count := 0
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			count++
		}
	}
	if count == 0 {
		r.Add(report.Error, "NAV-001", "No manifest item found with nav property (exactly one is required)")
	}
}

// NAV-002
func checkNavHasToc(ep *epub.EPUB, r *report.Report) {
	if ep.Package.Version < "3.0" {
		return
	}

	// Find the nav item
	var navHref string
	for _, item := range ep.Package.Manifest {
		if hasProperty(item.Properties, "nav") {
			navHref = item.Href
			break
		}
	}
	if navHref == "" {
		return // NAV-001 already reported
	}

	fullPath := ep.ResolveHref(navHref)
	data, err := ep.ReadFile(fullPath)
	if err != nil {
		return // File missing, handled elsewhere
	}

	if !navDocHasToc(data) {
		r.Add(report.Error, "NAV-002", "Required toc nav element (epub:type='toc') not found in navigation document")
	}
}

func hasProperty(properties, prop string) bool {
	for _, p := range strings.Fields(properties) {
		if p == prop {
			return true
		}
	}
	return false
}
