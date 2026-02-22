package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/adammathes/epubverify/pkg/report"
	"github.com/adammathes/epubverify/pkg/validate"
)

type checksFile struct {
	Checks []checkDef `json:"checks"`
}

type checkDef struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	Category       string      `json:"category"`
	Severity       string      `json:"severity"`
	Level          int         `json:"level"`
	FixtureInvalid interface{} `json:"fixture_invalid"` // string or []string
	FixtureValid   string      `json:"fixture_valid"`
}

type expectedFile struct {
	Fixture       string            `json:"fixture"`
	Valid         bool              `json:"valid"`
	Messages      []expectedMessage `json:"messages"`
	FatalCount    int               `json:"fatal_count"`
	ErrorCount    int               `json:"error_count"`
	ErrorCountMin *int              `json:"error_count_min"`
	WarningCount  int               `json:"warning_count"`
	Note          string            `json:"note"`
}

type expectedMessage struct {
	Severity       string `json:"severity"`
	CheckID        string `json:"check_id"`
	EpubcheckID    string `json:"epubcheck_id"`
	MessagePattern string `json:"message_pattern"`
	Note           string `json:"note"`
}

func specDir(t *testing.T) string {
	dir := os.Getenv("EPUBCHECK_SPEC_DIR")
	if dir == "" {
		// Try sibling directory first (../epubverify-spec relative to repo root)
		repoRoot := findRepoRoot(t)
		sibling := filepath.Join(repoRoot, "..", "epubverify-spec")
		if _, err := os.Stat(sibling); err == nil {
			return sibling
		}
		// Fall back to home directory
		dir = filepath.Join(os.Getenv("HOME"), "epubverify-spec")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("epubverify-spec directory not found at %s", dir)
	}
	return dir
}

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

func TestSpec(t *testing.T) {
	sd := specDir(t)

	// Load checks.json
	data, err := os.ReadFile(filepath.Join(sd, "checks.json"))
	if err != nil {
		t.Fatalf("reading checks.json: %v", err)
	}

	var cf checksFile
	if err := json.Unmarshal(data, &cf); err != nil {
		t.Fatalf("parsing checks.json: %v", err)
	}

	for _, check := range cf.Checks {
		fixtures := getInvalidFixtures(check)

		for _, fixture := range fixtures {
			fixtureName := filepath.Base(fixture)
			testName := check.ID + "_" + fixtureName

			t.Run(testName, func(t *testing.T) {
				expectedPath := filepath.Join(sd, "expected", fixture+".json")
				expData, err := os.ReadFile(expectedPath)
				if err != nil {
					t.Fatalf("reading expected file: %v", err)
				}

				var exp expectedFile
				if err := json.Unmarshal(expData, &exp); err != nil {
					t.Fatalf("parsing expected file: %v", err)
				}

				epubPath := filepath.Join(sd, "fixtures", "epub", fixture+".epub")
				rpt, err := validate.Validate(epubPath)
				if err != nil {
					t.Fatalf("validation error: %v", err)
				}

				if rpt.IsValid() != exp.Valid {
					t.Errorf("valid: got %v, want %v", rpt.IsValid(), exp.Valid)
				}

				if exp.ErrorCountMin != nil {
					if rpt.ErrorCount() < *exp.ErrorCountMin {
						t.Errorf("error_count: got %d, want >= %d", rpt.ErrorCount(), *exp.ErrorCountMin)
					}
				} else {
					if rpt.ErrorCount() != exp.ErrorCount {
						t.Errorf("error_count: got %d, want %d", rpt.ErrorCount(), exp.ErrorCount)
					}
				}

				if rpt.FatalCount() != exp.FatalCount {
					t.Errorf("fatal_count: got %d, want %d", rpt.FatalCount(), exp.FatalCount)
				}

				if rpt.WarningCount() != exp.WarningCount {
					t.Errorf("warning_count: got %d, want %d", rpt.WarningCount(), exp.WarningCount)
				}

				for _, em := range exp.Messages {
					found := false
					checkIDMatch := false
					re, err := regexp.Compile("(?i)" + em.MessagePattern)
					if err != nil {
						t.Errorf("bad pattern %q: %v", em.MessagePattern, err)
						continue
					}

					// Per spec: match on severity + message_pattern.
					// check_id matching is informational (spec says implementations
					// do not need identical message IDs).
					for _, msg := range rpt.Messages {
						if string(msg.Severity) == em.Severity &&
							re.MatchString(msg.Message) {
							found = true
							if msg.CheckID == em.CheckID {
								checkIDMatch = true
							}
							break
						}
					}

					if !found {
						t.Errorf("expected message not found: severity=%s pattern=%q",
							em.Severity, em.MessagePattern)
						t.Logf("got messages:")
						for _, msg := range rpt.Messages {
							t.Logf("  %s(%s): %s", msg.Severity, msg.CheckID, msg.Message)
						}
					} else if !checkIDMatch {
						t.Logf("note: check_id mismatch for %s: expected %s, got different ID (spec allows this)",
							em.MessagePattern, em.CheckID)
					}
				}
			})
		}

		if check.FixtureValid != "" {
			t.Run(check.ID+"_valid", func(t *testing.T) {
				epubPath := filepath.Join(sd, "fixtures", "epub", check.FixtureValid+".epub")
				rpt, err := validate.Validate(epubPath)
				if err != nil {
					t.Fatalf("validation error: %v", err)
				}

				for _, msg := range rpt.Messages {
					if msg.CheckID == check.ID {
						t.Errorf("valid fixture produced message for check %s: %s(%s): %s",
							check.ID, msg.Severity, msg.CheckID, msg.Message)
					}
				}
			})
		}
	}
}

func getInvalidFixtures(check checkDef) []string {
	switch v := check.FixtureInvalid.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var result []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// Ensure report package is used (for type reference).
var _ report.Severity
