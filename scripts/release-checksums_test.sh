#!/usr/bin/env bash

set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RELEASE_SCRIPT="$ROOT/scripts/release.sh"
TMP_ROOT="$(mktemp -d)"
FAILURES=0
VERSION=v9.8.7
REF=HEAD
REF_SHA=1111111111111111111111111111111111111111
ASSET_NAMES="cloudtab_darwin_amd64.tar.gz
cloudtab_darwin_arm64.tar.gz
cloudtab_linux_amd64.tar.gz
cloudtab_linux_arm64.tar.gz"

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
  REMOTE_DIR="$CASE_DIR/remote"
  CURL_LOG="$CASE_DIR/curl.log"
  GIT_LOG="$CASE_DIR/git.log"
  UPLOAD_LOG="$CASE_DIR/upload.log"
  CAPTURED_CHECKSUMS="$CASE_DIR/uploaded-checksums.txt"
  EXISTING_CHECKSUMS="$REMOTE_DIR/checksums.txt"
  mkdir -p "$MOCK_BIN" "$REMOTE_DIR"
  : > "$GIT_LOG"
  : > "$UPLOAD_LOG"

  local id=101
  local name
  while IFS= read -r name; do
    printf 'published-%s\n' "$name" > "$REMOTE_DIR/$name"
    printf '%s %s\n' "$id" "$name" >> "$CASE_DIR/assets.map"
    id=$((id + 1))
  done <<< "$ASSET_NAMES"

  cat > "$MOCK_BIN/go" <<'MOCK'
#!/usr/bin/env bash
if [ "${MOCK_GO_FAIL:-0}" = "1" ]; then
  echo "mock go build failure" >&2
  exit 1
fi
output=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
[ -n "$output" ] || exit 2
mkdir -p "$(dirname "$output")"
printf 'local-rebuild-%s-%s\n' "$GOOS" "$GOARCH" > "$output"
chmod +x "$output"
MOCK

  cat > "$MOCK_BIN/git" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$MOCK_GIT_LOG"
args=" $* "
case "$args" in
  *" rev-parse -q --verify ${MOCK_REF}^{commit} "*)
    printf '%s\n' "$MOCK_REF_SHA"
    ;;
  *" rev-parse -q --verify refs/tags/${MOCK_VERSION}^{commit} "*|*" rev-parse -q --verify ${MOCK_VERSION}^{commit} "*|*" rev-parse -q --verify ${MOCK_VERSION} "*)
    if [ -n "${MOCK_LOCAL_TAG_SHA:-}" ]; then
      printf '%s\n' "$MOCK_LOCAL_TAG_SHA"
    else
      exit 1
    fi
    ;;
  *" ls-remote "*)
    if [ -n "${MOCK_REMOTE_TAG_SHA:-}" ]; then
      printf '%s\trefs/tags/%s\n' "$MOCK_REMOTE_TAG_SHA" "$MOCK_VERSION"
    fi
    ;;
  *" worktree add "*)
    previous=""
    worktree=""
    for arg in "$@"; do
      if [ "$previous" = "--detach" ]; then
        worktree="$arg"
        break
      fi
      previous="$arg"
    done
    [ -n "$worktree" ] || exit 2
    mkdir -p "$worktree"
    ;;
  *" worktree remove --force "*)
    ;;
  *" tag ${MOCK_VERSION} "*|*" tag ${MOCK_VERSION} "*)
    ;;
  *" push "*)
    ;;
  *)
    echo "unexpected git command: $*" >&2
    exit 2
    ;;
esac
MOCK

  cat > "$MOCK_BIN/curl" <<'MOCK'
#!/usr/bin/env bash
output=""
data_file=""
method=GET
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o|--output)
      output="$2"
      shift 2
      ;;
    --data-binary)
      data_file="${2#@}"
      shift 2
      ;;
    -X)
      method="$2"
      shift 2
      ;;
    -H)
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
printf '%s %s\n' "$method" "$url" >> "$MOCK_CURL_LOG"

case "$url" in
  *'/releases?per_page=100')
    printf '[{"id":42,"tag_name":"%s"}]\n' "$MOCK_VERSION"
    ;;
  *'/releases/42/assets')
    first=1
    printf '['
    while read -r id name; do
      [ "$first" -eq 1 ] || printf ','
      printf '{"id":%s,"name":"%s"}' "$id" "$name"
      first=0
    done < "$MOCK_ASSET_MAP"
    if [ -f "$MOCK_EXISTING_CHECKSUMS" ]; then
      [ "$first" -eq 1 ] || printf ','
      printf '{"id":999,"name":"checksums.txt"}'
    fi
    printf ']\n'
    ;;
  *'/releases/assets/'*)
    id="${url##*/}"
    if [ "$id" = "999" ]; then
      cp "$MOCK_EXISTING_CHECKSUMS" "$output"
    else
      name="$(while read -r asset_id asset_name; do [ "$asset_id" = "$id" ] && printf '%s' "$asset_name"; done < "$MOCK_ASSET_MAP")"
      [ -n "$name" ] || exit 22
      cp "$MOCK_REMOTE_DIR/$name" "$output"
    fi
    ;;
  *'/releases/download/'*)
    name="${url##*/}"
    [ -f "$MOCK_REMOTE_DIR/$name" ] || exit 22
    cp "$MOCK_REMOTE_DIR/$name" "$output"
    ;;
  *'uploads.github.com/'*'name=checksums.txt')
    printf 'checksums.txt\n' >> "$MOCK_UPLOAD_LOG"
    cp "$data_file" "$MOCK_CAPTURED_CHECKSUMS"
    printf '{}\n'
    ;;
  *'uploads.github.com/'*)
    name="${url##*name=}"
    printf '%s\n' "$name" >> "$MOCK_UPLOAD_LOG"
    cp "$data_file" "$MOCK_REMOTE_DIR/$name"
    if ! grep -q "^[0-9][0-9]* $name$" "$MOCK_ASSET_MAP"; then
      next_id=$((200 + $(wc -l < "$MOCK_ASSET_MAP")))
      printf '%s %s\n' "$next_id" "$name" >> "$MOCK_ASSET_MAP"
    fi
    printf '{}\n'
    ;;
  *)
    echo "unexpected curl URL: $url" >&2
    exit 22
    ;;
esac
MOCK

  chmod +x "$MOCK_BIN/go" "$MOCK_BIN/git" "$MOCK_BIN/curl"
}

run_release() {
  MOCK_VERSION="$VERSION" \
  MOCK_REF="$REF" \
  MOCK_REF_SHA="$REF_SHA" \
  MOCK_LOCAL_TAG_SHA="${MOCK_LOCAL_TAG_SHA:-}" \
  MOCK_REMOTE_TAG_SHA="${MOCK_REMOTE_TAG_SHA:-}" \
  MOCK_GO_FAIL="${MOCK_GO_FAIL:-0}" \
  MOCK_CURL_LOG="$CURL_LOG" \
  MOCK_GIT_LOG="$GIT_LOG" \
  MOCK_UPLOAD_LOG="$UPLOAD_LOG" \
  MOCK_CAPTURED_CHECKSUMS="$CAPTURED_CHECKSUMS" \
  MOCK_REMOTE_DIR="$REMOTE_DIR" \
  MOCK_ASSET_MAP="$CASE_DIR/assets.map" \
  MOCK_EXISTING_CHECKSUMS="$EXISTING_CHECKSUMS" \
  GITHUB_TOKEN=test-token \
  PATH="$MOCK_BIN:/usr/bin:/bin" \
    "$RELEASE_SCRIPT" "$VERSION" "$REF"
}

write_expected_checksums() {
  local destination="$1"
  local name digest
  : > "$destination"
  while IFS= read -r name; do
    digest="$(sha256_file "$REMOTE_DIR/$name")"
    printf '%s  %s\n' "$digest" "$name" >> "$destination"
  done <<< "$ASSET_NAMES"
}

test_checksums_use_exact_published_assets() {
  local expected
  new_case
  expected="$CASE_DIR/expected-checksums.txt"
  write_expected_checksums "$expected"

  run_release >/dev/null 2>&1 || fail "release script failed"
  [ -f "$CAPTURED_CHECKSUMS" ] || fail "checksums.txt was not uploaded"
  cmp -s "$expected" "$CAPTURED_CHECKSUMS" || fail "checksums were not generated from the exact published assets"
  [ "$(wc -l < "$CAPTURED_CHECKSUMS" | tr -d ' ')" = "4" ] || fail "checksums.txt did not contain exactly four release assets"
  [ "$(grep -c '^checksums.txt$' "$UPLOAD_LOG")" = "1" ] || fail "checksums.txt was not uploaded exactly once"
  [ "$(wc -l < "$UPLOAD_LOG" | tr -d ' ')" = "1" ] || fail "preexisting archives were uploaded again"
}

test_existing_checksums_are_verified_idempotently() {
  new_case
  write_expected_checksums "$EXISTING_CHECKSUMS"
  if MOCK_LOCAL_TAG_SHA="$REF_SHA" MOCK_REMOTE_TAG_SHA="$REF_SHA" run_release >/dev/null 2>&1; then
    [ ! -s "$UPLOAD_LOG" ] || fail "uploaded an asset even though the full release already existed"
    grep -q '/releases/assets/999$' "$CURL_LOG" || fail "existing checksums.txt was not downloaded for verification"
  else
    fail "idempotent release rerun failed"
  fi
}

test_partial_release_rejects_stale_checksums() {
  local missing output
  new_case
  missing=cloudtab_linux_arm64.tar.gz
  write_expected_checksums "$EXISTING_CHECKSUMS"
  grep -v " $missing$" "$CASE_DIR/assets.map" > "$CASE_DIR/assets.tmp"
  mv "$CASE_DIR/assets.tmp" "$CASE_DIR/assets.map"
  grep -v "  $missing$" "$EXISTING_CHECKSUMS" > "$CASE_DIR/checksums.tmp"
  mv "$CASE_DIR/checksums.tmp" "$EXISTING_CHECKSUMS"

  if output=$(MOCK_LOCAL_TAG_SHA="$REF_SHA" MOCK_REMOTE_TAG_SHA="$REF_SHA" run_release 2>&1); then
    fail "release accepted stale checksums.txt after uploading a missing archive"
  fi
  case "$output" in
    *"existing checksums.txt does not match exact published assets"*) ;;
    *) fail "release did not report the checksum manifest mismatch" ;;
  esac
}

test_release_pins_ref_and_uses_detached_worktree() {
  new_case
  run_release >/dev/null 2>&1 || fail "release script failed"
  [ "$(grep -c "rev-parse -q --verify ${REF}^{commit}" "$GIT_LOG")" = "1" ] || fail "REF was not resolved exactly once"
  grep -q "worktree add --detach .* $REF_SHA$" "$GIT_LOG" || fail "build did not use a detached worktree at the resolved SHA"
  grep -q " tag $VERSION $REF_SHA$" "$GIT_LOG" || fail "tag was not created explicitly at the resolved SHA"
  grep -q "worktree remove --force " "$GIT_LOG" || fail "temporary worktree was not cleaned up"
}

test_local_tag_mismatch_fails() {
  local output
  new_case
  if output=$(MOCK_LOCAL_TAG_SHA=2222222222222222222222222222222222222222 MOCK_REMOTE_TAG_SHA="$REF_SHA" run_release 2>&1); then
    fail "release accepted a local tag pointing at another commit"
  fi
  case "$output" in
    *"local tag $VERSION"*"does not point to $REF_SHA"*) ;;
    *) fail "local tag mismatch error was not explicit" ;;
  esac
}

test_remote_tag_mismatch_fails() {
  local output
  new_case
  if output=$(MOCK_LOCAL_TAG_SHA="$REF_SHA" MOCK_REMOTE_TAG_SHA=3333333333333333333333333333333333333333 run_release 2>&1); then
    fail "release accepted a remote tag pointing at another commit"
  fi
  case "$output" in
    *"remote tag $VERSION"*"does not point to $REF_SHA"*) ;;
    *) fail "remote tag mismatch error was not explicit" ;;
  esac
}

test_interrupted_build_cleans_worktree() {
  new_case
  if MOCK_GO_FAIL=1 run_release >/dev/null 2>&1; then
    fail "mock build unexpectedly succeeded"
  fi
  grep -q "worktree add --detach .* $REF_SHA$" "$GIT_LOG" || fail "temporary detached worktree was not created"
  grep -q "worktree remove --force " "$GIT_LOG" || fail "temporary worktree survived an interrupted build"
}

run_test "checksums use exact published assets" test_checksums_use_exact_published_assets
run_test "existing checksums are verified idempotently" test_existing_checksums_are_verified_idempotently
run_test "partial release rejects stale checksums" test_partial_release_rejects_stale_checksums
run_test "release pins REF and uses detached worktree" test_release_pins_ref_and_uses_detached_worktree
run_test "local tag mismatch fails" test_local_tag_mismatch_fails
run_test "remote tag mismatch fails" test_remote_tag_mismatch_fails
run_test "interrupted build cleans worktree" test_interrupted_build_cleans_worktree

if [ "$FAILURES" -ne 0 ]; then
  echo "$FAILURES release checksum test(s) failed" >&2
  exit 1
fi

echo "All release checksum tests passed"
