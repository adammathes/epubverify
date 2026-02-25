#!/bin/bash
# Download EPUBs from public domain sources for stress testing.
# Uses 2-second delays between downloads to be respectful.
#
# Usage: bash stress-test/download-epubs.sh [--all | --gutenberg | --idpf]
#   --gutenberg  Download only Project Gutenberg EPUBs (default)
#   --idpf       Download only IDPF sample EPUBs
#   --all        Download from all sources
#
# EPUBs are saved to stress-test/epubs/

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EPUB_DIR="${SCRIPT_DIR}/epubs"
UA="epubverify-stress-test/0.1 (one-time research download)"
DELAY=2

mkdir -p "${EPUB_DIR}"

download() {
  local name="$1" url="$2"
  if [ -f "${EPUB_DIR}/${name}.epub" ]; then
    echo "SKIP: ${name}.epub already exists"
    return 0
  fi
  echo -n "Downloading ${name}... "
  if curl -sL -o "${EPUB_DIR}/${name}.epub" "${url}" -H "User-Agent: ${UA}" && \
     [ -s "${EPUB_DIR}/${name}.epub" ] && \
     file "${EPUB_DIR}/${name}.epub" 2>/dev/null | grep -q "Zip archive\|EPUB"; then
    local size
    size=$(stat -c%s "${EPUB_DIR}/${name}.epub" 2>/dev/null || stat -f%z "${EPUB_DIR}/${name}.epub" 2>/dev/null)
    echo "OK (${size} bytes)"
  else
    rm -f "${EPUB_DIR}/${name}.epub"
    echo "FAILED (not a valid EPUB/ZIP)"
    return 1
  fi
  sleep "${DELAY}"
}

download_gutenberg() {
  local id="$1" name="$2"
  # Try EPUB3 first, fall back to EPUB2
  download "${name}" "https://www.gutenberg.org/ebooks/${id}.epub3.images" || \
  download "${name}" "https://www.gutenberg.org/ebooks/${id}.epub.images" || true
}

gutenberg_epubs() {
  echo "=== Project Gutenberg EPUBs ==="
  echo ""

  # Classic novels — diverse content, languages, sizes
  download_gutenberg 1342  "pride-and-prejudice"
  download_gutenberg 84    "frankenstein"
  download_gutenberg 11    "alice-in-wonderland"
  download_gutenberg 1661  "sherlock-holmes"
  download_gutenberg 98    "tale-of-two-cities"
  download_gutenberg 1232  "prince"
  download_gutenberg 74    "tom-sawyer"
  download_gutenberg 2701  "moby-dick"
  download_gutenberg 16328 "beowulf"
  download_gutenberg 215   "call-of-the-wild"
  download_gutenberg 1260  "jane-eyre"
  download_gutenberg 768   "wuthering-heights"
  download_gutenberg 4300  "ulysses"
  download_gutenberg 2600  "war-and-peace"
  download_gutenberg 1080  "modest-proposal"
  download_gutenberg 5200  "metamorphosis"
  download_gutenberg 236   "jungle-book"
  download_gutenberg 1400  "great-expectations"
  download_gutenberg 46    "christmas-carol"       # has deprecated align attrs
  download_gutenberg 345   "dracula"
  download_gutenberg 3207  "leviathan"
  download_gutenberg 174   "dorian-gray"
  download_gutenberg 120   "treasure-island"
  download_gutenberg 2542  "constitution-of-usa"
  download_gutenberg 55    "oz-wizard"

  # More diverse content
  download_gutenberg 1952  "yellow-wallpaper"
  download_gutenberg 36    "war-of-worlds"
  download_gutenberg 43    "strange-case-jekyll-hyde"
  download_gutenberg 76    "huckleberry-finn"
  download_gutenberg 244   "study-in-scarlet"
  download_gutenberg 1184  "count-of-monte-cristo"
  download_gutenberg 6130  "iliad"
  download_gutenberg 135   "les-miserables"
  download_gutenberg 730   "oliver-twist"
  download_gutenberg 2814  "dubliners"
  download_gutenberg 219   "heart-of-darkness"
  download_gutenberg 514   "little-women"
  download_gutenberg 3600  "emma"
  download_gutenberg 996   "don-quixote"
  download_gutenberg 28054 "brothers-karamazov"
  download_gutenberg 158   "emma-goldman"
  download_gutenberg 1497  "republic-plato"
  download_gutenberg 160   "same-old-story"
  download_gutenberg 1727  "odyssey"
  download_gutenberg 161   "sense-and-sensibility"
  download_gutenberg 25344 "scarlet-pimpernel"
  download_gutenberg 2591  "grimms-fairy-tales"
  download_gutenberg 1250  "anthem"
  download_gutenberg 4363  "japanese-fairy-tales"
  download_gutenberg 2500  "siddhartha"

  # Edge cases: very large, non-English, math, unusual content
  download_gutenberg 128   "arabian-nights"
  download_gutenberg 10    "king-james-bible"       # very large
  download_gutenberg 100   "complete-shakespeare"   # very large
  download_gutenberg 7787  "esperanto-textbook"     # non-English
  download_gutenberg 37134 "elements-of-style"
  download_gutenberg 1404  "federalist-papers"
  download_gutenberg 59    "discourse-on-method"
  download_gutenberg 27827 "kamasutra"
  download_gutenberg 9603  "dream-red-chamber"      # Chinese classic
  download_gutenberg 7849  "kafka-castle"
  download_gutenberg 201   "flatland"               # geometry
  download_gutenberg 10007 "carmilla"               # horror
  download_gutenberg 21076 "euclid-elements"        # math
  download_gutenberg 216   "tao-te-ching"
  download_gutenberg 2388  "bhagavad-gita"
  download_gutenberg 2680  "meditations"
}

idpf_epubs() {
  echo ""
  echo "=== IDPF/W3C EPUB3 Sample EPUBs ==="
  echo ""

  local base="https://github.com/IDPF/epub3-samples/releases/download/20230704"

  download "page-blanche"         "${base}/page-blanche.epub"
  download "hefty-water"          "${base}/hefty-water.epub"
  download "childrens-literature" "${base}/childrens-literature.epub"
  download "linear-algebra"       "${base}/linear-algebra.epub"
  download "haruko-html"          "${base}/haruko-html-jpeg.epub"
  download "figure-gallery"       "${base}/figure-gallery-bindings.epub"
  download "cole-voyage"          "${base}/cole-voyage-of-life-tol.epub"
  download "trees"                "${base}/trees.epub"
  download "sous-le-vent"         "${base}/sous-le-vent.epub"
  download "mymedia-lite"         "${base}/cc-shared-culture.epub"

  # EPUB2 variant
  download "epubbooks-metamorphosis" "https://www.gutenberg.org/ebooks/5200.epub.images"
}

# Parse arguments
SOURCE="${1:---all}"

case "${SOURCE}" in
  --gutenberg) gutenberg_epubs ;;
  --idpf)      idpf_epubs ;;
  --all)       gutenberg_epubs; idpf_epubs ;;
  *)           echo "Usage: $0 [--all | --gutenberg | --idpf]"; exit 1 ;;
esac

echo ""
echo "=== Summary ==="
count=$(ls -1 "${EPUB_DIR}"/*.epub 2>/dev/null | wc -l)
echo "${count} EPUBs in ${EPUB_DIR}"
