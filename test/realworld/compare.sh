#!/bin/bash
#
# compare.sh - Compare epubverify results against reference epubcheck
#
# Runs both validators against all sample EPUBs and reports differences
# in validity verdicts, error counts, and warning counts.
#
# Prerequisites:
#   - epubverify binary (built via `go build` or `make build`)
#   - epubcheck JAR (Java must be installed)
#   - Sample EPUBs (run download-samples.sh first)
#
# Usage:
#   ./compare.sh [path-to-epubverify] [path-to-epubcheck.jar]
#
# Environment variables (optional):
#   EPUBVERIFY_BIN   Path to epubverify binary
#   EPUBCHECK_JAR    Path to epubcheck JAR file

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SAMPLES_DIR="$SCRIPT_DIR/samples"
RESULTS_DIR="$SCRIPT_DIR/results"

EPUBVERIFY_BIN="${1:-${EPUBVERIFY_BIN:-$SCRIPT_DIR/../../epubverify}}"
EPUBCHECK_JAR="${2:-${EPUBCHECK_JAR:-$HOME/tools/epubcheck-5.2.0/epubcheck.jar}}"

# Validate prerequisites
if [[ ! -x "$EPUBVERIFY_BIN" ]]; then
  echo "ERROR: epubverify binary not found at $EPUBVERIFY_BIN"
  echo "  Build it first: go build -o epubverify ."
  exit 2
fi

if [[ ! -f "$EPUBCHECK_JAR" ]]; then
  echo "ERROR: epubcheck JAR not found at $EPUBCHECK_JAR"
  echo "  Download it from https://github.com/w3c/epubcheck/releases"
  exit 2
fi

if ! ls "$SAMPLES_DIR"/*.epub >/dev/null 2>&1; then
  echo "ERROR: No sample EPUBs found in $SAMPLES_DIR"
  echo "  Run download-samples.sh first"
  exit 2
fi

mkdir -p "$RESULTS_DIR"

echo "epubverify: $EPUBVERIFY_BIN"
echo "epubcheck:  $EPUBCHECK_JAR"
echo "samples:    $SAMPLES_DIR"
echo ""

# Table header
printf "%-40s | %-12s | %-12s | %s\n" "SAMPLE" "EPUBVERIFY" "EPUBCHECK" "MATCH"
echo "-----------------------------------------+--------------+--------------+------"

pass=0
fail=0
total=0

for epub in "$SAMPLES_DIR"/*.epub; do
  name=$(basename "$epub" .epub)
  total=$((total + 1))

  # Run epubverify
  ev_json="$RESULTS_DIR/${name}.epubverify.json"
  "$EPUBVERIFY_BIN" "$epub" --json "$ev_json" >/dev/null 2>&1 || true

  ev_valid=$(python3 -c "import json; d=json.load(open('$ev_json')); print('VALID' if d['valid'] else 'INVALID')")
  ev_errors=$(python3 -c "import json; d=json.load(open('$ev_json')); print(d['error_count'])")
  ev_warnings=$(python3 -c "import json; d=json.load(open('$ev_json')); print(d['warning_count'])")

  # Run epubcheck
  ec_json="$RESULTS_DIR/${name}.epubcheck.json"
  java -jar "$EPUBCHECK_JAR" "$epub" --json "$ec_json" >/dev/null 2>&1 || true

  ec_valid=$(python3 -c "
import json
d=json.load(open('$ec_json'))
msgs = d.get('messages', [])
errors = sum(1 for m in msgs if m.get('severity') in ('ERROR', 'FATAL'))
print('VALID' if errors == 0 else 'INVALID')
")
  ec_errors=$(python3 -c "
import json
d=json.load(open('$ec_json'))
msgs = d.get('messages', [])
print(sum(1 for m in msgs if m.get('severity') in ('ERROR', 'FATAL')))
")
  ec_warnings=$(python3 -c "
import json
d=json.load(open('$ec_json'))
msgs = d.get('messages', [])
print(sum(1 for m in msgs if m.get('severity') == 'WARNING'))
")

  ev_summary="${ev_valid} E:${ev_errors} W:${ev_warnings}"
  ec_summary="${ec_valid} E:${ec_errors} W:${ec_warnings}"

  if [[ "$ev_valid" == "$ec_valid" ]]; then
    match="YES"
    pass=$((pass + 1))
  else
    match="NO ***"
    fail=$((fail + 1))
  fi

  printf "%-40s | %-12s | %-12s | %s\n" "$name" "$ev_summary" "$ec_summary" "$match"
done

echo ""
echo "Results: $pass/$total match on validity verdict ($fail mismatches)"

if [[ $fail -gt 0 ]]; then
  echo ""
  echo "MISMATCHES DETECTED - check results in $RESULTS_DIR"
  echo "  epubverify JSON: \$RESULTS_DIR/<name>.epubverify.json"
  echo "  epubcheck  JSON: \$RESULTS_DIR/<name>.epubcheck.json"
  exit 1
fi

echo "All samples agree on validity."
exit 0
