package stress_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CrawlEntry represents a single entry in the crawl manifest.
type CrawlEntry struct {
	SHA256            string `json:"sha256"`
	SourceURL         string `json:"source_url"`
	Source            string `json:"source"`
	DownloadedAt      string `json:"downloaded_at"`
	SizeBytes         int64  `json:"size_bytes"`
	EpubverifyVerdict string `json:"epubverify_verdict"`
	EpubcheckVerdict  string `json:"epubcheck_verdict"`
	Match             bool   `json:"match"`
	DiscrepancyIssue  string `json:"discrepancy_issue,omitempty"`
}

// CrawlManifest is the top-level crawl manifest structure.
type CrawlManifest struct {
	CrawlDate string       `json:"crawl_date"`
	EPUBs     []CrawlEntry `json:"epubs"`
	Summary   struct {
		TotalTested    int `json:"total_tested"`
		Agreement      int `json:"agreement"`
		FalsePositives int `json:"false_positives"`
		FalseNegatives int `json:"false_negatives"`
		Crashes        int `json:"crashes"`
	} `json:"summary"`
}

func TestCrawlerScriptExists(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "epub-crawler.sh")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("epub-crawler.sh does not exist at %s", path)
	}
}

func TestCrawlerScriptIsExecutable(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "epub-crawler.sh")
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		t.Skipf("epub-crawler.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("stat epub-crawler.sh: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("epub-crawler.sh is not executable (mode %o)", info.Mode())
	}
}

func TestCrawlerScriptHasSources(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "epub-crawler.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("epub-crawler.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read epub-crawler.sh: %v", err)
	}
	content := string(data)

	// Must support at least 3 source types from the ROADMAP
	requiredSources := []string{"gutenberg", "standardebooks", "feedbooks"}
	for _, src := range requiredSources {
		if !strings.Contains(strings.ToLower(content), src) {
			t.Errorf("epub-crawler.sh missing source: %s", src)
		}
	}
}

func TestCrawlerScriptHasRateLimiting(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "epub-crawler.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("epub-crawler.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read epub-crawler.sh: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "sleep") {
		t.Error("epub-crawler.sh must include rate limiting (sleep)")
	}
}

func TestCrawlerScriptHasSHA256Dedup(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "epub-crawler.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("epub-crawler.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read epub-crawler.sh: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "sha256") {
		t.Error("epub-crawler.sh must include SHA-256 deduplication")
	}
}

func TestCrawlerScriptHasUserAgent(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "epub-crawler.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("epub-crawler.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read epub-crawler.sh: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "User-Agent") {
		t.Error("epub-crawler.sh must include a User-Agent header")
	}
}

func TestValidateScriptExists(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "crawl-validate.sh")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("crawl-validate.sh does not exist at %s", path)
	}
}

func TestReportScriptExists(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "crawl-report.sh")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("crawl-report.sh does not exist at %s", path)
	}
}

func TestCrawlManifestSchema(t *testing.T) {
	// Verify the manifest schema can roundtrip through JSON
	manifest := CrawlManifest{
		CrawlDate: "2026-02-26T12:00:00Z",
		EPUBs: []CrawlEntry{
			{
				SHA256:            "abc123def456",
				SourceURL:         "https://gutenberg.org/ebooks/1342.epub3.images",
				Source:            "GUTENBERG",
				DownloadedAt:      "2026-02-26T12:00:00Z",
				SizeBytes:         123456,
				EpubverifyVerdict: "valid",
				EpubcheckVerdict:  "valid",
				Match:             true,
			},
			{
				SHA256:            "789ghi012jkl",
				SourceURL:         "https://standardebooks.org/ebooks/test.epub",
				Source:            "STANDARDEBOOKS",
				DownloadedAt:      "2026-02-26T12:01:00Z",
				SizeBytes:         456789,
				EpubverifyVerdict: "invalid",
				EpubcheckVerdict:  "valid",
				Match:             false,
				DiscrepancyIssue:  "#123",
			},
		},
	}
	manifest.Summary.TotalTested = 2
	manifest.Summary.Agreement = 1
	manifest.Summary.FalsePositives = 1
	manifest.Summary.FalseNegatives = 0
	manifest.Summary.Crashes = 0

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	var roundtripped CrawlManifest
	if err := json.Unmarshal(data, &roundtripped); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	if roundtripped.CrawlDate != manifest.CrawlDate {
		t.Errorf("crawl_date mismatch: %q vs %q", roundtripped.CrawlDate, manifest.CrawlDate)
	}
	if len(roundtripped.EPUBs) != 2 {
		t.Errorf("expected 2 epubs, got %d", len(roundtripped.EPUBs))
	}
	if roundtripped.EPUBs[0].SHA256 != "abc123def456" {
		t.Errorf("sha256 mismatch: %q", roundtripped.EPUBs[0].SHA256)
	}
	if roundtripped.EPUBs[1].Match != false {
		t.Error("match should be false for discrepancy entry")
	}
	if roundtripped.EPUBs[1].DiscrepancyIssue != "#123" {
		t.Errorf("discrepancy_issue mismatch: %q", roundtripped.EPUBs[1].DiscrepancyIssue)
	}
	if roundtripped.Summary.TotalTested != 2 {
		t.Errorf("total_tested mismatch: %d", roundtripped.Summary.TotalTested)
	}
}

func TestValidateScriptRunsBothTools(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "crawl-validate.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("crawl-validate.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read crawl-validate.sh: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "epubverify") {
		t.Error("crawl-validate.sh must run epubverify")
	}
	if !strings.Contains(content, "epubcheck") {
		t.Error("crawl-validate.sh must run epubcheck")
	}
}

func TestValidateScriptComparesVerdicts(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "crawl-validate.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("crawl-validate.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read crawl-validate.sh: %v", err)
	}
	content := string(data)

	// Must detect both types of discrepancy
	if !strings.Contains(content, "false_negative") || !strings.Contains(content, "false_positive") {
		t.Error("crawl-validate.sh must detect both false negatives and false positives")
	}
}

func TestReportScriptGeneratesSummary(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "crawl-report.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("crawl-report.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read crawl-report.sh: %v", err)
	}
	content := string(data)

	// Must produce a summary
	if !strings.Contains(content, "summary") && !strings.Contains(content, "SUMMARY") {
		t.Error("crawl-report.sh must generate a summary")
	}
}

func TestReportScriptSupportsGitHubIssues(t *testing.T) {
	path := filepath.Join(repoRoot(), "scripts", "crawl-report.sh")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("crawl-report.sh does not exist yet")
	}
	if err != nil {
		t.Fatalf("read crawl-report.sh: %v", err)
	}
	content := string(data)

	// Must have GitHub issue filing capability
	if !strings.Contains(content, "gh ") && !strings.Contains(content, "gh issue") {
		t.Error("crawl-report.sh must support GitHub issue filing via gh CLI")
	}
}

func TestGitHubActionsWorkflowExists(t *testing.T) {
	path := filepath.Join(repoRoot(), ".github", "workflows", "epub-crawl.yml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("epub-crawl.yml workflow does not exist at %s", path)
	}
}

func TestGitHubActionsWorkflowHasScheduleConfig(t *testing.T) {
	path := filepath.Join(repoRoot(), ".github", "workflows", "epub-crawl.yml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("epub-crawl.yml does not exist yet")
	}
	if err != nil {
		t.Fatalf("read epub-crawl.yml: %v", err)
	}
	content := string(data)

	// Schedule config should be present (even if commented out for now)
	if !strings.Contains(content, "schedule") {
		t.Error("epub-crawl.yml must have schedule configuration (can be commented out)")
	}
	if !strings.Contains(content, "cron") {
		t.Error("epub-crawl.yml must have a cron expression (can be commented out)")
	}
	// Must support manual dispatch
	if !strings.Contains(content, "workflow_dispatch") {
		t.Error("epub-crawl.yml must support manual workflow_dispatch")
	}
}

func TestMakefileHasCrawlTarget(t *testing.T) {
	path := filepath.Join(repoRoot(), "Makefile")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "crawl") {
		t.Error("Makefile must have a crawl-related target")
	}
}
