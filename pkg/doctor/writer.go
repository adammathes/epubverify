package doctor

import (
	"archive/zip"
	"io"
	"os"
)

// writeEPUB creates a new EPUB file from modified in-memory contents.
// It ensures the mimetype entry is written first, stored (not compressed),
// with no extra field — satisfying OCF-002 through OCF-005.
func writeEPUB(path string, files map[string][]byte, originalZip *zip.ReadCloser) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// Step 1: Write mimetype first, stored, no extra field.
	if mimedata, ok := files["mimetype"]; ok {
		header := &zip.FileHeader{
			Name:   "mimetype",
			Method: zip.Store,
		}
		header.SetMode(0644)
		// Ensure no extra field by leaving Extra nil
		mw, err := w.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := mw.Write(mimedata); err != nil {
			return err
		}
	}

	// Step 2: Write all other files.
	// Preserve original compression method and order from the original zip.
	for _, original := range originalZip.File {
		if original.Name == "mimetype" {
			continue // Already written
		}

		header := original.FileHeader
		// Use the modified content if available, otherwise copy original
		if modified, ok := files[original.Name]; ok {
			mw, err := w.CreateHeader(&header)
			if err != nil {
				return err
			}
			if _, err := mw.Write(modified); err != nil {
				return err
			}
		} else {
			// Copy original file unchanged
			rc, err := original.Open()
			if err != nil {
				return err
			}
			mw, err := w.CreateHeader(&header)
			if err != nil {
				rc.Close()
				return err
			}
			if _, err := io.Copy(mw, rc); err != nil {
				rc.Close()
				return err
			}
			rc.Close()
		}
	}

	return nil
}
