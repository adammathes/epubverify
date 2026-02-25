package validate

import (
	"testing"

	"github.com/adammathes/epubverify/pkg/report"
)

func TestCheckEpubTypeValid_PlainTypeAttribute(t *testing.T) {
	// A <style type="text/css"> should NOT trigger HTM-015
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <style type="text/css">body { color: black; }</style>
</head>
<body><p>Hello</p></body>
</html>`

	r := report.NewReport()
	checkEpubTypeValid([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "HTM-015" {
			t.Errorf("plain type attribute should not trigger HTM-015, got: %s", m.Message)
		}
	}
}

func TestCheckEpubTypeValid_NamespacedEpubType(t *testing.T) {
	// A proper epub:type attribute with a valid value should NOT trigger HTM-015
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Test</title></head>
<body>
  <nav epub:type="toc"><ol><li>Chapter 1</li></ol></nav>
</body>
</html>`

	r := report.NewReport()
	checkEpubTypeValid([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "HTM-015" {
			t.Errorf("valid epub:type should not trigger HTM-015, got: %s", m.Message)
		}
	}
}

// --- Block-in-Phrasing Content Model Tests ---

func TestCheckBlockInPhrasing_DivInP(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <p>text <div>block inside paragraph</div></p>
</body>
</html>`

	r := report.NewReport()
	checkBlockInPhrasing([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <p>")
	}
}

func TestCheckBlockInPhrasing_DivInH1(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <h1><div>block inside heading</div></h1>
</body>
</html>`

	r := report.NewReport()
	checkBlockInPhrasing([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <h1>")
	}
}

func TestCheckBlockInPhrasing_DivInSpan(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <p><span><div>block inside span</div></span></p>
</body>
</html>`

	r := report.NewReport()
	checkBlockInPhrasing([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <span>")
	}
}

func TestCheckBlockInPhrasing_NoFalsePositive(t *testing.T) {
	// Valid: <div> inside <div> is fine (both are flow content)
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <div><div>nested divs are OK</div></div>
  <section><p>paragraph in section is OK</p></section>
  <p><em><strong>nested inline is OK</strong></em></p>
</body>
</html>`

	r := report.NewReport()
	checkBlockInPhrasing([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for valid nesting: %s", m.Message)
		}
	}
}

func TestCheckBlockInPhrasing_SkipsSVGMathML(t *testing.T) {
	// SVG and MathML content should not trigger block-in-phrasing errors
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <p>
    <svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>
    <math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi></math>
  </p>
</body>
</html>`

	r := report.NewReport()
	checkBlockInPhrasing([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for SVG/MathML in <p>: %s", m.Message)
		}
	}
}

// --- Restricted Children Tests ---

func TestCheckRestrictedChildren_DivInUl(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <ul><div>div inside ul</div></ul>
</body>
</html>`

	r := report.NewReport()
	checkRestrictedChildren([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <ul>")
	}
}

func TestCheckRestrictedChildren_DivInTr(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <table><tr><div>div inside tr</div></tr></table>
</body>
</html>`

	r := report.NewReport()
	checkRestrictedChildren([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <tr>")
	}
}

func TestCheckRestrictedChildren_NoFalsePositive(t *testing.T) {
	// Valid nesting: <li> inside <ul>, <td> inside <tr>
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <ul><li>item 1</li><li>item 2</li></ul>
  <ol><li>item 1</li></ol>
  <table><tr><td>cell</td><th>header</th></tr></table>
</body>
</html>`

	r := report.NewReport()
	checkRestrictedChildren([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for valid nesting: %s", m.Message)
		}
	}
}

func TestCheckEpubTypeValid_InvalidEpubType(t *testing.T) {
	// A proper epub:type attribute with an invalid value SHOULD trigger HTM-015
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Test</title></head>
<body>
  <section epub:type="madeupvalue"><p>Hello</p></section>
</body>
</html>`

	r := report.NewReport()
	checkEpubTypeValid([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "HTM-015" {
			found = true
			break
		}
	}
	if !found {
		t.Error("invalid epub:type value should trigger HTM-015")
	}
}
