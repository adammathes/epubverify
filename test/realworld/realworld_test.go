package realworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adammathes/epubverify/pkg/validate"
)

// knownInvalid lists samples that are genuinely invalid (both epubcheck and
// epubverify agree). These are kept in the corpus to verify we detect real
// errors, not just to check for false positives.
var knownInvalid = map[string]bool{
	"fb-art-of-war.epub":     true, // mimetype trailing CRLF, NCX UID mismatch, bad date
	"fb-odyssey.epub":        true, // mimetype trailing CRLF, NCX UID mismatch
	"fb-republic.epub":       true, // mimetype trailing CRLF, NCX UID mismatch
	"fb-sherlock-study.epub": true, // mimetype trailing CRLF, NCX UID mismatch
}

// TestRealWorldSamples validates downloaded EPUB samples and checks for
// false positives. Samples not in knownInvalid are expected to be valid
// (no errors, no warnings), matching the reference epubcheck tool.
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

			if knownInvalid[name] {
				// Known-invalid: verify we detect errors
				if rpt.IsValid() {
					t.Errorf("expected invalid (known-invalid sample), but got valid")
				}
				return
			}

			// All other samples should be valid
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

// TestKnownInvalidExpectedErrors verifies that known-invalid samples
// produce specific expected error check IDs.
func TestKnownInvalidExpectedErrors(t *testing.T) {
	dir := os.Getenv("REALWORLD_SAMPLES_DIR")
	if dir == "" {
		dir = filepath.Join(findRepoRoot(t), "test", "realworld", "samples")
	}

	expectations := map[string][]string{
		"fb-art-of-war.epub":     {"OCF-003", "E2-010"},
		"fb-odyssey.epub":        {"OCF-003", "E2-010"},
		"fb-republic.epub":       {"OCF-003", "E2-010"},
		"fb-sherlock-study.epub": {"OCF-003", "E2-010"},
	}

	for name, expectedIDs := range expectations {
		epubPath := filepath.Join(dir, name)
		if _, err := os.Stat(epubPath); os.IsNotExist(err) {
			t.Skipf("sample %s not found (run download-samples.sh)", name)
			continue
		}

		t.Run(name, func(t *testing.T) {
			rpt, err := validate.Validate(epubPath)
			if err != nil {
				t.Fatalf("validation failed: %v", err)
			}

			foundIDs := make(map[string]bool)
			for _, m := range rpt.Messages {
				if m.Severity == "ERROR" {
					foundIDs[m.CheckID] = true
				}
			}

			for _, id := range expectedIDs {
				if !foundIDs[id] {
					t.Errorf("expected error %s not found in report", id)
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
