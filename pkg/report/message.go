package report

import "fmt"

// Severity levels for validation messages.
type Severity string

const (
	Fatal   Severity = "FATAL"
	Error   Severity = "ERROR"
	Warning Severity = "WARNING"
	Info    Severity = "INFO"
	Usage   Severity = "USAGE"
)

// Message represents a single validation finding.
type Message struct {
	Severity Severity `json:"severity"`
	CheckID  string   `json:"check_id"`
	Message  string   `json:"message"`
	Location string   `json:"location,omitempty"`
}

func (m Message) String() string {
	if m.Location != "" {
		return fmt.Sprintf("%s(%s): %s [%s]", m.Severity, m.CheckID, m.Message, m.Location)
	}
	return fmt.Sprintf("%s(%s): %s", m.Severity, m.CheckID, m.Message)
}

// Report collects all messages from a validation run.
type Report struct {
	Messages []Message `json:"messages"`
}

// NewReport creates an empty report.
func NewReport() *Report {
	return &Report{}
}

// Add appends a message to the report.
func (r *Report) Add(sev Severity, checkID string, msg string) {
	r.Messages = append(r.Messages, Message{
		Severity: sev,
		CheckID:  checkID,
		Message:  msg,
	})
}

// AddWithLocation appends a message with a location to the report.
func (r *Report) AddWithLocation(sev Severity, checkID string, msg string, location string) {
	r.Messages = append(r.Messages, Message{
		Severity: sev,
		CheckID:  checkID,
		Message:  msg,
		Location: location,
	})
}

// FatalCount returns the number of FATAL messages.
func (r *Report) FatalCount() int {
	n := 0
	for _, m := range r.Messages {
		if m.Severity == Fatal {
			n++
		}
	}
	return n
}

// ErrorCount returns the number of ERROR messages.
func (r *Report) ErrorCount() int {
	n := 0
	for _, m := range r.Messages {
		if m.Severity == Error {
			n++
		}
	}
	return n
}

// WarningCount returns the number of WARNING messages.
func (r *Report) WarningCount() int {
	n := 0
	for _, m := range r.Messages {
		if m.Severity == Warning {
			n++
		}
	}
	return n
}

// IsValid returns true if there are no FATAL or ERROR messages.
func (r *Report) IsValid() bool {
	return r.FatalCount() == 0 && r.ErrorCount() == 0
}

// DowngradeToInfo changes the severity of WARNING messages whose CheckID
// is in the given set to INFO. This is used to suppress warnings for
// checks that are valid but not flagged by the reference implementation.
func (r *Report) DowngradeToInfo(checkIDs map[string]bool) {
	for i := range r.Messages {
		if r.Messages[i].Severity == Warning && checkIDs[r.Messages[i].CheckID] {
			r.Messages[i].Severity = Info
		}
	}
}
