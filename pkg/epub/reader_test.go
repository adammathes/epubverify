package epub

import (
	"os"
	"path/filepath"
	"testing"
)

// testdataDir returns the path to the testdata directory in the repo root.
func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "testdata")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}

func TestOpen(t *testing.T) {
	td := testdataDir(t)
	ep, err := Open(filepath.Join(td, "fixtures/epub3/00-minimal/minimal.epub"))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	if len(ep.Files) == 0 {
		t.Fatal("no files found in EPUB")
	}

	if _, ok := ep.Files["mimetype"]; !ok {
		t.Error("mimetype file not found")
	}
	if _, ok := ep.Files["META-INF/container.xml"]; !ok {
		t.Error("container.xml not found")
	}
}

func TestParseContainer(t *testing.T) {
	td := testdataDir(t)
	ep, err := Open(filepath.Join(td, "fixtures/epub3/00-minimal/minimal.epub"))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	if err := ep.ParseContainer(); err != nil {
		t.Fatal(err)
	}

	if ep.RootfilePath != "EPUB/package.opf" {
		t.Errorf("rootfile path: got %q, want %q", ep.RootfilePath, "EPUB/package.opf")
	}
}

func TestResolveHref_PercentEncoded(t *testing.T) {
	// ResolveHref must URL-decode manifest hrefs because ZIP entry names
	// use decoded forms while OPF hrefs are IRI-encoded.
	ep := &EPUB{RootfilePath: "EPUB/package.opf"}

	tests := []struct {
		href string
		want string
	}{
		{"chapter.xhtml", "EPUB/chapter.xhtml"},
		{"chapter%20one.xhtml", "EPUB/chapter one.xhtml"},
		{"my%20chapter%20%28two%29.xhtml", "EPUB/my chapter (two).xhtml"},
		{"content%20files/extra%20doc.xhtml", "EPUB/content files/extra doc.xhtml"},
		{"sub/page.xhtml", "EPUB/sub/page.xhtml"},
		{"file%2Bplus.xhtml", "EPUB/file+plus.xhtml"},
	}

	for _, tt := range tests {
		t.Run(tt.href, func(t *testing.T) {
			got := ep.ResolveHref(tt.href)
			if got != tt.want {
				t.Errorf("ResolveHref(%q) = %q, want %q", tt.href, got, tt.want)
			}
		})
	}
}

func TestResolveHref_NoOPFDir(t *testing.T) {
	ep := &EPUB{RootfilePath: "content.opf"}

	tests := []struct {
		href string
		want string
	}{
		{"chapter.xhtml", "chapter.xhtml"},
		{"chapter%20one.xhtml", "chapter one.xhtml"},
	}

	for _, tt := range tests {
		t.Run(tt.href, func(t *testing.T) {
			got := ep.ResolveHref(tt.href)
			if got != tt.want {
				t.Errorf("ResolveHref(%q) = %q, want %q", tt.href, got, tt.want)
			}
		})
	}
}

func TestParseOPF(t *testing.T) {
	td := testdataDir(t)
	ep, err := Open(filepath.Join(td, "fixtures/epub3/00-minimal/minimal.epub"))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	if err := ep.ParseContainer(); err != nil {
		t.Fatal(err)
	}
	if err := ep.ParseOPF(); err != nil {
		t.Fatal(err)
	}

	pkg := ep.Package
	if pkg == nil {
		t.Fatal("package is nil")
	}

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"version", pkg.Version, "3.0"},
		{"unique-identifier", pkg.UniqueIdentifier, "q"},
		{"title count", len(pkg.Metadata.Titles), 1},
		{"title", pkg.Metadata.Titles[0].Value, "Minimal EPUB 3.0"},
		{"identifier count", len(pkg.Metadata.Identifiers), 1},
		{"language count", len(pkg.Metadata.Languages), 1},
		{"language", pkg.Metadata.Languages[0], "en"},
		{"modified", pkg.Metadata.Modified, "2017-06-14T00:00:01Z"},
		{"manifest count", len(pkg.Manifest), 2},
		{"spine count", len(pkg.Spine), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}
