package synthetic

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adammathes/epubverify/pkg/validate"
)

// TestSyntheticSamples validates synthetic edge-case EPUBs that are designed
// to exercise specific validation code paths (fallback chains, FXL rendition
// overrides, SMIL media overlays, font obfuscation, percent-encoded filenames,
// multiple renditions, foreign resources, complex CSS, etc.).
//
// All synthetic EPUBs are valid per epubcheck and must pass without errors.
//
// Generate synthetic samples:
//
//	python3 test/realworld/create-edge-cases.py
//
// Or set SYNTHETIC_SAMPLES_DIR to point at a directory of EPUBs.
func TestSyntheticSamples(t *testing.T) {
	dir := os.Getenv("SYNTHETIC_SAMPLES_DIR")
	if dir == "" {
		dir = filepath.Join(findRepoRoot(t), "test", "synthetic", "samples")
	}

	entries, err := filepath.Glob(filepath.Join(dir, "*.epub"))
	if err != nil {
		t.Fatalf("globbing samples: %v", err)
	}
	if len(entries) == 0 {
		t.Skipf("no synthetic EPUBs found in %s (run create-edge-cases.py first)", dir)
	}

	for _, epub := range entries {
		name := filepath.Base(epub)
		t.Run(name, func(t *testing.T) {
			rpt, err := validate.Validate(epub)
			if err != nil {
				t.Fatalf("validation failed: %v", err)
			}

			if !rpt.IsValid() {
				t.Errorf("expected valid, got invalid (errors=%d, warnings=%d)",
					rpt.ErrorCount(), rpt.WarningCount())
				for _, m := range rpt.Messages {
					if m.Severity == "ERROR" || m.Severity == "WARNING" {
						t.Logf("  %s(%s): %s [%s]", m.Severity, m.CheckID, m.Message, m.Location)
					}
				}
			}
		})
	}
}

// findRepoRoot walks up from the test file location to find the repo root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}
