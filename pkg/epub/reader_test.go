package epub

import (
	"os"
	"path/filepath"
	"testing"
)

func specDir(t *testing.T) string {
	dir := os.Getenv("EPUBCHECK_SPEC_DIR")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), "epubcheck-spec")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("epubcheck-spec not found at %s", dir)
	}
	return dir
}

func TestOpen(t *testing.T) {
	sd := specDir(t)
	ep, err := Open(filepath.Join(sd, "fixtures/epub/valid/minimal-epub3.epub"))
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
	sd := specDir(t)
	ep, err := Open(filepath.Join(sd, "fixtures/epub/valid/minimal-epub3.epub"))
	if err != nil {
		t.Fatal(err)
	}
	defer ep.Close()

	if err := ep.ParseContainer(); err != nil {
		t.Fatal(err)
	}

	if ep.RootfilePath != "OEBPS/content.opf" {
		t.Errorf("rootfile path: got %q, want %q", ep.RootfilePath, "OEBPS/content.opf")
	}
}

func TestParseOPF(t *testing.T) {
	sd := specDir(t)
	ep, err := Open(filepath.Join(sd, "fixtures/epub/valid/minimal-epub3.epub"))
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
		{"unique-identifier", pkg.UniqueIdentifier, "uid"},
		{"title count", len(pkg.Metadata.Titles), 1},
		{"title", pkg.Metadata.Titles[0], "Test Book"},
		{"identifier count", len(pkg.Metadata.Identifiers), 1},
		{"language count", len(pkg.Metadata.Languages), 1},
		{"language", pkg.Metadata.Languages[0], "en"},
		{"modified", pkg.Metadata.Modified, "2025-01-01T00:00:00Z"},
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
