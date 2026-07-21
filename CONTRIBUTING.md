# Contributing to cloudtab

We keep the contribution surface small on purpose: **most contributions are a single new file — one Mapper per Tencent Cloud resource type**.

## Adding a new resource type in ~30 lines

1. Find the Terraform resource type you want to price, e.g. `tencentcloud_mysql_instance`.
2. Find its `InquiryPrice*` API in [Tencent Cloud docs](https://cloud.tencent.com/document/api).
3. Create `internal/resources/<name>.go` implementing `Mapper`:

```go
type MySQLInstance struct{}

func (MySQLInstance) Extract(r parser.PlannedResource) (pricing.PriceRequest, error) {
    // Read plan attributes → build PriceRequest with Product / Action / Params.
}
func (MySQLInstance) Parse(req pricing.PriceRequest, raw []byte) ([]output.CostComponent, error) {
    // Decode the InquiryPrice* JSON → typed CostComponents.
}
```

4. Register it in `internal/resources/registry.go`:

```go
r.Register("tencentcloud_mysql_instance", &MySQLInstance{})
```

5. If the product isn't wired yet in `internal/pricing/engine.go`, add a `queryXxx` branch that follows the same shape as `queryCVM`.
6. Add a fixture under `testdata/` and a `_test.go` next to the mapper using a recorded response.
7. Send a PR — include the API doc URL and one real-world plan snippet.

## Guidelines

- **One resource per file.** Reviewers can look at the file and the API doc side by side.
- **No hand-maintained price tables** unless the product has no `InquiryPrice*` API (COS/CDN/SCF etc.).
- **PREPAID vs POSTPAID**: PREPAID returns fixed prices (use `DiscountPrice`); POSTPAID returns hourly rates (multiply by `730` for monthly).
- **`usage.yml` inputs** (traffic in GB, requests/mo, egress) go through the same Extract path — read from `parser.UsageOverrides[address]` when we ship M5.

## Local dev

```bash
git clone https://github.com/susunola/cloudtab
cd cloudtab
go build ./...
go test ./...
```

## Code of conduct

Be kind, be direct. No spec creep, no non-code drama. If in doubt, open an issue first.
