# cloudtab 使用指南

一份**从零到 PR 评论**的完整手册。目标读者：会用 Terraform、想在 apply 前看见每月账单的工程师。

- [1. 安装](#1-安装)
- [2. 准备腾讯云凭据](#2-准备腾讯云凭据)
- [3. 生成 Terraform plan](#3-生成-terraform-plan)
- [4. `breakdown` — 单次成本估算](#4-breakdown--单次成本估算)
- [5. `diff` — 两个 plan 的成本增量](#5-diff--两个-plan-的成本增量)
- [6. GitHub Action 集成](#6-github-action-集成)
- [7. 本地缓存与 QPS 限流](#7-本地缓存与-qps-限流)
- [8. FAQ / 常见错误](#8-faq--常见错误)
- [9. 如何贡献新资源类型](#9-如何贡献新资源类型)

---

## 1. 安装

**推荐（有 Go 环境）：**
```bash
go install github.com/susunola/cloudtab/cmd/cloudtab@latest
cloudtab --help
```

**从 Release 下载二进制：**
```bash
# macOS arm64
curl -sfL https://github.com/susunola/cloudtab/releases/latest/download/cloudtab_darwin_arm64.tar.gz \
  | tar -xz -C /usr/local/bin cloudtab

# Linux amd64
curl -sfL https://github.com/susunola/cloudtab/releases/latest/download/cloudtab_linux_amd64.tar.gz \
  | sudo tar -xz -C /usr/local/bin cloudtab
```

**从源码构建：**
```bash
git clone https://github.com/susunola/cloudtab
cd cloudtab
go build -o cloudtab ./cmd/cloudtab
```

---

## 2. 准备腾讯云凭据

`cloudtab` 只调**只读**的 `InquiryPrice*` 接口，不需要写权限。**强烈建议**用 CAM 建一个专用子账号，最小权限：

**方法一：控制台**
1. 打开 [CAM 控制台](https://console.cloud.tencent.com/cam) → 用户 → 新建子用户 → 编程访问
2. 建自定义策略 `CloudtabReadOnly`，策略语法：

```json
{
  "version": "2.0",
  "statement": [
    {
      "effect": "allow",
      "action": [
        "cvm:InquiryPriceRunInstances",
        "cbs:InquiryPriceCreateDisks",
        "clb:InquiryPriceCreateLoadBalancer",
        "cdb:DescribeDBPrice",
        "redis:InquiryPriceCreateInstance"
      ],
      "resource": "*"
    }
  ]
}
```

3. 将策略关联到子用户 → 拿到 SecretId / SecretKey

**注入到环境：**
```bash
export TENCENTCLOUD_SECRET_ID=AKIDxxxx
export TENCENTCLOUD_SECRET_KEY=xxxxxxxx
```

推荐把它们写进 `direnv` 的 `.envrc` 或 1Password / Vault 里，**不要**提交进仓库。

---

## 3. 生成 Terraform plan

`cloudtab` 吃的是 `terraform show -json <planfile>` 的输出，不是 `.tf` 源码。

```bash
cd your-terraform-project/
terraform init
terraform plan -out=tf.plan
terraform show -json tf.plan > plan.json
```

三步之后你会得到 `plan.json`，这就是 cloudtab 的输入。

> 💡 **CI 中**：`terraform plan -out=tfplan && terraform show -json tfplan > plan.json` 已经是主流姿势，不需要为 cloudtab 单独改流水线。

---

## 4. `breakdown` — 单次成本估算

**最小命令：**
```bash
cloudtab breakdown --path plan.json
```

**输出（table）：**
```
+--------------------------------+-------------------+-----------------+
|            RESOURCE            |     COMPONENT     |  MONTHLY (CNY)  |
+--------------------------------+-------------------+-----------------+
| tencentcloud_instance.api      | Compute (S5.MED2) |          214.20 |
|                                | Public bandwidth  |           35.04 |
| tencentcloud_cbs_storage.data  | CBS PREMIUM (50GB)|           45.00 |
| tencentcloud_clb_instance.pub  | CLB (OPEN)        |          138.90 |
+--------------------------------+-------------------+-----------------+
|                                | TOTAL             |          433.14 |
+--------------------------------+-------------------+-----------------+

Skipped resources:
  - tencentcloud_cos_bucket.assets: unsupported resource type
```

**参数：**

| flag | 默认 | 说明 |
|---|---|---|
| `--path` | `plan.json` | terraform show 输出路径 |
| `--region` | `ap-guangzhou` | 默认地域；plan 里显式指定的 region 优先 |
| `--usage-file` | 空 | usage 假设 YAML（按资源 address 覆盖参数，如 `mem_size`、`monthly_*`） |
| `--format` | `table` | `table` / `json` |

**JSON 输出（供脚本消费）：**
```bash
cloudtab breakdown --path plan.json --format json | jq '.resources[].components[].monthly_cost'
```

**带 usage.yml 覆盖（M5）：**
```yaml
# usage.yml
# address 必须和 plan.json 中的 resource_changes.address 一致
tencentcloud_redis_instance.cache:
  mem_size: 4096
```

```bash
cloudtab breakdown --path plan.json --usage-file usage.yml
```

---

## 5. `diff` — 两个 plan 的成本增量

这是 **PR review 场景的主用法**。对比"合入前"和"合入后"两份 plan：

```bash
# 基线 plan：先 checkout base 分支
git checkout main
terraform plan -out=plan.old.tfplan
terraform show -json plan.old.tfplan > plan.old.json

# 新 plan
git checkout your-feature-branch
terraform plan -out=plan.new.tfplan
terraform show -json plan.new.tfplan > plan.new.json

cloudtab diff --before plan.old.json --after plan.new.json --format markdown
```

**Markdown 输出**（贴到 PR 评论里很好看）：

```markdown
## 💰 cloudtab — Tencent Cloud cost estimate

**Monthly change:** `+312.48 CNY` (before `1204.10` → after `1516.58`)

|  | Resource | Before | After | Δ Monthly |
|---|---|---:|---:|---:|
| + | `tencentcloud_instance.api[0]` | 0.00 | 214.20 | **+214.20** |
| ~ | `tencentcloud_clb_instance.public` | 138.90 | 192.18 | **+53.28** |
| - | `tencentcloud_eip.legacy` | 24.00 | 0.00 | **-24.00** |
```

**符号约定：**

| Kind | 含义 |
|---|---|
| `+` | 新增资源 |
| `-` | 删除资源 |
| `~` | 参数变更（如 `instance_type` 从 `S5.MED2` 换到 `S5.LARGE4`） |
| `=` | 无变化（表格模式显示，markdown 模式隐藏） |

**参数：**

| flag | 默认 | 说明 |
|---|---|---|
| `--before` | 必填 | 基线 plan.json |
| `--after` | 必填 | 新 plan.json |
| `--region` | `ap-guangzhou` | 默认地域 |
| `--before-usage-file` | 空 | 基线 plan 的 usage 覆盖 YAML |
| `--after-usage-file` | 空 | 新 plan 的 usage 覆盖 YAML |
| `--format` | `table` | `table` / `json` / `markdown` |

---

## 6. GitHub Action 集成

**Setup（一次性）：**

1. 在仓库 Settings → Secrets → Actions 添加：
   - `TENCENTCLOUD_SECRET_ID`
   - `TENCENTCLOUD_SECRET_KEY`

2. 在仓库根加 `.github/workflows/cloudtab.yml`：

```yaml
name: cloudtab

on:
  pull_request:
    paths: ["**.tf", "**.tfvars"]

permissions:
  contents: read
  pull-requests: write   # 用来贴评论

jobs:
  estimate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: susunola/cloudtab@v0
        env:
          TENCENTCLOUD_SECRET_ID:  ${{ secrets.TENCENTCLOUD_SECRET_ID }}
          TENCENTCLOUD_SECRET_KEY: ${{ secrets.TENCENTCLOUD_SECRET_KEY }}
        with:
          path: "infra/"                                    # .tf 所在目录
          region: "ap-guangzhou"
          base-ref: ${{ github.event.pull_request.base.ref }}
```

之后每个改动了 `.tf` 的 PR 都会自动出现**一条 sticky 评论**（同一 PR 的多次推送会 in-place 更新而不是刷屏）。

**Action 内部做的事：**
1. `terraform init && terraform plan -out=plan.new.tfplan` → 新 plan
2. `git worktree` 切到 base ref，重复一次 → 老 plan
3. `cloudtab diff --before ... --after ... --format markdown > comment.md`
4. `marocchino/sticky-pull-request-comment@v2` 贴到 PR

---

## 7. 本地缓存与 QPS 限流

**为什么要缓存**：腾讯云 `InquiryPrice*` 接口默认 QPS = 10，同一份 plan 反复跑（比如 CI 里同 PR 触发 3 次）会浪费额度。

**缓存位置**：`$HOME/.cloudtab/cache.db`（自动创建）

**缓存粒度**：`sha256(product|action|region|params)` → 原始 JSON 响应。参数完全一致 = 命中，任一 param 变化 = miss + 重新询价。

**清空缓存**：
```bash
rm -rf ~/.cloudtab/
```

**关闭缓存**（比如做 CI dry-run）：暂未提供 flag，直接删目录或用 `TMPDIR` 隔离。

---

## 8. FAQ / 常见错误

**Q: `tencentcloud: missing credentials (set TENCENTCLOUD_SECRET_ID / TENCENTCLOUD_SECRET_KEY ...)`**  
A: 只对 **腾讯云** 资源校验凭证，且**按需**触发——纯 AWS / 阿里云 / 华为云 plan 不需要腾讯密钥。环境变量没设或为空时会报这个错。`echo $TENCENTCLOUD_SECRET_ID` 检查一下；CI 里检查 `env:` 是否传进来。

**Q: `tencent api AuthFailure.SignatureFailure: The provided credentials could not be validated`**  
A: SecretId/Key 错了，或子账号没有对应产品的 `InquiryPrice*` 权限。回到 [第 2 节](#2-准备腾讯云凭据) 检查策略。

**Q: `tencent api LimitExceeded: Request rate exceeded`**  
A: 撞到 QPS 上限。这通常发生在关掉缓存 + 大量资源的 plan。等一分钟重试，或让缓存生效。

**Q: `unsupported resource type: tencentcloud_xxx`**  
A: 这个资源类型还没写 Mapper。要么忽略（会出现在 Skipped 里，不影响其他资源），要么按 [第 9 节](#9-如何贡献新资源类型) 补一个。

**Q: PREPAID 资源的成本对不上腾讯云控制台？**  
A: 我们用的是 `Price.InstancePrice.DiscountPrice`（已经含官方 discount）。控制台可能叠加优惠券、代金券、返现——这些**账单侧**优惠 API 不会返回，属于已知差距。

**Q: `terraform_wrapper` 冲突 / `tfplan` 权限**  
A: Action 里已经 `terraform_wrapper: false`。如果本地跑 `terraform show` 报权限，检查 `plan.tfplan` 是不是被 gitignore 且当前用户可读。

**Q: 支持 `for_each` / `count` 展开的资源吗？**  
A: 支持。`plan.json` 里每个实例都是独立的 `resource_changes` 项，`address` 长这样：`tencentcloud_instance.api[0]`, `tencentcloud_instance.api[1]`。

**Q: 支持流量类费用（EIP TRAFFIC / CDN / COS）吗？**  
A: 目前核心覆盖的是可直接询价的资源（CVM/CBS/CLB/MySQL/Redis）。对于 usage-driven 场景，已支持通过 `usage.yml` 覆盖资源参数（`--usage-file` / `--before-usage-file` / `--after-usage-file`）来注入假设；COS/CDN/SCF 的静态价格表仍在后续里程碑。

---

## 9. 如何贡献新资源类型

见 [CONTRIBUTING.md](../CONTRIBUTING.md)，简版三步：

1. **找 API**：在 [Tencent Cloud API 文档](https://cloud.tencent.com/document/api) 搜 `InquiryPrice`
2. **写 Mapper**：`internal/resources/<name>.go`，两个方法 `Extract` + `Parse`
3. **注册 + 加分支**：`registry.go` + `pricing/engine.go` 里加 `queryXxx`

一次 PR 通常 30-60 行代码 + 一个 fixture。参考 `internal/resources/cvm_instance.go` 就够了。

---

## 相关链接

- 项目 README: [README.md](../README.md)
- 架构设计: [docs/design.md](design.md)
- 架构图（交互式 HTML）: [docs/cloudtab-architecture.html](cloudtab-architecture.html)
- 腾讯云 API 索引: https://cloud.tencent.com/document/api
- Terraform Provider（腾讯云）: https://registry.terraform.io/providers/tencentcloudstack/tencentcloud/latest/docs
