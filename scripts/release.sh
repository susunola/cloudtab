#!/usr/bin/env bash
# Build cross-platform binaries and publish them as a GitHub release.
#
# Usage:
#   GITHUB_TOKEN=ghp_xxx scripts/release.sh <VERSION> [GITREF]
#
#   <VERSION>  Release tag, e.g. v0.3.1
#   [GITREF]   Git ref to build from (default: HEAD). Use a tag like v0.3.0
#              to build a historical release faithfully.
#
# Behaviour:
#   - Builds 4 binaries (darwin/linux × amd64/arm64), binary named `cloudtab`,
#     packaged as cloudtab_<os>_<arch>.tar.gz
#   - Creates the git tag (if missing) and pushes it (if missing on remote)
#   - Resolves the GitHub release by tag (reuses existing, else creates)
#   - Uploads each asset, skipping ones that already exist
#
# Idempotent: safe to re-run.

set -euo pipefail

OWNER=susunola
REPO=cloudtab
VERSION="${1:?VERSION required (e.g. v0.3.1)}"
REF="${2:-HEAD}"
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

if [[ -z "$TOKEN" ]]; then
  echo "ERROR: set GITHUB_TOKEN (or GH_TOKEN) with repo scope." >&2
  exit 1
fi

# Use the ambient Go toolchain. GOROOT/GOPATH/GOMODCACHE are only overridden
# if you export them yourself; otherwise `go` resolves its own defaults.
if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: 'go' not found in PATH. Install Go 1.25+ and retry." >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$(mktemp -d)"
echo "Build dir: $OUT"

# ---- 1) Build 4 binaries ------------------------------------------------
SRC="$ROOT"
if [[ "$REF" != "HEAD" ]]; then
  WT="$OUT/worktree"
  git -C "$ROOT" worktree add "$WT" "$REF" >/dev/null
  SRC="$WT"
fi

echo "==> Building 4 binaries for $VERSION from $REF"
for os in darwin linux; do
  for arch in amd64 arm64; do
    bin="$OUT/${os}_${arch}/cloudtab"
    mkdir -p "$(dirname "$bin")"
    echo "  $os/$arch"
    ( cd "$SRC" && GOOS=$os GOARCH=$arch CGO_ENABLED=0 \
        go build -trimpath -ldflags "-s -w -X main.Version=$VERSION" -o "$bin" ./cmd/cloudtab )
    tar -czf "$OUT/cloudtab_${os}_${arch}.tar.gz" -C "$(dirname "$bin")" cloudtab
  done
done
[[ "$REF" != "HEAD" ]] && git -C "$ROOT" worktree remove --force "$WT" >/dev/null

# ---- 2) Tag (if missing) + push (if missing on remote) ------------------
if ! git -C "$ROOT" rev-parse -q --verify "$VERSION" >/dev/null; then
  git -C "$ROOT" tag "$VERSION"
fi
if ! git -C "$ROOT" ls-remote --tags --quiet "https://x-access-token:${TOKEN}@github.com/${OWNER}/${REPO}.git" "refs/tags/$VERSION" | grep -q "$VERSION"; then
  echo "==> Pushing tag $VERSION"
  git -C "$ROOT" push "https://x-access-token:${TOKEN}@github.com/${OWNER}/${REPO}.git" "refs/tags/$VERSION"
fi

# ---- 3) Release body ----------------------------------------------------
BODY_FILE="$OUT/body.md"
if [[ "$VERSION" == "v0.3.1" ]]; then
cat > "$BODY_FILE" <<'EOF'
## cloudtab v0.3.1 — Multi-cloud robustness

Patch release addressing the 10 code-review findings:

- **Huawei Cloud**: corrected `usage_factor` (now `"Duration"`) across all 9 mappers; `project_id` is injected from config (`HUAWEI_PROJECT_ID`) instead of being mis-set to the region.
- **Lazy Tencent credentials**: the engine no longer requires Tencent keys at creation — pure Alibaba/Huawei plans run without them.
- **Configurable cache TTL** (`--cache-ttl`) and **fail-on-error skip mode** (`--fail-on-error`, default off → soft-skip on error).
- **SDK request timeouts** wired for Alibaba and Huawei backends.
- **18 new request-body snapshot tests** lock the exact API payloads forwarded to Alibaba `GetPayAsYouGoPrice` and Huawei `ListOnDemandResourceRatings`.

All 45 mappers (Tencent 19 / AWS 8 / Alibaba 9 / Huawei 9) pass `go vet` and `go test -race`.

> Note: per-resource `usage_factor` values, `size_measure_id=17` (GB), and EIP/ELB `upflow` still warrant a live-API sanity check.
EOF
elif [[ "$VERSION" == "v0.3.2" ]]; then
cat > "$BODY_FILE" <<'EOF'
## cloudtab v0.3.2 — Validation item resolved (Huawei/Alibaba request params)

This release closes the last open validation item from the 2026-07-23 code
review: real-API reconciliation of Huawei/Alibaba pricing request parameters
against the official BSS docs. 9 concrete mapper corrections that would have
produced empty/obviously-wrong prices:

### Huawei Cloud (`ListOnDemandResourceRatings`)
- `usage_measure_id` corrected from `1` → **`4` (hour)** for `Duration`; `upflow` uses **`10` (GB)**.
- **EIP** now modeled as two billable parts: the public IP (`hws.resource.type.ip`) and the linear bandwidth (`hws.resource.type.bandwidth`, `size_measure_id 15` = Mbps). By-traffic → `upflow`/`10`, by-bandwidth → `Duration`/`4`.
- EVS disk carries `resource_size` + `size_measure_id 17` (GB).

### Alibaba Cloud (`GetPayAsYouGoPrice`)
- Correct product codes: **disk → `yundisk`**, **NAT → `nat_gw`** (was `disk`/`natgateway`).
- `Config` strings now use the documented `PropertyCode:Value` format (e.g. `DataDisk.Size:100,DataDisk.Category:cloud_essd`, `InstanceType:ecs.x,ImageOs:linux`, `Bandwidth:5120`, `InternetChargeType:1`, `ISP:BGP`).
- **EIP and NAT are per-DAY priced** → monthly run-rate uses `daysPerMonth` (~30.42), not `hoursPerMonth`.

### Tests
- All 45 mappers pass `go vet` and `go test -race` (10/10).
- Request-body snapshot tests lock the exact payloads; EIP/NAT parse tests assert the day→month conversion.

> Still worth a live-API confirmation when credentials are handy: Alibaba VPN `Bandwidth` config format (applied doc-consistent `Bandwidth:<mbps>`), and a real Huawei EIP by-traffic quote. Everything else is doc-aligned.
EOF
elif [[ "$VERSION" == "v0.3.3" ]]; then
cat > "$BODY_FILE" <<'EOF'
## cloudtab v0.3.3 — Tencent pricing fixes + English localization

This release folds in the first community pull request and the team-review
remediation round. Rebuild recommended for anyone testing Tencent-Cloud plans:
the v0.3.2 binaries returned **zero prices** for CVM/CBS/CLB.

### Tencent Cloud pricing (PR #1)
- **CVM / CBS / CLB**: `Parse()` now unwraps the `{"Response":{...}}` envelope
  instead of reading top-level fields — previously every price came back **0**.
  Currency is read from the API response rather than hardcoded CNY.
- **CLB**: `Extract()` now sends the required `LoadBalancerChargeType`
  (was failing with `MissingParameter`).
- **MongoDB**: adds the mandatory `ClusterType="REPLSET"` + `ReplicateSetNum=1`
  defaults so quotes succeed.

### Multi-cloud correctness & robustness (team-review round)
- Cache-open failures now warn on stderr instead of failing silently.
- Diff output de-branded (no vendor-specific title), merges skipped resources.
- Alibaba SDK read/connect timeouts wired through (was missed in v0.3.1).
- MySQL / PostgreSQL fall back to `OriginalPrice` when discounted price is absent.

### Project hygiene
- Full English localization: all code comments translated; internal Chinese
  design/review docs removed; English README + visual architecture kept as the
  public docs.
- `scripts/release.sh` uses the ambient `go` toolchain (no hardcoded paths).

### Tests
- All 55 mappers pass `go vet` and `go test -race` (5/5 packages).
EOF
else
  echo "Release $VERSION" > "$BODY_FILE"
fi

# ---- 4) Resolve release (reuse existing, else create) -------------------
echo "==> Resolving release $VERSION"
RELEASE_ID=$(curl -fsS -H "Authorization: Bearer $TOKEN" \
  "https://api.github.com/repos/$OWNER/$REPO/releases?per_page=100" \
  | python3 -c "import sys,json; v=sys.argv[1]; print(next((str(r['id']) for r in json.load(sys.stdin) if r['tag_name']==v),''))" "$VERSION")

if [[ -z "$RELEASE_ID" ]]; then
  echo "  creating release"
  python3 - "$VERSION" "$BODY_FILE" > "$OUT/release.json" <<'PY'
import sys, json
ver, body = sys.argv[1], open(sys.argv[2]).read()
print(json.dumps({"tag_name": ver, "name": ver, "body": body, "draft": False, "prerelease": False}))
PY
  RELEASE_ID=$(curl -fsS -X POST "https://api.github.com/repos/$OWNER/$REPO/releases" \
    -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
    --data-binary @"$OUT/release.json" \
    | python3 -c "import sys,json;print(json.load(sys.stdin)['id'])")
fi
echo "  release id: $RELEASE_ID"

# ---- 5) Upload assets (skip existing) -----------------------------------
echo "==> Uploading assets"
EXISTING=$(curl -fsS -H "Authorization: Bearer $TOKEN" \
  "https://api.github.com/repos/$OWNER/$REPO/releases/$RELEASE_ID/assets" \
  | python3 -c "import sys,json;print('\n'.join(a['name'] for a in json.load(sys.stdin)))")

for f in "$OUT"/cloudtab_*.tar.gz; do
  name=$(basename "$f")
  if echo "$EXISTING" | grep -qx "$name"; then
    echo "  skip $name (already present)"
    continue
  fi
  echo "  upload $name"
  curl -fsS -X POST "https://uploads.github.com/repos/$OWNER/$REPO/releases/$RELEASE_ID/assets?name=$name" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/gzip" \
    --data-binary "@$f"
done

echo ""
echo "✓ Done: https://github.com/$OWNER/$REPO/releases/tag/$VERSION"
