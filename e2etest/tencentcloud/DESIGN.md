# cloudtab E2E 价格准确性测试设计文档

## 1. 目标

对 cloudtab 支持的 **19 种腾讯云资源类型**，逐一验证：

1. **API 可达性** — cloudtab 构造的 `PriceRequest` 能被腾讯云真实 API 接受并返回有效价格
2. **价格解析正确性** — cloudtab 的 `Parse` 函数能从真实 API 响应中正确提取价格
3. **月费计算正确性** — cloudtab 的小时→月（×730）、分→元（÷100）、折扣价选择等换算逻辑正确

每个产品独立测试，产出原始 API 请求/响应、cloudtab 计算结果、以及对比报告。

---

## 2. 目录结构

```
e2etest/tencentcloud/
├── DESIGN.md              ← 本文档
├── run.go                 ← 测试编排程序（Go，import cloudtab 内部包）
├── cvm/
│   ├── main.tf            ← Terraform 配置（按量 + 包月两种计费）
│   ├── plan.json          ← terraform show -json 生成
│   ├── raw_price.json     ← 原始 API 请求 + 响应（每资源一条）
│   ├── cloudtab_price.json← cloudtab Parse 后的成本组件
│   └── report.md          ← 人工可读的对比报告
├── mysql/
│   ├── main.tf
│   ├── plan.json
│   ├── raw_price.json
│   ├── cloudtab_price.json
│   └── report.md
├── cbs/
│   └── ...
├── eip/
│   └── ...                ← StaticMapper，无 API 调用，只验证 Estimate
├── clb/
│   └── ...
├── postgresql/
│   └── ...
├── redis/
│   └── ...
├── vpn_gateway/
│   └── ...
├── mongodb/
│   └── ...
├── mariadb/
│   └── ...
├── cynosdb/
│   └── ...
├── lighthouse/
│   └── ...
├── ecm/
│   └── ...
├── sqlserver/
│   └── ...
├── dcdb/
│   └── ...
├── gaap/
│   └── ...
├── cwp/
│   └── ...
├── cloudhsm/
│   └── ...
└── domain/
    └── ...
```

---

## 3. 文件格式规范

### 3.1 raw_price.json

每个资源一条记录，包含 cloudtab 发出的请求和腾讯云返回的原始响应：

```json
{
  "resource_address": "tencentcloud_instance.test_postpaid",
  "resource_type": "tencentcloud_instance",
  "timestamp": "2026-07-24T15:30:00+08:00",
  "request": {
    "product": "cvm",
    "action": "InquiryPriceRunInstances",
    "region": "ap-guangzhou",
    "params": {
      "Placement": {"Zone": "ap-guangzhou-3"},
      "ImageId": "img-d8216ykb",
      "InstanceType": "SA2.MEDIUM4",
      "InstanceChargeType": "POSTPAID_BY_HOUR",
      "InstanceCount": 1,
      "SystemDisk": {"DiskType": "CLOUD_PREMIUM", "DiskSize": 50}
    }
  },
  "response": {
    "raw": "{\"Response\":{\"Price\":{\"InstancePrice\":{\"UnitPrice\":0.04,\"UnitPriceDiscount\":0.03,\"ChargeUnit\":\"HOUR\"}}}}",
    "price_fields": {
      "UnitPrice": 0.04,
      "UnitPriceDiscount": 0.03,
      "ChargeUnit": "HOUR",
      "_unit": "元",
      "_path": "Response.Price.InstancePrice"
    }
  },
  "error": null
}
```

**说明**：
- `response.raw` — SDK `ToJsonString()` 的完整原始 JSON
- `response.price_fields` — 从 raw 中提取的关键价格字段（便于人工检查），`_unit` 标注单位（元/分），`_path` 标注 JSON 路径
- 多资源场景下，`raw_price.json` 是一个数组，每个元素对应一个资源

### 3.2 cloudtab_price.json

cloudtab `Parse` 函数输出的成本组件，与 `output.Report` 中的 `ResourceCost` 结构一致：

```json
{
  "address": "tencentcloud_instance.test_postpaid",
  "type": "tencentcloud_instance",
  "components": [
    {
      "name": "Compute (SA2.MEDIUM4)",
      "unit": "HOUR",
      "hourly_cost": 0.03,
      "monthly_cost": 21.90,
      "currency": "CNY"
    }
  ]
}
```

### 3.3 report.md

人工可读的对比报告，包含验证结论：

```markdown
# CVM 价格验证报告

> 生成时间: 2026-07-24 15:30:00
> 资源类型: tencentcloud_instance
> 定价 API: cvm:InquiryPriceRunInstances

## 测试资源

| 资源地址 | 计费模式 | 规格 | 可用区 |
|---|---|---|---|
| tencentcloud_instance.test_postpaid | POSTPAID_BY_HOUR | SA2.MEDIUM4 | ap-guangzhou-3 |
| tencentcloud_instance.test_prepaid | PREPAID (1月) | SA2.MEDIUM4 | ap-guangzhou-3 |

## API 原始返回

### test_postpaid (POSTPAID_BY_HOUR)

| 字段 | 值 | 说明 |
|---|---|---|
| UnitPrice | 0.04 元/h | 原价 |
| UnitPriceDiscount | 0.03 元/h | 折扣价（实际计费） |
| ChargeUnit | HOUR | 按小时计费 |

JSON 路径: `Response.Price.InstancePrice`

### test_prepaid (PREPAID)

| 字段 | 值 | 说明 |
|---|---|---|
| DiscountPrice | 25.59 元 | 1个月折扣总价 |

JSON 路径: `Response.Price.InstancePrice`

## cloudtab 计算结果

| 资源 | 组件 | 小时费 | 月费 | 币种 |
|---|---|---|---|---|
| test_postpaid | Compute (SA2.MEDIUM4) | 0.03 | 21.90 | CNY |
| test_prepaid | Compute (SA2.MEDIUM4) | 0 | 25.59 | CNY |

## 验证

### test_postpaid

| 检查项 | API 值 | cloudtab 值 | 计算 | 结果 |
|---|---|---|---|---|
| API 价格 > 0 前置检查 | 0.03 | — | UnitPriceDiscount=0.03 | ✅ PASS |
| 小时费 = UnitPriceDiscount | 0.03 | 0.03 | — | ✅ PASS |
| 月费 = 小时费 × 730 | 21.90 | 21.90 | 0.03×730=21.90 | ✅ PASS |

### test_prepaid

| 检查项 | API 值 | cloudtab 值 | 计算 | 结果 |
|---|---|---|---|---|
| API 价格 > 0 前置检查 | 25.59 | — | DiscountPrice=25.59 | ✅ PASS |
| 月费 = DiscountPrice | 25.59 | 25.59 | — | ✅ PASS |
| 小时费 = 0 (PREPAID 无小时费) | 0 | 0 | — | ✅ PASS |

## 结论

✅ 全部通过 — cloudtab 对 CVM 的价格查询和计算准确。
```

---

## 4. run.go 架构

### 4.1 职责

`run.go` 是一个独立的 Go 程序（`package main`），import cloudtab 的内部包，完成以下流程：

> **设计决策（分支 1）**：run.go 直接 import 内部包（`parser`、`resources`、`pricing`）而非调用 `cloudtab breakdown` CLI 二进制。理由：run.go 需要获取 `PriceRequest`（Extract 输出）和 `raw []byte`（Query 输出）等中间数据来生成 `raw_price.json`，这些数据 CLI 不暴露。经确认 `priceReport`/`priceResource` 函数仅做编排（registry lookup、StaticMapper 类型断言、错误处理、并发），不涉及价格计算逻辑，因此直接调用 Extract→Query→Parse 三步管线与 CLI 走的是完全相同的价格计算代码路径。

```
对每个产品目录:
  1. terraform init + plan + show -json → plan.json
     (如果 terraform plan 报 "Unsupported resource type" → 标注 "manual test required"，跳过)
  2. parser.LoadPlanJSON(plan.json) → []PlannedResource
  3. 对每个 PlannedResource:
     a. registry.Lookup(type) → Mapper
     b. 如果是 StaticMapper (类型断言) → mapper.Estimate(resource) → CostComponent
        (只产出 cloudtab_price.json，不产出 raw_price.json，无 API 调用)
     c. 否则:
        i.  mapper.Extract(resource) → PriceRequest          ← 保存到 raw_price.json.request
        ii. engine.Query(PriceRequest) → raw []byte           ← 保存到 raw_price.json.response
        iii.mapper.Parse(PriceRequest, raw) → []CostComponent ← 保存到 cloudtab_price.json
  4. 从 raw 响应中提取关键价格字段 → 填充 raw_price.json.response.price_fields
  5. 对比 price_fields vs cloudtab CostComponent → 生成 report.md
  6. 打印汇总表
```

### 4.2 核心数据结构

```go
// TestCase 定义一个产品的 E2E 测试配置
type TestCase struct {
    Name         string   // 目录名，如 "cvm"
    ResourceType string   // Terraform 类型，如 "tencentcloud_instance"
    SkipTerraform bool    // true=跳过 terraform plan，直接用已有 plan.json
}

// RawPriceRecord 保存一个资源的完整请求-响应链
type RawPriceRecord struct {
    ResourceAddress string                 `json:"resource_address"`
    ResourceType    string                 `json:"resource_type"`
    Timestamp       string                 `json:"timestamp"`
    Request         map[string]interface{} `json:"request"`     // PriceRequest 展平
    Response        RawResponse            `json:"response"`
    Error           string                 `json:"error,omitempty"`
}

type RawResponse struct {
    Raw        string                 `json:"raw"`          // SDK ToJsonString() 原始输出
    PriceFields map[string]interface{} `json:"price_fields"` // 提取的关键字段
}

// ProductValidator 从原始 API 响应中提取价格字段并定义验证规则
type ProductValidator interface {
    // ExtractPriceFields 从 raw JSON 响应中提取关键价格字段
    ExtractPriceFields(raw []byte) map[string]interface{}
    // Validate 对比 API 价格字段与 cloudtab 的 CostComponent
    Validate(fields map[string]interface{}, comps []output.CostComponent) []CheckResult
}

type CheckResult struct {
    Name     string  // 检查项名称
    APIValue float64 // API 返回的值
    GotValue float64 // cloudtab 计算的值
    Formula  string  // 计算公式（如 "0.03×730=21.90"）
    Status   string  // PASS | SUSPICIOUS | FAIL
    // PASS: abs(api-got) < 0.01 且 api > 0
    // SUSPICIOUS: api == 0（可能 Response 包装层解析 bug）
    // FAIL: abs(api-got) >= 0.01
}
```

### 4.3 产品验证器

每个产品需要一个 `ProductValidator` 来定义：
- 从哪个 JSON 路径提取价格字段
- 如何对比 API 值和 cloudtab 值

按 API 响应结构和价格字段差异，共有 **14 个独立验证器**。每个验证器在注释中标注它镜像的 Mapper Parse 函数，以便 Parse 变更时同步更新。

> **验证器-Mapper 耦合说明**：验证器本质上是在复制 Mapper 的 Parse 逻辑（提取哪个字段、如何换算）。这是一种有意识的耦合——只有逐字段对比才能抓到"读错字段"类的 bug（如读 `UnitPrice` 而非 `UnitPriceDiscount`）。代价是 Parse 变更时需同步更新验证器。

| # | 验证器 | 覆盖产品 | 镜像的 Parse 函数 | 响应路径 | 价格单位 | POSTPAID 小时费来源 | PREPAID 月费来源 |
|---|---|---|---|---|---|---|---|
| 1 | cvmClbValidator | CVM, CLB | `CVMInstance.Parse` / `CLBInstance.Parse` | `Response.Price.InstancePrice` | 元 | `UnitPriceDiscount` | `DiscountPrice` |
| 2 | cbsValidator | CBS | `CBSStorage.Parse` | `Response.DiskPrice` | 元 | `UnitPriceDiscount` | `DiscountPrice` |
| 3 | cdbFenValidator | MySQL, PostgreSQL, MariaDB, SQLServer, DCDB | 各自 `Parse`（共用 `parseTencentPrice`） | `Response.{Price, OriginalPrice}` | 分 | `Price/100` | `Price/100` |
| 4 | redisValidator | Redis | `RedisInstance.Parse` | `Response.{Price, HighPrecisionPrice, AmountUnit}` | 分/微分 | `normalizeTencentAmount` | 同左 |
| 5 | mongodbValidator | MongoDB | `MongoDBInstance.Parse` | `Response.Price` | 元 | `UnitPrice` | `DiscountPrice` |
| 6 | cynosdbValidator | CynosDB | `CynosDBCluster.Parse` | `Response.{InstancePrice, StoragePrice}` | 分(int64) | `UnitPriceDiscount/100`（双组件） | `TotalPriceDiscount/100` |
| 7 | lighthouseValidator | Lighthouse | `LighthouseInstance.Parse` | `Response.Price.InstancePrice` | 元 | 无（仅月费） | `DiscountPrice` |
| 8 | ecmValidator | ECM | `ECMInstance.Parse` | `Response.InstancePrice` | 分(uint64) | `DiscountPrice/100` | 无（仅 POSTPAID） |
| 9 | gaapValidator | GAAP | `GAAPProxy.Parse` | `Response.{ProxyDailyPrice, DiscountProxyDailyPrice}` | 元/天 | 无（天价→月） | `DiscountProxyDailyPrice × daysPerMonth` |
| 10 | vpnValidator | VPN | `VPNGateway.Parse` | `Response.Price.{InstancePrice, BandwidthPrice}` | 元 | `UnitPrice`（注意：非 UnitPriceDiscount） | `DiscountPrice` |
| 11 | staticValidator | EIP | `EIP.Estimate` | 无 API | — | — | — |
| 12 | cwpValidator | CWP | `YunjingLicense.Parse` | `Response.{OriginalPrice, DiscountPrice}` | 元 | 无（仅月费） | `preferDiscount(DiscountPrice, OriginalPrice)` |
| 13 | cloudhsmValidator | CloudHSM | `CloudHSMInstance.Parse` | `Response.{TotalCost, OriginalCost}` | 元 | 无（仅月费） | `TotalCost` (fallback `OriginalCost`) |
| 14 | domainValidator | Domain | `DomainRegistration.Parse` | `Response.PriceList[].{RealPrice, Price}` | 元/年 | 无（÷12→月） | `RealPrice/12` (fallback `Price/12`) |

**关键差异注意**：
- 验证器 1-2（CVM/CBS/CLB）用 `UnitPriceDiscount`（折扣价），验证器 5/10（MongoDB/VPN）用 `UnitPrice`（原价）——验证器必须与 Mapper 保持一致
- 验证器 6（CynosDB）有两个组件（实例+存储），需分别验证
- 验证器 9（GAAP）是天价，需 ×(730/24) 转月费
- 验证器 11（EIP）无 API 调用，只验证 Estimate 返回的组件结构

### 4.4 执行流程

```
main()
  ├── 解析参数（--products=cvm,mysql 或 --all）
  ├── 初始化 pricing.Engine（读取 TENCENTCLOUD_SECRET_ID/KEY，NoCache=true）
  ├── 遍历 TestCase 列表
  │   ├── 如果 plan.json 不存在 → 运行 terraform init/plan/show
  │   │   └── 如果 terraform plan 报 "Unsupported resource type"
  │   │       → 标注 "manual test required"，跳过该产品
  │   ├── parser.LoadPlanJSON(plan.json)
  │   ├── 对每个 resource:
  │   │   ├── registry.Lookup(type) → Mapper
  │   │   ├── StaticMapper 类型断言:
  │   │   │   ├── 是 StaticMapper → mapper.Estimate(resource) → CostComponents
  │   │   │   │   (只产出 cloudtab_price.json，不产出 raw_price.json，无 API 调用)
  │   │   │   └── 否 → mapper.Extract(resource) → PriceRequest
  │   │   │       ├── engine.Query(PriceRequest) → raw response
  │   │   │       ├── mapper.Parse(PriceRequest, raw) → CostComponents
  │   │   │       ├── 保存 raw_price.json
  │   │   │       └── 保存 cloudtab_price.json
  │   │   └── (API 错误 → 记录到 raw_price.json.error，标注 "API ERROR"，不影响其他产品)
  │   ├── 运行 ProductValidator → 生成 CheckResults（含 > 0 前置检查 + 浮点容差）
  │   └── 生成 report.md
  └── 打印汇总表（所有产品的 PASS/SUSPICIOUS/FAIL/API_ERROR/MANUAL）
```

---

## 5. 全部 19 种产品的测试配置

### 5.1 产品总表

| # | 目录 | Terraform Type | 定价 API | 价格单位 | Static | 本期实现 |
|---|---|---|---|---|---|---|
| 1 | `cvm` | `tencentcloud_instance` | cvm:InquiryPriceRunInstances | 元 | 否 | ✅ |
| 2 | `cbs` | `tencentcloud_cbs_storage` | cbs:InquiryPriceCreateDisks | 元 | 否 | ✅ |
| 3 | `eip` | `tencentcloud_eip` | 无（静态占位） | — | 是 | ✅ |
| 4 | `clb` | `tencentcloud_clb_instance` | clb:InquiryPriceCreateLoadBalancer | 元 | 否 | ✅ |
| 5 | `mysql` | `tencentcloud_mysql_instance` | cdb:DescribeDBPrice | 分 | 否 | ✅ |
| 6 | `postgresql` | `tencentcloud_postgresql_instance` | postgres:InquiryPriceCreateDBInstances | 分 | 否 | ✅ |
| 7 | `redis` | `tencentcloud_redis_instance` | redis:InquiryPriceCreateInstance | 分/微分 | 否 | ✅ |
| 8 | `vpn_gateway` | `tencentcloud_vpn_gateway` | vpc:InquiryPriceCreateVpnGateway | 元 | 否 | ✅ |
| 9 | `mongodb` | `tencentcloud_mongodb_instance` | mongodb:InquirePriceCreateDBInstances | 元 | 否 | ✅ |
| 10 | `mariadb` | `tencentcloud_mariadb_instance` | mariadb:DescribePrice | 分 | 否 | ✅ |
| 11 | `cynosdb` | `tencentcloud_cynosdb_cluster` | cynosdb:InquirePriceCreate | 分 | 否 | ✅ |
| 12 | `lighthouse` | `tencentcloud_lighthouse_instance` | lighthouse:InquirePriceCreateInstances | 元 | 否 | ✅ |
| 13 | `ecm` | `tencentcloud_ecm_instance` | ecm:DescribePriceRunInstance | 分 | 否 | ✅ |
| 14 | `sqlserver` | `tencentcloud_sqlserver_instance` | sqlserver:InquiryPriceCreateDBInstances | 分 | 否 | ✅ |
| 15 | `dcdb` | `tencentcloud_dcdb_instance` | dcdb:DescribeDCDBPrice | 分 | 否 | ✅ |
| 16 | `gaap` | `tencentcloud_gaap_proxy` | gaap:InquiryPriceCreateProxy | 元/天 | 否 | ✅ |
| 17 | `cwp` | `tencentcloud_cwp_license_order` | yunjing:InquiryPriceOpenProVersionPrepaid | 元 | 否 | ✅ |
| 18 | `cloudhsm` | `tencentcloud_cloudhsm_instance` | cloudhsm:InquiryPriceBuyVsm | 元 | 否 | ✅ |
| 19 | `domain` | `tencentcloud_domain_registration` | domain:DescribeDomainPriceList | 元 | 否 | ✅ |

> **TF Provider 支持说明（分支 4）**：上表"本期实现"列表示计划编写 TF 配置和验证器。实际是否能通过 `terraform plan` 生成 plan.json，取决于腾讯云 Terraform provider 是否支持该 resource type。run.go 在 `terraform plan` 阶段自动检测——报 "Unsupported resource type" 的产品自动标注 "manual test required" 并跳过，不影响其他产品。

### 5.2 各产品 TF 配置规格

每个产品的 `main.tf` 需要至少覆盖 **POSTPAID（按量计费）** 一种模式；有 PREPAID 的产品同时覆盖 **包年包月** 模式。

#### 1. cvm — CVM 实例

```hcl
# POSTPAID
resource "tencentcloud_instance" "postpaid" {
  instance_type        = "SA2.MEDIUM4"
  image_id             = <data source>
  availability_zone    = "ap-guangzhou-3"
  instance_charge_type = "POSTPAID_BY_HOUR"
  system_disk_type     = "CLOUD_PREMIUM"
  system_disk_size     = 50
}
# PREPAID (1月)
resource "tencentcloud_instance" "prepaid" {
  instance_type                        = "SA2.MEDIUM4"
  image_id                             = <data source>
  availability_zone                    = "ap-guangzhou-3"
  instance_charge_type                 = "PREPAID"
  instance_charge_type_prepaid_period  = 1
  system_disk_type                     = "CLOUD_PREMIUM"
  system_disk_size                     = 50
}
```

**Extract 必填字段**: `instance_type`, `image_id`, `availability_zone`
**验证**: POSTPAID hourly=UnitPriceDiscount, monthly=hourly×730; PREPAID monthly=DiscountPrice

#### 2. cbs — CBS 云硬盘

```hcl
resource "tencentcloud_cbs_storage" "postpaid" {
  storage_type      = "CLOUD_PREMIUM"
  storage_size      = 100
  availability_zone = "ap-guangzhou-3"
  charge_type       = "POSTPAID_BY_HOUR"
}
```

**Extract 必填字段**: `storage_type`, `storage_size`, `availability_zone`
**验证**: POSTPAID hourly=UnitPriceDiscount, monthly=hourly×730

#### 3. eip — 弹性公网 IP（StaticMapper）

```hcl
resource "tencentcloud_eip" "test" {
  internet_max_bandwidth_out = 10
  internet_charge_type       = "TRAFFIC_POSTPAID_BY_HOUR"
}
```

**验证**: Estimate 返回零成本占位组件，组件名和币种非空

#### 4. clb — CLB 负载均衡

```hcl
resource "tencentcloud_clb_instance" "postpaid" {
  network_type          = "OPEN"
  internet_charge_type  = "TRAFFIC_POSTPAID_BY_HOUR"
  internet_max_bandwidth_out = 10
}
```

**Extract 必填字段**: `network_type`（默认 OPEN）
**验证**: POSTPAID hourly=UnitPriceDiscount, monthly=hourly×730

#### 5. mysql — TencentDB MySQL

```hcl
resource "tencentcloud_mysql_instance" "postpaid" {
  availability_zone = "ap-guangzhou-3"
  mem_size          = 4000    # MB
  volume_size       = 200     # GB
  charge_type       = "POSTPAID"
  cpu               = 2
}
# PREPAID
resource "tencentcloud_mysql_instance" "prepaid" {
  availability_zone = "ap-guangzhou-3"
  mem_size          = 4000
  volume_size       = 200
  charge_type       = "PREPAID"
  prepaid_period    = 1
  cpu               = 2
}
```

**Extract 必填字段**: `availability_zone`, `mem_size`, `volume_size`
**验证**: POSTPAID hourly=Price/100（分→元）, monthly=hourly×730; PREPAID monthly=Price/100

#### 6. postgresql — TencentDB PostgreSQL

```hcl
resource "tencentcloud_postgresql_instance" "postpaid" {
  availability_zone    = "ap-guangzhou-3"
  spec_code            = "cdb.pg.z1.2g"
  storage              = 100
  instance_charge_type = "POSTPAID_BY_HOUR"
}
```

**Extract 必填字段**: `availability_zone`, `spec_code`, `storage`
**验证**: 同 MySQL（分→元，POSTPAID ×730）

#### 7. redis — TencentDB Redis

```hcl
resource "tencentcloud_redis_instance" "postpaid" {
  availability_zone  = "ap-guangzhou-3"
  type               = "Redis4.0"
  mem_size           = 1024  # MB
  charge_type        = "POSTPAID"
  redis_shard_num    = 1
  redis_replicas_num = 1
}
```

**Extract 必填字段**: `availability_zone`, `mem_size`（`type_id` 默认 6）
**验证**: POSTPAID hourly=normalizeTencentAmount(Price, AmountUnit), monthly=hourly×730

#### 8. vpn_gateway — VPN 网关

```hcl
resource "tencentcloud_vpn_gateway" "postpaid" {
  bandwidth   = 10
  charge_type = "POSTPAID_BY_HOUR"
}
# PREPAID
resource "tencentcloud_vpn_gateway" "prepaid" {
  bandwidth      = 20
  charge_type    = "PREPAID"
  prepaid_period = 1
}
```

**Extract 必填字段**: `bandwidth`
**验证**: POSTPAID hourly=UnitPrice（注意 VPN 用 UnitPrice 而非 UnitPriceDiscount）, monthly=hourly×730

#### 9. mongodb — TencentDB MongoDB

```hcl
resource "tencentcloud_mongodb_instance" "prepaid" {
  available_zone  = "ap-guangzhou-3"
  memory          = 4    # GB
  volume          = 100  # GB
  charge_type     = "PREPAID"
  prepaid_period  = 1
  node_num        = 3
}
# POSTPAID
resource "tencentcloud_mongodb_instance" "postpaid" {
  available_zone = "ap-guangzhou-3"
  memory         = 4
  volume         = 100
  charge_type    = "POSTPAID_BY_HOUR"
}
```

**Extract 必填字段**: `available_zone`, `memory`, `volume`
**验证**: POSTPAID hourly=UnitPrice, monthly=hourly×730; PREPAID monthly=DiscountPrice（元）

#### 10. mariadb — MariaDB

```hcl
resource "tencentcloud_mariadb_instance" "prepaid" {
  zones                = ["ap-guangzhou-3", "ap-guangzhou-4"]
  memory               = 8
  storage              = 200
  instance_charge_type = "PREPAID"
  period               = 1
}
# POSTPAID
resource "tencentcloud_mariadb_instance" "postpaid" {
  availability_zone = "ap-guangzhou-3"
  memory            = 8
  storage           = 200
  charge_type       = "POSTPAID"
}
```

**Extract 必填字段**: `memory`, `storage`（zones 或 availability_zone）
**验证**: POSTPAID hourly=Price/100, monthly=hourly×730; PREPAID monthly=Price/100

#### 11. cynosdb — TDSQL-C

```hcl
resource "tencentcloud_cynosdb_cluster" "prepaid" {
  available_zone = "ap-guangzhou-3"
  cpu            = 2
  memory         = 4
  storage_limit  = 100
  charge_type    = "PREPAID"
  prepaid_period = 1
  instance_count = 1
}
# POSTPAID
resource "tencentcloud_cynosdb_cluster" "postpaid" {
  available_zone = "ap-guangzhou-3"
  cpu            = 2
  memory         = 4
  charge_type    = "POSTPAID"
}
```

**Extract 必填字段**: `available_zone`, `cpu`, `memory`
**验证**: POSTPAID hourly=UnitPriceDiscount/100, monthly=hourly×730（实例+存储两个组件）; PREPAID monthly=TotalPriceDiscount/100

#### 12. lighthouse — 轻量应用服务器

```hcl
resource "tencentcloud_lighthouse_instance" "test" {
  bundle_id      = "bundle_gen_01"
  blueprint_id   = "lhbp-xxx"
  instance_count = 1
}
```

**Extract 必填字段**: `bundle_id`
**验证**: PREPAID monthly=DiscountPrice（元，直接月费）

#### 13. ecm — 边缘计算模块

```hcl
resource "tencentcloud_ecm_instance" "test" {
  instance_type    = "ec.small1.medium2"
  instance_count   = 1
  system_disk_size = 50
  system_disk_type = "CLOUD_PREMIUM"
}
```

**Extract 必填字段**: `instance_type`, `instance_count`, `system_disk_size`, `system_disk_type`
**验证**: POSTPAID hourly=DiscountPrice/100, monthly=hourly×730

#### 14. sqlserver — SQL Server

```hcl
resource "tencentcloud_sqlserver_instance" "prepaid" {
  availability_zone = "ap-guangzhou-3"
  memory            = 4
  storage           = 100
  charge_type       = "PREPAID"
  prepaid_period    = 1
  cpu               = 2
}
# POSTPAID
resource "tencentcloud_sqlserver_instance" "postpaid" {
  zone        = "ap-guangzhou-3"
  memory      = 4
  storage     = 100
  charge_type = "POSTPAID_BY_HOUR"
}
```

**Extract 必填字段**: `availability_zone`/`zone`, `memory`, `storage`
**验证**: 同 MariaDB（分→元）

#### 15. dcdb — TDSQL MySQL

```hcl
resource "tencentcloud_dcdb_instance" "prepaid" {
  zones                = ["ap-guangzhou-3", "ap-guangzhou-4"]
  shard_memory         = 8
  shard_storage        = 200
  shard_count          = 2
  shard_node_count     = 2
  instance_charge_type = "PREPAID"
  prepaid_period       = 1
}
# POSTPAID
resource "tencentcloud_dcdb_instance" "postpaid" {
  availability_zone = "ap-guangzhou-3"
  shard_memory      = 8
  shard_storage     = 200
  shard_count       = 2
  charge_type       = "POSTPAID"
}
```

**Extract 必填字段**: `shard_memory`, `shard_storage`, `shard_count`（zones 或 availability_zone）
**验证**: 同 MariaDB/SQLServer（分→元）

#### 16. gaap — 全球应用加速

```hcl
resource "tencentcloud_gaap_proxy" "test" {
  access_region     = "Guangzhou"
  realserver_region = "Beijing"
  bandwidth         = 10
  concurrent        = 2
}
```

**Extract 必填字段**: `access_region`, `realserver_region`, `bandwidth`, `concurrent`
**验证**: monthly=DiscountProxyDailyPrice × daysPerMonth（元/天 → 元/月）

#### 17. cwp — 主机安全（云镜）

```hcl
# Extract 只读 license_num（可选，默认 1），无必填字段。
# 资源只需存在为 tencentcloud_cwp_license_order 类型即可。
resource "tencentcloud_cwp_license_order" "test" {
  license_num = 1
}
```

**Extract 必填字段**: 无（`license_num` 可选，默认 1）
**验证**: monthly=preferDiscount(DiscountPrice, OriginalPrice)（元/月，PREPAID）

#### 18. cloudhsm — 云加密机

```hcl
# Extract 只读 goods_num（可选，默认 1），无必填字段。
# 资源只需存在为 tencentcloud_cloudhsm_instance 类型即可。
resource "tencentcloud_cloudhsm_instance" "test" {
  goods_num = 1
}
```

**Extract 必填字段**: 无（`goods_num` 可选，默认 1）
**验证**: monthly=preferDiscount(TotalCost, OriginalCost)（元/月，PREPAID）

#### 19. domain — 域名注册

```hcl
resource "tencentcloud_domain_registration" "test" {
  domain_name = "test.com"
  period      = 1
}
```

**Extract 必填字段**: `domain_name`
**验证**: monthly=RealPrice/12（元/年 → 元/月，除以 domainPriceUnitDivisor=1）

---

## 6. 验证规则汇总

### 6.1 通用规则

| 规则 | 公式 | 适用场景 |
|---|---|---|
| **API 价格 > 0** | `API_price > 0`，否则标记 SUSPICIOUS | 所有产品的前置检查 |
| **浮点容差** | `abs(api - cloudtab) < 0.01`（绝对误差 < 1 分钱） | 所有数值比较 |
| 分→元 | `priceYuan = priceFen / 100` | MySQL/PostgreSQL/MariaDB/SQLServer/DCDB/CynosDB/ECM |
| 小时→月 | `monthly = hourly × 730` | 所有 POSTPAID 产品 |
| 天→月 | `monthly = daily × (730/24)` | GAAP |
| 折扣优先 | `effective = DiscountPrice > 0 ? DiscountPrice : OriginalPrice` | PREPAID 产品 |
| PREPAID 无小时费 | `hourly = 0` | 所有 PREPAID 产品 |

> **> 0 前置检查说明**：如果 API 返回的价格字段为 0（如 `UnitPriceDiscount=0`、`DiscountPrice=0`），说明 API 可能未正确返回价格（如 Response 包装层解析 bug 再现）。此时 `0 == 0` 会产生假阳性，因此必须在数值比较前检查 API 价格 > 0。若 API 价格为 0，该检查项标记为 `SUSPICIOUS` 而非 `PASS`。

> **浮点容差说明**：float64 运算可能产生微小误差（如 `0.03 × 730 = 21.900000000000002`），所有数值比较使用 `abs(api - cloudtab) < 0.01` 容差。0.01 元（1 分钱）足以覆盖 float64 精度误差，同时不会掩盖真正的计算错误。

### 6.2 按验证器的验证规则

> 以下所有 `==` 比较均使用 `abs(api - cloudtab) < 0.01` 容差。所有 API 价格字段需先通过 > 0 前置检查。

**验证器 1（CVM, CLB）** — `Response.Price.InstancePrice`:
```
前置: UnitPriceDiscount > 0 (POSTPAID) 或 DiscountPrice > 0 (PREPAID)
POSTPAID (ChargeUnit=HOUR):
  abs(hourly_cloudtab - UnitPriceDiscount) < 0.01
  abs(monthly_cloudtab - hourly × 730) < 0.01
PREPAID (ChargeUnit=MONTH):
  abs(monthly_cloudtab - DiscountPrice) < 0.01
  hourly_cloudtab == 0
```

**验证器 2（CBS）** — `Response.DiskPrice`:
```
同验证器 1，只是路径从 Price.InstancePrice 改为 DiskPrice
```

**验证器 3（MySQL, PostgreSQL, MariaDB, SQLServer, DCDB）** — `Response.{Price, OriginalPrice}` (分):
```
前置: Price > 0
POSTPAID:
  abs(hourly_cloudtab - Price / 100) < 0.01
  abs(monthly_cloudtab - hourly × 730) < 0.01
PREPAID:
  abs(monthly_cloudtab - Price / 100) < 0.01
  hourly_cloudtab == 0
```

**验证器 4（Redis）** — `Response.{Price, HighPrecisionPrice, AmountUnit}`:
```
前置: normalizeTencentAmount(Price|HighPrecisionPrice, AmountUnit) > 0
priceYuan = normalizeTencentAmount(Price|HighPrecisionPrice, AmountUnit)
POSTPAID (BillingMode=0):
  abs(hourly_cloudtab - priceYuan) < 0.01
  abs(monthly_cloudtab - priceYuan × 730) < 0.01
PREPAID (BillingMode=1):
  abs(monthly_cloudtab - priceYuan) < 0.01
```

**验证器 5（MongoDB）** — `Response.Price` (元):
```
前置: UnitPrice > 0 (POSTPAID) 或 DiscountPrice > 0 (PREPAID)
POSTPAID: abs(hourly_cloudtab - UnitPrice) < 0.01, abs(monthly - hourly×730) < 0.01
PREPAID:  abs(monthly_cloudtab - DiscountPrice) < 0.01, hourly == 0
```

**验证器 6（CynosDB）** — `Response.{InstancePrice, StoragePrice}` (分, int64, 双组件):
```
前置: InstancePrice.UnitPriceDiscount > 0 (POSTPAID) 或 InstancePrice.TotalPriceDiscount > 0 (PREPAID)
POSTPAID: 实例组件 hourly=UnitPriceDiscount/100, monthly=hourly×730
          存储组件同理（StoragePrice 路径）
PREPAID:  实例组件 monthly=TotalPriceDiscount/100, 存储组件同理
```

**验证器 7（Lighthouse）** — `Response.Price.InstancePrice` (元, 仅月费):
```
前置: DiscountPrice > 0
abs(monthly_cloudtab - DiscountPrice) < 0.01, hourly == 0
```

**验证器 8（ECM）** — `Response.InstancePrice` (分, uint64, 仅 POSTPAID):
```
前置: DiscountPrice > 0
abs(hourly_cloudtab - DiscountPrice/100) < 0.01, abs(monthly - hourly×730) < 0.01
```

**验证器 9（GAAP）** — `Response.{ProxyDailyPrice, DiscountProxyDailyPrice}` (元/天):
```
前置: DiscountProxyDailyPrice > 0（或 ProxyDailyPrice > 0 作为 fallback）
abs(monthly_cloudtab - DiscountProxyDailyPrice × daysPerMonth) < 0.01, hourly == 0
```

**验证器 10（VPN）** — `Response.Price.{InstancePrice, BandwidthPrice}` (元):
```
前置: InstancePrice.UnitPrice > 0 (POSTPAID) 或 DiscountPrice > 0 (PREPAID)
POSTPAID: abs(hourly_cloudtab - UnitPrice) < 0.01, abs(monthly - hourly×730) < 0.01
          （注意：VPN 用 UnitPrice 而非 UnitPriceDiscount）
PREPAID:  abs(monthly_cloudtab - DiscountPrice) < 0.01, hourly == 0
```

**验证器 11（EIP, StaticMapper）**:
```
无 API 调用，验证 Estimate 返回:
  - 至少 1 个 CostComponent
  - MonthlyCost == 0（占位）
  - Currency 非空
```

**验证器 12（CWP, YunjingLicense）** — `Response.{OriginalPrice, DiscountPrice}` (元):
```
前置: DiscountPrice > 0 或 OriginalPrice > 0
monthly = preferDiscount(DiscountPrice, OriginalPrice)  # 元，不除以100
abs(monthly_cloudtab - apiMonthly) < 0.01
hourly == 0 (always PREPAID)
```

**验证器 13（CloudHSM）** — `Response.{TotalCost, OriginalCost}` (元):
```
前置: TotalCost > 0 或 OriginalCost > 0
monthly = TotalCost > 0 ? TotalCost : OriginalCost  # 元，不除以100
abs(monthly_cloudtab - apiMonthly) < 0.01
hourly == 0 (always PREPAID)
注意: 字段名是 TotalCost/OriginalCost，不是 Price/OriginalPrice
```

**验证器 14（Domain）** — `Response.PriceList[].{RealPrice, Price}` (元/年):
```
前置: RealPrice > 0 或 Price > 0
yearly = RealPrice > 0 ? RealPrice : Price  # 元/年（domainPriceUnitDivisor=1）
monthly = yearly / 12
abs(monthly_cloudtab - apiMonthly) < 0.01
hourly == 0
```

---

## 7. 执行流程

### 7.1 前置条件

```bash
# 1. 构建二进制
cd /Users/kakazhou/workspace/code/terraform/cloudtab
GOSUMDB=sum.golang.org go build -o cloudtab ./cmd/cloudtab

# 2. 设置凭证
export TENCENTCLOUD_SECRET_ID=AKIDxxxx
export TENCENTCLOUD_SECRET_KEY=xxxxxxxx

# 3. 安装 Terraform（已有 v1.13.5）
```

### 7.2 运行方式

```bash
# 运行全部产品
cd /Users/kakazhou/workspace/code/terraform/cloudtab/e2etest/tencentcloud
GOSUMDB=sum.golang.org go run run.go --all

# 只运行指定产品
GOSUMDB=sum.golang.org go run run.go --products=cvm,mysql

# 跳过 terraform plan（复用已有 plan.json）
GOSUMDB=sum.golang.org go run run.go --products=cvm --skip-terraform
```

### 7.3 输出汇总

`run.go` 在终端打印汇总表：

```
PRODUCT       RESOURCES   API CALLS   PASS   SUSP   FAIL   STATUS
cvm           2           2           4      0      0      ✅ ALL PASS
mysql         2           2           4      0      0      ✅ ALL PASS
cbs           1           1           2      0      0      ✅ ALL PASS
eip           1           0           1      0      0      ✅ ALL PASS (static)
clb           1           1           2      0      0      ✅ ALL PASS
cwp           -           -           -      -      -      ⏭ MANUAL TEST REQUIRED
...
──────────────────────────────────────────────────────────────────────
TOTAL         28          26          52     0      0      ✅ ALL PASS
```

状态含义：
- **PASS** — cloudtab 计算值与 API 值一致（浮点容差 < 0.01）
- **SUSPICIOUS** — API 返回价格为 0（可能 Response 包装层解析 bug），需人工检查
- **FAIL** — cloudtab 计算值与 API 值偏差 >= 0.01
- **MANUAL** — TF provider 不支持该 resource type，需手写 plan.json 手动测试
- **API_ERROR** — API 调用失败（如无效的 bundle_id），需修正 TF 配置

### 7.4 失败处理

当某个检查项 FAIL 或 SUSPICIOUS 时：
- `report.md` 中标记 ❌ FAIL 或 ⚠️ SUSPICIOUS 并显示差异
- 终端汇总表显示 FAIL/SUSPICIOUS 计数
- FAIL → 退出码为 1（可用于 CI）
- SUSPICIOUS → 退出码为 0（警告但不阻断）

---

## 8. 注意事项

### 8.1 Terraform 配置的挑战

部分产品的 Terraform provider 字段与 cloudtab Extract 读取的字段名可能不一致（如 `available_zone` vs `availability_zone`）。`run.go` 应从 `plan.json` 的 `resource_changes[].change.after` 中读取字段，而不是依赖 Terraform provider 的 schema 文档。

对于字段不确定的产品，先运行 `terraform plan` 生成 plan.json，检查 `after` 中的实际字段名，再与 Mapper 的 Extract 函数对照。

### 8.2 TF Provider 支持检测

并非所有资源类型都在腾讯云 Terraform provider 中存在。`run.go` 在 `terraform plan` 阶段自动检测：
- 如果 `terraform plan` 报错 "Unsupported resource type" → 该产品标注 "manual test required"，跳过测试
- 如果 `terraform plan` 成功 → 继续执行 Extract→Query→Parse 流程
- 如果 API 调用返回错误（如无效的 bundle_id） → 记录到 `raw_price.json` 的 `error` 字段，标注 "API ERROR"

不支持 TF plan 的产品仍可通过手写 `plan.json` 进行测试（使用 `--skip-terraform` 参数）。

### 8.3 缓存干扰

run.go 必须用 `NoCache: true` 初始化 pricing.Engine。否则可能从上次运行的 BoltDB 缓存（`~/.cloudtab/cache.db`）中返回旧价格，而非真实 API 响应。缓存会掩盖 API 价格变更，导致 E2E 测试失去意义。

### 8.4 已知的 Response 包装层问题

本次测试已修复了 CVM/CBS/CLB 三个 Mapper 不读取 `Response` 包装层的 bug。其他 Mapper（MongoDB, CynosDB, Lighthouse, ECM, GAAP, VPN, Redis, Yunjing, CloudHSM, Domain）已经正确处理了 `Response` 路径。E2E 测试将验证所有 Mapper 在真实 API 下都能正确读取价格。

### 8.5 实现优先级

1. **第一批（已验证）**: cvm — 已有可工作的 TF 配置和 plan.json
2. **第二批**: mysql, cbs, clb — 常用产品，API 结构已知
3. **第三批**: postgresql, redis, mongodb, mariadb — 数据库类产品
4. **第四批**: cynosdb, lighthouse, ecm, sqlserver, dcdb — 较少使用的产品
5. **第五批**: gaap, cwp, cloudhsm, domain, eip — 特殊产品
