package main

import (
	"fmt"
	"os"

	"github.com/adammathes/epubverify/pkg/report"
	"github.com/adammathes/epubverify/pkg/validate"
)

const version = "0.1.0"

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: epubverify <file.epub> [--json <output.json | ->] [--version]")
		os.Exit(2)
	}

	// Handle --version
	for _, arg := range args {
		if arg == "--version" {
			fmt.Printf("epubverify %s\n", version)
			os.Exit(0)
		}
	}

	epubPath := args[0]
	var jsonOutput string

	for i := 1; i < len(args); i++ {
		if args[i] == "--json" && i+1 < len(args) {
			jsonOutput = args[i+1]
			i++
		}
	}

	r, err := validate.Validate(epubPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(2)
	}

	// Text output to stderr
	r.WriteText(os.Stderr)

	// JSON output: always write to stdout for tool interop, and to file if --json specified
	if jsonOutput == "" || jsonOutput == "-" {
		if err := r.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(2)
		}
	} else {
		// Write to both stdout (for piping) and the specified file
		if err := r.WriteJSON(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(2)
		}
		if err := writeJSON(r, jsonOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
			os.Exit(2)
		}
	}

	// Exit codes: 0=valid, 1=errors, 2=fatal
	if r.FatalCount() > 0 {
		os.Exit(2)
	}
	if r.ErrorCount() > 0 {
		os.Exit(1)
	}
	os.Exit(0)
}

func writeJSON(r *report.Report, path string) error {
	if path == "-" {
		return r.WriteJSON(os.Stdout)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return r.WriteJSON(f)
}
