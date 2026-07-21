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
- 腾讯云每个产品都有 `InquiryPriceXxx`（我们已经验证 CVM/CBS/CDB/CLB/Redis 都有）
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
- 出海用户：腾讯云海外（intl）走同样 API，只是 endpoint 不同（intl.tencentcloudapi.com），代码里加一个 env switch 即可

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
