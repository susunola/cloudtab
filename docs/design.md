# cloudtab 技术设计

## 一、Infracost 是怎么工作的（拆解）

看完 infracost/infracost 源码（Go，MIT 许可），核心链路：

```
terraform-config-dir ─┐
                      ├──▶ [HCL parser + evaluator] ──▶ [resource list w/ attrs]
plan.json             ┘                                            │
                                                                   ▼
                                                          [per-resource "provider"]
                                                          e.g. aws_instance.go
                                                                   │
                                             ┌─────────────────────┴─────────────────────┐
                                             ▼                                           ▼
                                    build GraphQL query                            usage.yml
                                    Products(filter:{...}) {                      (monthly_data_transfer,
                                       prices { USD }                              request_count, ...)
                                    }
                                             │
                                             ▼
                                    Infracost Pricing API
                                    (GraphQL, self-hosted price DB
                                    synced from AWS Bulk / Azure Retail / GCP)
                                             │
                                             ▼
                                    [CostComponent list]
                                             │
                                             ▼
                                    [table / html / json / diff]
```

### 关键点

**1. 定价库自建 vs 直接调云厂商 API**

Infracost 选自建（GraphQL server + PostgreSQL），原因：
- AWS/Azure/GCP 的官方价格接口是"bulk"（一次几百 MB JSON），不适合线上直接查
- 有些价格云厂商不给现成 API，需要爬产品页
- 想做统一 filter DSL

**代价**：需要长期人力维护（Infracost 有 ~20 人团队，一半在做定价数据）。

**2. Resource → Cost 映射靠"provider go 文件"**

每个 tf resource 一个 go 文件，硬编码"这个资源有哪些 CostComponent、每个组件怎么查价、usage.yml 里对应哪些字段"。全靠人肉写 + 测试，扩展性一般但胜在稳定。

例：`internal/providers/terraform/aws/instance.go` 大约 300 行，处理 EC2 各种边界（spot / reserved / EBS / EIP 附带 / detailed monitoring）。

**3. Diff = 两次 breakdown 相减**

不是真的调 diff API，而是本地存 `costEstimate` 对象，前后对比。

## 二、cloudtab 的差异化设计

### 决策 1：不自建定价库，直接调询价 API

**理由**：
- 腾讯云大部分产品都有询价接口（已验证并接入 CVM/CBS/CDB/CLB/Redis/PostgreSQL/VPN 网关/MongoDB/MariaDB/TDSQL-C/Lighthouse/ECM/SQL Server/TDSQL MySQL(dcdb)/GAAP/主机安全(yunjing)/云加密机(cloudhsm)/域名注册(domain)，共 18 个引擎询价产品 + EIP 静态估价；注意命名不统一：`InquiryPriceXxx`、`InquirePriceXxx`、`DescribePrice`、`DescribeDCDBPrice`、`DescribeDomainPriceList`）
- **未接入 BM/ES/EMR 的理由**：BM（黑石物理机）与 ES（Elasticsearch）没有"创建实例询价"接口，只能按已有实例 ID 询价，对 plan 估算无意义；EMR 需要深层嵌套的多节点 ResourceSpec（Master/Core/Task），Terraform plan 无法干净映射，属高风险。三者暂不纳入。
- 返回值包含 `UnitPrice` / `UnitPriceDiscount` / `OriginalPrice` / `DiscountPrice`，官方一致
- 少了"同步价格库"这一层，MVP 交付时间从 6 个月 → 2 周

**代价**：
- 每次都要调 API → 加**本地 BoltDB 缓存 24h TTL**，同一 plan 里同型号资源只查一次
- 用户必须提供腾讯云 AK/SK（Infracost 不需要用户凭据）→ 但只需要 `InquiryPrice*` 读权限，风险低
- 部分资源没询价 API（COS 对象存储、CDN 流量、SCF 计费）→ 走**静态价格表**（价格从官网抓取，YAML 维护，季度更新一次）

### 决策 2：先支持 plan.json，不支持 HCL 直读

**理由**：Infracost 早期版本也是 plan.json 优先，HCL 直读的 evaluator（`hcl_parser`）是巨坑：`for_each` 里的 `data source` 无法求值、module 递归解析、provider alias、workspace 变量…

**MVP 只吃 plan.json 就够用**：99% 的 CI/CD 场景都能跑 `terraform plan -out plan.tfplan && terraform show -json plan.tfplan`。

### 决策 3：Mapper 抽象

每个 tf 资源类型一个 `Mapper`，仅两个方法：

```go
type Mapper interface {
    Extract(r PlannedResource) (PriceRequest, error)          // plan attrs → InquiryPrice params
    Parse(req PriceRequest, raw []byte) ([]CostComponent, error) // API response → cost breakdown
}
```

好处：
- 加新资源类型 = 加一个文件，不改核心
- 单测容易：录一次 API 响应，之后离线跑
- 用户可以外部扩展（未来加插件机制）

### 决策 4：Usage 假设通过 YAML

完全照抄 Infracost 的 usage.yml，兼容心智：

```yaml
tencentcloud_cos_bucket.static_site:
  monthly_storage_gb: 200
  monthly_get_requests: 1000000
  monthly_put_requests: 10000
  monthly_data_transfer_out_gb: 500
```

## 三、MVP 里程碑与工程量估算

| 里程碑 | 内容 | 工程量 |
|---|---|---|
| **M1**（本仓库当前状态） | CVM 询价端到端跑通，table 输出 | ~1 周 |
| **M2** | 加 CBS/EIP/CLB/MySQL/Redis，BoltDB 缓存 | ~2 周 |
| **M3** | usage.yml，COS/CDN/SCF 静态价格表，diff 命令 | ~2 周 |
| **M4** | JSON 输出 + GitHub Action + PR 评论 | ~1 周 |
| **M5** | 官方 Terraform Registry 前 20 大资源全覆盖，`init/plan/HCL` fallback | ~1 月 |
| **M6**（可选） | VSCode 插件 code lens，AI Skill（对接 CodeBuddy/Cursor） | ~1 月 |

M1-M4 大约 6 周就有可用产品，比 Infracost 早期路径快得多，因为不用建价格库。

## 四、竞品与生态

- **国内目前无对标产品**（Infracost 明确 not supporting Tencent Cloud）
- 腾讯云自己的[费用中心账单](https://console.cloud.tencent.com/expense/bill)是**事后**的，PR 阶段无
- [tencentcloudstack/terraform-provider-tencentcloud](https://github.com/tencentcloudstack/terraform-provider-tencentcloud) 是官方 provider，成熟度高，可以稳定依赖
- 出海用户：腾讯云海外（intl）走同样 API，只是 endpoint 不同（intl.tencentcloudapi.com）。**已实现**（见 §六「国内/国际站切换」）

## 五、开源策略建议

1. **License**：Apache 2.0（Infracost 是 Apache 2.0，友好，允许被商业化包装）
2. **命名**：`cloudtab` / `tccost-tf` / `qcloud-cost`（`tencentcloud_` 是 provider 名，不要占 tf 命名空间）
3. **首个 release 前**：写 3-5 个真实 fixture plan（web app / k8s / 数据平台），保证输出稳定
4. **社区冷启动**：发到腾讯云开发者社区、SegmentFault、GitHub Trending，`terraform` `finops` 标签
5. **和官方合作**：可以直接联系腾讯云 Terraform Provider 团队（CNB/GitHub 都能找到），获得推荐可能性很大

## 六、需要提前规避的坑

1. **询价 API 有 QPS 限制**（10/s）→ 缓存必须做好，同一 breakdown 里去重
2. **PREPAID vs POSTPAID_BY_HOUR 的价格返回结构不同**（PREPAID 直接给 `DiscountPrice` 月费，POSTPAID 给 `UnitPriceDiscount` 时费）→ Mapper 里判断分支
3. **可用区细化**（ap-shanghai-2 vs ap-shanghai-5，S5.MEDIUM4 在不同 zone 价格一致但需要真实 zone 传入）→ 用户没写 zone 时给个默认或报错
4. **镜像 ID 是必填项**但询价场景其实和镜像无关 → 用一个公共镜像常量占位（`img-pmqg1cw7`）
5. **跨区域**（跨 region 的 plan）→ Mapper 从 `r.Region` 取 region，不是全局默认

## 七、国内 / 国际站切换（已实现）

腾讯云有**两套完全独立的站点**，账号体系不互通：

| | 国内站 | 国际站 |
|---|---|---|
| API host | `<product>.tencentcloudapi.com` | `<product>.intl.tencentcloudapi.com` |
| 账号 / AK-SK | 独立 | 独立 |

**关键认知：站点由 AK/SK 决定，不是 region。** 两个站点的 region 名字大量重叠（都有
`ap-guangzhou`/`ap-singapore` 等），所以**不能靠 region 推断站点**——同一个 AK/SK
只在它注册的那个站有效，用错站直接鉴权失败。

**实现要点**：

- `pricing.Config.Site` 选择站点：`""`/`domestic`/`cn`/`china` → 国内站；
  `intl`/`international`/`global`/`overseas` → 国际站；其他非空值当作**字面 root
  domain 覆盖**（专有云/代理网关场景）。
- **用 SDK 原生的 `HttpProfile.RootDomain`**（`GetServiceDomain` 会拼成
  `<product>.<rootDomain>`），**不再硬写 `HttpProfile.Endpoint`**——`Endpoint` 是完整
  host、优先级高于 `RootDomain`，一旦写死就会把所有产品钉死在国内站，国际站 AK/SK 失效。
  这正是本次修复的核心 bug（原 `engine.go` 写死了 `product + ".tencentcloudapi.com"`）。
- 国内站 `RootDomain=""` 走 SDK 默认（= `tencentcloudapi.com`），**默认行为与历史完全一致**（向后兼容）。
- **缓存按 site 隔离**：cache key 用规范化 root domain 做前缀命名空间，避免国内价被当成国际价（或反之）返回——两站价格/币种可能不同。
- CLI：`--site` flag（优先）> `TENCENTCLOUD_SITE` 环境变量 > 默认国内站。

## 八、多云支持 / AWS 接入（已实现）

cloudtab 从「只支持腾讯云」演进为「多云询价框架」，首个新增云厂商是 **AWS**。核心原则：**抽象出 provider 后端，AWS 与腾讯云各是一个实现；腾讯云既有行为 100% 不变，依赖零漂移。**

### 8.1 Provider 抽象

- `internal/pricing` 里定义了一个最小的 `backend` 接口：`query(PriceRequest) ([]byte, error)`。
- **腾讯云不走 backend**——它的逻辑仍直接挂在 `*Engine` 上（handlers registry + invoke + SDK client 缓存），保持既有结构与向后兼容。
- **AWS 是一个被委托的 backend**（`aws_backend.go` 的 `awsBackend`）。
- `PriceRequest` 增加 `Provider` 字段，`provider()` 方法在为空时**默认返回 `tencentcloud`**——所有历史调用（不带 provider）行为完全不变。
- `Engine.dispatch(req)` 按 `req.provider()` 路由：
  - `tencentcloud`（或空）→ 走 handlers / invoke（原路径）；
  - `aws` → 走 `awsBackend().query(req)`；
  - 其他 → 明确报错（不静默）。
- **懒加载 AWS 后端**：`Engine` 用 `sync.Once` 在**首次 AWS 请求**时才解析 AWS SDK / 凭证。纯腾讯云的 plan 不需要任何 AWS 凭证，也不会初始化 AWS SDK。

### 8.2 AWS Price List API 模型

- AWS 只有**一个** `GetProducts` 操作，靠 `ServiceCode`（如 `AmazonEC2`）+ 一组 `TERM_MATCH` 过滤器参数化。
- **询价 endpoint 恒为 `us-east-1`**；被询价的区域用 `location` 过滤器表达，且值是**人类可读区域名**（如 `US East (N. Virginia)`），不是 region code。`aws_common.go` 的 `awsRegionToLocation` 维护 region→location 映射（~21 区），未命中回落默认区。
- 返回 `PriceList []string`，每个元素是一份完整的产品 JSON 文档：
  `{"product":{"attributes":{...}}, "terms":{"OnDemand":{"<term>":{"priceDimensions":{"<rate>":{"unit":"Hrs","pricePerUnit":{"USD":"..."}}}}}}}`。
- `aws_backend.go` 负责分页（尊重 `MaxResults`，默认/上限 100）并把所有页拼成 JSON 数组返回；`aws_common.go` 的 `parseAWSPriceList` / `firstOnDemandUSD` 负责取第一个非零 OnDemand 单价。

### 8.3 已接入的 AWS 产品（6 个 mapper）

| Terraform 资源 | ServiceCode | 关键过滤器 | 计费口径 |
|---|---|---|---|
| `aws_instance` | AmazonEC2 | instanceType / location / tenancy / OS=Linux / preInstalledSw=NA / capacitystatus=Used | 时费 × 730 |
| `aws_ebs_volume` | AmazonEC2 | volumeApiName（gp2/gp3/io1…）/ location / productFamily=Storage | GB-月单价 × size |
| `aws_db_instance` | AmazonRDS | instanceType(db.*) / databaseEngine / deploymentOption(Single/Multi-AZ) / location | 时费 × 730 |
| `aws_elasticache_cluster` | AmazonElastiCache | instanceType(cache.*) / cacheEngine(Redis/Memcached) / location | 时费 × 节点数 × 730 |
| `aws_lb` | AWSELB | productFamily(Application/Network/Gateway LB) / location / usagetype 含 `LoadBalancerUsage` | 固定时费 × 730 |
| `aws_elb` | AWSELB | productFamily=Load Balancer（Classic）/ location / usagetype 含 `LoadBalancerUsage` | 固定时费 × 730 |

- **引擎名翻译**：Terraform 里是小写 id（`mysql`/`postgres`/`aurora-mysql`…），Price List 里是显示名（`MySQL`/`PostgreSQL`/`Aurora MySQL`…）。`awsRDSEngine` / `awsCacheEngine` 显式映射，**不支持的引擎直接报错**，绝不用错 SKU 静默询价。
- **ELB 多 SKU 消歧**：AWSELB 家族一个产品会返回多个 SKU（`LoadBalancerUsage` 固定时费 + `LCUUsage` + `DataProcessing-Bytes`）。新增 `parseAWSPriceListMatching(raw, "LoadBalancerUsage")` 只锁定固定时费那条；**LCU / 数据处理是用量驱动，明确排除并在组件名里标注**（`... (base, excl. LCU/data)`）。

### 8.4 刻意不接入的 AWS 产品（按设计排除）

- **`aws_s3_bucket`（S3）与 `aws_eip`（EIP）不注册。** 它们的成本**纯用量驱动**：S3 取决于实际存储 GB / 请求数 / 出网流量；EIP 取决于是否绑定、绑定时长。**一份 Terraform plan 里根本没有这些运行时数据**，任何月度数字都是编造。因此选择「documented exclusion」而非塞一个假的 mapper（`registry.go` 有注释说明，README 也有 "Not priced from a plan (by design)" 说明）。

### 8.5 币种处理（USD vs CNY）

- 腾讯云返回 **CNY**，AWS 返回 **USD**。输出表按组件带 **Currency 列**。
- **TOTAL 只在币种统一时求和**；混合 provider（同一 plan 里既有腾讯云又有 AWS）时不做跨币种汇总，避免把 ¥ 和 $ 直接相加的错误。

### 8.6 依赖隔离（零漂移铁律的延续）

- 新增 `aws-sdk-go-v2 v1.43.0` / `config` / `service/pricing v1.44.0` / `credentials`（+ smithy-go 等 transitive），全部**只增不改**。
- `go mod tidy` 后：4 个直接用到的 AWS 包从 `// indirect` 提升为直接 require（additive），**19 个腾讯云依赖全部仍在 v1.0.1000，零漂移**（`grep tencentcloud go.mod | grep -v v1.0.1000` 为空验证）。
- CLI：`--aws-access-key-id` / `--aws-secret-access-key` / `--aws-session-token`（或对应环境变量）；纯腾讯云 plan 无需提供。
