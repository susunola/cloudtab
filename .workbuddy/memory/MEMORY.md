# cloudtab Project Memory

## Build Environment
- **Go toolchain**: Go 1.25.12 at `/Users/atom/sdk/go1.25.12/`
- **Build command prefix**: `GOROOT=/Users/atom/sdk/go1.25.12 GOPATH=/Users/atom/.workbuddy/binaries/go/gopath GOMODCACHE=/Users/atom/.workbuddy/binaries/go/gopath/pkg/mod PATH=$GOROOT/bin:$PATH`
- **go.mod**: `go 1.25.0`

## GitHub
- Repo: `susunola/cloudtab`
- Push: PAT with `repo`+`workflow` scope via `https://<PAT>@github.com/susunola/cloudtab.git main`
- Pre-push hook shows `/bin/ps: Operation not permitted` warning (non-blocking)

## Architecture
- Handler registry pattern: `internal/pricing/handlers.go` — each cloud product registers `productHandler{product, newClient, action->invoker map}`
- Mapper pattern: `internal/resources/` — each resource type has its own mapper file
- Engine: `internal/pricing/engine.go` — dispatch, retry, dedup, cache
- AWS backend: `internal/pricing/aws_backend.go` — lazy-loaded, separate from Tencent handlers
- **Never refactor clean architecture**; add new products via handlers.go entry + resources/ mapper

## Key Conventions
- **Zero dependency drift**: All 19 Tencent SDK deps at v1.0.1000 — never upgrade existing deps
- **Anti-fabrication**: Only price resources deterministically derivable from plan JSON
- **PREPAID queries force Period/TimeSpan=1 month** (cloudtab reports monthly run-rate)
- **Currency**: Tencent = CNY, AWS = USD; TOTAL only summed when all same currency
- **10/10 `-race` must pass** before any commit
- **Auto commit + push** after each update per standing convention
- **Version**: current `v0.2.0`
