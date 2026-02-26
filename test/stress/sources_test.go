package stress_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sourceEntry represents a parsed line from epub-sources.txt.
type sourceEntry struct {
	Source      string
	URL        string
	Description string
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func parseEPUBSources(t *testing.T) []sourceEntry {
	t.Helper()
	path := filepath.Join(repoRoot(), "stress-test", "epub-sources.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open epub-sources.txt: %v", err)
	}
	defer f.Close()

	var entries []sourceEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		entries = append(entries, sourceEntry{
			Source:      strings.TrimSpace(parts[0]),
			URL:        strings.TrimSpace(parts[1]),
			Description: strings.TrimSpace(parts[2]),
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("error reading epub-sources.txt: %v", err)
	}
	return entries
}

func countBySource(entries []sourceEntry) map[string]int {
	counts := make(map[string]int)
	for _, e := range entries {
		counts[e.Source]++
	}
	return counts
}

func TestSourcesMinimumCount(t *testing.T) {
	entries := parseEPUBSources(t)
	if len(entries) < 200 {
		t.Errorf("expected at least 200 EPUB sources, got %d", len(entries))
	}
}

func TestSourcesDiversity(t *testing.T) {
	entries := parseEPUBSources(t)
	counts := countBySource(entries)

	// Must have at least 4 distinct source categories
	if len(counts) < 4 {
		t.Errorf("expected at least 4 distinct source categories, got %d: %v", len(counts), counts)
	}

	// Required source categories
	required := []string{"GUTENBERG", "IDPF", "STANDARDEBOOKS", "FEEDBOOKS"}
	for _, src := range required {
		if counts[src] == 0 {
			t.Errorf("missing required source category: %s", src)
		}
	}
}

func TestSourcesNonEnglish(t *testing.T) {
	entries := parseEPUBSources(t)

	nonEnglishKeywords := []string{
		"Chinese", "Japanese", "French", "German", "Spanish", "Russian",
		"Arabic", "Hindi", "Korean", "Italian", "Portuguese", "Latin",
		"Greek", "Esperanto", "CJK", "RTL", "non-English", "Cyrillic",
	}

	var nonEnglishCount int
	for _, e := range entries {
		desc := strings.ToLower(e.Description)
		for _, kw := range nonEnglishKeywords {
			if strings.Contains(desc, strings.ToLower(kw)) {
				nonEnglishCount++
				break
			}
		}
	}

	if nonEnglishCount < 10 {
		t.Errorf("expected at least 10 non-English EPUBs, got %d", nonEnglishCount)
	}
}

func TestSourcesEPUB2(t *testing.T) {
	entries := parseEPUBSources(t)

	var epub2Count int
	for _, e := range entries {
		desc := strings.ToLower(e.Description)
		url := strings.ToLower(e.URL)
		if strings.Contains(desc, "epub2") || strings.Contains(url, ".epub.images") {
			epub2Count++
		}
	}

	if epub2Count < 10 {
		t.Errorf("expected at least 10 EPUB2 sources, got %d", epub2Count)
	}
}

func TestSourcesLargeEPUBs(t *testing.T) {
	entries := parseEPUBSources(t)

	largeKeywords := []string{"very large", "large", "bible", "complete works", "encyclopedia", "dictionary"}
	var largeCount int
	for _, e := range entries {
		desc := strings.ToLower(e.Description)
		for _, kw := range largeKeywords {
			if strings.Contains(desc, kw) {
				largeCount++
				break
			}
		}
	}

	if largeCount < 5 {
		t.Errorf("expected at least 5 large EPUBs, got %d", largeCount)
	}
}

func TestSourcesUniqueURLs(t *testing.T) {
	entries := parseEPUBSources(t)
	seen := make(map[string]bool)
	for _, e := range entries {
		if seen[e.URL] {
			t.Errorf("duplicate URL: %s", e.URL)
		}
		seen[e.URL] = true
	}
}

func TestDownloadScriptConsistency(t *testing.T) {
	// Verify download-epubs.sh references enough download calls
	path := filepath.Join(repoRoot(), "stress-test", "download-epubs.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read download-epubs.sh: %v", err)
	}
	content := string(data)

	// Count download function calls (download_gutenberg, download, etc.)
	downloadCalls := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "download_gutenberg ") ||
			strings.HasPrefix(line, "download_gutenberg_epub2 ") ||
			strings.HasPrefix(line, "download_standardebooks ") ||
			strings.HasPrefix(line, "download_feedbooks ") ||
			(strings.HasPrefix(line, "download ") && !strings.HasPrefix(line, "download()")) {
			downloadCalls++
		}
	}

	if downloadCalls < 200 {
		t.Errorf("expected at least 200 download calls in download-epubs.sh, got %d", downloadCalls)
	}
}
