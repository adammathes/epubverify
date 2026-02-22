package main

import (
	"fmt"
	"os"

	"github.com/adammathes/epubverify/pkg/doctor"
	"github.com/adammathes/epubverify/pkg/report"
	"github.com/adammathes/epubverify/pkg/validate"
)

const version = "0.1.0"

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: epubverify <file.epub> [--json <output.json | ->] [--doctor [-o output.epub]] [--version]")
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
	var doctorMode bool
	var doctorOutput string

	for i := 1; i < len(args); i++ {
		if args[i] == "--json" && i+1 < len(args) {
			jsonOutput = args[i+1]
			i++
		}
		if args[i] == "--doctor" {
			doctorMode = true
		}
		if args[i] == "-o" && i+1 < len(args) {
			doctorOutput = args[i+1]
			i++
		}
	}

	if doctorMode {
		runDoctor(epubPath, doctorOutput)
		return
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

func runDoctor(inputPath, outputPath string) {
	result, err := doctor.Repair(inputPath, outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Doctor error: %v\n", err)
		os.Exit(2)
	}

	beforeErrors := result.BeforeReport.ErrorCount() + result.BeforeReport.FatalCount()
	beforeWarnings := result.BeforeReport.WarningCount()

	if len(result.Fixes) == 0 {
		fmt.Fprintf(os.Stderr, "No fixable issues found (%d errors, %d warnings remain).\n", beforeErrors, beforeWarnings)
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "Applied %d fixes:\n", len(result.Fixes))
	for _, fix := range result.Fixes {
		if fix.File != "" {
			fmt.Fprintf(os.Stderr, "  [%s] %s (%s)\n", fix.CheckID, fix.Description, fix.File)
		} else {
			fmt.Fprintf(os.Stderr, "  [%s] %s\n", fix.CheckID, fix.Description)
		}
	}

	afterErrors := result.AfterReport.ErrorCount() + result.AfterReport.FatalCount()
	afterWarnings := result.AfterReport.WarningCount()

	fmt.Fprintf(os.Stderr, "\nBefore: %d errors, %d warnings\n", beforeErrors, beforeWarnings)
	fmt.Fprintf(os.Stderr, "After:  %d errors, %d warnings\n", afterErrors, afterWarnings)

	if outputPath == "" {
		outputPath = inputPath + ".fixed.epub"
	}
	fmt.Fprintf(os.Stderr, "Output: %s\n", outputPath)
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
