package validate

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

func TestValidateValid(t *testing.T) {
	td := testdataDir(t)
	r, err := Validate(filepath.Join(td, "fixtures/epub3/00-minimal/minimal.epub"))
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

func TestValidateValidEPUB2(t *testing.T) {
	td := testdataDir(t)
	r, err := Validate(filepath.Join(td, "fixtures/epub2/epub/ocf-minimal-valid.epub"))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsValid() {
		t.Error("expected valid EPUB 2")
		for _, m := range r.Messages {
			t.Logf("  %s", m)
		}
	}
}
