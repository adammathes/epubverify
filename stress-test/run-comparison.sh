#!/bin/bash
# Run both epubverify and epubcheck on all EPUBs and capture JSON output.
#
# Usage: bash stress-test/run-comparison.sh [epub_dir]
#
# Environment variables:
#   EPUBVERIFY    Path to epubverify binary (default: ./epubverify)
#   EPUBCHECK_JAR Path to epubcheck JAR (default: $HOME/tools/epubcheck-5.1.0/epubcheck.jar)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EPUB_DIR="${1:-${SCRIPT_DIR}/epubs}"
RESULTS_DIR="${SCRIPT_DIR}/results"
EPUBVERIFY="${EPUBVERIFY:-${SCRIPT_DIR}/../epubverify}"
EPUBCHECK_JAR="${EPUBCHECK_JAR:-${HOME}/tools/epubcheck-5.1.0/epubcheck.jar}"

# Verify tools exist
if [ ! -x "${EPUBVERIFY}" ]; then
  echo "ERROR: epubverify not found at ${EPUBVERIFY}"
  echo "  Run 'make build' first, or set EPUBVERIFY=/path/to/binary"
  exit 2
fi
if [ ! -f "${EPUBCHECK_JAR}" ]; then
  echo "ERROR: epubcheck JAR not found at ${EPUBCHECK_JAR}"
  echo "  Set EPUBCHECK_JAR=/path/to/epubcheck.jar"
  exit 2
fi

mkdir -p "${RESULTS_DIR}/epubverify" "${RESULTS_DIR}/epubcheck"

total=0
for epub in "${EPUB_DIR}"/*.epub; do
  [ -f "$epub" ] || continue
  name=$(basename "$epub" .epub)
  total=$((total + 1))
  echo "=== [${total}] Testing: ${name} ==="

  # Run epubverify (JSON output)
  echo -n "  epubverify... "
  timeout 60 "${EPUBVERIFY}" "${epub}" --json "${RESULTS_DIR}/epubverify/${name}.json" \
    2>"${RESULTS_DIR}/epubverify/${name}.stderr" || true
  ev_exit=$?
  echo "exit=${ev_exit}"

  # Run epubcheck (JSON output)
  echo -n "  epubcheck...  "
  timeout 120 java -jar "${EPUBCHECK_JAR}" "${epub}" \
    --json "${RESULTS_DIR}/epubcheck/${name}.json" \
    2>"${RESULTS_DIR}/epubcheck/${name}.stderr" >/dev/null || true
  ec_exit=$?
  echo "exit=${ec_exit}"
done

echo ""
echo "=== DONE: ${total} EPUBs tested ==="
echo "Results saved to ${RESULTS_DIR}"
echo ""
echo "Run 'bash stress-test/analyze-results.sh' to compare results."
