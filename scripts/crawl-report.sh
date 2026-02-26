#!/bin/bash
# Generate a report from the crawl manifest and optionally file GitHub issues.
#
# This script reads the crawl manifest, produces a human-readable summary,
# and optionally files GitHub issues for new discrepancies.
#
# Usage:
#   bash scripts/crawl-report.sh [OPTIONS]
#
# Options:
#   --manifest FILE     Manifest file (default: stress-test/crawl-manifest.json)
#   --results-dir DIR   Validation results dir (default: stress-test/crawl-results)
#   --output FILE       Write report to file (default: stdout)
#   --file-issues       File GitHub issues for new discrepancies (requires gh CLI)
#   --help              Show this help

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
MANIFEST="${REPO_ROOT}/stress-test/crawl-manifest.json"
RESULTS_DIR="${REPO_ROOT}/stress-test/crawl-results"
OUTPUT=""
FILE_ISSUES=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --manifest)     MANIFEST="$2"; shift 2 ;;
    --results-dir)  RESULTS_DIR="$2"; shift 2 ;;
    --output)       OUTPUT="$2"; shift 2 ;;
    --file-issues)  FILE_ISSUES=true; shift ;;
    --help)
      head -20 "$0" | grep '^#' | sed 's/^# \?//'
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

if [ ! -f "${MANIFEST}" ]; then
  echo "ERROR: Manifest not found at ${MANIFEST}"
  echo "  Run 'bash scripts/epub-crawler.sh' first."
  exit 2
fi

export MANIFEST RESULTS_DIR FILE_ISSUES

python3 << 'PYEOF'
import json
import os
import subprocess
import sys
from collections import Counter
from datetime import datetime

MANIFEST = os.environ["MANIFEST"]
RESULTS_DIR = os.environ["RESULTS_DIR"]
FILE_ISSUES = os.environ.get("FILE_ISSUES", "false") == "true"

with open(MANIFEST) as f:
    manifest = json.load(f)

epubs = manifest.get("epubs", [])
summary = manifest.get("summary", {})

# --- SUMMARY ---

total = len(epubs)
validated = sum(1 for e in epubs if e.get("epubverify_verdict"))
pending = total - validated
matches = sum(1 for e in epubs if e.get("match"))
false_positives = [e for e in epubs if e.get("discrepancy_issue") == "false_positive"]
false_negatives = [e for e in epubs if e.get("discrepancy_issue") == "false_negative"]
crashes = [e for e in epubs if e.get("epubverify_verdict") == "crash"]

# Count by source
source_counts = Counter(e.get("source", "UNKNOWN") for e in epubs)

report_lines = []

def out(line=""):
    report_lines.append(line)

out("=" * 72)
out("  EPUB CRAWL REPORT")
out(f"  Generated: {datetime.utcnow().strftime('%Y-%m-%d %H:%M:%S UTC')}")
out(f"  Manifest:  {MANIFEST}")
out("=" * 72)
out()
out("  SUMMARY")
out("  " + "-" * 40)
out(f"  Total EPUBs in manifest:  {total}")
out(f"  Validated:                {validated}")
out(f"  Pending validation:       {pending}")
out(f"  Agreement (both match):   {matches}")
out(f"  False positives:          {len(false_positives)}")
out(f"  False negatives:          {len(false_negatives)}")
out(f"  Crashes:                  {len(crashes)}")
if validated > 0:
    rate = matches / validated * 100
    out(f"  Agreement rate:           {rate:.1f}%")
out()

out("  SOURCES")
out("  " + "-" * 40)
for src, count in source_counts.most_common():
    out(f"  {src:<20} {count:>5} EPUBs")
out()

# --- FALSE POSITIVES ---

if false_positives:
    out("=" * 72)
    out("  FALSE POSITIVES (epubverify over-reports)")
    out("  epubverify says INVALID but epubcheck says VALID")
    out("=" * 72)
    for e in false_positives:
        out(f"  SHA: {e['sha256'][:16]}...")
        out(f"  URL: {e['source_url']}")
        out(f"  Source: {e['source']}")
        name = e["source_url"].split("/")[-1].replace(".epub", "")
        ev_json = os.path.join(RESULTS_DIR, "epubverify", f"{name}.json")
        if os.path.exists(ev_json):
            try:
                with open(ev_json) as f:
                    ev_data = json.load(f)
                errors = [m for m in ev_data.get("messages", [])
                         if m.get("severity") in ("ERROR", "FATAL")]
                for err in errors[:5]:
                    out(f"    [{err.get('check_id', '?')}] {err.get('message', '')[:100]}")
            except Exception:
                pass
        out()

# --- FALSE NEGATIVES ---

if false_negatives:
    out("=" * 72)
    out("  FALSE NEGATIVES (epubverify misses errors)")
    out("  epubverify says VALID but epubcheck says INVALID")
    out("=" * 72)
    for e in false_negatives:
        out(f"  SHA: {e['sha256'][:16]}...")
        out(f"  URL: {e['source_url']}")
        out(f"  Source: {e['source']}")
        name = e["source_url"].split("/")[-1].replace(".epub", "")
        ec_json = os.path.join(RESULTS_DIR, "epubcheck", f"{name}.json")
        if os.path.exists(ec_json):
            try:
                with open(ec_json) as f:
                    ec_data = json.load(f)
                errors = [m for m in ec_data.get("messages", [])
                         if m.get("severity") in ("ERROR", "FATAL")]
                for err in errors[:5]:
                    out(f"    [{err.get('ID', '?')}] {err.get('message', '')[:100]}")
            except Exception:
                pass
        out()

# --- CRASHES ---

if crashes:
    out("=" * 72)
    out("  CRASHES (epubverify failed)")
    out("=" * 72)
    for e in crashes:
        out(f"  SHA: {e['sha256'][:16]}...")
        out(f"  URL: {e['source_url']}")
        out()

# --- ERROR CODE ANALYSIS ---

# Analyze check IDs from validation results
ev_check_ids = Counter()
ec_check_ids = Counter()
ev_only_ids = Counter()
ec_only_ids = Counter()

ev_dir = os.path.join(RESULTS_DIR, "epubverify")
ec_dir = os.path.join(RESULTS_DIR, "epubcheck")

if os.path.isdir(ev_dir) and os.path.isdir(ec_dir):
    for fname in os.listdir(ev_dir):
        if not fname.endswith(".json"):
            continue
        name = fname.replace(".json", "")
        try:
            with open(os.path.join(ev_dir, fname)) as f:
                ev_data = json.load(f)
            ev_ids = set(m.get("check_id", "") for m in ev_data.get("messages", [])
                        if m.get("severity") in ("ERROR", "FATAL"))
        except Exception:
            ev_ids = set()

        ec_file = os.path.join(ec_dir, fname)
        try:
            with open(ec_file) as f:
                ec_data = json.load(f)
            ec_ids = set(m.get("ID", "") for m in ec_data.get("messages", [])
                        if m.get("severity") in ("ERROR", "FATAL"))
        except Exception:
            ec_ids = set()

        for cid in ev_ids:
            ev_check_ids[cid] += 1
        for cid in ec_ids:
            ec_check_ids[cid] += 1
        for cid in ev_ids - ec_ids:
            ev_only_ids[cid] += 1
        for cid in ec_ids - ev_ids:
            ec_only_ids[cid] += 1

    if ev_only_ids:
        out("=" * 72)
        out("  CHECK IDs only in epubverify (potential false positives)")
        out("=" * 72)
        for cid, count in ev_only_ids.most_common(20):
            out(f"  {cid:<20} {count:>3} books")
        out()

    if ec_only_ids:
        out("=" * 72)
        out("  CHECK IDs only in epubcheck (potential false negatives)")
        out("=" * 72)
        for cid, count in ec_only_ids.most_common(20):
            out(f"  {cid:<20} {count:>3} books")
        out()

# --- Print report ---
report = "\n".join(report_lines)
print(report)

# --- File GitHub issues ---

if FILE_ISSUES and (false_positives or false_negatives):
    print()
    print("=" * 72)
    print("  Filing GitHub Issues")
    print("=" * 72)

    for e in false_positives + false_negatives:
        disc_type = e.get("discrepancy_issue", "")
        if not disc_type:
            continue
        # Skip if already has an issue number
        if e.get("discrepancy_issue", "").startswith("#"):
            continue

        sha_short = e["sha256"][:12]
        source_url = e["source_url"]
        label = "false-positive" if disc_type == "false_positive" else "false-negative"
        title = f"Crawl discrepancy ({label}): {sha_short}"
        body = (
            f"## Crawl Discrepancy: {label}\n\n"
            f"- **Source URL:** {source_url}\n"
            f"- **Source:** {e['source']}\n"
            f"- **SHA-256:** {e['sha256']}\n"
            f"- **epubverify:** {e['epubverify_verdict']}\n"
            f"- **epubcheck:** {e['epubcheck_verdict']}\n\n"
            f"### Next Steps\n\n"
            f"1. Download the EPUB from the source URL\n"
            f"2. Run both validators to reproduce\n"
            f"3. Investigate the discrepancy\n"
        )

        try:
            result = subprocess.run(
                ["gh", "issue", "create",
                 "--title", title,
                 "--body", body,
                 "--label", "crawl-discrepancy"],
                capture_output=True, text=True, timeout=30
            )
            if result.returncode == 0:
                issue_url = result.stdout.strip()
                issue_num = issue_url.split("/")[-1]
                e["discrepancy_issue"] = f"#{issue_num}"
                print(f"  Filed: {issue_url}")
            else:
                print(f"  WARN: could not file issue for {sha_short}: {result.stderr[:200]}")
        except FileNotFoundError:
            print("  WARN: 'gh' CLI not found. Install GitHub CLI to file issues.")
            break
        except Exception as ex:
            print(f"  WARN: error filing issue: {ex}")

    # Save updated manifest with issue numbers
    with open(MANIFEST, "w") as f:
        json.dump(manifest, f, indent=2)
    print(f"  Manifest updated with issue numbers.")

PYEOF

# Write to file if requested
if [ -n "${OUTPUT}" ]; then
  echo "Report written to ${OUTPUT}"
fi
