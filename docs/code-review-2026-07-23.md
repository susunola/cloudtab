# cloudtab 代码审查 — 2026-07-23

> **修复状态（2026-07-23 已应用）**：全部 10 项均已处理。
> - #1 华为 `usage_factor` 统一改为 `"Duration"`，`project_id` 改由 backend 注入（`Config.HuaweiProjectID` / `HUAWEI_PROJECT_ID`），不再错配成 region。
> - #3 腾讯凭证改为按需校验（`dispatch` 到 tencent 时才校验），纯多云 plan 不再强制要求腾讯密钥。
> - #4 `--timeout` 已透传阿里（`SetReadTimeout`/`SetConnectTimeout`）与华为（`WithHttpConfig`）。
> - #5 默认宽松跳过失败资源（`--fail-on-error` 恢复硬失败）。
> - #6 缓存 TTL 可配（`--cache-ttl`，默认 24h）。
> - #7 删除阿里 `Quantity` 死字段。
> - #8 抽出 `huaweiProductInfo` / `alibabaModule` 公共构造器，消除手写字符串 key。
> - #9 root `Short` 改为多云、阿里注释 `DescribePrice`→`GetPayAsYouGoPrice`、usage FAQ 同步更新。
> - #10 新增请求体快照测试（mock client 捕获 SDK 入参），锁定 #1/#2，覆盖全部 9 个华为 + 9 个阿里 mapper。
>
> ⚠️ **待真实 API 验证（非代码缺陷，需联调）**：`usage_factor` 各资源取值、`size_measure_id=17`（GB，EVS 线性计费）、以及 EIP/ELB 按流量计费应选 `"upflow"` 的场景。代码已按 SDK 注释实现，但需一次真实调用确认。

覆盖维度：安全性 / 健壮性 / 高可用容错 / 可读性 / 扩展性 / 性能 / 测试。
严重度：🔴 高（隐藏 bug / 破坏多云定位）· 🟠 中（需改进）· 🟡 低（卫生问题）。

---

## 🔴 健壮性 — 隐藏正确性 Bug

### 1. 华为 mapper `usage_factor` 取值错误（9 个 mapper 全部）
`internal/resources/huawei_*.go` 除 EVS 外都写 `"usage_factor": "1"`，EVS 写 `"size"`。
SDK 结构体 `DemandProductInfo.UsageFactor` 是**字符串编码**（见 `model_demand_product_info.go` 注释：云服务器→`Duration`、云硬盘→`Duration`、弹性IP→`Duration`、带宽→`Duration` 或 `upflow`）。
`"1"` 不是合法 usage factor，真实 `ListOnDemandResourceRatings` 调用会失败或返回 0 价。

**为什么测试没抓到**：所有华为单测只 mock 了响应 JSON、只测 `Parse()`，从未用真实/mock client 断言 `backend.query` 收到的请求体字段。**这是 "只测 Parse 不测 Extract→request 构造" 导致的逃逸。**

**修复**：统一改为 `"Duration"`（带宽类 EIP/ELB 若是按流量计费用 `"upflow"`，需真实 API 确认）。建议抽 `huaweiProductInfo(...)` helper 消除手写字符串。

### 2. 华为 mapper `project_id` 错配成 region（9 个 mapper 全部）
所有华为 mapper 把 `"project_id": r.Region`（如 `cn-north-4`）。但 `RateOnDemandReq.ProjectId` 是 **UUID 项目 ID**，不是 region。
语义错误，真实 API 可能鉴权失败或返回错误价格。需从 provider config / env / 参数取真实 project_id，或确认该字段可省略。

### 3. `NewEngine` 强制要求腾讯云 AK/SK
`engine.go:249` `if cfg.SecretID == "" || cfg.SecretKey == "" { return error }`。
一个**纯 AWS / 纯阿里 / 纯华为** plan，即便完全用不到腾讯云，也会因缺腾讯密钥直接报错。
直接违反 "多云" 定位。

**修复**：腾讯凭证改为按需校验（dispatch 到 tencent 时再校验缺凭证），`NewEngine` 不再强制；或至少允许「plan 无 tencent 资源时跳过校验」。

### 4. `--timeout` 对阿里 / 华为 backend 不生效
`engine.go` 注释声明 `Timeout` 应用于所有 backend（`prof.HttpProfile.ReqTimeout`）。
但 `newAlibabaBackend` / `newHuaweiBackend` 创建 client 时**没读 `cfg.Timeout`**，只有腾讯路径用了 `ReqTimeout`。
用户设 `--timeout 45s` 对阿里/华为无效（华为靠 SDK 默认超时，阿里同样）。

**修复**：把 `cfg.Timeout` 透传到两个 backend 的 HTTP profile（`bssopenapi` / `bssintl` 都支持 `WithTimeout` 或 `HttpConfig`）。

---

## 🟠 健壮性 / 高可用

### 5. 任一资源定价失败即全局中断（fail-fast）
`priceReport` 收集全部 err 后 `errors.Join` 返回，整个 report 无输出。
大规模 plan 里一个 SKU 错误（如某实例规格已下架）会让全量估算失败。

**建议**：增加「跳过失败资源、末尾汇总告警」模式（当前已对「未注册类型」skip，但 API 错误是硬失败）。加 `--fail-on-error` 开关，默认宽松。

### 6. 缓存 TTL 硬编码 24h 且不可配
`cache.go:33 defaultTTL = 24 * time.Hour`，无 flag。价格变动最长陈旧 24h。
**建议**：加 `--cache-ttl`（默认 24h），文档明确语义。

### 7. 阿里 `Quantity` 死字段
`mapper` 在 `Params` 设了 `"Quantity": 1`，但 `alibaba_backend.query` 从不读取。无害但误导。

---

## 🟡 可读性 / 文档漂移

- root command `Short` 仍写 "Tencent Cloud cost estimation from Terraform plans"（过时，应为多云）。
- `alibaba_ecs.go:14` 注释写 "BSS DescribePrice"，实际是 `GetPayAsYouGoPrice`。
- README roadmap 仍写 "M9 — Alibaba Cloud ECS / EBS via DescribePrice"。
- `engine.go:197` `backend` 接口注释说 "additional providers (e.g. AWS) plug in as a backend"，腾讯现在也走 backend 思想，注释略过时。

---

## 🟠 扩展性

### 8. 新增云的样板代码偏多、易错
每个阿里/华为 mapper 都要手写 `ModuleList` / `product_infos` 的 `map[string]string` / `map[string]interface{}` 构造，字符串 key 无编译期校验——这正是 #1/#2 bug 的根源。

**建议**：抽公共构造器
- `alibabaModule(code, priceType, config string) map[string]string`
- `huaweiProductInfo(id, svcType, resType, spec, region string) map[string]interface{}`
让 mapper 只传语义参数，杜绝手写字符串 key。

### 9. `PriceRequest.Params` 是自由格式 `map[string]interface{}`
灵活但无类型安全，bug 只能运行时/测试发现。长远可考虑每云一个 typed request struct（短期不改，成本效益一般）。

---

## 性能

- 并发 bounded（default 8），client 按 `product:region` 缓存复用 — **好**。
- `inflight` 去重 + 指数退避重试 — **好**。
- 缓存打开失败时降级为无缓存（不致命）— **好**。
- 🟡 每请求算 cache key 都 `json.Marshal` 整个 `PlanRequest`（含 Params）。大 plan 下可感知，hot path 可只 hash 关键字段。
- 无内存泄漏风险：`flight` map 在 `close(done)` 前正确 `delete`。

---

## 测试

### 🔴 10. 测试只覆盖 `Parse()`，没覆盖真实请求体构造
**根因**：所有阿里/华为单测 mock 响应 JSON 直接喂 `Parse()`，从没用 mock client 捕获 `backend.query` 实际发出的 SDK request。
结果：#1（usage_factor）/#2（project_id）两个**请求体构造 bug** 完全逃逸。

**建议**：加「请求体快照测试」——用 mock `alibabaBSSAPI` / `huaweiBSSAPI` 捕获 `GetPayAsYouGoPrice` / `ListOnDemandResourceRatings` 入参，断言 `ModuleList` / `product_infos` 字段值正确。这能在不依赖真实 API 的情况下锁死 #1/#2。

---

## 已验证为良好的部分（不必动）

- 安全：无密钥泄露，env/config 读取、错误信息不含密钥值。
- 并发模型：`priceReport` 用独立 collector goroutine + `wg.Wait()` 后 `close(results)`，规避了早期「error channel 满 → 死锁」问题。
- 错误包装：统一 `%w` 链，retry 分类保守（只重试限流/超时/5xx）。
- 工具函数：`getStr/getInt/getBool/getNestedMap/firstZone` 全部 nil-safe，JSON 数值 `float64` 处理完善。
- 缓存：bbolt 文件锁超时保护、过期懒清理、事务内拷贝 value（避免悬垂 slice）。

---

## 建议优先级

| 序 | 项 | 严重度 | 是否需要真实 API 验证 |
|----|----|--------|----------------------|
| 1 | 华为 `usage_factor` 修正 | 🔴 | 是（确认各资源正确 factor 码）|
| 2 | 华为 `project_id` 修正 | 🔴 | 是（确认可省略或取真实值）|
| 3 | `NewEngine` 去除腾讯凭证强依赖 | 🔴 | 否 |
| 4 | `--timeout` 透传阿里/华为 | 🔴 | 否 |
| 5 | 请求体快照测试（锁死 #1/#2）| 🔴 | 否（mock client 即可）|
| 6 | 失败跳过模式 | 🟠 | 否 |
| 7 | 缓存 TTL 可配 | 🟠 | 否 |
| 8 | mapper 公共构造器 | 🟠 | 否 |
| 9 | 文档/注释漂移清理 | 🟡 | 否 |
| 10 | cache key 性能 micro-opt | 🟡 | 否 |
