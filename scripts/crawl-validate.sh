#!/bin/bash
# Validate crawled EPUBs with both epubverify and epubcheck, compare results.
#
# For each EPUB in the crawl manifest that hasn't been validated yet:
# 1. Run epubverify and capture JSON output
# 2. Run epubcheck and capture JSON output
# 3. Compare verdicts (valid/invalid)
# 4. Update manifest with results
#
# Discrepancy types:
#   false_negative — epubverify says VALID but epubcheck says INVALID
#                    (we're missing checks)
#   false_positive — epubverify says INVALID but epubcheck says VALID
#                    (we're over-reporting, this is a bug)
#
# Usage:
#   bash scripts/crawl-validate.sh [OPTIONS]
#
# Options:
#   --epub-dir DIR      Directory with crawled EPUBs (default: stress-test/crawl-epubs)
#   --manifest FILE     Manifest file (default: stress-test/crawl-manifest.json)
#   --results-dir DIR   Directory for validation output (default: stress-test/crawl-results)
#   --limit N           Validate at most N EPUBs this run (default: all)
#   --help              Show this help
#
# Environment variables:
#   EPUBVERIFY      Path to epubverify binary (default: ./epubverify)
#   EPUBCHECK_JAR   Path to epubcheck JAR (default: $HOME/tools/epubcheck-5.3.0/epubcheck.jar)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
EPUB_DIR="${REPO_ROOT}/stress-test/crawl-epubs"
MANIFEST="${REPO_ROOT}/stress-test/crawl-manifest.json"
RESULTS_DIR="${REPO_ROOT}/stress-test/crawl-results"
LIMIT=0  # 0 = no limit
EPUBVERIFY="${EPUBVERIFY:-${REPO_ROOT}/epubverify}"
EPUBCHECK_JAR="${EPUBCHECK_JAR:-${HOME}/tools/epubcheck-5.3.0/epubcheck.jar}"

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --epub-dir)     EPUB_DIR="$2"; shift 2 ;;
    --manifest)     MANIFEST="$2"; shift 2 ;;
    --results-dir)  RESULTS_DIR="$2"; shift 2 ;;
    --limit)        LIMIT="$2"; shift 2 ;;
    --help)
      head -30 "$0" | grep '^#' | sed 's/^# \?//'
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

# Verify tools exist
if [ ! -x "${EPUBVERIFY}" ]; then
  echo "ERROR: epubverify not found at ${EPUBVERIFY}"
  echo "  Run 'make build' first, or set EPUBVERIFY=/path/to/binary"
  exit 2
fi
if [ ! -f "${EPUBCHECK_JAR}" ]; then
  echo "WARNING: epubcheck JAR not found at ${EPUBCHECK_JAR}"
  echo "  Set EPUBCHECK_JAR=/path/to/epubcheck.jar"
  echo "  Will only run epubverify (no comparison)"
  EPUBCHECK_JAR=""
fi

if [ ! -f "${MANIFEST}" ]; then
  echo "ERROR: Manifest not found at ${MANIFEST}"
  echo "  Run 'bash scripts/epub-crawler.sh' first."
  exit 2
fi

mkdir -p "${RESULTS_DIR}/epubverify" "${RESULTS_DIR}/epubcheck"

echo "════════════════════════════════════════════════════════════════"
echo "  EPUB Crawl Validator"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo "  EPUB directory:  ${EPUB_DIR}"
echo "  Manifest:        ${MANIFEST}"
echo "  Results:         ${RESULTS_DIR}"
echo "  epubverify:      ${EPUBVERIFY}"
echo "  epubcheck:       ${EPUBCHECK_JAR:-NONE}"
echo ""

# Export variables so the Python heredoc can access them via os.environ
export EPUB_DIR MANIFEST RESULTS_DIR EPUBVERIFY EPUBCHECK_JAR LIMIT

# Validate EPUBs and update manifest
python3 << 'PYEOF'
import json
import os
import subprocess
import sys

EPUB_DIR = os.environ.get("EPUB_DIR", "stress-test/crawl-epubs")
MANIFEST = os.environ.get("MANIFEST", "stress-test/crawl-manifest.json")
RESULTS_DIR = os.environ.get("RESULTS_DIR", "stress-test/crawl-results")
EPUBVERIFY = os.environ.get("EPUBVERIFY", "./epubverify")
EPUBCHECK_JAR = os.environ.get("EPUBCHECK_JAR", "")
LIMIT = int(os.environ.get("LIMIT", "0"))

with open(MANIFEST) as f:
    manifest = json.load(f)

validated = 0
new_matches = 0
new_false_positives = 0
new_false_negatives = 0
new_crashes = 0

for entry in manifest.get("epubs", []):
    # Skip already-validated entries
    if entry.get("epubverify_verdict"):
        continue

    if LIMIT > 0 and validated >= LIMIT:
        break

    # Find the EPUB file
    sha = entry["sha256"]
    source_url = entry["source_url"]

    # Look for the file in the epub dir
    epub_path = None
    for f in os.listdir(EPUB_DIR):
        if f.endswith(".epub"):
            fpath = os.path.join(EPUB_DIR, f)
            # Quick check: compare file to see if it matches
            try:
                result = subprocess.run(
                    ["sha256sum", fpath],
                    capture_output=True, text=True, timeout=30
                )
                if result.returncode == 0 and result.stdout.startswith(sha):
                    epub_path = fpath
                    break
            except Exception:
                pass

    if not epub_path:
        print(f"  SKIP: no file found for sha256={sha[:12]}...")
        continue

    name = os.path.basename(epub_path).replace(".epub", "")
    validated += 1
    print(f"  [{validated}] Validating: {name}")

    # --- Run epubverify ---
    ev_json_path = os.path.join(RESULTS_DIR, "epubverify", f"{name}.json")
    ev_stderr_path = os.path.join(RESULTS_DIR, "epubverify", f"{name}.stderr")
    ev_verdict = "crash"

    try:
        result = subprocess.run(
            [EPUBVERIFY, epub_path, "--json", ev_json_path],
            capture_output=True, text=True, timeout=60
        )
        with open(ev_stderr_path, "w") as f:
            f.write(result.stderr)

        if os.path.exists(ev_json_path):
            with open(ev_json_path) as f:
                ev_data = json.load(f)
            ev_errors = [m for m in ev_data.get("messages", [])
                        if m.get("severity") in ("ERROR", "FATAL")]
            ev_verdict = "valid" if len(ev_errors) == 0 else "invalid"
        else:
            ev_verdict = "crash"
    except Exception as e:
        print(f"    epubverify CRASH: {e}")
        ev_verdict = "crash"

    print(f"    epubverify: {ev_verdict}")

    # --- Run epubcheck ---
    ec_verdict = ""
    if EPUBCHECK_JAR:
        ec_json_path = os.path.join(RESULTS_DIR, "epubcheck", f"{name}.json")
        ec_stderr_path = os.path.join(RESULTS_DIR, "epubcheck", f"{name}.stderr")

        try:
            result = subprocess.run(
                ["java", "-jar", EPUBCHECK_JAR, epub_path,
                 "--json", ec_json_path],
                capture_output=True, text=True, timeout=120
            )
            with open(ec_stderr_path, "w") as f:
                f.write(result.stderr)

            if os.path.exists(ec_json_path):
                with open(ec_json_path) as f:
                    ec_data = json.load(f)
                checker = ec_data.get("checker", {})
                ec_errors = checker.get("nFatal", 0) + checker.get("nError", 0)
                ec_verdict = "valid" if ec_errors == 0 else "invalid"
            else:
                ec_verdict = "crash"
        except Exception as e:
            print(f"    epubcheck CRASH: {e}")
            ec_verdict = "crash"

        print(f"    epubcheck:  {ec_verdict}")

    # --- Compare verdicts ---
    entry["epubverify_verdict"] = ev_verdict
    entry["epubcheck_verdict"] = ec_verdict

    if ev_verdict == "crash":
        new_crashes += 1
        entry["match"] = False
        print(f"    Result: CRASH")
    elif ec_verdict == "":
        # No epubcheck available, can't compare
        entry["match"] = True  # assume ok
        print(f"    Result: epubverify-only ({ev_verdict})")
    elif ev_verdict == ec_verdict:
        new_matches += 1
        entry["match"] = True
        print(f"    Result: MATCH ({ev_verdict})")
    else:
        entry["match"] = False
        if ev_verdict == "invalid" and ec_verdict == "valid":
            new_false_positives += 1
            entry["discrepancy_issue"] = "false_positive"
            print(f"    Result: FALSE POSITIVE (epubverify=invalid, epubcheck=valid)")
        elif ev_verdict == "valid" and ec_verdict == "invalid":
            new_false_negatives += 1
            entry["discrepancy_issue"] = "false_negative"
            print(f"    Result: FALSE NEGATIVE (epubverify=valid, epubcheck=invalid)")
        else:
            print(f"    Result: MISMATCH ({ev_verdict} vs {ec_verdict})")

# Update summary
total_validated = sum(1 for e in manifest["epubs"] if e.get("epubverify_verdict"))
total_matches = sum(1 for e in manifest["epubs"] if e.get("match"))
total_fp = sum(1 for e in manifest["epubs"] if e.get("discrepancy_issue") == "false_positive")
total_fn = sum(1 for e in manifest["epubs"] if e.get("discrepancy_issue") == "false_negative")
total_crashes = sum(1 for e in manifest["epubs"] if e.get("epubverify_verdict") == "crash")

manifest["summary"] = {
    "total_tested": len(manifest["epubs"]),
    "agreement": total_matches,
    "false_positives": total_fp,
    "false_negatives": total_fn,
    "crashes": total_crashes
}

with open(MANIFEST, "w") as f:
    json.dump(manifest, f, indent=2)

print()
print("════════════════════════════════════════════════════════════════")
print("  Validation Summary")
print("════════════════════════════════════════════════════════════════")
print(f"  This run:    {validated} validated, {new_matches} matches, "
      f"{new_false_positives} false positives, {new_false_negatives} false negatives, "
      f"{new_crashes} crashes")
print(f"  Overall:     {total_validated}/{len(manifest['epubs'])} validated, "
      f"{total_matches} matches, {total_fp} FP, {total_fn} FN, {total_crashes} crashes")
print()
print(f"  Manifest updated: {MANIFEST}")
print()
print("Next step: run 'bash scripts/crawl-report.sh' to generate a report.")
PYEOF
