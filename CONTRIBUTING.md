# Contributing to cloudtab

Thanks for helping extend cloudtab! This guide covers the conventions that keep
the project healthy. Most of them are enforced by CI (`scripts/check.sh` runs
`gofmt`, `go vet`, `go build`, and `go test -race ./...`).

## Adding a resource type (Mapper)

A "Mapper" is the unit of support for one Terraform resource type. It lives in
`internal/resources/` as one file implementing the `Mapper` interface:

```go
type Mapper interface {
    Extract(r parser.PlannedResource) (pricing.PriceRequest, error)
    Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error)
}
```

- `Extract` turns plan attributes into a neutral `PriceRequest` (provider,
  product, action, region, params).
- `Parse` decodes the raw provider response into `CostComponent`s.

Then register it once in `internal/resources/registry.go`
(`DefaultRegistry`). Each mapper needs:

1. A unit test in `internal/resources/mapper_integration_test.go` (or a
   dedicated `_test.go`) covering `Extract` (param shaping) and `Parse`
   (cost math) for every billing mode it supports.
2. A row in the "Supported resources" table in `README.md`, and an updated
   resource-type count in the README header.

## Branch baseline (important)

**Base your PR branch on the latest `main` and rebase before opening/updating the
PR.** Do not fork a branch off an old commit or a sibling PR's branch.

Why: the registry (`internal/resources/registry.go`) and the integration test
that asserts the registered-type count (`expectedAllTypes` in
`mapper_integration_test.go`) must stay in lock-step. A PR branched from a stale
commit adds a mapper whose count the merge target no longer expects, producing
conflicts and a `slice length != map count` test failure on merge. PRs that are
strict supersets of each other (e.g. one contains all of another's commits)
should be merged as a single PR — the narrower one is then closed, and GitHub
keeps both authors' commits.

To rebase before pushing:

```bash
git fetch origin
git rebase origin/main
```

## Language and content conventions

- **English only** in tracked files (comments, docs, and user-facing strings).
  Use `CNY` not `元`, `cents` not `分`, `hour` not `小时`, `10k` not `万`.
- No tooling/agent branding and no hardcoded local paths (e.g. `/Users/...`)
  in tracked files.

## Pricing integrity

- **Never fabricate.** Only price values deterministically derivable from the
  plan + provider API. Usage-driven resources (S3, EIP idle, COS/CDN/CFS/SCF)
  are reported as zero-cost placeholders with a note, never as a made-up number.
- If a provider response yields **no positive cost** (empty or business-failure
  payload), return an error from `Parse` rather than emitting `$0`. The engine
  then skips the resource with a note. When you add such a guard to a shared
  helper, update every caller that previously discarded the error.

## Dependencies

- **Zero dependency drift.** All existing module dependencies stay at their
  pinned versions — never upgrade an existing dep. New dependencies require
  discussion.

## Workflow

- Run `bash scripts/check.sh` before every commit; better, install it as a
  pre-commit hook: `ln -sf ../../scripts/check.sh .git/hooks/pre-commit`.
- **Push early.** CI only runs on push/PR, so changes left uncommitted bypass
  the safety net. Avoid accumulating unreviewed "fix" files in the working tree.
- **Every bug fix ships a regression test** asserting the exact broken property
  (guard the bug *class*, not just the one repro).
