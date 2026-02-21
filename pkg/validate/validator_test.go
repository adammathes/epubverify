package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

type expectedJSON struct {
	Fixture       string            `json:"fixture"`
	Valid         bool              `json:"valid"`
	Messages      []expectedMessage `json:"messages"`
	FatalCount    int               `json:"fatal_count"`
	ErrorCount    int               `json:"error_count"`
	ErrorCountMin *int              `json:"error_count_min"`
	WarningCount  int               `json:"warning_count"`
	Note          string            `json:"note,omitempty"`
}

type expectedMessage struct {
	Severity       string `json:"severity"`
	CheckID        string `json:"check_id"`
	EpubcheckID    string `json:"epubcheck_id"`
	MessagePattern string `json:"message_pattern"`
	Note           string `json:"note"`
}

func loadExpected(t *testing.T, path string) expectedJSON {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read expected file %s: %v", path, err)
	}
	var exp expectedJSON
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("Failed to parse expected file %s: %v", path, err)
	}
	return exp
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

func TestValidateValidEPUB2(t *testing.T) {
	sd := specDir(t)
	r, err := Validate(filepath.Join(sd, "fixtures/epub/valid/minimal-epub2.epub"))
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

// TestValidateAgainstExpected runs all test fixtures against expected results.
func TestValidateAgainstExpected(t *testing.T) {
	sd := specDir(t)

	categories := []string{"invalid", "valid"}
	for _, cat := range categories {
		expectedDir := filepath.Join(sd, "expected", cat)
		entries, err := os.ReadDir(expectedDir)
		if err != nil {
			t.Fatalf("Failed to read expected dir %s: %v", expectedDir, err)
		}

		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}

			name := strings.TrimSuffix(entry.Name(), ".json")
			t.Run(cat+"/"+name, func(t *testing.T) {
				exp := loadExpected(t, filepath.Join(expectedDir, entry.Name()))

				epubPath := filepath.Join(sd, "fixtures/epub", cat, name+".epub")
				if _, err := os.Stat(epubPath); os.IsNotExist(err) {
					t.Skipf("Fixture %s not found", epubPath)
				}

				r, err := Validate(epubPath)
				if err != nil {
					t.Fatalf("Validate error: %v", err)
				}

				// Check valid status
				if r.IsValid() != exp.Valid {
					t.Errorf("valid: got %v, want %v", r.IsValid(), exp.Valid)
					for _, m := range r.Messages {
						t.Logf("  %s", m)
					}
				}

				// Check error counts with min support
				if exp.ErrorCountMin != nil {
					if r.ErrorCount() < *exp.ErrorCountMin {
						t.Errorf("error_count: got %d, want at least %d", r.ErrorCount(), *exp.ErrorCountMin)
					}
				} else {
					if r.ErrorCount() < exp.ErrorCount {
						t.Errorf("error_count: got %d, want at least %d", r.ErrorCount(), exp.ErrorCount)
					}
				}

				if r.FatalCount() < exp.FatalCount {
					t.Errorf("fatal_count: got %d, want at least %d", r.FatalCount(), exp.FatalCount)
				}

				if r.WarningCount() < exp.WarningCount {
					t.Errorf("warning_count: got %d, want at least %d", r.WarningCount(), exp.WarningCount)
				}

				// Check that expected messages are present (by pattern and severity)
				for _, em := range exp.Messages {
					found := false
					pattern, err := regexp.Compile("(?i)" + em.MessagePattern)
					if err != nil {
						t.Errorf("Invalid message_pattern %q: %v", em.MessagePattern, err)
						continue
					}

					for _, m := range r.Messages {
						if string(m.Severity) == em.Severity && pattern.MatchString(m.Message) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected message not found: severity=%s pattern=%q check_id=%s",
							em.Severity, em.MessagePattern, em.CheckID)
						t.Logf("  Actual messages:")
						for _, m := range r.Messages {
							t.Logf("    %s", m)
						}
					}
				}
			})
		}
	}
}
