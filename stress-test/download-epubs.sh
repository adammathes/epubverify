#!/bin/bash
# Download EPUBs from public domain sources for stress testing.
# Uses 2-second delays between downloads to be respectful.
#
# Usage: bash stress-test/download-epubs.sh [--all | --gutenberg | --idpf | --standardebooks | --feedbooks | --epub2]
#   --gutenberg      Download only Project Gutenberg EPUBs
#   --idpf           Download only IDPF sample EPUBs
#   --standardebooks Download only Standard Ebooks EPUBs
#   --feedbooks      Download only Feedbooks EPUBs
#   --epub2          Download only EPUB2 variants
#   --all            Download from all sources (default)
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

download_gutenberg_epub2() {
  local id="$1" name="$2"
  # Force EPUB2 format
  download "${name}" "https://www.gutenberg.org/ebooks/${id}.epub.images" || true
}

download_standardebooks() {
  local author="$1" title="$2" name="$3"
  # Standard Ebooks GitHub releases
  download "${name}" "https://standardebooks.org/ebooks/${author}/${title}/downloads/${author}_${title}.epub" || true
}

download_feedbooks() {
  local id="$1" name="$2"
  download "${name}" "https://www.feedbooks.com/book/${id}.epub" || true
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

  # --- New additions: poetry, drama, science, philosophy ---
  download_gutenberg 1064  "masque-of-red-death"
  download_gutenberg 23    "narrative-of-frederick-douglass"
  download_gutenberg 408   "souls-of-black-folk"
  download_gutenberg 829   "gulliver-travels"
  download_gutenberg 2148  "utopia"
  download_gutenberg 5230  "up-from-slavery"
  download_gutenberg 3825  "psychopathology-of-everyday-life"
  download_gutenberg 1399  "anna-karenina"
  download_gutenberg 7370  "second-treatise-of-government"
  download_gutenberg 30254 "crime-and-punishment"
  download_gutenberg 45    "anne-of-green-gables"
  download_gutenberg 4517  "emile"
  download_gutenberg 6593  "history-of-tom-jones"
  download_gutenberg 4217  "portrait-of-a-lady"
  download_gutenberg 2554  "crime-and-punishment-alt"
  download_gutenberg 1998  "thus-spake-zarathustra"
  download_gutenberg 2009  "origin-of-species"
  download_gutenberg 4078  "thus-spake-zarathustra-pt2"
  download_gutenberg 1228  "on-liberty"
  download_gutenberg 1597  "andersens-fairy-tales"
  download_gutenberg 28885 "nicomachean-ethics"
  download_gutenberg 2413  "poetics-aristotle"
  download_gutenberg 815   "democracy-in-america-v1"
  download_gutenberg 816   "democracy-in-america-v2"
  download_gutenberg 35    "time-machine"

  # --- Non-English Gutenberg books ---
  download_gutenberg 17489 "les-fleurs-du-mal"       # French poetry (Baudelaire)
  download_gutenberg 4280  "divina-commedia"          # Italian (Dante)
  download_gutenberg 7000  "faust"                    # German (Goethe)
  download_gutenberg 2000  "don-quijote-spanish"      # Spanish
  download_gutenberg 57242 "art-of-war-chinese"       # Chinese (Sun Tzu)
  download_gutenberg 7178  "werther-german"           # German (Goethe)
  download_gutenberg 23393 "candide-french"           # French (Voltaire)
  download_gutenberg 52521 "anna-karenina-russian"    # Russian (Tolstoy)
  download_gutenberg 7993  "chekhov-stories-russian"  # Russian (Chekhov)
  download_gutenberg 14880 "journey-to-west-chinese"  # Chinese classic
  download_gutenberg 60041 "lysistrata-greek"         # Greek (Aristophanes)
  download_gutenberg 22367 "baburnama"                # Persian/Mughal history
  download_gutenberg 24022 "sertoes-portuguese"       # Portuguese (da Cunha)
  download_gutenberg 7205  "carmen-french"            # French (Merimee)
  download_gutenberg 15238 "rubaiyat"                 # Persian poetry
  download_gutenberg 8492  "decameron-italian"         # Italian (Boccaccio)
  download_gutenberg 14765 "les-aventures-de-telemaque" # French (Fenelon)
  download_gutenberg 20580 "sakuntala"                 # Hindi/Sanskrit drama
  download_gutenberg 7452  "korean-fairy-tales"        # Korean
  download_gutenberg 19505 "panchatantra"              # Hindi/Sanskrit fables
  download_gutenberg 38145 "dead-souls-russian"        # Russian (Gogol)

  # --- Very large and reference works ---
  download_gutenberg 3206  "common-sense-paine"
  download_gutenberg 20203 "encyclopedia-britannica-v1" # very large reference
  download_gutenberg 3176  "decline-and-fall-v1"        # very large (Gibbon)
  download_gutenberg 890   "wealth-of-nations-v1"       # very large (Adam Smith)
  download_gutenberg 1041  "complete-poetical-works"    # very large poetry (Shakespeare)
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

  # Additional IDPF EPUB3 samples
  download "wasteland"            "${base}/wasteland-otf.epub"
  download "epub30-spec"          "${base}/epub30-spec.epub"
  download "georgia-pls-ssml"     "${base}/georgia-pls-ssml.epub"
  download "indexing-for-eds"     "${base}/indexing-for-editors-screens.epub"
  download "jlreq-in-english"    "${base}/jlreq-in-english.epub"
  download "kusamakura-ja"       "${base}/kusamakura-japanese-vertical-writing.epub"
  download "moby-dick-mo"        "${base}/moby-dick-mo.epub"
  download "accessible-epub3"    "${base}/accessible-epub3.epub"
}

standardebooks_epubs() {
  echo ""
  echo "=== Standard Ebooks EPUBs ==="
  echo ""

  # Standard Ebooks releases via their structured URL pattern
  # These are high-quality EPUB3 with rich accessibility metadata and custom se:* vocabulary
  download_standardebooks "jane-austen" "persuasion" "se-persuasion"
  download_standardebooks "oscar-wilde" "the-importance-of-being-earnest" "se-earnest"
  download_standardebooks "h-g-wells" "the-time-machine" "se-time-machine"
  download_standardebooks "virginia-woolf" "mrs-dalloway" "se-mrs-dalloway"
  download_standardebooks "f-scott-fitzgerald" "the-great-gatsby" "se-great-gatsby"
  download_standardebooks "kate-chopin" "the-awakening" "se-the-awakening"
  download_standardebooks "james-joyce" "a-portrait-of-the-artist-as-a-young-man" "se-portrait-artist"
  download_standardebooks "joseph-conrad" "lord-jim" "se-lord-jim"
  download_standardebooks "edith-wharton" "the-age-of-innocence" "se-age-of-innocence"
  download_standardebooks "fyodor-dostoevsky" "notes-from-underground" "se-notes-underground"
  download_standardebooks "leo-tolstoy" "the-death-of-ivan-ilyich" "se-ivan-ilyich"
  download_standardebooks "e-m-forster" "a-room-with-a-view" "se-room-with-view"
  download_standardebooks "mark-twain" "a-connecticut-yankee-in-king-arthurs-court" "se-ct-yankee"
  download_standardebooks "agatha-christie" "the-mysterious-affair-at-styles" "se-styles"
  download_standardebooks "edgar-allan-poe" "short-fiction" "se-poe-short-fiction"
  download_standardebooks "h-p-lovecraft" "short-fiction" "se-lovecraft-short-fiction"
  download_standardebooks "p-g-wodehouse" "my-man-jeeves" "se-my-man-jeeves"
  download_standardebooks "jack-london" "white-fang" "se-white-fang"
  download_standardebooks "rudyard-kipling" "just-so-stories" "se-just-so-stories"
  download_standardebooks "willa-cather" "my-antonia" "se-my-antonia"
  download_standardebooks "henry-james" "the-turn-of-the-screw" "se-turn-of-screw"
  download_standardebooks "franz-kafka" "the-trial" "se-the-trial"
  download_standardebooks "voltaire" "candide" "se-candide"
  download_standardebooks "w-e-b-du-bois" "the-souls-of-black-folk" "se-souls-black-folk"
  download_standardebooks "charlotte-bronte" "villette" "se-villette"
  download_standardebooks "charles-dickens" "david-copperfield" "se-david-copperfield"
  download_standardebooks "george-eliot" "middlemarch" "se-middlemarch"
  download_standardebooks "thomas-hardy" "tess-of-the-durbervilles" "se-tess"
  download_standardebooks "william-shakespeare" "the-tempest" "se-the-tempest"
  download_standardebooks "marcus-aurelius" "meditations" "se-meditations"
}

feedbooks_epubs() {
  echo ""
  echo "=== Feedbooks Public Domain EPUBs ==="
  echo ""

  # Feedbooks public domain catalog — Calibre-generated EPUB3 output patterns
  download_feedbooks 6       "fb-metamorphosis"
  download_feedbooks 14      "fb-adventures-huck-finn"
  download_feedbooks 27      "fb-uncle-toms-cabin"
  download_feedbooks 28      "fb-scarlet-letter"
  download_feedbooks 39      "fb-the-yellow-wallpaper"
  download_feedbooks 53      "fb-secret-garden"
  download_feedbooks 54      "fb-jungle-book"
  download_feedbooks 59      "fb-tale-of-peter-rabbit"
  download_feedbooks 61      "fb-the-raven"
  download_feedbooks 62      "fb-gift-of-the-magi"
  download_feedbooks 66      "fb-legend-sleepy-hollow"
  download_feedbooks 69      "fb-wizard-of-oz"
  download_feedbooks 70      "fb-anne-of-green-gables"
  download_feedbooks 73      "fb-the-prince"
  download_feedbooks 77      "fb-sonnets-shakespeare"
  download_feedbooks 137     "fb-three-musketeers"
  download_feedbooks 138     "fb-count-of-monte-cristo-fb"
  download_feedbooks 289     "fb-candide"
  download_feedbooks 715     "fb-le-petit-prince-french"   # French
  download_feedbooks 928     "fb-madame-bovary-french"     # French
}

epub2_epubs() {
  echo ""
  echo "=== EPUB2 Variant EPUBs ==="
  echo ""

  # Force EPUB2 format for legacy path coverage
  download_gutenberg_epub2 5200  "epub2-metamorphosis"
  download_gutenberg_epub2 1342  "epub2-pride-and-prejudice"
  download_gutenberg_epub2 84    "epub2-frankenstein"
  download_gutenberg_epub2 11    "epub2-alice-in-wonderland"
  download_gutenberg_epub2 1661  "epub2-sherlock-holmes"
  download_gutenberg_epub2 74    "epub2-tom-sawyer"
  download_gutenberg_epub2 345   "epub2-dracula"
  download_gutenberg_epub2 98    "epub2-tale-of-two-cities"
  download_gutenberg_epub2 46    "epub2-christmas-carol"
  download_gutenberg_epub2 174   "epub2-dorian-gray"
  download_gutenberg_epub2 768   "epub2-wuthering-heights"
  download_gutenberg_epub2 1260  "epub2-jane-eyre"
  download_gutenberg_epub2 76    "epub2-huckleberry-finn"
  download_gutenberg_epub2 514   "epub2-little-women"
  download_gutenberg_epub2 120   "epub2-treasure-island"
  download_gutenberg_epub2 55    "epub2-oz-wizard"

  # Old IDPF EPUB2 sample
  download "epubbooks-metamorphosis" "https://www.gutenberg.org/ebooks/5200.epub.images"
}

# Parse arguments
SOURCE="${1:---all}"

case "${SOURCE}" in
  --gutenberg)      gutenberg_epubs ;;
  --idpf)           idpf_epubs ;;
  --standardebooks) standardebooks_epubs ;;
  --feedbooks)      feedbooks_epubs ;;
  --epub2)          epub2_epubs ;;
  --all)            gutenberg_epubs; idpf_epubs; standardebooks_epubs; feedbooks_epubs; epub2_epubs ;;
  *)                echo "Usage: $0 [--all | --gutenberg | --idpf | --standardebooks | --feedbooks | --epub2]"; exit 1 ;;
esac

echo ""
echo "=== Summary ==="
count=$(ls -1 "${EPUB_DIR}"/*.epub 2>/dev/null | wc -l)
echo "${count} EPUBs in ${EPUB_DIR}"
