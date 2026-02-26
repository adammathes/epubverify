// generate-test-epubs.go creates EPUB test files of various sizes for benchmarking.
package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir := "benchmarks/corpus"
	os.MkdirAll(dir, 0755)

	sizes := []struct {
		name     string
		chapters int
		paraPerCh int
	}{
		{"tiny-1ch", 1, 5},
		{"small-5ch", 5, 20},
		{"medium-20ch", 20, 50},
		{"large-50ch", 50, 100},
		{"xlarge-100ch", 100, 200},
	}

	for _, s := range sizes {
		path := filepath.Join(dir, s.name+".epub")
		if err := generateEPUB(path, s.chapters, s.paraPerCh); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating %s: %v\n", path, err)
			os.Exit(1)
		}
		fi, _ := os.Stat(path)
		fmt.Printf("Generated %s (%d KB)\n", path, fi.Size()/1024)
	}
}

func generateEPUB(path string, chapters, parasPerChapter int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// mimetype must be first, stored (not compressed)
	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, err := w.CreateHeader(mh)
	if err != nil {
		return err
	}
	mw.Write([]byte("application/epub+zip"))

	// META-INF/container.xml
	addFile(w, "META-INF/container.xml", `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)

	// Navigation document
	var navItems strings.Builder
	for i := 1; i <= chapters; i++ {
		navItems.WriteString(fmt.Sprintf(`      <li><a href="chapter%d.xhtml">Chapter %d</a></li>`+"\n", i, i))
	}

	addFile(w, "OEBPS/nav.xhtml", fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Table of Contents</title></head>
<body>
  <nav epub:type="toc" id="toc">
    <h1>Table of Contents</h1>
    <ol>
%s    </ol>
  </nav>
</body>
</html>`, navItems.String()))

	// OPF package document
	var manifestItems strings.Builder
	var spineItems strings.Builder

	manifestItems.WriteString(`    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")
	manifestItems.WriteString(`    <item id="style" href="style.css" media-type="text/css"/>` + "\n")

	for i := 1; i <= chapters; i++ {
		manifestItems.WriteString(fmt.Sprintf(`    <item id="ch%d" href="chapter%d.xhtml" media-type="application/xhtml+xml"/>`, i, i))
		manifestItems.WriteString("\n")
		spineItems.WriteString(fmt.Sprintf(`    <itemref idref="ch%d"/>`, i))
		spineItems.WriteString("\n")
	}

	addFile(w, "OEBPS/content.opf", fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="uid">urn:uuid:bench-%d-%d</dc:identifier>
    <dc:title>Benchmark EPUB (%d chapters)</dc:title>
    <dc:language>en</dc:language>
    <meta property="dcterms:modified">2024-01-01T00:00:00Z</meta>
  </metadata>
  <manifest>
%s  </manifest>
  <spine>
%s  </spine>
</package>`, chapters, parasPerChapter, chapters, manifestItems.String(), spineItems.String()))

	// CSS
	addFile(w, "OEBPS/style.css", `body { font-family: serif; margin: 1em; line-height: 1.6; }
h1 { font-size: 1.5em; margin-bottom: 0.5em; }
p { text-indent: 1.5em; margin: 0.3em 0; }
.emphasis { font-style: italic; }
.bold { font-weight: bold; }`)

	// Generate chapter content
	loremParagraphs := []string{
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.",
		"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.",
		"Curabitur pretium tincidunt lacus. Nulla gravida orci a odio. Nullam varius, turpis et commodo pharetra, est eros bibendum elit, nec luctus magna felis sollicitudin mauris. Integer in mauris eu nibh euismod gravida.",
		"Praesent blandit dolor. Sed non quam. In vel mi sit amet augue congue elementum. Morbi in ipsum sit amet pede facilisis laoreet. Donec lacus nunc, viverra nec, blandit vel, egestas et, augue.",
		"Vestibulum tincidunt malesuada tellus. Ut ultrices ultrices enim. Curabitur sit amet mauris. Morbi in dui quis est pulvinar ullamcorper. Nulla facilisi. Integer lacinia sollicitudin massa.",
	}

	for i := 1; i <= chapters; i++ {
		var body strings.Builder
		for j := 0; j < parasPerChapter; j++ {
			p := loremParagraphs[j%len(loremParagraphs)]
			if j%3 == 0 {
				body.WriteString(fmt.Sprintf(`    <p class="emphasis">%s</p>`+"\n", p))
			} else if j%5 == 0 {
				body.WriteString(fmt.Sprintf(`    <p class="bold">%s</p>`+"\n", p))
			} else {
				body.WriteString(fmt.Sprintf(`    <p>%s</p>`+"\n", p))
			}
		}

		addFile(w, fmt.Sprintf("OEBPS/chapter%d.xhtml", i), fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <title>Chapter %d</title>
  <link rel="stylesheet" type="text/css" href="style.css"/>
</head>
<body>
  <h1>Chapter %d: The Journey Continues</h1>
%s</body>
</html>`, i, i, body.String()))
	}

	return nil
}

func addFile(w *zip.Writer, name, content string) {
	fw, _ := w.Create(name)
	fw.Write([]byte(content))
}
