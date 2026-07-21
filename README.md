<p align="center">
  <img src="docs/logo.svg" alt="cloudtab" width="220"/>
</p>

<h1 align="center">cloudtab</h1>

<p align="center">
  <b>Multi-cloud cost estimation for Terraform — Infracost, but for the clouds it doesn't cover.</b>
</p>

<p align="center">
  <i>Currently supports <b>Tencent Cloud</b>. AWS / Alibaba Cloud / Huawei Cloud on the roadmap.</i>
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="docs/usage.md">Usage guide</a> ·
  <a href="docs/design.md">Design</a> ·
  <a href="#roadmap">Roadmap</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/status-alpha-orange"/>
  <img src="https://img.shields.io/badge/go-1.22+-00ADD8?logo=go"/>
  <img src="https://img.shields.io/badge/license-MIT-blue"/>
</p>

---

`cloudtab` reads a Terraform plan and tells you what it will cost — **before** you `apply`. Same UX as Infracost, but focused on the clouds Infracost doesn't cover (Tencent today; AWS / Alibaba / Huawei next).

- 🔌 **Zero setup** — one binary, provider credentials as env vars, one command
- 💰 **Real prices, not guesses** — calls each cloud's official inquiry APIs; no hand-maintained price tables
- 🔀 **PR diffs** — `cloudtab diff` shows `+/-/~` per resource with monthly Δ
- 🤖 **CI-friendly** — GitHub Action ships a sticky PR comment
- 🧩 **Pluggable** — one Go file per resource type; contribute a Mapper, cover more products or providers

---

## Quick start

```bash
# 1. install
go install github.com/susunola/cloudtab/cmd/cloudtab@latest

# 2. set credentials (read-only CAM policy: QcloudCVMInnerReadOnly + Inquiry*)
export TENCENTCLOUD_SECRET_ID=AKIDxxxx
export TENCENTCLOUD_SECRET_KEY=xxxxxxxx

# 3. generate a plan and estimate cost
terraform plan -out=tf.plan
terraform show -json tf.plan > plan.json
cloudtab breakdown --path plan.json --region ap-guangzhou
```

Sample output:

```
+--------------------------------+------------------+-----------------+
|            RESOURCE            |    COMPONENT     |  MONTHLY (CNY)  |
+--------------------------------+------------------+-----------------+
| tencentcloud_instance.api      | Compute (S5.MED) |          214.20 |
| tencentcloud_cbs_storage.data  | CBS PREMIUM_50GB |           45.00 |
+--------------------------------+------------------+-----------------+
|                                | TOTAL            |          259.20 |
+--------------------------------+------------------+-----------------+
```

## Supported resources

| Product | Terraform type | Pricing API |
|---|---|---|
| CVM | `tencentcloud_instance` | `cvm:InquiryPriceRunInstances` |
| CBS | `tencentcloud_cbs_storage` | `cbs:InquiryPriceCreateDisks` |
| EIP | `tencentcloud_eip` | No official InquiryPrice API (static placeholder + note) |
| CLB | `tencentcloud_clb_instance` | `clb:InquiryPriceCreateLoadBalancer` |
| MySQL | `tencentcloud_mysql_instance` | `cdb:DescribeDBPrice` |
| PostgreSQL | `tencentcloud_postgresql_instance` | `postgres:InquiryPriceCreateDBInstances` |
| Redis | `tencentcloud_redis_instance` | `redis:InquiryPriceCreateInstance` |
| VPN Gateway | `tencentcloud_vpn_gateway` | `vpc:InquiryPriceCreateVpnGateway` |
| MongoDB | `tencentcloud_mongodb_instance` | `mongodb:InquirePriceCreateDBInstances` |
| MariaDB | `tencentcloud_mariadb_instance` | `mariadb:DescribePrice` |
| TDSQL-C | `tencentcloud_cynosdb_cluster` | `cynosdb:InquirePriceCreate` |

**Coming next**: COS, CDN, CFS, SCF (usage-driven + static price tables). See [issues](https://github.com/susunola/cloudtab/issues) or contribute a Mapper — [CONTRIBUTING.md](CONTRIBUTING.md).

## GitHub Action

```yaml
# .github/workflows/cloudtab.yml
on: pull_request
jobs:
  cost:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: susunola/cloudtab@v0
        env:
          TENCENTCLOUD_SECRET_ID:  ${{ secrets.TENCENTCLOUD_SECRET_ID }}
          TENCENTCLOUD_SECRET_KEY: ${{ secrets.TENCENTCLOUD_SECRET_KEY }}
        with:
          path: "infra/"
          base-ref: ${{ github.event.pull_request.base.ref }}
```

A sticky PR comment like [this](examples/pr-comment.md) shows up on every PR that changes `.tf` files.

## Design in 60 seconds

```
plan.json ─► parser ─► mapper (per resource type) ─► pricing engine ─► InquiryPrice* API
                                                         │
                                                         └─► local cache (sha256 → JSON)
                                                                       │
                                                                       ▼
                                                              table / JSON / markdown
```

Full architecture in [`docs/design.md`](docs/design.md) and the [visual architecture](docs/cloudtab-architecture.html).

## Roadmap

Tencent Cloud (current):
- [x] **M1** — CVM end-to-end
- [x] **M2** — CBS, EIP, CLB
- [x] **M3** — `diff` command + markdown output
- [x] **M4** — GitHub Action + sticky PR comment
- [x] **M5** — TencentDB MySQL/Redis, `usage.yml` override wiring (`--usage-file`, `--before-usage-file`, `--after-usage-file`)
- [ ] **M6** — Static price table for COS/CDN/SCF (no InquiryPrice API)

Multi-cloud (next):
- [ ] **M7** — Provider abstraction (`internal/providers/{tencent,aws,aliyun,huawei}/`)
- [ ] **M8** — AWS EC2 / EBS / ELB via Pricing API (falls back to `infracost` where sensible)
- [ ] **M9** — Alibaba Cloud ECS / EBS via `DescribePrice`
- [ ] **M10** — Huawei Cloud ECS / EVS

Beyond:
- [ ] VS Code inline hints (`Save as .tf → cost changes here`)

## Why not just use the console price calculator?

The console calculator lives outside your PR flow and requires re-entering every parameter manually. `cloudtab` reads the *actual* plan you'll apply and answers **"what changes"** — the thing reviewers actually care about.

## License

MIT © cloudtab contributors. Not affiliated with Tencent Cloud, AWS, Alibaba Cloud, or Huawei Cloud.
