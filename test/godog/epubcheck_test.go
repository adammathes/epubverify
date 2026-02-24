package godog_test

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/adammathes/epubverify/pkg/report"
	"github.com/adammathes/epubverify/pkg/validate"
	"github.com/cucumber/godog"
)

// testdataRoot returns the absolute path to the testdata directory.
func testdataRoot(t *testing.T) string {
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

func TestFeatures(t *testing.T) {
	root := testdataRoot(t)
	featuresDir := filepath.Join(root, "features")
	fixturesDir := filepath.Join(root, "fixtures")

	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			initializeScenario(ctx, fixturesDir)
		},
		Options: &godog.Options{
			Format:        "pretty",
			Paths:         []string{featuresDir},
			TestingT:      t,
			StopOnFailure: false,
			Strict:        false,
		},
	}

	if suite.Run() != 0 {
		// Non-zero means failures occurred; godog already reported them
		// through the testing.T integration.
	}
}

// scenarioState holds per-scenario state for step definitions.
type scenarioState struct {
	fixturesDir string
	basePath    string // relative path inside fixtures, e.g. "epub3/04-ocf"
	result      *report.Report
	lastMessage string // last message text for "the message contains" steps

	// assertedIndices tracks which message indices have been explicitly
	// asserted by error/warning/fatal/info/usage steps. Used by the
	// "no other errors or warnings" step.
	assertedIndices map[int]bool

	// configuration
	epubVersion    string // "2" or "3"
	checkMode      string // "epub", "package", "xhtml", "svg", "nav"
	profile        string
	reportingLevel string // "INFO", "USAGE", etc.
}

// resolveFixturePath maps a feature-file path like '/epub3/04-ocf/files/'
// or '/epub2/files/epub/' to the fixtures directory.
//
// The epubcheck source tree has:
//
//	epub3/<section>/files/<fixtures>   -> testdata/fixtures/epub3/<section>/<fixtures>
//	epub2/files/<subdir>/<fixtures>    -> testdata/fixtures/epub2/<subdir>/<fixtures>
//
// We strip the leading '/' and remove the 'files/' path segment.
func (s *scenarioState) resolveFixturePath(path string) string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	var filtered []string
	for _, p := range parts {
		if p != "files" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, "/")
}

// fixtureFullPath returns the absolute path for a fixture name.
// If the fixture is a directory (unpackaged EPUB), it zips it first.
func (s *scenarioState) fixtureFullPath(name string) (string, error) {
	base := filepath.Join(s.fixturesDir, s.basePath)

	if filepath.Ext(name) != "" {
		fullPath := filepath.Join(base, name)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
		return fullPath, fmt.Errorf("fixture file not found: %s", fullPath)
	}

	dirPath := filepath.Join(base, name)
	if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
		return zipEPUBDir(dirPath)
	}

	epubPath := filepath.Join(base, name+".epub")
	if _, err := os.Stat(epubPath); err == nil {
		return epubPath, nil
	}

	return "", fmt.Errorf("fixture not found: tried %s (dir) and %s", dirPath, epubPath)
}

// markAsserted records that a message at the given index has been
// explicitly checked by an assertion step.
func (s *scenarioState) markAsserted(idx int) {
	if s.assertedIndices == nil {
		s.assertedIndices = make(map[int]bool)
	}
	s.assertedIndices[idx] = true
}

// zipEPUBDir creates a temporary .epub file from an unpackaged EPUB directory.
func zipEPUBDir(dir string) (string, error) {
	tmp, err := os.CreateTemp("", "epubverify-test-*.epub")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer tmp.Close()

	w := zip.NewWriter(tmp)
	defer w.Close()

	mimetypePath := filepath.Join(dir, "mimetype")
	if data, err := os.ReadFile(mimetypePath); err == nil {
		header := &zip.FileHeader{
			Name:   "mimetype",
			Method: zip.Store,
		}
		mw, err := w.CreateHeader(header)
		if err != nil {
			return "", fmt.Errorf("writing mimetype: %w", err)
		}
		if _, err := mw.Write(data); err != nil {
			return "", fmt.Errorf("writing mimetype data: %w", err)
		}
	}

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if rel == "mimetype" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}

		fw, err := w.Create(rel)
		if err != nil {
			return fmt.Errorf("creating zip entry %s: %w", rel, err)
		}
		if _, err := fw.Write(data); err != nil {
			return fmt.Errorf("writing zip entry %s: %w", rel, err)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking fixture dir: %w", err)
	}

	return tmp.Name(), nil
}

func initializeScenario(ctx *godog.ScenarioContext, fixturesDir string) {
	s := &scenarioState{
		fixturesDir: fixturesDir,
		epubVersion: "3",
		checkMode:   "epub",
	}

	// ================================================================
	// Given steps
	// ================================================================

	ctx.Step(`^EPUB test files located at '([^']*)'$`, func(path string) error {
		s.basePath = s.resolveFixturePath(path)
		return nil
	})
	ctx.Step(`^test files located at '([^']*)'$`, func(path string) error {
		s.basePath = s.resolveFixturePath(path)
		return nil
	})

	ctx.Step(`^EPUBCheck with default settings$`, func() error {
		s.epubVersion = "3"
		s.checkMode = "epub"
		return nil
	})
	ctx.Step(`^EPUBCheck configured to check EPUB 2(?:\.0\.1)? rules$`, func() error {
		s.epubVersion = "2"
		return nil
	})
	ctx.Step(`^EPUBCheck configured to check EPUB 3 rules$`, func() error {
		s.epubVersion = "3"
		return nil
	})
	ctx.Step(`^EPUBCheck configured to check a Package Document$`, func() error {
		s.checkMode = "package"
		return nil
	})
	ctx.Step(`^EPUBCheck configured to check an XHTML Content Document$`, func() error {
		s.checkMode = "xhtml"
		return nil
	})
	ctx.Step(`^EPUBCheck configured to check an SVG Content Document$`, func() error {
		s.checkMode = "svg"
		return nil
	})
	ctx.Step(`^EPUBCheck configured to check a navigation document$`, func() error {
		s.checkMode = "nav"
		return nil
	})
	ctx.Step(`^EPUBCheck configured with the '([^']*)' profile$`, func(profile string) error {
		s.profile = profile
		return nil
	})

	ctx.Step(`^the reporting level (?:is )?set to (\w+)\s*$`, func(level string) error {
		s.reportingLevel = strings.ToUpper(strings.TrimSpace(level))
		return nil
	})

	// ================================================================
	// When steps
	// ================================================================

	ctx.Step(`^checking EPUB '([^']*)'$`, func(name string) error {
		s.result = nil
		s.lastMessage = ""
		s.assertedIndices = nil

		path, err := s.fixtureFullPath(name)
		if err != nil {
			return fmt.Errorf("fixture lookup: %w", err)
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".opf", ".xhtml", ".svg", ".smil":
			return godog.ErrPending
		default:
			rpt, err := validate.Validate(path)
			if err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			s.result = rpt
		}
		return nil
	})

	ctx.Step(`^checking (?:file|document) '([^']*)'$`, func(name string) error {
		s.result = nil
		s.lastMessage = ""
		s.assertedIndices = nil

		path, err := s.fixtureFullPath(name)
		if err != nil {
			return fmt.Errorf("fixture lookup: %w", err)
		}

		ext := filepath.Ext(path)
		switch ext {
		case ".opf", ".xhtml", ".svg", ".smil":
			return godog.ErrPending
		default:
			// Full EPUB (directory or .epub)
			rpt, valErr := validate.Validate(path)
			if valErr != nil {
				return fmt.Errorf("validation failed: %w", valErr)
			}
			s.result = rpt
		}
		return nil
	})

	// ================================================================
	// Then steps — assertion helpers
	// ================================================================

	// No errors or warnings at all (also skips already-asserted messages)
	ctx.Step(`^no errors or warnings are reported\s*$`, func() error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		var issues []string
		for i, m := range s.result.Messages {
			if s.assertedIndices[i] {
				continue
			}
			if m.Severity == report.Fatal || m.Severity == report.Error || m.Severity == report.Warning {
				issues = append(issues, m.String())
			}
		}
		if len(issues) > 0 {
			return fmt.Errorf("expected no errors or warnings, but got:\n  %s", strings.Join(issues, "\n  "))
		}
		return nil
	})

	// No error or warning (singular)
	ctx.Step(`^no error or warning is reported$`, func() error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		var issues []string
		for i, m := range s.result.Messages {
			if s.assertedIndices[i] {
				continue
			}
			if m.Severity == report.Fatal || m.Severity == report.Error || m.Severity == report.Warning {
				issues = append(issues, m.String())
			}
		}
		if len(issues) > 0 {
			return fmt.Errorf("expected no errors or warnings, but got:\n  %s", strings.Join(issues, "\n  "))
		}
		return nil
	})

	// No OTHER errors or warnings (only un-asserted ones count)
	ctx.Step(`^no other errors or warnings are reported\s*$`, func() error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		var unexpected []string
		for i, m := range s.result.Messages {
			if s.assertedIndices[i] {
				continue
			}
			if m.Severity == report.Fatal || m.Severity == report.Error || m.Severity == report.Warning {
				unexpected = append(unexpected, m.String())
			}
		}
		if len(unexpected) > 0 {
			return fmt.Errorf("unexpected errors/warnings:\n  %s", strings.Join(unexpected, "\n  "))
		}
		return nil
	})

	// No usages
	ctx.Step(`^no (?:other )?usages are reported$`, func() error {
		return nil
	})

	// ----------------------------------------------------------------
	// Error assertions
	// ----------------------------------------------------------------

	// "error CODE is reported N times"
	ctx.Step(`^error ([A-Z]+-\d+\w*) is reported (\d+) times?\b`, func(code string, n int) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		count := 0
		for i, m := range s.result.Messages {
			if m.Severity == report.Error && m.CheckID == code {
				count++
				s.lastMessage = m.Message
				s.markAsserted(i)
			}
		}
		if count != n {
			return fmt.Errorf("expected error %s reported %d times, got %d.\nGot messages:\n%s",
				code, n, count, formatMessages(s.result.Messages))
		}
		return nil
	})

	// "error CODE is reported" (with optional parenthetical comment)
	ctx.Step(`^(?:the )?error ([A-Z]+-\d+\w*) is reported\b`, func(code string) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		for i, m := range s.result.Messages {
			if m.Severity == report.Error && m.CheckID == code {
				s.lastMessage = m.Message
				s.markAsserted(i)
				return nil
			}
		}
		return fmt.Errorf("expected error %s but it was not reported.\nGot messages:\n%s",
			code, formatMessages(s.result.Messages))
	})

	// ----------------------------------------------------------------
	// Fatal error assertions
	// ----------------------------------------------------------------

	ctx.Step(`^fatal error ([A-Z]+-\d+\w*) is reported\b`, func(code string) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		for i, m := range s.result.Messages {
			if m.Severity == report.Fatal && m.CheckID == code {
				s.lastMessage = m.Message
				s.markAsserted(i)
				return nil
			}
		}
		return fmt.Errorf("expected fatal error %s but it was not reported.\nGot messages:\n%s",
			code, formatMessages(s.result.Messages))
	})

	// ----------------------------------------------------------------
	// Warning assertions
	// ----------------------------------------------------------------

	ctx.Step(`^warning ([A-Z]+-\d+\w*) is reported (\d+) times?\b`, func(code string, n int) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		count := 0
		for i, m := range s.result.Messages {
			if m.Severity == report.Warning && m.CheckID == code {
				count++
				s.lastMessage = m.Message
				s.markAsserted(i)
			}
		}
		if count != n {
			return fmt.Errorf("expected warning %s reported %d times, got %d.\nGot messages:\n%s",
				code, n, count, formatMessages(s.result.Messages))
		}
		return nil
	})

	ctx.Step(`^warning ([A-Z]+-\d+\w*) is reported\b`, func(code string) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		for i, m := range s.result.Messages {
			if m.Severity == report.Warning && m.CheckID == code {
				s.lastMessage = m.Message
				s.markAsserted(i)
				return nil
			}
		}
		return fmt.Errorf("expected warning %s but it was not reported.\nGot messages:\n%s",
			code, formatMessages(s.result.Messages))
	})

	// ----------------------------------------------------------------
	// Info assertions
	// ----------------------------------------------------------------

	ctx.Step(`^info ([A-Z]+-\d+\w*) is reported (\d+) times?\b`, func(code string, n int) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		count := 0
		for i, m := range s.result.Messages {
			if m.Severity == report.Info && m.CheckID == code {
				count++
				s.lastMessage = m.Message
				s.markAsserted(i)
			}
		}
		if count != n {
			return fmt.Errorf("expected info %s reported %d times, got %d.\nGot messages:\n%s",
				code, n, count, formatMessages(s.result.Messages))
		}
		return nil
	})

	ctx.Step(`^info ([A-Z]+-\d+\w*) is reported\b`, func(code string) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		for i, m := range s.result.Messages {
			if m.Severity == report.Info && m.CheckID == code {
				s.lastMessage = m.Message
				s.markAsserted(i)
				return nil
			}
		}
		return fmt.Errorf("expected info %s but it was not reported.\nGot messages:\n%s",
			code, formatMessages(s.result.Messages))
	})

	// ----------------------------------------------------------------
	// Usage assertions
	// ----------------------------------------------------------------

	ctx.Step(`^[Uu]sage ([A-Z]+-\d+\w*) is reported (\d+) times?\b`, func(code string, n int) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		count := 0
		for i, m := range s.result.Messages {
			if m.Severity == report.Usage && m.CheckID == code {
				count++
				s.lastMessage = m.Message
				s.markAsserted(i)
			}
		}
		if count != n {
			return fmt.Errorf("expected usage %s reported %d times, got %d.\nGot messages:\n%s",
				code, n, count, formatMessages(s.result.Messages))
		}
		return nil
	})
	ctx.Step(`^[Uu]sage ([A-Z]+-\d+\w*) is reported\b`, func(code string) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		for i, m := range s.result.Messages {
			if m.Severity == report.Usage && m.CheckID == code {
				s.lastMessage = m.Message
				s.markAsserted(i)
				return nil
			}
		}
		return fmt.Errorf("expected usage %s but it was not reported.\nGot messages:\n%s",
			code, formatMessages(s.result.Messages))
	})

	// ----------------------------------------------------------------
	// Message content assertions
	// ----------------------------------------------------------------

	ctx.Step(`^the message contains "([^"]*)"$`, func(text string) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		if strings.Contains(s.lastMessage, text) {
			return nil
		}
		for _, m := range s.result.Messages {
			if strings.Contains(m.Message, text) {
				return nil
			}
		}
		return fmt.Errorf("no message contains %q.\nGot messages:\n%s",
			text, formatMessages(s.result.Messages))
	})

	ctx.Step(`^the message contains '([^']*)'`, func(text string) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		if strings.Contains(s.lastMessage, text) {
			return nil
		}
		for _, m := range s.result.Messages {
			if strings.Contains(m.Message, text) {
				return nil
			}
		}
		return fmt.Errorf("no message contains %q.\nGot messages:\n%s",
			text, formatMessages(s.result.Messages))
	})

	// ----------------------------------------------------------------
	// Table-based assertions
	// ----------------------------------------------------------------

	ctx.Step(`^the following errors are reported\b`, func(table *godog.Table) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		for _, row := range table.Rows {
			if len(row.Cells) < 2 {
				continue
			}
			code := strings.TrimSpace(row.Cells[0].Value)
			text := strings.TrimSpace(row.Cells[1].Value)
			found := false
			for i, m := range s.result.Messages {
				if m.Severity == report.Error && m.CheckID == code && strings.Contains(m.Message, text) {
					found = true
					s.markAsserted(i)
					break
				}
			}
			if !found {
				return fmt.Errorf("expected error %s with message containing %q not found.\nGot:\n%s",
					code, text, formatMessages(s.result.Messages))
			}
		}
		return nil
	})

	ctx.Step(`^(?:the )?following warnings are reported\b`, func(table *godog.Table) error {
		if s.result == nil {
			return fmt.Errorf("no validation result available")
		}
		for _, row := range table.Rows {
			if len(row.Cells) < 2 {
				continue
			}
			code := strings.TrimSpace(row.Cells[0].Value)
			text := strings.TrimSpace(row.Cells[1].Value)
			found := false
			for i, m := range s.result.Messages {
				if m.Severity == report.Warning && m.CheckID == code && strings.Contains(m.Message, text) {
					found = true
					s.markAsserted(i)
					break
				}
			}
			if !found {
				return fmt.Errorf("expected warning %s with message containing %q not found.\nGot:\n%s",
					code, text, formatMessages(s.result.Messages))
			}
		}
		return nil
	})

	// ----------------------------------------------------------------
	// Misc assertions
	// ----------------------------------------------------------------

	ctx.Step(`^all messages have line and column info$`, func() error {
		return godog.ErrPending
	})

	// ----------------------------------------------------------------
	// Filename checker steps
	// ----------------------------------------------------------------

	ctx.Step(`^checking file name '([^']*)'$`, func(name string) error {
		s.result = nil
		s.lastMessage = ""
		s.assertedIndices = nil
		epub2 := s.epubVersion == "2"
		s.result = validate.ValidateFilenameString(name, epub2)
		return nil
	})
	ctx.Step(`^checking file name containing code point (.+)$`, func(cp string) error {
		s.result = nil
		s.lastMessage = ""
		s.assertedIndices = nil
		// Parse "U+XXXX" to a rune
		cp = strings.TrimSpace(cp)
		cp = strings.TrimPrefix(cp, "U+")
		n, err := strconv.ParseInt(cp, 16, 32)
		if err != nil {
			return fmt.Errorf("invalid codepoint %q: %v", cp, err)
		}
		r := rune(n)
		name := "prefix" + string(r) + "suffix"
		epub2 := s.epubVersion == "2"
		s.result = validate.ValidateFilenameString(name, epub2)
		return nil
	})

	// ----------------------------------------------------------------
	// Viewport parser steps (pending — viewport parser not yet implemented)
	// ----------------------------------------------------------------

	ctx.Step(`^parsing viewport (.+)$`, func(vp string) error {
		return godog.ErrPending
	})
	ctx.Step(`^the parsed viewport equals (.+)$`, func(vp string) error {
		return godog.ErrPending
	})
	ctx.Step(`^error <error> is returned$`, func() error {
		return godog.ErrPending
	})
	ctx.Step(`^no error is returned$`, func() error {
		return godog.ErrPending
	})
}

// validateSingleOPF wraps a standalone .opf file in a minimal EPUB and validates it.
// This simulates epubcheck's "package document" check mode.
func validateSingleOPF(opfPath, fixturesDir, basePath string) (*report.Report, error) {
	opfData, err := os.ReadFile(opfPath)
	if err != nil {
		return nil, fmt.Errorf("reading OPF: %w", err)
	}

	tmp, err := os.CreateTemp("", "epubverify-opf-*.epub")
	if err != nil {
		return nil, fmt.Errorf("creating temp EPUB: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	w := zip.NewWriter(tmp)

	// mimetype
	header := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := w.CreateHeader(header)
	mw.Write([]byte("application/epub+zip"))

	// container.xml
	cw, _ := w.Create("META-INF/container.xml")
	cw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// The OPF itself
	ow, _ := w.Create("OEBPS/content.opf")
	ow.Write(opfData)

	// Parse the OPF to find referenced files and create stubs for them
	referencedFiles := extractManifestHrefsFromOPF(opfData)
	opfDir := filepath.Dir(opfPath)
	for _, href := range referencedFiles {
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			continue
		}
		// Try to find the real file alongside the OPF
		localPath := filepath.Join(opfDir, href)
		if data, readErr := os.ReadFile(localPath); readErr == nil {
			fw, _ := w.Create("OEBPS/" + href)
			fw.Write(data)
		} else {
			// Create a minimal stub
			fw, _ := w.Create("OEBPS/" + href)
			fw.Write([]byte{})
		}
	}

	w.Close()
	tmp.Close()

	rpt, err := validate.ValidateWithOptions(tmpPath, validate.Options{SingleFileMode: true})
	if err != nil {
		return nil, err
	}
	return rpt, nil
}

// extractManifestHrefsFromOPF parses an OPF and returns the manifest item hrefs.
func extractManifestHrefsFromOPF(data []byte) []string {
	var hrefs []string
	content := string(data)

	// Simple regex-based extraction to avoid XML namespace issues
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "<item") {
			continue
		}
		idx := strings.Index(line, `href="`)
		if idx < 0 {
			idx = strings.Index(line, `href='`)
		}
		if idx < 0 {
			continue
		}
		idx += 6
		quote := line[idx-1]
		end := strings.IndexByte(line[idx:], quote)
		if end > 0 {
			hrefs = append(hrefs, line[idx:idx+end])
		}
	}
	return hrefs
}

// formatMessages returns a human-readable string of all messages.
func formatMessages(msgs []report.Message) string {
	if len(msgs) == 0 {
		return "  (no messages)"
	}
	var sb strings.Builder
	for _, m := range msgs {
		sb.WriteString("  ")
		sb.WriteString(m.String())
		sb.WriteString("\n")
	}
	return sb.String()
}
