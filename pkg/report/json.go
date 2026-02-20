package report

import (
	"encoding/json"
	"io"
)

// JSONOutput is the JSON structure written to output files.
type JSONOutput struct {
	Valid        bool      `json:"valid"`
	Messages     []Message `json:"messages"`
	FatalCount   int       `json:"fatal_count"`
	ErrorCount   int       `json:"error_count"`
	WarningCount int       `json:"warning_count"`
}

// WriteJSON writes the report in JSON format to w.
func (r *Report) WriteJSON(w io.Writer) error {
	out := JSONOutput{
		Valid:        r.IsValid(),
		Messages:     r.Messages,
		FatalCount:   r.FatalCount(),
		ErrorCount:   r.ErrorCount(),
		WarningCount: r.WarningCount(),
	}
	if out.Messages == nil {
		out.Messages = []Message{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
