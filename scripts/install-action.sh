#!/usr/bin/env bash

set -euo pipefail

OWNER=susunola
REPO=cloudtab
REQUESTED_VERSION="${1:-latest}"
INSTALL_DIR="${CLOUDTAB_INSTALL_DIR:-/usr/local/bin}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cloudtab-install.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

valid_version() {
  [[ "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

VERSION="$REQUESTED_VERSION"
if [[ "$VERSION" == "latest" ]]; then
  RELEASE_JSON="$TMP_DIR/latest.json"
  curl -fsSL -o "$RELEASE_JSON" \
    "https://api.github.com/repos/$OWNER/$REPO/releases/latest"
  VERSION="$(sed -nE 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)"[[:space:]]*,?[[:space:]]*$/\1/p' "$RELEASE_JSON")"
fi

valid_version "$VERSION" || fail "invalid cloudtab version: $VERSION"

case "$(uname -s)" in
  Linux) OS=linux ;;
  Darwin) OS=darwin ;;
  *) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

ASSET="cloudtab_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$OWNER/$REPO/releases/download/$VERSION"
ARCHIVE="$TMP_DIR/$ASSET"
CHECKSUMS="$TMP_DIR/checksums.txt"

curl -fsSL -o "$ARCHIVE" "$BASE_URL/$ASSET"
curl -fsSL -o "$CHECKSUMS" "$BASE_URL/checksums.txt"

EXPECTED=""
MATCHES=0
while read -r digest filename; do
  filename="${filename#\*}"
  if [[ "$filename" == "$ASSET" ]]; then
    EXPECTED="$digest"
    MATCHES=$((MATCHES + 1))
  fi
done < "$CHECKSUMS"

[[ "$MATCHES" -eq 1 ]] || fail "checksums.txt must contain exactly one entry for $ASSET"
[[ "$EXPECTED" =~ ^[0-9A-Fa-f]{64}$ ]] || fail "invalid SHA-256 checksum for $ASSET"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$ARCHIVE")"
else
  command -v shasum >/dev/null 2>&1 || fail "no SHA-256 utility found"
  ACTUAL="$(shasum -a 256 "$ARCHIVE")"
fi
ACTUAL="${ACTUAL%%[[:space:]]*}"
EXPECTED="$(printf '%s' "$EXPECTED" | tr '[:upper:]' '[:lower:]')"
ACTUAL="$(printf '%s' "$ACTUAL" | tr '[:upper:]' '[:lower:]')"
[[ "$ACTUAL" == "$EXPECTED" ]] || fail "SHA-256 checksum mismatch for $ASSET"

mkdir -p "$INSTALL_DIR"
tar -xzf "$ARCHIVE" -C "$INSTALL_DIR" cloudtab
[[ -x "$INSTALL_DIR/cloudtab" ]] || fail "installed cloudtab is not executable"
"$INSTALL_DIR/cloudtab" --help >/dev/null

echo "Installed cloudtab $VERSION to $INSTALL_DIR/cloudtab"
