# cloudtab 团队协作审查 — 合并清单（2026-07-24）

> 六位专家并行审查 + 编排者逐条核验后的权威合并版。
> 覆盖维度：架构稳定性(Adam) / 代码质量(Alex) / 并发性能(Atom) / CLI体验(Amos) / 文档一致性(Allen) / 视觉呈现(小U)。
>
> **核验状态标注**：
> - ✅ **已核实**：编排者已读代码/实测确认属实
> - ⚠️ **需真实API验证**：逻辑成立但需一次真实云 API 调用确认
> - ❌ **已剔除**：经核验判断不成立，不采纳
>
> 前置：`go build/vet/test` 全过；`go test -race -count=3` 零竞态（Atom 实跑）。上一轮 code-review-2026-07-23 的 10 项确已修复未复发。

---

## 一、最高优先级（先修这几条）

| # | 问题 | 位置 | 核验 | 发现者 |
|---|------|------|------|--------|
| A1 | **缓存打开失败被静默吞掉** — 用户以为缓存生效，实则每次全量打 API，可能触发限流且变慢，毫无提示 | `engine.go:271` `return e, nil` 丢弃 openCache err | ✅ 已核实 | Adam |
| A2 | **PR comment/markdown 标题硬编码 "Tencent Cloud"** — 纯 AWS/多云 plan 的 PR 评论显示单云，与"多云"定位直接矛盾 | `internal/output/diff.go:129` | ✅ 已核实（**4 人独立命中同一处**） | 小U/Amos/Allen/Adam |
| A3 | **阿里 `--timeout` 未透传** — 用户设超时对阿里资源无效；上一轮曾标称"已修复"实为漏修 | `alibaba_backend.go:51` 裸 `NewClientWithAccessKey` | ✅ 已核实（有 SDK 默认 30s 兜底，故 🟠 非 🔴） | Amos/Alex |
| A4 | **diff 静默丢弃 skipped 资源** — 新增一个暂不支持的资源，diff 无任何提示，用户误以为"没变化=免费"，数据完整性缺陷 | `output/diff.go` ComputeDiff 忽略 before/after.Skipped | ✅ 已核实 | Amos |
| A5 | **阿里/华为限流错误可能不被重试** — `isRetryable` 靠英文子串匹配，阿里/华为 SDK 错误可能是中文，限流静默不重试 | `engine.go:445` + `retryableSignatures` 缺 `systembusy`/`throttling.user` | ⚠️ 需真实API验证 | Adam/Atom |
| A6 | **文档与代码大面积不同步** — README 仍写 "fail-fast"（默认早已改宽松跳过）；usage.md 参数表缺 8 个已实现 flag；action.yml 仍写 "Tencent Cloud" | README / usage.md / action.yml | ✅ 已核实（Amos/Allen 交叉印证） | Allen/Amos |

---

## 二、🔴/🟠 功能与稳定性

### 已核实
- **A1 缓存静默失败**（Adam #6）— 建议 `NewEngine` 在 openCache 失败时输出 warning 到 stderr，而非静默降级。
- **A3 阿里超时**（Amos #3 / Alex 复核）— 阿里 BSS `Client` 有 `SetReadTimeout`/`SetConnectTimeout`，在 `newAlibabaBackend` 返回前调用。**同时应修正 code-review-2026-07-23.md #4 的"已修复"表述**（它写了阿里已透传，实际没有）。
- **A4 diff 丢 skipped**（Amos #10）— `DiffReport` 加 skipped 字段，`ComputeDiff` 合并 before/after skipped，markdown 加 `> ⚠️ N resources skipped (unsupported)`。
- **默认 region=ap-guangzhou 对多云是陷阱**（Amos #7）— help 文案暗示对所有云生效，纯 AWS/阿里 plan 若 provider 未写 region 可能静默查错价。建议 help 改为"未显式指定 region 的资源的默认值"并加多云说明。
- **MySQL/PostgreSQL Parse 缺 OriginalPrice 回退**（Alex #1/#2）— Price=0 但 OriginalPrice>0（无折扣）时月费误算为 0；同类 mapper 已用 `discountedYuanFromCents` 有回退。根因是 `parseTencentPrice` 嵌套 OriginalPrice 未传播（Alex #4）。
- **阿里/华为凭证未在 newEngine() 注入 Config**（Alex #10 / Adam #10 交叉印证）— 与 AWS 处理不对称；虽有 backend env fallback 冗余，但 CLI 层统一注入更一致。
- **阿里 BSS client 硬编码 cn-hangzhou**（Adam #7）。

### 需二次核验（勿盲信）
- **华为 CCE `cluster_version` / DCS `engine` / RDS `db.0.type` 读取后 `_=` 丢弃**（Alex #5/#6/#7）— ⚠️ 可能是死代码，也可能确实无需入参。尤其 DCS engine（Redis/Memcached 价格不同）若真丢弃则是价格准确性缺陷。**需对照华为 BSS 文档确认，勿直接删。**
- **阿里 MongoDB/RDS 把 storage 拼进 DBInstanceClass 的 Config 字符串**（`class:storage`）是否被 BSS API 正确解析 — ⚠️ 需真实 API 验证（属"拼串格式"存疑，非字段丢弃）。

### ❌ 已剔除
- ~~Alex #3「AlibabaMongoDB storage 被静默丢弃」~~ — 核验为误判：`alibaba_redis_mongo.go:66` 已把 storage 拼进 Config（与 AlibabaRDS 一致），未丢弃。
- ~~Atom #1「worker panic 导致静默死锁」定 🔴~~ — 核验为定性错误：实测 worker 无 recover 的 panic 是**进程 crash 带 stack 退出**（易排查），非"静默 hang"。**降级为 🟠 健壮性改进**：加 recover 可把单资源 panic 转为一条 error 而不拖垮整个报告，建议仍采纳。
- ~~Allen「--timeout 多云透传已验证一致」~~ — 与 A3 冲突，以 Amos/Alex 为准（阿里实际未透传）。

---

## 三、🟠 体验与交互（Amos）

- diff 子命令 12 个 `diffXxx` flag 变量全量复制，`--format` help 两边不一致（breakdown 漏 markdown 而代码其实支持）
- 三个云凭证缺失错误不告诉用户"哪些云、plan 里有几个该云资源"
- 默认 domestic site + 宽松跳过组合：国际站用户会静默看到偏低价格无提示
- table 输出显示所有 `=` 无变化资源（markdown 已过滤），大 plan 噪音大
- 建议新增 `cloudtab doctor` 自检命令 + 用自带 `testdata/example.plan.json` 做 5 分钟上手体验

---

## 四、🟠 视觉与呈现（小U）

- **PR comment 无 sticky/折叠/汇总行** — 招牌卖点，50+ 资源时刷屏无总计行，reviewer 直接关掉。加 totals 行 + `<details>` 折叠 + `<!-- cloudtab:start/end -->` 边界
- **`optimization-architecture.svg` 不支持深色模式** — 硬编码白底黑字，GitHub 暗色主题撕裂。改 CSS 变量 + `prefers-color-scheme`
- 终端金额无千分位、长资源名无截断、diff 涨跌缺颜色双信号
- logo 深色对比度低、架构 HTML 默认深色与浅色 README 割裂、badge 与 logo 品牌色不成色板
- 553KB 架构 PNG 是 commit 噪声（与 SVG 重复且未被引用）

---

## 五、🟡 低严重度 / 卫生问题（合并去重）

- **README 声称 45 种资源，实际注册 55 种**（Allen）✅ 已核实（`grep -c r.Register` = 55），少报 10 种
- **`registry.go:82` / `pricehelper.go:40` 注释仍写 "BSS DescribePrice"**（实际 GetPayAsYouGoPrice）✅ 上一轮说修没修
- `code-review-2026-07-23.md` 中文乱码疑似编码错乱（小U）— 建议 iconv 修复
- `isRetryable` 的 `"eof"` 子串过宽，建议 `"unexpected eof"`；`"timeout"` 可能误伤参数校验错误（Atom #2a/#2b）
- CacheKey `json.Marshal` 整个请求，大 plan ~50ms/1000 资源，可优化为只 hash 关键字段（Atom #3，小U 补充观察也提到）
- 3 处 `fmt.Sprintf` 无格式参数、`daysPerMonth` 定义位置、`tencentSimplePrice.Original` 字段名与 tag 不一致（Alex #8/#9/#12）
- backend 接口无 `Close()`（CLI 无害，嵌服务会连接泄漏）、新增 provider 需改 engine.go 多处 boilerplate（Adam #2/#12）
- `diff.go:52` 币种写死 `CNY`，混合云 footer 求和会显示错币种（小U 补充观察）
- JSON 输出缺 version/generated_at/plan_path 等元信息；breakdown 缺 markdown 输出；plan 解析失败不提示 tfplan 转换（Amos）
- design.md 里程碑表与 README roadmap 不一致、License 写 Apache 2.0（实际 MIT）（Allen）

---

## 六、全员确认的亮点（无需改动）

- 上一轮 #1/#2 已被 `huaweiProductInfo` helper + 请求体快照测试彻底锁死，未复发
- 分层依赖方向正确无循环；并发 collector 模型 `-race -count=3` 零告警
- 自制 singleflight（lock→register→unlock→execute→lock→delete→close）时序正确
- bbolt 缓存：超时锁、过期懒清理、事务内 copy value 全部正确
- 错误 `%w` 包装链完整；retry 分类保守（只重试限流/超时/5xx）；backend 懒加载纯腾讯 plan 不触发其他云 SDK

---

## 七、建议修复顺序

1. **立即（半天内）**：A1 缓存静默、A2/diff标题去Tencent化、A3 阿里超时、README fail-fast 描述修正
2. **本周**：A4 diff 报告 skipped、A5 阿里/华为重试覆盖（需 API 验证错误码）、usage.md 补 8 个 flag、注释 DescribePrice→GetPayAsYouGoPrice、README 45→55
3. **下周**：MySQL/PG OriginalPrice 回退、worker 加 recover、PR comment sticky+折叠、架构 SVG 深色适配、默认 region 文案
4. **需真实 API 联调后再动**：华为 CCE/DCS/RDS 丢字段、阿里 storage 拼串格式、限流错误码实测

---

**编排者核验声明**：本清单在六份专家报告基础上，已亲自读代码/实测核验 8 处关键点，剔除 2 条误判（Alex storage、Atom panic 定性）、裁决 1 处冲突（阿里超时 Amos vs Allen）、标注 5 处需真实 API 验证。标 ✅ 的可放心动手，标 ⚠️ 的务必先验证再改。
