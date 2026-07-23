<p align="center">
  <img src="docs/logo.svg" alt="cloudtab" width="220"/>
</p>

<h1 align="center">cloudtab</h1>

<p align="center">
  <b>Multi-cloud cost estimation for Terraform — Infracost, but for the clouds it doesn't cover.</b>
</p>

<p align="center">
  <i>Currently supports <b>Tencent Cloud</b> and <b>AWS</b>. Alibaba Cloud / Huawei Cloud on the roadmap.</i>
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

`cloudtab` reads a Terraform plan and tells you what it will cost — **before** you `apply`. Same UX as Infracost, but focused on the clouds Infracost doesn't cover (Tencent Cloud + AWS today; Alibaba / Huawei next). A single plan may mix Tencent and AWS resources — each is routed to its own pricing backend by the resource's provider prefix.

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

# 2b. (optional) AWS credentials — only needed if the plan has aws_* resources.
#     Uses the standard AWS credential chain; the pricing:GetProducts action is
#     the only permission required (attach AWSPriceListServiceFullAccess or an
#     equivalent read-only policy).
export AWS_ACCESS_KEY_ID=AKIAxxxx
export AWS_SECRET_ACCESS_KEY=xxxxxxxx

# 3. generate a plan and estimate cost
terraform plan -out=tf.plan
terraform show -json tf.plan > plan.json
cloudtab breakdown --path plan.json --region ap-guangzhou
```

> A pure-Tencent plan needs **no** AWS credentials — the AWS SDK is only
> initialised the first time an `aws_*` resource is priced.

### Chinese-mainland vs International site

Tencent Cloud runs **two independent sites** with separate account systems: the
Chinese-mainland site (`*.tencentcloudapi.com`) and the International site
(`*.intl.tencentcloudapi.com`). A given `SecretId`/`SecretKey` is registered on
exactly **one** of them, so the site is chosen by your **credential — not the
region** (both sites expose overlapping region names such as `ap-guangzhou` /
`ap-singapore`). Select it explicitly to match your key:

```bash
# International-site credential
cloudtab breakdown --path plan.json --region ap-singapore --site intl
# or via env (flag takes precedence)
export TENCENTCLOUD_SITE=intl
```

`--site` accepts `domestic` (default) or `intl`; any other value is passed
through verbatim as a custom root domain (e.g. a private-cloud gateway). Prices
are cached separately per site, so switching sites never returns a stale
cross-site price.

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

### Tencent Cloud

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
| Lighthouse | `tencentcloud_lighthouse_instance` | `lighthouse:InquirePriceCreateInstances` |
| ECM (Edge) | `tencentcloud_ecm_instance` | `ecm:DescribePriceRunInstance` |
| SQL Server | `tencentcloud_sqlserver_instance` | `sqlserver:InquiryPriceCreateDBInstances` |
| TDSQL MySQL | `tencentcloud_dcdb_instance` | `dcdb:DescribeDCDBPrice` |
| GAAP | `tencentcloud_gaap_proxy` | `gaap:InquiryPriceCreateProxy` |
| CWP (Host Security) | `tencentcloud_cwp_license_order` | `yunjing:InquiryPriceOpenProVersionPrepaid` |
| CloudHSM | `tencentcloud_cloudhsm_instance` | `cloudhsm:InquiryPriceBuyVsm` |
| Domain | `tencentcloud_domain_registration` | `domain:DescribeDomainPriceList` |

### AWS

Priced live via the **AWS Price List (Pricing) API** (`pricing:GetProducts`). The
Pricing API endpoint is always `us-east-1`; the region being priced is expressed
through a `location` filter, so any commercial AWS region is supported.

| Product | Terraform type | Priced component |
|---|---|---|
| EC2 | `aws_instance` | On-demand instance hour (Linux, shared tenancy) × 730 |
| EBS | `aws_ebs_volume` | Per-GB-month storage rate × provisioned size |
| RDS | `aws_db_instance` | On-demand DB instance hour (per engine + Single/Multi-AZ) × 730 |
| ElastiCache | `aws_elasticache_cluster` | On-demand node hour × node count × 730 |
| ELB (ALB/NLB/GWLB) | `aws_lb` | Fixed load-balancer hour (base, **excl.** LCU/data-processing) × 730 |
| ELB (Classic) | `aws_elb` | Fixed load-balancer hour (base, **excl.** data-processing) × 730 |
| Aurora | `aws_rds_cluster_instance` | On-demand Aurora instance hour (per engine, Single-AZ) × 730 |
| Redshift | `aws_redshift_cluster` | On-demand node hour × node count × 730 (**excl.** RA3 managed storage) |
| OpenSearch / ES | `aws_opensearch_domain`, `aws_elasticsearch_domain` | On-demand data-node hour × instance count × 730 |
| DocumentDB | `aws_docdb_cluster_instance` | On-demand instance hour × 730 |
| Neptune | `aws_neptune_cluster_instance` | On-demand instance hour × 730 |
| MemoryDB | `aws_memorydb_cluster` | On-demand node hour × (shards × (1 + replicas/shard)) × 730 |
| Amazon MQ | `aws_mq_broker` | On-demand broker hour (per engine + Single/Multi-AZ) × 730 |
| MSK | `aws_msk_cluster` | On-demand broker hour × broker count × 730 |
| DynamoDB (provisioned) | `aws_dynamodb_table` | Provisioned RCU-hour + WCU-hour × 730 (PAY_PER_REQUEST skipped) |
| EKS | `aws_eks_cluster` | Control-plane hour (per cluster) × 730 |
| NAT gateway | `aws_nat_gateway` | Fixed hourly rate (base, **excl.** data-processing) × 730 |

AWS prices are quoted in **USD**; a mixed-provider plan shows a per-component
`Currency` column and only sums a grand total when the currency is uniform.

> **Not priced from a plan (by design):** `aws_s3_bucket`, `aws_eip` and
> `aws_efs_file_system` are purely usage-driven — S3/EFS cost depends on GB
> stored / requests / egress, and an EIP is only billed while idle/unattached or
> as a public-IPv4 hourly charge. A Terraform plan carries none of those figures,
> so any monthly number would be fabricated. They are intentionally left
> unregistered rather than reported as $0. DynamoDB is priced only in
> `PROVISIONED` mode (RCU/WCU are in the plan); `PAY_PER_REQUEST` tables are
> skipped for the same reason.

**Coming next**: COS, CDN, CFS, SCF (usage-driven + static price tables), more AWS services. See [issues](https://github.com/susunola/cloudtab/issues) or contribute a Mapper — [CONTRIBUTING.md](CONTRIBUTING.md).

> **Why not BM / ES / EMR?** These have no *create-instance* pricing API — Bare Metal (`bm`) and Elasticsearch (`es`) can only price an existing instance by ID, and EMR requires a deeply-nested multi-node cluster spec that a Terraform plan doesn't map cleanly. Pricing them from a plan would be guesswork, so they are intentionally out of scope.

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
