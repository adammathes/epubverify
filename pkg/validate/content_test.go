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
