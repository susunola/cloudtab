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

5. If the product isn't wired yet, register **one `productHandler` in `internal/pricing/handlers.go`** — a client factory plus an `Action → invoker` map. You never touch the engine core (`engine.go`): its `Query`/`invoke` path is fully generic and dispatches through the `handlers` registry.

```go
// internal/pricing/handlers.go
"cdb": {
    product: "cdb",
    newClient: func(cred *tcCommon.Credential, region string, prof *tcProfile.ClientProfile) (interface{}, error) {
        return cdb.NewClient(cred, region, prof)
    },
    actions: map[string]actionInvoker{
        "DescribeDBPrice": func(client interface{}, params map[string]interface{}) ([]byte, error) {
            in := cdb.NewDescribeDBPriceRequest()
            if err := bindParams(params, in); err != nil {
                return nil, err
            }
            out, err := client.(*cdb.Client).DescribeDBPrice(in)
            return sdkResult(out, err)
        },
    },
},
```

6. Add a fixture under `testdata/` and a `_test.go` next to the mapper using a recorded response.
7. Send a PR — include the API doc URL and one real-world plan snippet.

> **Why two layers?** `resources/*.go` is *what* to price (plan → request, response → cost); `pricing/handlers.go` is *how* to call the SDK. Keeping them apart means a new product touches exactly two well-known files and never the dispatch/cache/concurrency core.

## Guidelines

- **One resource per file.** Reviewers can look at the file and the API doc side by side.
- **No hand-maintained price tables** unless the product has no `InquiryPrice*` API (COS/CDN/SCF etc.).
- **PREPAID vs POSTPAID**: PREPAID returns fixed prices (use `DiscountPrice`); POSTPAID returns hourly rates (multiply by `730` for monthly).
- **`usage.yml` inputs** (traffic in GB, requests/mo, egress) go through the same Extract path — they are merged into the resource's attributes before `Extract`, so a mapper just reads them like any other field.

## Local dev

```bash
git clone https://github.com/susunola/cloudtab
cd cloudtab
go build ./...
go test ./...
```

## Code of conduct

Be kind, be direct. No spec creep, no non-code drama. If in doubt, open an issue first.
