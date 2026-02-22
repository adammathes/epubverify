#!/bin/bash
#
# download-samples.sh - Download public domain EPUB samples for testing
#
# Downloads a curated set of freely available EPUBs from Project Gutenberg
# and Feedbooks. These are used to compare epubverify output against the
# reference epubcheck tool.
#
# Usage: ./download-samples.sh [--force]
#   --force  Re-download even if files already exist
#
# Be polite: this script downloads a small, fixed set of files with a
# delay between requests. Do not modify it to bulk-scrape any site.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SAMPLES_DIR="$SCRIPT_DIR/samples"
FORCE="${1:-}"

mkdir -p "$SAMPLES_DIR"

# Curated list of EPUBs.
# Format: filename|URL|description
#
# Sources:
#   - Project Gutenberg (gutenberg.org): Public domain, EPUB 3 with images
#   - Feedbooks (feedbooks.com): Public domain, EPUB 2
SAMPLES=(
  # --- Project Gutenberg EPUB 3 (valid EPUBs) ---
  "pg11-alice.epub|https://www.gutenberg.org/ebooks/11.epub3.images|Alice in Wonderland (EPUB 3)"
  "pg84-frankenstein.epub|https://www.gutenberg.org/ebooks/84.epub3.images|Frankenstein (EPUB 3)"
  "pg1342-pride-and-prejudice.epub|https://www.gutenberg.org/ebooks/1342.epub3.images|Pride and Prejudice (EPUB 3, large, epub:type=normal)"
  "pg1661-sherlock.epub|https://www.gutenberg.org/ebooks/1661.epub3.images|Sherlock Holmes (EPUB 3)"
  "pg2701-moby-dick.epub|https://www.gutenberg.org/ebooks/2701.epub3.images|Moby Dick (EPUB 3, complex TOC)"
  "pg74-twain-tom-sawyer.epub|https://www.gutenberg.org/ebooks/74.epub3.images|Tom Sawyer (EPUB 3)"
  "pg98-dickens-two-cities.epub|https://www.gutenberg.org/ebooks/98.epub3.images|A Tale of Two Cities (EPUB 3)"
  "pg345-dracula.epub|https://www.gutenberg.org/ebooks/345.epub3.images|Dracula (EPUB 3)"
  "pg1080-dante-inferno.epub|https://www.gutenberg.org/ebooks/1080.epub3.images|Dante's Inferno (EPUB 3)"
  "pg4300-joyce-ulysses.epub|https://www.gutenberg.org/ebooks/4300.epub3.images|Ulysses (EPUB 3, large)"
  "pg2600-war-and-peace.epub|https://www.gutenberg.org/ebooks/2600.epub3.images|War and Peace (EPUB 3, multiple contributors)"
  # Poetry and drama
  "pg1041-shakespeare-sonnets.epub|https://www.gutenberg.org/ebooks/1041.epub3.images|Shakespeare's Sonnets (EPUB 3, poetry)"
  "pg1524-hamlet.epub|https://www.gutenberg.org/ebooks/1524.epub3.images|Hamlet (EPUB 3, drama)"
  # Non-English
  "pg996-don-quixote-es.epub|https://www.gutenberg.org/ebooks/996.epub3.images|Don Quixote Spanish original (EPUB 3, large, Spanish)"
  "pg2000-don-quixote-es.epub|https://www.gutenberg.org/ebooks/2000.epub3.images|Don Quixote English translation (EPUB 3)"
  "pg17989-les-miserables-fr.epub|https://www.gutenberg.org/ebooks/17989.epub3.images|Les Miserables (EPUB 3, French)"
  "pg7000-grimm-de.epub|https://www.gutenberg.org/ebooks/7000.epub3.images|Grimm's Fairy Tales (EPUB 3, German, contributor IDs)"
  "pg25328-tao-te-ching-zh.epub|https://www.gutenberg.org/ebooks/25328.epub3.images|Tao Te Ching (EPUB 3, Chinese)"
  "pg1982-siddhartha-jp.epub|https://www.gutenberg.org/ebooks/1982.epub3.images|Siddhartha (EPUB 3)"
  "pg5200-kafka-metamorphosis.epub|https://www.gutenberg.org/ebooks/5200.epub3.images|Metamorphosis (EPUB 3, translator as contributor)"
  # EPUB 2
  "pg46-christmas-carol-epub2.epub|https://www.gutenberg.org/ebooks/46.epub.noimages|A Christmas Carol (EPUB 2, nested navPoints)"

  # --- Feedbooks EPUB 2 (known-invalid: mimetype CRLF, NCX UID mismatch) ---
  "fb-sherlock-study.epub|https://www.feedbooks.com/book/4453.epub|A Study in Scarlet - Feedbooks (EPUB 2)"
  "fb-art-of-war.epub|https://www.feedbooks.com/book/168.epub|Art of War - Feedbooks (EPUB 2)"
  "fb-odyssey.epub|https://www.feedbooks.com/book/3676.epub|The Odyssey - Feedbooks (EPUB 2)"
  "fb-republic.epub|https://www.feedbooks.com/book/4940.epub|The Republic - Feedbooks (EPUB 2)"
)

downloaded=0
skipped=0
failed=0

for entry in "${SAMPLES[@]}"; do
  IFS='|' read -r filename url description <<< "$entry"
  dest="$SAMPLES_DIR/$filename"

  if [[ -f "$dest" && "$FORCE" != "--force" ]]; then
    echo "SKIP  $filename (already exists)"
    skipped=$((skipped + 1))
    continue
  fi

  echo "GET   $filename - $description"
  curl -L -s -o "$dest" "$url"

  # Verify it's actually a ZIP/EPUB, not an HTML error page
  if file "$dest" | grep -q "EPUB\|Zip"; then
    echo "  OK  $(du -h "$dest" | cut -f1)"
    downloaded=$((downloaded + 1))
  else
    echo "  FAIL  Downloaded file is not a valid EPUB ($(file -b "$dest"))"
    rm -f "$dest"
    failed=$((failed + 1))
  fi

  # Be polite: 1 second between requests
  sleep 1
done

echo ""
echo "Done. Downloaded: $downloaded, Skipped: $skipped, Failed: $failed"
echo "Samples directory: $SAMPLES_DIR"
echo "Total EPUBs: $(ls "$SAMPLES_DIR"/*.epub 2>/dev/null | wc -l)"
