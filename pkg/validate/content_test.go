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

// --- Void Element Children Tests ---

func TestCheckVoidElementChildren_ChildInBr(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <p>text <br><span>child inside br</span></br> more</p>
</body>
</html>`

	r := report.NewReport()
	checkVoidElementChildren([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for child element inside <br>")
	}
}

func TestCheckVoidElementChildren_ChildInHr(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <hr><p>content inside hr</p></hr>
</body>
</html>`

	r := report.NewReport()
	checkVoidElementChildren([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for child element inside <hr>")
	}
}

func TestCheckVoidElementChildren_NoFalsePositive(t *testing.T) {
	// Self-closing void elements should not trigger errors
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <p>text <br/> more <img src="test.png" alt="test"/> end</p>
  <hr/>
  <p><input type="text"/></p>
</body>
</html>`

	r := report.NewReport()
	checkVoidElementChildren([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for self-closing void elements: %s", m.Message)
		}
	}
}

// --- Table Content Model Tests ---

func TestCheckTableContentModel_PInTable(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <table><p>paragraph directly in table</p></table>
</body>
</html>`

	r := report.NewReport()
	checkTableContentModel([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <p> as direct child of <table>")
	}
}

func TestCheckTableContentModel_DivInTable(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <table><div>div directly in table</div></table>
</body>
</html>`

	r := report.NewReport()
	checkTableContentModel([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> as direct child of <table>")
	}
}

func TestCheckTableContentModel_NoFalsePositive(t *testing.T) {
	// Valid table structure: only valid children
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <table>
    <caption>A table</caption>
    <thead><tr><th>Header</th></tr></thead>
    <tbody><tr><td>Cell</td></tr></tbody>
    <tfoot><tr><td>Footer</td></tr></tfoot>
  </table>
</body>
</html>`

	r := report.NewReport()
	checkTableContentModel([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for valid table structure: %s", m.Message)
		}
	}
}

// --- DL/Hgroup Restricted Children Tests ---

func TestCheckRestrictedChildren_SpanInDl(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <dl><span>span inside dl</span></dl>
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
		t.Error("expected RSC-005 for <span> inside <dl>")
	}
}

func TestCheckRestrictedChildren_DlValidChildren(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <dl><dt>Term</dt><dd>Definition</dd></dl>
  <dl><div><dt>Term</dt><dd>Definition</dd></div></dl>
</body>
</html>`

	r := report.NewReport()
	checkRestrictedChildren([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for valid dl structure: %s", m.Message)
		}
	}
}

func TestCheckRestrictedChildren_DivInHgroup(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <hgroup><div>div inside hgroup</div></hgroup>
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
		t.Error("expected RSC-005 for <div> inside <hgroup>")
	}
}

func TestCheckRestrictedChildren_HgroupValidChildren(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <hgroup><h1>Title</h1><p>Subtitle</p></hgroup>
</body>
</html>`

	r := report.NewReport()
	checkRestrictedChildren([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for valid hgroup: %s", m.Message)
		}
	}
}

// --- Interactive Nesting Tests ---

func TestCheckInteractiveNesting_ButtonInA(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <a href="#"><button>click me</button></a>
</body>
</html>`

	r := report.NewReport()
	checkInteractiveNesting([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <button> inside <a>")
	}
}

func TestCheckInteractiveNesting_InputInButton(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <button><input type="text"/></button>
</body>
</html>`

	r := report.NewReport()
	checkInteractiveNesting([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <input> inside <button>")
	}
}

func TestCheckInteractiveNesting_NoFalsePositive(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <a href="#">link</a>
  <button>button</button>
  <div><a href="#">link in div</a><button>button in div</button></div>
</body>
</html>`

	r := report.NewReport()
	checkInteractiveNesting([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for non-nested interactive: %s", m.Message)
		}
	}
}

// --- Transparent Content Model Tests ---

func TestCheckTransparentContentModel_DivInAInP(t *testing.T) {
	// <p><a><div>...</div></a></p> — <a> inherits phrasing from <p>
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <p><a href="#"><div>block in transparent a in p</div></a></p>
</body>
</html>`

	r := report.NewReport()
	checkTransparentContentModel([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <a> inside <p> (transparent inheritance)")
	}
}

func TestCheckTransparentContentModel_DivInInsInSpan(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <p><ins><div>block in ins in p</div></ins></p>
</body>
</html>`

	r := report.NewReport()
	checkTransparentContentModel([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <ins> inside <p> (transparent inheritance)")
	}
}

func TestCheckTransparentContentModel_NoFalsePositive(t *testing.T) {
	// <div><a><div>...</div></a></div> — <a> inherits flow from <div>, so block is OK
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <div><a href="#"><div>block in a in div is OK</div></a></div>
  <section><ins><p>paragraph in ins in section is OK</p></ins></section>
</body>
</html>`

	r := report.NewReport()
	checkTransparentContentModel([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005: %s", m.Message)
		}
	}
}

// --- Figcaption Position Tests ---

func TestCheckFigcaptionPosition_MiddleFigcaption(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <figure>
    <p>Before</p>
    <figcaption>Caption in middle</figcaption>
    <p>After</p>
  </figure>
</body>
</html>`

	r := report.NewReport()
	checkFigcaptionPosition([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <figcaption> in middle of <figure>")
	}
}

func TestCheckFigcaptionPosition_FirstOrLast(t *testing.T) {
	// Valid positions: first child or last child
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <figure>
    <figcaption>Caption first</figcaption>
    <p>Content</p>
  </figure>
  <figure>
    <p>Content</p>
    <figcaption>Caption last</figcaption>
  </figure>
</body>
</html>`

	r := report.NewReport()
	checkFigcaptionPosition([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for valid figcaption position: %s", m.Message)
		}
	}
}

// --- Picture Content Model Tests ---

func TestCheckPictureContentModel_InvalidChild(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <picture>
    <div>invalid child</div>
    <img src="test.png" alt="test"/>
  </picture>
</body>
</html>`

	r := report.NewReport()
	checkPictureContentModel([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <div> inside <picture>")
	}
}

func TestCheckPictureContentModel_SourceAfterImg(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <picture>
    <img src="test.png" alt="test"/>
    <source srcset="test2.png"/>
  </picture>
</body>
</html>`

	r := report.NewReport()
	checkPictureContentModel([]byte(xhtml), "test.xhtml", r)

	found := false
	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected RSC-005 for <source> after <img> in <picture>")
	}
}

func TestCheckPictureContentModel_ValidStructure(t *testing.T) {
	xhtml := `<?xml version="1.0" encoding="UTF-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Test</title></head>
<body>
  <picture>
    <source srcset="large.png" media="(min-width: 800px)"/>
    <source srcset="small.png"/>
    <img src="fallback.png" alt="test"/>
  </picture>
</body>
</html>`

	r := report.NewReport()
	checkPictureContentModel([]byte(xhtml), "test.xhtml", r)

	for _, m := range r.Messages {
		if m.CheckID == "RSC-005" {
			t.Errorf("unexpected RSC-005 for valid picture structure: %s", m.Message)
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
