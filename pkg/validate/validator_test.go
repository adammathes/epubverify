package validate

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

func TestValidateValid(t *testing.T) {
	sd := specDir(t)
	r, err := Validate(filepath.Join(sd, "fixtures/epub/valid/minimal-epub3.epub"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsValid() {
		t.Error("expected valid EPUB")
		for _, m := range r.Messages {
			t.Logf("  %s", m)
		}
	}
}

func TestValidateInvalid(t *testing.T) {
	sd := specDir(t)
	tests := []struct {
		fixture string
		checkID string
	}{
		{"invalid/ocf-mimetype-missing.epub", "OCF-001"},
		{"invalid/ocf-container-missing.epub", "OCF-006"},
		{"invalid/opf-missing-dc-title.epub", "OPF-001"},
		{"invalid/spine-bad-idref.epub", "OPF-009"},
		{"invalid/nav-missing.epub", "NAV-001"},
		{"invalid/content-malformed-xhtml.epub", "HTM-001"},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			r, err := Validate(filepath.Join(sd, "fixtures/epub", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if r.IsValid() {
				t.Error("expected invalid EPUB")
			}
			found := false
			for _, m := range r.Messages {
				if m.CheckID == tt.checkID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected check %s to fire", tt.checkID)
				for _, m := range r.Messages {
					t.Logf("  %s", m)
				}
			}
		})
	}
}
