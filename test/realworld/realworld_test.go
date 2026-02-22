package realworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adammathes/epubverify/pkg/validate"
)

// TestRealWorldSamples validates downloaded EPUB samples and checks for
// false positives. Each sample is expected to be valid (no errors, no
// warnings), matching the reference epubcheck tool.
//
// Samples must be downloaded first:
//
//	./test/realworld/download-samples.sh
//
// Or set REALWORLD_SAMPLES_DIR to point at a directory of EPUBs.
func TestRealWorldSamples(t *testing.T) {
	dir := os.Getenv("REALWORLD_SAMPLES_DIR")
	if dir == "" {
		dir = filepath.Join(findRepoRoot(t), "test", "realworld", "samples")
	}

	entries, err := filepath.Glob(filepath.Join(dir, "*.epub"))
	if err != nil {
		t.Fatalf("globbing samples: %v", err)
	}
	if len(entries) == 0 {
		t.Skipf("no sample EPUBs found in %s (run download-samples.sh first)", dir)
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
					t.Logf("  %s(%s): %s [%s]", m.Severity, m.CheckID, m.Message, m.Location)
				}
			}

			if rpt.ErrorCount() != 0 {
				t.Errorf("expected 0 errors, got %d", rpt.ErrorCount())
			}
			if rpt.WarningCount() != 0 {
				t.Errorf("expected 0 warnings, got %d", rpt.WarningCount())
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
