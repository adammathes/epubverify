#!/bin/bash
# Analyze stress test results: compare epubverify vs epubcheck output.
#
# Usage: bash stress-test/analyze-results.sh [results_dir]
#
# Reads JSON output from both validators and produces a detailed report
# showing agreement/disagreement, false positives, false negatives,
# and per-check-ID frequency tables.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RESULTS_DIR="${1:-${SCRIPT_DIR}/results}"

if [ ! -d "${RESULTS_DIR}/epubverify" ] || [ ! -d "${RESULTS_DIR}/epubcheck" ]; then
  echo "ERROR: Results not found in ${RESULTS_DIR}"
  echo "  Run 'bash stress-test/run-comparison.sh' first."
  exit 2
fi

export RESULTS_DIR

python3 << 'PYEOF'
import json
import os
import sys
from collections import Counter

RESULTS_DIR = os.environ["RESULTS_DIR"]
ev_dir = os.path.join(RESULTS_DIR, "epubverify")
ec_dir = os.path.join(RESULTS_DIR, "epubcheck")

def load_json(path):
    try:
        with open(path) as f:
            return json.load(f)
    except Exception:
        return None

def extract_messages(data, tool):
    msgs = []
    if not data:
        return msgs
    if tool == "epubverify":
        for m in data.get("messages", []):
            msgs.append({
                "severity": m.get("severity", "").upper(),
                "check_id": m.get("check_id", ""),
                "message": m.get("message", ""),
                "location": m.get("location", "")
            })
    elif tool == "epubcheck":
        for m in data.get("messages", []):
            locs = m.get("locations", [{}])
            msgs.append({
                "severity": m.get("severity", "").upper(),
                "check_id": m.get("ID", ""),
                "message": m.get("message", ""),
                "location": locs[0].get("path", "") if locs else ""
            })
    return msgs

books = sorted(set(
    f.replace(".json", "")
    for d in [ev_dir, ec_dir]
    for f in os.listdir(d)
    if f.endswith(".json")
))

false_positives = []
false_negatives = []
crashes = []
ev_false_positive_checks = Counter()
ev_all_checks = Counter()
ec_only_checks = Counter()
ev_only_checks = Counter()
valid_agreement = 0
invalid_agreement = 0
disagree = 0

print("=" * 80)
print("STRESS TEST COMPARISON: epubverify vs epubcheck")
print("=" * 80)
print(f"\nTotal books tested: {len(books)}\n")

print("-" * 80)
print(f"{'Book':<35} {'epubverify':<15} {'epubcheck':<15} {'Match?'}")
print("-" * 80)

for book in books:
    ev_data = load_json(os.path.join(ev_dir, f"{book}.json"))
    ec_data = load_json(os.path.join(ec_dir, f"{book}.json"))

    if ev_data is None:
        crashes.append(book)
        print(f"{book:<35} {'CRASH':<15} {'?':<15} CRASH")
        continue

    ev_msgs = extract_messages(ev_data, "epubverify")
    ec_msgs = extract_messages(ec_data, "epubcheck")

    ev_errors = [m for m in ev_msgs if m["severity"] in ("ERROR", "FATAL")]
    ec_errors = [m for m in ec_msgs if m["severity"] in ("ERROR", "FATAL")]
    ev_warnings = [m for m in ev_msgs if m["severity"] == "WARNING"]
    ec_warnings = [m for m in ec_msgs if m["severity"] == "WARNING"]

    ev_valid = len(ev_errors) == 0
    ec_valid = len(ec_errors) == 0

    ev_status = f"{'VALID' if ev_valid else 'INVALID'}({len(ev_errors)}E/{len(ev_warnings)}W)"
    ec_status = f"{'VALID' if ec_valid else 'INVALID'}({len(ec_errors)}E/{len(ec_warnings)}W)"

    match = ev_valid == ec_valid

    for m in ev_msgs:
        ev_all_checks[m["check_id"]] += 1

    if match:
        if ev_valid:
            valid_agreement += 1
        else:
            invalid_agreement += 1
        print(f"{book:<35} {ev_status:<15} {ec_status:<15} YES")
    else:
        disagree += 1
        if not ev_valid and ec_valid:
            false_positives.append((book, ev_errors, ec_errors))
            for e in ev_errors:
                ev_false_positive_checks[e["check_id"]] += 1
            print(f"{book:<35} {ev_status:<15} {ec_status:<15} FALSE POSITIVE")
        elif ev_valid and not ec_valid:
            false_negatives.append((book, ev_errors, ec_errors))
            print(f"{book:<35} {ev_status:<15} {ec_status:<15} FALSE NEGATIVE")
        else:
            print(f"{book:<35} {ev_status:<15} {ec_status:<15} DISAGREE")

    ev_check_ids = set(m["check_id"] for m in ev_msgs)
    ec_check_ids = set(m["check_id"] for m in ec_msgs)
    for cid in ev_check_ids - ec_check_ids:
        ev_only_checks[cid] += 1
    for cid in ec_check_ids - ev_check_ids:
        ec_only_checks[cid] += 1

print("-" * 80)
print(f"\nSUMMARY:")
print(f"  Both agree VALID:   {valid_agreement}")
print(f"  Both agree INVALID: {invalid_agreement}")
print(f"  Disagreements:      {disagree}")
print(f"  Crashes:            {len(crashes)}")
print()
print(f"  FALSE POSITIVES (epubverify wrong errors): {len(false_positives)}")
print(f"  FALSE NEGATIVES (epubverify missed errors): {len(false_negatives)}")

if false_positives:
    print(f"\n{'=' * 80}")
    print("FALSE POSITIVES (epubverify reports errors that epubcheck does not)")
    print("=" * 80)
    for book, ev_errs, ec_errs in false_positives:
        print(f"\n  {book}:")
        seen = set()
        for e in ev_errs:
            key = (e["check_id"], e["message"][:80])
            if key not in seen:
                seen.add(key)
                loc = f" @ {e['location']}" if e["location"] else ""
                print(f"    [{e['check_id']}] {e['message'][:100]}{loc}")

if false_negatives:
    print(f"\n{'=' * 80}")
    print("FALSE NEGATIVES (epubcheck reports errors that epubverify misses)")
    print("=" * 80)
    for book, ev_errs, ec_errs in false_negatives:
        print(f"\n  {book}:")
        seen = set()
        for e in ec_errs:
            key = (e["check_id"], e["message"][:80])
            if key not in seen:
                seen.add(key)
                loc = f" @ {e['location']}" if e["location"] else ""
                print(f"    [{e['check_id']}] {e['message'][:100]}{loc}")

if ev_false_positive_checks:
    print(f"\n{'=' * 80}")
    print("FALSE POSITIVE CHECK IDs (most common)")
    print("=" * 80)
    for cid, count in ev_false_positive_checks.most_common(20):
        print(f"  {cid}: {count} occurrences")

print(f"\n{'=' * 80}")
print("ALL epubverify CHECK IDs (frequency)")
print("=" * 80)
for cid, count in ev_all_checks.most_common(30):
    print(f"  {cid}: {count}")

print(f"\n{'=' * 80}")
print("CHECK IDs only in epubverify (not in epubcheck)")
print("=" * 80)
for cid, count in ev_only_checks.most_common(20):
    print(f"  {cid}: {count} books")
if not ev_only_checks:
    print("  (none)")

print(f"\n{'=' * 80}")
print("CHECK IDs only in epubcheck (not in epubverify)")
print("=" * 80)
for cid, count in ec_only_checks.most_common(20):
    print(f"  {cid}: {count} books")
if not ec_only_checks:
    print("  (none)")

if crashes:
    print(f"\n{'=' * 80}")
    print("CRASHES (epubverify failed to produce output)")
    print("=" * 80)
    for book in crashes:
        stderr_file = os.path.join(ev_dir, f"{book}.stderr")
        if os.path.exists(stderr_file):
            with open(stderr_file) as f:
                stderr = f.read().strip()[-300:]
            print(f"  {book}: {stderr}")
PYEOF
