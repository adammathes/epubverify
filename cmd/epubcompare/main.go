// Command epubcompare runs both epubverify and epubcheck against a directory
// of EPUB files and compares the results to find discrepancies.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// --- manifest types (shared with epubfuzz) ---

type Fault struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type EPUBSpec struct {
	ID          int     `json:"id"`
	Version     string  `json:"version"`
	Faults      []Fault `json:"faults"`
	Filename    string  `json:"filename"`
	NumChapters int     `json:"num_chapters"`
}

// --- epubverify JSON output ---

type EVResult struct {
	Valid        bool        `json:"valid"`
	Messages     []EVMessage `json:"messages"`
	FatalCount   int         `json:"fatal_count"`
	ErrorCount   int         `json:"error_count"`
	WarningCount int         `json:"warning_count"`
}

type EVMessage struct {
	Severity string `json:"severity"`
	CheckID  string `json:"check_id"`
	Message  string `json:"message"`
	Location string `json:"location,omitempty"`
}

// --- epubcheck JSON output ---

type ECResult struct {
	Messages []ECMessage `json:"messages"`
	Checker  struct {
		NFatal   int `json:"nFatal"`
		NError   int `json:"nError"`
		NWarning int `json:"nWarning"`
	} `json:"checker"`
}

type ECMessage struct {
	ID        string `json:"ID"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Locations []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Col  int    `json:"column"`
	} `json:"locations"`
}

// --- discrepancy ---

type Discrepancy struct {
	EPUB           string   `json:"epub"`
	Faults         []string `json:"faults"`
	Version        string   `json:"version"`
	Type           string   `json:"type"`
	EVValid        bool     `json:"ev_valid"`
	ECValid        bool     `json:"ec_valid"`
	Detail         string   `json:"detail"`
	EVErrors       []string `json:"ev_errors,omitempty"`
	ECErrors       []string `json:"ec_errors,omitempty"`
	EVOnlyCheckIDs []string `json:"ev_only_check_ids,omitempty"`
	ECOnlyCheckIDs []string `json:"ec_only_check_ids,omitempty"`
}

func runEpubverify(epubPath, evBinary string) (*EVResult, string, error) {
	cmd := exec.Command(evBinary, epubPath, "--json", "-")
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	var result EVResult
	if jerr := json.Unmarshal(out, &result); jerr != nil {
		// Try to find JSON in the output
		idx := strings.Index(outStr, "{")
		if idx >= 0 {
			if jerr2 := json.Unmarshal([]byte(outStr[idx:]), &result); jerr2 != nil {
				return nil, outStr, fmt.Errorf("parse epubverify json: %w (raw: %.500s)", jerr2, outStr)
			}
		} else {
			return nil, outStr, fmt.Errorf("parse epubverify: %w (err=%v raw=%.500s)", jerr, err, outStr)
		}
	}

	return &result, outStr, nil
}

func runEpubcheck(epubPath, ecJar string) (*ECResult, string, error) {
	tmpJSON := epubPath + ".ec.json"
	defer os.Remove(tmpJSON)

	cmd := exec.Command("java", "-jar", ecJar, epubPath, "-j", tmpJSON)
	out, _ := cmd.CombinedOutput()
	outStr := string(out)

	data, err := os.ReadFile(tmpJSON)
	if err != nil {
		return nil, outStr, fmt.Errorf("read epubcheck json: %w", err)
	}

	var result ECResult
	if jerr := json.Unmarshal(data, &result); jerr != nil {
		return nil, outStr, fmt.Errorf("parse epubcheck json: %w", jerr)
	}

	return &result, outStr, nil
}

func main() {
	synthDir := "testdata/synthetic"
	evBinary := "./epubverify"
	ecJar := "/tmp/epubcheck-5.1.0/epubcheck.jar"

	if len(os.Args) > 1 {
		synthDir = os.Args[1]
	}
	if len(os.Args) > 2 {
		evBinary = os.Args[2]
	}
	if len(os.Args) > 3 {
		ecJar = os.Args[3]
	}

	// Load manifest
	manifestData, err := os.ReadFile(filepath.Join(synthDir, "manifest.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read manifest: %v\n", err)
		os.Exit(1)
	}

	var specs []EPUBSpec
	if err := json.Unmarshal(manifestData, &specs); err != nil {
		fmt.Fprintf(os.Stderr, "parse manifest: %v\n", err)
		os.Exit(1)
	}

	var discrepancies []Discrepancy
	validityMatches := 0
	validityMismatches := 0
	evCrashes := 0
	ecCrashes := 0

	for _, spec := range specs {
		epubPath := filepath.Join(synthDir, spec.Filename)
		faultNames := make([]string, len(spec.Faults))
		for i, f := range spec.Faults {
			faultNames[i] = f.Name
		}
		faultStr := strings.Join(faultNames, ",")
		if faultStr == "" {
			faultStr = "(valid)"
		}

		fmt.Printf("[%3d] %s v%s %s ... ", spec.ID, spec.Filename, spec.Version, faultStr)

		// Run epubverify
		evResult, evRaw, evErr := runEpubverify(epubPath, evBinary)
		if evErr != nil {
			fmt.Printf("EV_CRASH\n")
			evCrashes++
			discrepancies = append(discrepancies, Discrepancy{
				EPUB:    spec.Filename,
				Faults:  faultNames,
				Version: spec.Version,
				Type:    "ev_crash",
				Detail:  fmt.Sprintf("epubverify crashed: %v\n%.1000s", evErr, evRaw),
			})
			continue
		}

		// Run epubcheck
		ecResult, ecRaw, ecErr := runEpubcheck(epubPath, ecJar)
		if ecErr != nil {
			fmt.Printf("EC_CRASH\n")
			ecCrashes++
			discrepancies = append(discrepancies, Discrepancy{
				EPUB:    spec.Filename,
				Faults:  faultNames,
				Version: spec.Version,
				Type:    "ec_crash",
				Detail:  fmt.Sprintf("epubcheck failed: %v\n%.1000s", ecErr, ecRaw),
			})
			continue
		}

		// Determine validity
		evValid := evResult.Valid
		ecValid := (ecResult.Checker.NFatal == 0 && ecResult.Checker.NError == 0)

		// Collect error-level check IDs
		evCheckIDs := map[string]bool{}
		var evErrors []string
		for _, m := range evResult.Messages {
			if m.Severity == "FATAL" || m.Severity == "ERROR" {
				evCheckIDs[m.CheckID] = true
				evErrors = append(evErrors, fmt.Sprintf("%s(%s): %s", m.Severity, m.CheckID, m.Message))
			}
		}
		ecCheckIDs := map[string]bool{}
		var ecErrors []string
		for _, m := range ecResult.Messages {
			if m.Severity == "FATAL" || m.Severity == "ERROR" {
				ecCheckIDs[m.ID] = true
				ecErrors = append(ecErrors, fmt.Sprintf("%s(%s): %s", m.Severity, m.ID, m.Message))
			}
		}

		if evValid == ecValid {
			validityMatches++
			fmt.Printf("MATCH valid=%v\n", evValid)

			// Even when matching, track check differences for invalid EPUBs
			if !evValid {
				var evOnly, ecOnly []string
				for id := range evCheckIDs {
					if !ecCheckIDs[id] {
						evOnly = append(evOnly, id)
					}
				}
				for id := range ecCheckIDs {
					if !evCheckIDs[id] {
						ecOnly = append(ecOnly, id)
					}
				}
				if len(evOnly) > 0 || len(ecOnly) > 0 {
					sort.Strings(evOnly)
					sort.Strings(ecOnly)
					discrepancies = append(discrepancies, Discrepancy{
						EPUB:           spec.Filename,
						Faults:         faultNames,
						Version:        spec.Version,
						Type:           "check_difference",
						EVValid:        evValid,
						ECValid:        ecValid,
						Detail:         "Both invalid, but different checks flagged",
						EVErrors:       evErrors,
						ECErrors:       ecErrors,
						EVOnlyCheckIDs: evOnly,
						ECOnlyCheckIDs: ecOnly,
					})
				}
			}
		} else {
			validityMismatches++
			discType := "false_positive" // epubverify says invalid, epubcheck says valid
			if evValid && !ecValid {
				discType = "false_negative" // epubverify says valid, epubcheck says invalid
			}

			var evOnly, ecOnly []string
			for id := range evCheckIDs {
				if !ecCheckIDs[id] {
					evOnly = append(evOnly, id)
				}
			}
			for id := range ecCheckIDs {
				if !evCheckIDs[id] {
					ecOnly = append(ecOnly, id)
				}
			}
			sort.Strings(evOnly)
			sort.Strings(ecOnly)

			discrepancies = append(discrepancies, Discrepancy{
				EPUB:           spec.Filename,
				Faults:         faultNames,
				Version:        spec.Version,
				Type:           discType,
				EVValid:        evValid,
				ECValid:        ecValid,
				Detail:         fmt.Sprintf("epubverify=%v epubcheck=%v", evValid, ecValid),
				EVErrors:       evErrors,
				ECErrors:       ecErrors,
				EVOnlyCheckIDs: evOnly,
				ECOnlyCheckIDs: ecOnly,
			})
			fmt.Printf("MISMATCH ev=%v ec=%v (%s)\n", evValid, ecValid, discType)
		}
	}

	// Write detailed results
	resultsPath := filepath.Join(synthDir, "comparison_results.json")
	data, _ := json.MarshalIndent(discrepancies, "", "  ")
	os.WriteFile(resultsPath, data, 0o644)

	// Print summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("                   SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Printf("Total EPUBs tested:    %d\n", len(specs))
	fmt.Printf("Validity matches:      %d\n", validityMatches)
	fmt.Printf("Validity mismatches:   %d\n", validityMismatches)
	fmt.Printf("epubverify crashes:    %d\n", evCrashes)
	fmt.Printf("epubcheck crashes:     %d\n", ecCrashes)
	fmt.Println()

	// Count by type
	typeCounts := map[string]int{}
	for _, d := range discrepancies {
		typeCounts[d.Type]++
	}
	fmt.Println("Discrepancy breakdown:")
	types := []string{"false_negative", "false_positive", "check_difference", "ev_crash", "ec_crash"}
	for _, t := range types {
		if c, ok := typeCounts[t]; ok {
			fmt.Printf("  %-20s %d\n", t+":", c)
		}
	}

	// Print false negatives (bugs in epubverify: it says valid but epubcheck says invalid)
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  FALSE NEGATIVES (epubverify missed errors)")
	fmt.Println("═══════════════════════════════════════════════════")
	for _, d := range discrepancies {
		if d.Type == "false_negative" {
			fmt.Printf("\n  %s (v%s, faults: %s)\n", d.EPUB, d.Version, strings.Join(d.Faults, ", "))
			fmt.Printf("  epubcheck errors that epubverify missed:\n")
			for _, e := range d.ECErrors {
				fmt.Printf("    - %s\n", e)
			}
		}
	}

	// Print false positives
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  FALSE POSITIVES (epubverify reported spurious)")
	fmt.Println("═══════════════════════════════════════════════════")
	for _, d := range discrepancies {
		if d.Type == "false_positive" {
			fmt.Printf("\n  %s (v%s, faults: %s)\n", d.EPUB, d.Version, strings.Join(d.Faults, ", "))
			fmt.Printf("  epubverify errors not in epubcheck:\n")
			for _, e := range d.EVErrors {
				fmt.Printf("    - %s\n", e)
			}
		}
	}

	// Print crashes
	for _, d := range discrepancies {
		if d.Type == "ev_crash" {
			fmt.Printf("\n  CRASH: %s — %s\n", d.EPUB, d.Detail[:min(200, len(d.Detail))])
		}
	}

	fmt.Printf("\nDetailed results: %s\n", resultsPath)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
