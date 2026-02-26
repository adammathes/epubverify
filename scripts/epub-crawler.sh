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
#   --source SOURCE    Crawl only this source (gutenberg, standardebooks, feedbooks)
#   --limit N          Download at most N new EPUBs per source (default: 20)
#   --epub-dir DIR     Directory to store EPUBs (default: stress-test/crawl-epubs)
#   --manifest FILE    Manifest file path (default: stress-test/crawl-manifest.json)
#   --dry-run          Show what would be downloaded without downloading
#   --help             Show this help
#
# Sources:
#   gutenberg       — Project Gutenberg catalog (thousands of public domain EPUBs)
#   standardebooks  — Standard Ebooks GitHub releases (high-quality EPUB3)
#   feedbooks       — Feedbooks public domain catalog
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

  # Standard Ebooks OPDS catalog endpoint returns a list of books.
  # We fetch the catalog and extract EPUB download URLs.
  local catalog_url="https://standardebooks.org/opds"
  local catalog_file="${EPUB_DIR}/.se-catalog.xml"

  echo "  Fetching Standard Ebooks OPDS catalog..."
  if ! curl -sL -o "${catalog_file}" "${catalog_url}" \
       -H "User-Agent: ${UA}" \
       --connect-timeout 30 \
       --max-time 60; then
    echo "  WARN: could not fetch Standard Ebooks catalog"
    return 0
  fi

  # Extract EPUB download links from OPDS feed
  # Links look like: <link href="/ebooks/xxx/downloads/xxx.epub" type="application/epub+zip" .../>
  local urls
  urls=$(grep -oP 'href="\K/ebooks/[^"]+\.epub(?=")' "${catalog_file}" 2>/dev/null | head -100 || true)

  for path in ${urls}; do
    [ "${count}" -ge "${LIMIT}" ] && break

    local url="https://standardebooks.org${path}"
    # Extract a name from the path: /ebooks/author/title/downloads/file.epub
    local name
    name=$(echo "${path}" | sed 's|.*/||; s|\.epub$||; s|[^a-zA-Z0-9_-]|-|g')
    name="crawl-se-${name}"

    if download_epub "${name}" "${url}" "STANDARDEBOOKS"; then
      count=$((count + 1))
    fi

    sleep "${CRAWL_DELAY}"
  done

  rm -f "${catalog_file}"
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
  gutenberg)       crawl_gutenberg ;;
  standardebooks)  crawl_standardebooks ;;
  feedbooks)       crawl_feedbooks ;;
  all)
    crawl_gutenberg
    crawl_standardebooks
    crawl_feedbooks
    ;;
  *)
    echo "Unknown source: ${SOURCE}" >&2
    echo "Available sources: gutenberg, standardebooks, feedbooks" >&2
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
