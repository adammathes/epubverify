#!/bin/bash
#
# download-samples.sh - Download public domain EPUB samples for testing
#
# Downloads a curated set of freely available EPUBs from Project Gutenberg.
# These are used to compare epubverify output against the reference
# epubcheck tool.
#
# Usage: ./download-samples.sh [--force]
#   --force  Re-download even if files already exist
#
# Be polite: this script downloads a small, fixed set of files.
# Do not modify it to bulk-scrape any site.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SAMPLES_DIR="$SCRIPT_DIR/samples"
FORCE="${1:-}"

mkdir -p "$SAMPLES_DIR"

# Curated list of public domain EPUBs from Project Gutenberg.
# Format: filename|URL|description
SAMPLES=(
  "pg11-alice.epub|https://www.gutenberg.org/ebooks/11.epub3.images|Alice's Adventures in Wonderland (EPUB 3)"
  "pg84-frankenstein.epub|https://www.gutenberg.org/ebooks/84.epub3.images|Frankenstein (EPUB 3)"
  "pg1342-pride-and-prejudice.epub|https://www.gutenberg.org/ebooks/1342.epub3.images|Pride and Prejudice (EPUB 3)"
  "pg1661-sherlock.epub|https://www.gutenberg.org/ebooks/1661.epub3.images|Adventures of Sherlock Holmes (EPUB 3)"
  "pg2701-moby-dick.epub|https://www.gutenberg.org/ebooks/2701.epub3.images|Moby Dick (EPUB 3)"
)

downloaded=0
skipped=0

for entry in "${SAMPLES[@]}"; do
  IFS='|' read -r filename url description <<< "$entry"
  dest="$SAMPLES_DIR/$filename"

  if [[ -f "$dest" && "$FORCE" != "--force" ]]; then
    echo "SKIP  $filename (already exists)"
    ((skipped++))
    continue
  fi

  echo "GET   $filename - $description"
  curl -L -s -o "$dest" "$url"

  # Verify it's actually a ZIP/EPUB, not an HTML error page
  if file "$dest" | grep -q "EPUB\|Zip"; then
    echo "  OK  $(du -h "$dest" | cut -f1)"
    ((downloaded++))
  else
    echo "  FAIL  Downloaded file is not a valid EPUB ($(file -b "$dest"))"
    rm -f "$dest"
  fi

  # Be polite: 1 second between requests
  sleep 1
done

echo ""
echo "Done. Downloaded: $downloaded, Skipped: $skipped"
echo "Samples directory: $SAMPLES_DIR"
