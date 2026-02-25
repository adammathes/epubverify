package validate

import "strings"

// ViewportParseError represents a parsing error type for viewport meta content.
type ViewportParseError string

const (
	ViewportNullOrEmpty       ViewportParseError = "NULL_OR_EMPTY"
	ViewportAssignUnexpected  ViewportParseError = "ASSIGN_UNEXPECTED"
	ViewportValueEmpty        ViewportParseError = "VALUE_EMPTY"
	ViewportNameEmpty         ViewportParseError = "NAME_EMPTY"
	ViewportLeadingSeparator  ViewportParseError = "LEADING_SEPARATOR"
	ViewportTrailingSeparator ViewportParseError = "TRAILING_SEPARATOR"
)

// ViewportProperty represents a single parsed viewport property with its name
// and associated values.
type ViewportProperty struct {
	Name   string
	Values []string // nil for name-only properties (no '=' sign)
}

// ViewportResult holds the result of parsing a viewport meta content string.
type ViewportResult struct {
	Properties []ViewportProperty
	Err        ViewportParseError // non-empty if parsing failed
}

// String returns the normalized string representation of the parsed viewport.
// Properties are separated by ";", multiple values for the same key are joined
// by ",". Name-only properties appear as "name=".
func (r ViewportResult) String() string {
	var parts []string
	for _, p := range r.Properties {
		if len(p.Values) == 0 {
			parts = append(parts, p.Name+"=")
		} else {
			parts = append(parts, p.Name+"="+strings.Join(p.Values, ","))
		}
	}
	return strings.Join(parts, ";")
}

// ParseViewport parses a viewport meta tag content string according to the
// EPUB 3.3 viewport meta syntax.
// See https://www.w3.org/TR/epub-33/#app-viewport-meta-syntax
func ParseViewport(input string) ViewportResult {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ViewportResult{Err: ViewportNullOrEmpty}
	}

	if isViewportSeparator(trimmed[0]) {
		return ViewportResult{Err: ViewportLeadingSeparator}
	}
	if isViewportSeparator(trimmed[len(trimmed)-1]) {
		return ViewportResult{Err: ViewportTrailingSeparator}
	}

	pos := 0

	// Ordered map: track first-seen key order and collect values per key.
	type entry struct {
		values []string
	}
	var order []string
	props := make(map[string]*entry)

	for pos < len(trimmed) {
		// Skip whitespace and separators between properties.
		for pos < len(trimmed) && (isViewportWhitespace(trimmed[pos]) || isViewportSeparator(trimmed[pos])) {
			pos++
		}
		if pos >= len(trimmed) {
			break
		}

		// Parse name: characters until '=', separator, whitespace, or end.
		nameStart := pos
		for pos < len(trimmed) && trimmed[pos] != '=' && !isViewportSeparator(trimmed[pos]) && !isViewportWhitespace(trimmed[pos]) {
			pos++
		}
		name := strings.ToLower(trimmed[nameStart:pos])
		if name == "" {
			return ViewportResult{Err: ViewportNameEmpty}
		}

		// Skip whitespace after name.
		for pos < len(trimmed) && isViewportWhitespace(trimmed[pos]) {
			pos++
		}

		if pos < len(trimmed) && trimmed[pos] == '=' {
			// Skip '='.
			pos++

			// Skip whitespace after '='.
			for pos < len(trimmed) && isViewportWhitespace(trimmed[pos]) {
				pos++
			}

			// Check for unexpected '=' (e.g. "p1==v1").
			if pos < len(trimmed) && trimmed[pos] == '=' {
				return ViewportResult{Err: ViewportAssignUnexpected}
			}

			// Parse value: characters until '=', separator, whitespace, or end.
			valueStart := pos
			for pos < len(trimmed) && trimmed[pos] != '=' && !isViewportSeparator(trimmed[pos]) && !isViewportWhitespace(trimmed[pos]) {
				pos++
			}
			value := trimmed[valueStart:pos]

			if value == "" {
				return ViewportResult{Err: ViewportValueEmpty}
			}

			// Check for unexpected '=' after value (e.g. "p1=v1=v").
			if pos < len(trimmed) && trimmed[pos] == '=' {
				return ViewportResult{Err: ViewportAssignUnexpected}
			}

			// Record property value.
			if e, ok := props[name]; ok {
				e.values = append(e.values, value)
			} else {
				order = append(order, name)
				props[name] = &entry{values: []string{value}}
			}
		} else if pos >= len(trimmed) || isViewportSeparator(trimmed[pos]) {
			// Name-only property (no '=' sign).
			if _, ok := props[name]; !ok {
				order = append(order, name)
				props[name] = &entry{}
			}
		} else {
			// After name + whitespace, next char is not '=', not separator,
			// not end — e.g. "p1 v1" where '=' is missing.
			return ViewportResult{Err: ViewportValueEmpty}
		}
	}

	// Build result in first-seen key order.
	result := make([]ViewportProperty, len(order))
	for i, name := range order {
		result[i] = ViewportProperty{
			Name:   name,
			Values: props[name].values,
		}
	}

	return ViewportResult{Properties: result}
}

func isViewportWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isViewportSeparator(c byte) bool {
	return c == ',' || c == ';'
}
