#!/bin/bash
# Download and install epubcheck for use with the crawl validation pipeline.
#
# Installs to $HOME/tools/epubcheck-<version>/ by default, which is the
# location expected by crawl-validate.sh and run-comparison.sh.
#
# Usage:
#   bash scripts/install-epubcheck.sh [OPTIONS]
#
# Options:
#   --version VERSION   epubcheck version to install (default: 5.3.0)
#   --dest DIR          Install directory (default: $HOME/tools)
#   --help              Show this help

set -euo pipefail

VERSION="5.3.0"
DEST_DIR="${HOME}/tools"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)  VERSION="$2"; shift 2 ;;
    --dest)     DEST_DIR="$2"; shift 2 ;;
    --help)
      sed -n '2,/^$/{ s/^# //; s/^#$//; p }' "$0"
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

JAR_PATH="${DEST_DIR}/epubcheck-${VERSION}/epubcheck.jar"

# Check if already installed
if [ -f "${JAR_PATH}" ]; then
  echo "epubcheck ${VERSION} is already installed at ${JAR_PATH}"
  java -jar "${JAR_PATH}" --version 2>/dev/null || true
  exit 0
fi

# Check for Java
if ! command -v java &>/dev/null; then
  echo "ERROR: Java is required to run epubcheck but was not found." >&2
  echo "  Install Java 11+ (e.g., 'apt install default-jre' or 'brew install openjdk')" >&2
  exit 1
fi

echo "Installing epubcheck ${VERSION} to ${DEST_DIR}/epubcheck-${VERSION}/"
echo ""

# Download
mkdir -p "${DEST_DIR}"
DOWNLOAD_URL="https://github.com/w3c/epubcheck/releases/download/v${VERSION}/epubcheck-${VERSION}.zip"
TMP_ZIP="$(mktemp /tmp/epubcheck-XXXXXX.zip)"

echo "  Downloading from ${DOWNLOAD_URL}..."
if ! curl -sL -o "${TMP_ZIP}" "${DOWNLOAD_URL}"; then
  echo "ERROR: Failed to download epubcheck." >&2
  rm -f "${TMP_ZIP}"
  exit 1
fi

# Extract
echo "  Extracting..."
unzip -q -o "${TMP_ZIP}" -d "${DEST_DIR}/"
rm -f "${TMP_ZIP}"

# Verify
if [ ! -f "${JAR_PATH}" ]; then
  echo "ERROR: Installation failed — JAR not found at ${JAR_PATH}" >&2
  exit 1
fi

echo ""
echo "  Installed successfully!"
echo ""
java -jar "${JAR_PATH}" --version 2>/dev/null || true
echo ""
echo "  JAR location: ${JAR_PATH}"
echo "  This is the default path used by crawl-validate.sh."
echo ""
echo "  To use a custom path, set: export EPUBCHECK_JAR=${JAR_PATH}"
