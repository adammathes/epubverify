package report

import (
	"fmt"
	"io"
)

// WriteText writes human-readable validation output to w.
func (r *Report) WriteText(w io.Writer) {
	for _, m := range r.Messages {
		fmt.Fprintln(w, m.String())
	}
	if r.IsValid() {
		fmt.Fprintln(w, "No errors or warnings detected.")
	} else {
		fmt.Fprintf(w, "Check finished. Errors: %d, Warnings: %d, Fatal: %d\n",
			r.ErrorCount(), r.WarningCount(), r.FatalCount())
	}
}
