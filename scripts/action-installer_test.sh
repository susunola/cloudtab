#!/usr/bin/env bash

set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INSTALLER="$ROOT/scripts/install-action.sh"
ACTION="$ROOT/action.yml"
TMP_ROOT="$(mktemp -d)"
REAL_TAR="$(command -v tar)"
FAILURES=0

cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d ' ' -f 1
  else
    shasum -a 256 "$1" | cut -d ' ' -f 1
  fi
}

fail() {
  echo "FAIL: $1" >&2
  return 1
}

run_test() {
  local name="$1"
  shift
  if "$@"; then
    echo "PASS: $name"
  else
    FAILURES=$((FAILURES + 1))
  fi
}

new_case() {
  CASE_DIR="$(mktemp -d "$TMP_ROOT/case.XXXXXX")"
  MOCK_BIN="$CASE_DIR/bin"
  INSTALL_DIR="$CASE_DIR/install"
  CURL_LOG="$CASE_DIR/curl.log"
  TAR_LOG="$CASE_DIR/tar.log"
  RELEASE_JSON="$CASE_DIR/release.json"
  CHECKSUMS="$CASE_DIR/checksums.txt"
  ARCHIVE="$CASE_DIR/archive.tar.gz"
  mkdir -p "$MOCK_BIN" "$INSTALL_DIR" "$CASE_DIR/payload"

  cat > "$MOCK_BIN/uname" <<'MOCK'
#!/usr/bin/env bash
case "${1:-}" in
  -s) printf '%s\n' "$MOCK_UNAME_S" ;;
  -m) printf '%s\n' "$MOCK_UNAME_M" ;;
  *) exit 2 ;;
esac
MOCK

  cat > "$MOCK_BIN/curl" <<'MOCK'
#!/usr/bin/env bash
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o|--output)
      output="$2"
      shift 2
      ;;
    -*)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done
printf '%s\n' "$url" >> "$MOCK_CURL_LOG"
case "$url" in
  */releases/latest)
    source_file="$MOCK_RELEASE_JSON"
    ;;
  */checksums.txt)
    if [ "${MOCK_CHECKSUM_MISSING:-0}" = "1" ]; then
      exit 22
    fi
    source_file="$MOCK_CHECKSUMS"
    ;;
  *.tar.gz)
    source_file="$MOCK_ARCHIVE"
    ;;
  *)
    exit 22
    ;;
esac
if [ -n "$output" ]; then
  cp "$source_file" "$output"
else
  cat "$source_file"
fi
MOCK

  cat > "$MOCK_BIN/tar" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$MOCK_TAR_LOG"
exec "$MOCK_REAL_TAR" "$@"
MOCK

  chmod +x "$MOCK_BIN/uname" "$MOCK_BIN/curl" "$MOCK_BIN/tar"
  cat > "$CASE_DIR/payload/cloudtab" <<'BIN'
#!/usr/bin/env bash
exit 0
BIN
  chmod +x "$CASE_DIR/payload/cloudtab"
  "$REAL_TAR" -czf "$ARCHIVE" -C "$CASE_DIR/payload" cloudtab
}

write_checksum() {
  local asset="$1"
  local digest
  digest="$(sha256_file "$ARCHIVE")"
  printf '%s  %s\n' "$digest" "$asset" > "$CHECKSUMS"
}

run_installer() {
  local version="$1"
  MOCK_UNAME_S="$MOCK_OS" \
  MOCK_UNAME_M="$MOCK_ARCH" \
  MOCK_RELEASE_JSON="$RELEASE_JSON" \
  MOCK_CHECKSUMS="$CHECKSUMS" \
  MOCK_ARCHIVE="$ARCHIVE" \
  MOCK_CURL_LOG="$CURL_LOG" \
  MOCK_TAR_LOG="$TAR_LOG" \
  MOCK_REAL_TAR="$REAL_TAR" \
  CLOUDTAB_INSTALL_DIR="$INSTALL_DIR" \
  PATH="$MOCK_BIN:$PATH" \
    "$INSTALLER" "$version"
}

test_action_delegates_to_installer() {
  grep -q 'scripts/install-action.sh' "$ACTION" || fail "action.yml does not invoke scripts/install-action.sh"
}

test_pretty_json_uses_exact_tag_name() {
  new_case
  MOCK_OS=Linux
  MOCK_ARCH=x86_64
  cat > "$RELEASE_JSON" <<'JSON'
{
  "url": "https://example.invalid/tag_name/v9.9.9",
  "name": "tag_name must not be treated as a key",
  "tag_name": "v1.2.3",
  "body": "another tag_name value: v8.8.8"
}
JSON
  write_checksum "cloudtab_linux_amd64.tar.gz"

  run_installer latest >/dev/null 2>&1 || fail "latest install failed for pretty JSON"
  [ -x "$INSTALL_DIR/cloudtab" ] || fail "cloudtab was not installed"
  grep -q '/download/v1.2.3/cloudtab_linux_amd64.tar.gz$' "$CURL_LOG" || fail "installer did not use the exact tag_name value"
  grep -q -- '-xzf .*/cloudtab_linux_amd64.tar.gz -C' "$TAR_LOG" || fail "archive was not extracted from a temporary file"
}

test_rejects_invalid_and_multiline_explicit_versions() {
  local version
  for version in '1.2.3' '../v1.2.3' $'v1.2.3\nv9.9.9'; do
    new_case
    MOCK_OS=Linux
    MOCK_ARCH=x86_64
    : > "$RELEASE_JSON"
    : > "$CHECKSUMS"
    if run_installer "$version" >/dev/null 2>&1; then
      fail "accepted invalid version: $version"
      return 1
    fi
    [ ! -s "$CURL_LOG" ] || fail "performed a download for invalid version: $version"
  done
}

test_rejects_invalid_latest_version() {
  new_case
  MOCK_OS=Linux
  MOCK_ARCH=x86_64
  cat > "$RELEASE_JSON" <<'JSON'
{
  "tag_name": "../../v1.2.3"
}
JSON
  : > "$CHECKSUMS"
  if run_installer latest >/dev/null 2>&1; then
    fail "accepted invalid latest tag"
  fi
  [ "$(wc -l < "$CURL_LOG" | tr -d ' ')" = "1" ] || fail "downloaded assets after invalid latest tag"
}

test_checksum_success() {
  new_case
  MOCK_OS=Linux
  MOCK_ARCH=amd64
  : > "$RELEASE_JSON"
  write_checksum "cloudtab_linux_amd64.tar.gz"

  run_installer v2.3.4 >/dev/null 2>&1 || fail "install failed with a valid checksum"
  [ -x "$INSTALL_DIR/cloudtab" ] || fail "valid archive was not installed"
}

test_checksum_mismatch_prevents_extraction() {
  new_case
  MOCK_OS=Linux
  MOCK_ARCH=amd64
  : > "$RELEASE_JSON"
  printf '%064d  cloudtab_linux_amd64.tar.gz\n' 0 > "$CHECKSUMS"

  if run_installer v2.3.4 >/dev/null 2>&1; then
    fail "accepted a checksum mismatch"
  fi
  [ ! -e "$INSTALL_DIR/cloudtab" ] || fail "extracted an archive with a bad checksum"
  [ ! -s "$TAR_LOG" ] || fail "ran tar before rejecting a bad checksum"
}

test_missing_checksums_prevents_extraction() {
  new_case
  MOCK_OS=Linux
  MOCK_ARCH=amd64
  : > "$RELEASE_JSON"
  : > "$CHECKSUMS"

  if MOCK_CHECKSUM_MISSING=1 run_installer v2.3.4 >/dev/null 2>&1; then
    fail "installed without checksums.txt"
  fi
  [ ! -e "$INSTALL_DIR/cloudtab" ] || fail "installed when checksums.txt was missing"
  [ ! -s "$TAR_LOG" ] || fail "ran tar without checksums.txt"
}

test_architecture_mapping() {
  local os arch expected
  while read -r os arch expected; do
    new_case
    MOCK_OS="$os"
    MOCK_ARCH="$arch"
    : > "$RELEASE_JSON"
    write_checksum "cloudtab_${expected}.tar.gz"

    run_installer v3.4.5 >/dev/null 2>&1 || fail "install failed for $os/$arch"
    grep -q "/cloudtab_${expected}.tar.gz$" "$CURL_LOG" || fail "wrong asset selected for $os/$arch"
  done <<'CASES'
Linux x86_64 linux_amd64
Linux aarch64 linux_arm64
Darwin arm64 darwin_arm64
CASES
}

run_test "action delegates to installer" test_action_delegates_to_installer
run_test "pretty JSON exact tag extraction" test_pretty_json_uses_exact_tag_name
run_test "invalid and multiline explicit versions" test_rejects_invalid_and_multiline_explicit_versions
run_test "invalid latest version" test_rejects_invalid_latest_version
run_test "checksum success" test_checksum_success
run_test "checksum mismatch" test_checksum_mismatch_prevents_extraction
run_test "missing checksums.txt" test_missing_checksums_prevents_extraction
run_test "architecture mapping" test_architecture_mapping

if [ "$FAILURES" -ne 0 ]; then
  echo "$FAILURES installer test(s) failed" >&2
  exit 1
fi

echo "All installer tests passed"
