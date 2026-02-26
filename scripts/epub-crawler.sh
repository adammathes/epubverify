#!/bin/bash
# Crawl public EPUB sources, download new EPUBs, deduplicate by SHA-256.
#
# This script discovers EPUBs from multiple public domain sources, downloads
# ones we haven't seen before, and adds them to the crawl manifest for
# validation with both epubverify and epubcheck.
#
# EPUBs are never committed to the repo — they're stored in a gitignored
# directory and tracked by SHA-256 hash in the manifest.
#
# Usage:
#   bash scripts/epub-crawler.sh [OPTIONS]
#
# Options:
#   --source SOURCE    Crawl only this source (gutenberg, standardebooks, feedbooks, internetarchive, oapen)
#   --limit N          Download at most N new EPUBs per source (default: 20)
#   --epub-dir DIR     Directory to store EPUBs (default: stress-test/crawl-epubs)
#   --manifest FILE    Manifest file path (default: stress-test/crawl-manifest.json)
#   --dry-run          Show what would be downloaded without downloading
#   --help             Show this help
#
# Sources:
#   gutenberg        — Project Gutenberg catalog (thousands of public domain EPUBs)
#   standardebooks   — Standard Ebooks GitHub releases (high-quality EPUB3)
#   feedbooks        — Feedbooks public domain catalog
#   internetarchive  — Internet Archive texts collection (huge variety, many rough)
#   oapen            — OAPEN scholarly open-access EPUBs (footnotes, citations)
#
# Environment variables:
#   CRAWL_DELAY       Seconds between requests (default: 2)
#   CRAWL_USER_AGENT  User-Agent header (default: epubverify-crawler/0.1)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
EPUB_DIR="${REPO_ROOT}/stress-test/crawl-epubs"
MANIFEST="${REPO_ROOT}/stress-test/crawl-manifest.json"
LIMIT=20
SOURCE="all"
DRY_RUN=false
CRAWL_DELAY="${CRAWL_DELAY:-2}"
UA="${CRAWL_USER_AGENT:-epubverify-crawler/0.1 (automated epub validation research)}"

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --source)     SOURCE="$2"; shift 2 ;;
    --limit)      LIMIT="$2"; shift 2 ;;
    --epub-dir)   EPUB_DIR="$2"; shift 2 ;;
    --manifest)   MANIFEST="$2"; shift 2 ;;
    --dry-run)    DRY_RUN=true; shift ;;
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

mkdir -p "${EPUB_DIR}"

# --- Manifest management ---

init_manifest() {
  if [ ! -f "${MANIFEST}" ]; then
    cat > "${MANIFEST}" << 'JSONEOF'
{
  "crawl_date": "",
  "epubs": [],
  "summary": {
    "total_tested": 0,
    "agreement": 0,
    "false_positives": 0,
    "false_negatives": 0,
    "crashes": 0
  }
}
JSONEOF
  fi
}

# Check if a SHA-256 hash already exists in the manifest
sha256_exists() {
  local hash="$1"
  python3 -c "
import json, sys
with open('${MANIFEST}') as f:
    m = json.load(f)
for e in m.get('epubs', []):
    if e.get('sha256') == '${hash}':
        sys.exit(0)
sys.exit(1)
" 2>/dev/null
}

# Check if a source URL already exists in the manifest
url_exists() {
  local url="$1"
  python3 -c "
import json, sys
with open('${MANIFEST}') as f:
    m = json.load(f)
for e in m.get('epubs', []):
    if e.get('source_url') == sys.argv[1]:
        sys.exit(0)
sys.exit(1)
" "$url" 2>/dev/null
}

# Add an entry to the manifest
add_to_manifest() {
  local sha256="$1" source_url="$2" source="$3" size_bytes="$4"
  local downloaded_at
  downloaded_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  python3 -c "
import json, sys
with open('${MANIFEST}') as f:
    m = json.load(f)
m['crawl_date'] = '${downloaded_at}'
m['epubs'].append({
    'sha256': '${sha256}',
    'source_url': sys.argv[1],
    'source': '${source}',
    'downloaded_at': '${downloaded_at}',
    'size_bytes': ${size_bytes},
    'epubverify_verdict': '',
    'epubcheck_verdict': '',
    'match': False,
    'discrepancy_issue': ''
})
m['summary']['total_tested'] = len(m['epubs'])
with open('${MANIFEST}', 'w') as f:
    json.dump(m, f, indent=2)
" "$source_url"
}

# --- Download helper ---

download_epub() {
  local name="$1" url="$2" source="$3"
  local filepath="${EPUB_DIR}/${name}.epub"

  # Skip if already downloaded locally
  if [ -f "${filepath}" ]; then
    echo "  SKIP (file exists): ${name}"
    return 1
  fi

  # Skip if URL already in manifest
  if url_exists "${url}"; then
    echo "  SKIP (in manifest): ${name}"
    return 1
  fi

  if [ "${DRY_RUN}" = true ]; then
    echo "  DRY-RUN: would download ${name} from ${url}"
    return 0
  fi

  echo -n "  Downloading ${name}... "
  if curl -sL -o "${filepath}" "${url}" \
       -H "User-Agent: ${UA}" \
       --connect-timeout 30 \
       --max-time 120 && \
     [ -s "${filepath}" ] && \
     file "${filepath}" 2>/dev/null | grep -q "Zip archive\|EPUB"; then

    local size sha256
    size=$(stat -c%s "${filepath}" 2>/dev/null || stat -f%z "${filepath}" 2>/dev/null)
    sha256=$(sha256sum "${filepath}" 2>/dev/null | cut -d' ' -f1 || shasum -a 256 "${filepath}" | cut -d' ' -f1)

    # Check for SHA-256 duplicate
    if sha256_exists "${sha256}"; then
      echo "DUPLICATE (sha256 match)"
      rm -f "${filepath}"
      return 1
    fi

    add_to_manifest "${sha256}" "${url}" "${source}" "${size}"
    echo "OK (${size} bytes, sha256=${sha256:0:12}...)"
    return 0
  else
    rm -f "${filepath}"
    echo "FAILED (not a valid EPUB/ZIP)"
    return 1
  fi
}

# --- Source crawlers ---

crawl_gutenberg() {
  echo ""
  echo "=== Crawling Project Gutenberg ==="
  echo ""

  local count=0

  # Strategy: enumerate Gutenberg EPUB IDs by trying sequential ranges.
  # Gutenberg has ~70,000 books. We start from a random offset and try
  # a batch, skipping IDs that don't exist or that we already have.
  #
  # For reproducibility, we use the current date as a seed to pick
  # a starting range, so each day crawls a different section.
  local day_seed
  day_seed=$(date +%j)  # day of year (1-366)
  local start_id=$(( (day_seed * 173) % 60000 + 1000 ))

  echo "  Starting from ID ${start_id} (day seed=${day_seed})"

  for offset in $(seq 0 200); do
    [ "${count}" -ge "${LIMIT}" ] && break
    local id=$((start_id + offset))

    # Try EPUB3 first, then EPUB2
    local url="https://www.gutenberg.org/ebooks/${id}.epub3.images"
    local name="crawl-gutenberg-${id}"

    if download_epub "${name}" "${url}" "GUTENBERG"; then
      count=$((count + 1))
    else
      # Try EPUB2 fallback
      url="https://www.gutenberg.org/ebooks/${id}.epub.images"
      if download_epub "${name}" "${url}" "GUTENBERG"; then
        count=$((count + 1))
      fi
    fi

    sleep "${CRAWL_DELAY}"
  done

  echo ""
  echo "  Gutenberg: downloaded ${count} new EPUBs"
}

crawl_standardebooks() {
  echo ""
  echo "=== Crawling Standard Ebooks ==="
  echo ""

  local count=0

  # Strategy: scrape the paginated ebooks listing to discover book paths,
  # then construct download URLs. The OPDS catalog requires Patrons Circle
  # authentication, but the ebooks listing and direct downloads are public.
  #
  # We use the day-of-year seed to pick a starting page, so each day
  # crawls a different set of books from the ~1000+ catalog.
  local day_seed
  day_seed=$(date +%j)
  local start_page=$(( (day_seed * 3) % 25 + 1 ))

  echo "  Browsing ebooks listing (starting page ${start_page}, day seed=${day_seed})"

  for page_offset in $(seq 0 5); do
    [ "${count}" -ge "${LIMIT}" ] && break
    local page=$((start_page + page_offset))

    local listing_url="https://standardebooks.org/ebooks?page=${page}&per-page=48"
    local listing_file="${EPUB_DIR}/.se-listing.html"

    if ! curl -sL -o "${listing_file}" "${listing_url}" \
         -H "User-Agent: ${UA}" \
         --connect-timeout 30 \
         --max-time 60; then
      echo "  WARN: could not fetch Standard Ebooks page ${page}"
      continue
    fi

    # Extract unique book paths from the listing page.
    # Links look like: /ebooks/author/title or /ebooks/author/title/translator
    # Filter to paths with at least author+title (2+ segments after /ebooks/)
    local book_paths
    book_paths=$(grep -oP '/ebooks/[a-z][a-z0-9/-]+(?=")' "${listing_file}" 2>/dev/null \
      | grep -vP '/ebooks/[^/]+$' \
      | sort -u || true)

    for book_path in ${book_paths}; do
      [ "${count}" -ge "${LIMIT}" ] && break

      # Construct download URL: /ebooks/a/b/c -> /ebooks/a/b/c/downloads/a_b_c.epub
      # The ?source=feed parameter is required — without it SE returns an HTML page.
      local filename
      filename=$(echo "${book_path}" | sed 's|^/ebooks/||; s|/|_|g')
      local url="https://standardebooks.org${book_path}/downloads/${filename}.epub?source=feed"
      local name="crawl-se-${filename}"

      if download_epub "${name}" "${url}" "STANDARDEBOOKS"; then
        count=$((count + 1))
      fi

      sleep "${CRAWL_DELAY}"
    done
  done

  rm -f "${EPUB_DIR}/.se-listing.html"
  echo ""
  echo "  Standard Ebooks: downloaded ${count} new EPUBs"
}

crawl_feedbooks() {
  echo ""
  echo "=== Crawling Feedbooks Public Domain ==="
  echo ""

  local count=0

  # Feedbooks public domain catalog — sequential IDs
  # Try a range of book IDs
  local day_seed
  day_seed=$(date +%j)
  local start_id=$(( (day_seed * 7) % 1000 + 1 ))

  echo "  Starting from ID ${start_id} (day seed=${day_seed})"

  for offset in $(seq 0 100); do
    [ "${count}" -ge "${LIMIT}" ] && break
    local id=$((start_id + offset))
    local url="https://www.feedbooks.com/book/${id}.epub"
    local name="crawl-feedbooks-${id}"

    if download_epub "${name}" "${url}" "FEEDBOOKS"; then
      count=$((count + 1))
    fi

    sleep "${CRAWL_DELAY}"
  done

  echo ""
  echo "  Feedbooks: downloaded ${count} new EPUBs"
}

crawl_internetarchive() {
  echo ""
  echo "=== Crawling Internet Archive ==="
  echo ""

  local count=0

  # Strategy: use the Advanced Search API to find items containing EPUB files.
  # We page through results using the day-of-year as a seed so each run covers
  # a different slice of the ~millions of text items.
  #
  # If the search API is unavailable, fall back to a curated list of known
  # popular IA identifiers that contain EPUB files.
  local day_seed
  day_seed=$(date +%j)
  local page=$(( (day_seed * 13) % 200 + 1 ))

  echo "  Searching page ${page} (day seed=${day_seed})"

  local search_url="https://archive.org/advancedsearch.php?q=mediatype%3Atexts+format%3AEPUB&fl%5B%5D=identifier&sort%5B%5D=downloads+desc&rows=50&page=${page}&output=json"
  local search_file="${EPUB_DIR}/.ia-search.json"

  local identifiers=""

  if curl -sL -o "${search_file}" "${search_url}" \
       -H "User-Agent: ${UA}" \
       --connect-timeout 30 \
       --max-time 60 2>/dev/null; then

    # Validate the response is actually JSON before parsing
    identifiers=$(python3 -c "
import json, sys
try:
    with open('${search_file}') as f:
        data = json.load(f)
    for doc in data.get('response', {}).get('docs', []):
        ident = doc.get('identifier', '')
        if ident:
            print(ident)
except (json.JSONDecodeError, KeyError, TypeError):
    pass
" 2>/dev/null)
  fi

  if [ -z "${identifiers}" ]; then
    echo "  WARN: search API returned no results (page ${page}), using fallback list"
    # Fallback: curated list of popular IA items known to contain EPUBs.
    # These are well-known public domain works with high download counts.
    local -a fallback_ids=(
      "alice_in_wonderland_lewis_carroll"
      "dracula_by_bram_stoker"
      "frankenstein_by_mary_shelley"
      "pride_and_prejudice_by_jane_austen"
      "the_adventures_of_sherlock_holmes"
      "the_count_of_monte_cristo"
      "the_jungle_book_by_rudyard_kipling"
      "the_war_of_the_worlds_by_h_g_wells"
      "treasure_island_by_robert_louis_stevenson"
      "a_tale_of_two_cities_by_charles_dickens"
      "moby_dick_by_herman_melville"
      "the_picture_of_dorian_gray"
      "the_time_machine_by_h_g_wells"
      "the_metamorphosis_by_franz_kafka"
      "the_great_gatsby_by_f_scott_fitzgerald"
      "heart_of_darkness_by_joseph_conrad"
      "the_yellow_wallpaper_by_charlotte_perkins_gilman"
      "little_women_by_louisa_may_alcott"
      "the_strange_case_of_dr_jekyll_and_mr_hyde"
      "the_importance_of_being_earnest_by_oscar_wilde"
    )
    # Rotate through the fallback list using day seed
    local start_idx=$(( day_seed % ${#fallback_ids[@]} ))
    identifiers=""
    for i in $(seq 0 $((${#fallback_ids[@]} - 1))); do
      local idx=$(( (start_idx + i) % ${#fallback_ids[@]} ))
      identifiers="${identifiers}${fallback_ids[$idx]}"$'\n'
    done
  fi

  rm -f "${search_file}"

  for ia_id in ${identifiers}; do
    [ "${count}" -ge "${LIMIT}" ] && break

    # Fetch item metadata to find the actual EPUB filename
    local meta_url="https://archive.org/metadata/${ia_id}/files"
    local meta_file="${EPUB_DIR}/.ia-meta.json"

    if ! curl -sL -o "${meta_file}" "${meta_url}" \
         -H "User-Agent: ${UA}" \
         --connect-timeout 30 \
         --max-time 30 2>/dev/null; then
      sleep "${CRAWL_DELAY}"
      continue
    fi

    # Find first .epub file in the item's file list.
    # The IA metadata API returns {"result": [...]} with file entries.
    local epub_filename
    epub_filename=$(python3 -c "
import json, sys
try:
    with open('${meta_file}') as f:
        data = json.load(f)
    # The API returns files in a 'result' array
    files = data.get('result', [])
    for entry in files:
        name = entry.get('name', '')
        if name.lower().endswith('.epub'):
            print(name)
            break
except (json.JSONDecodeError, KeyError, TypeError):
    pass
" 2>/dev/null)

    if [ -z "${epub_filename}" ]; then
      # Fallback: try downloading {id}.epub directly (common naming pattern)
      local direct_url="https://archive.org/download/${ia_id}/${ia_id}.epub"
      local name="crawl-ia-${ia_id}"
      if download_epub "${name}" "${direct_url}" "INTERNETARCHIVE"; then
        count=$((count + 1))
      fi
      sleep "${CRAWL_DELAY}"
      continue
    fi

    local url="https://archive.org/download/${ia_id}/${epub_filename}"
    local name="crawl-ia-${ia_id}"

    if download_epub "${name}" "${url}" "INTERNETARCHIVE"; then
      count=$((count + 1))
    fi

    sleep "${CRAWL_DELAY}"
  done

  rm -f "${EPUB_DIR}/.ia-meta.json"
  echo ""
  echo "  Internet Archive: downloaded ${count} new EPUBs"
}

crawl_oapen() {
  echo ""
  echo "=== Crawling OAPEN (Open Access Publishing) ==="
  echo ""

  local count=0

  # Strategy: use the OAI-PMH ListRecords endpoint to discover items, then
  # check each handle page for downloadable EPUB bitstreams.
  # We offset by day-of-year to rotate through different record batches.
  local day_seed
  day_seed=$(date +%j)

  # OAPEN uses DSpace — fetch recent records via OAI-PMH
  local oai_url="https://library.oapen.org/oai/request?verb=ListRecords&metadataPrefix=oai_dc"
  local oai_file="${EPUB_DIR}/.oapen-oai.xml"

  echo "  Fetching OAPEN OAI-PMH records..."
  if ! curl -sL -o "${oai_file}" "${oai_url}" \
       -H "User-Agent: ${UA}" \
       --connect-timeout 30 \
       --max-time 60; then
    echo "  WARN: could not reach OAPEN OAI-PMH endpoint"
    return 0
  fi

  # Extract handle IDs from OAI-PMH response
  # Records contain identifiers like: oai:library.oapen.org:20.500.12657/12345
  local handles
  handles=$(grep -oP '20\.500\.12657/\K[0-9]+' "${oai_file}" 2>/dev/null | sort -u | head -100 || true)

  if [ -z "${handles}" ]; then
    echo "  WARN: no handles found in OAPEN OAI-PMH response"
    rm -f "${oai_file}"
    return 0
  fi

  echo "  Found $(echo "${handles}" | wc -w) unique handles"

  for handle_id in ${handles}; do
    [ "${count}" -ge "${LIMIT}" ] && break

    # Fetch the handle page and look for EPUB bitstream links
    local handle_url="https://library.oapen.org/handle/20.500.12657/${handle_id}"
    local handle_file="${EPUB_DIR}/.oapen-handle.html"

    if ! curl -sL -o "${handle_file}" "${handle_url}" \
         -H "User-Agent: ${UA}" \
         --connect-timeout 30 \
         --max-time 30; then
      sleep "${CRAWL_DELAY}"
      continue
    fi

    # Look for EPUB download link in the page
    # Bitstream links look like: /bitstream/handle/20.500.12657/{id}/{filename}.epub
    local epub_path
    epub_path=$(grep -oP '/bitstream/handle/20\.500\.12657/[^"]+\.epub' "${handle_file}" 2>/dev/null | head -1 || true)

    if [ -z "${epub_path}" ]; then
      sleep "${CRAWL_DELAY}"
      continue
    fi

    local url="https://library.oapen.org${epub_path}"
    local name="crawl-oapen-${handle_id}"

    if download_epub "${name}" "${url}" "OAPEN"; then
      count=$((count + 1))
    fi

    sleep "${CRAWL_DELAY}"
  done

  rm -f "${oai_file}" "${EPUB_DIR}/.oapen-handle.html"
  echo ""
  echo "  OAPEN: downloaded ${count} new EPUBs"
}

# --- Main ---

init_manifest

echo "════════════════════════════════════════════════════════════════"
echo "  EPUB Crawler — epubverify stress test"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo "  EPUB directory: ${EPUB_DIR}"
echo "  Manifest:       ${MANIFEST}"
echo "  Limit per src:  ${LIMIT}"
echo "  Rate limit:     ${CRAWL_DELAY}s"
echo "  Dry run:        ${DRY_RUN}"

case "${SOURCE}" in
  gutenberg)        crawl_gutenberg ;;
  standardebooks)   crawl_standardebooks ;;
  feedbooks)        crawl_feedbooks ;;
  internetarchive)  crawl_internetarchive ;;
  oapen)            crawl_oapen ;;
  all)
    crawl_gutenberg
    crawl_standardebooks
    crawl_feedbooks
    crawl_internetarchive
    crawl_oapen
    ;;
  *)
    echo "Unknown source: ${SOURCE}" >&2
    echo "Available sources: gutenberg, standardebooks, feedbooks, internetarchive, oapen" >&2
    exit 1
    ;;
esac

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "  Crawl complete"
echo "════════════════════════════════════════════════════════════════"

# Print manifest summary
python3 -c "
import json
with open('${MANIFEST}') as f:
    m = json.load(f)
total = len(m.get('epubs', []))
validated = sum(1 for e in m['epubs'] if e.get('epubverify_verdict'))
print(f'  Total EPUBs in manifest: {total}')
print(f'  Validated:               {validated}')
print(f'  Pending validation:      {total - validated}')
"

echo ""
echo "Next step: run 'bash scripts/crawl-validate.sh' to validate new EPUBs."
