# Statement Summary v1/v2 正确性与运行风险审查执行计划

日期：2026-07-23

本文是本次审查的执行记录。原始英文版为 `statement-summary-v1-v2-audit-execplan.md`。

## 目标和验收标准

TiDB 当前有两套 statement summary 实现：

- v1：老的内存实现，位于 `pkg/util/stmtsummary`。
- v2：持久化实现，位于 `pkg/util/stmtsummary/v2`。

两者都会按 statement digest 聚合 SQL 执行信息，并通过 statement summary 相关的 information_schema 表对外暴露。

本次审查的目标是在已知的 PR #69914 中 v2 `Add` 与 `ClearInternal` race 之外，继续系统性查找 statement summary v1/v2 中的 bug。审查范围包括：

- SQL 可见正确性；
- 统计周期和 history 行为；
- eviction 与 `Others`；
- v2 持久化；
- 并发访问；
- shutdown / rotate；
- 内存、文件描述符、goroutine 等资源边界；
- 关键路径性能风险。

验收标准：

- 每个确认问题尽量有可复现证据，例如定向 Go test、race detector 输出、benchmark、或 TiUP/RealTiKV 场景。
- 由 v1、v2、并发/资源等多个视角独立审查，并互相反证。
- 最终交付 `statement-summary-v1-v2-audit-report.md` 及中文版，记录审查版本、确认问题、影响、证据和复现步骤。

## 进度

- [x] 2026-07-23 09:49Z：读取仓库说明、`PLANS.md`、TiDB 测试相关 skill、failpoint 测试 runner、WIP 验证策略。
- [x] 2026-07-23 09:49Z：确定审查基线：当前 `origin/master` 加 PR #69914 的两个 commit，同时保留当前 `origin/master` 作为对比。
- [x] 2026-07-23 10:15Z：梳理 v1/v2 子系统、sysvar 分发、information_schema reader、持久化、shutdown 等调用链。
- [x] 2026-07-23 10:15Z：完成三条独立初审线：v1 语义；v2 语义/持久化；并发、性能、内存和跨版本一致性。
- [x] 2026-07-23 11:10Z：将可信候选问题转为最小复现，或说明为什么本地不适合确定性复现。定向失败测试已确认 history、panic、聚合、FD、SQL 路由、截断、时间过滤、持久化等问题；race 测试确认 v1/v2 evicted 表 race。
- [x] 2026-07-23 11:20Z：进行交叉反证，检查各 reviewer 发现的问题是否成立、是否重复、是否遗漏同类路径。
- [x] 2026-07-23 11:35Z：进行最终收敛。持久化/资源 final reviewer 未发现新的独立根因。窄范围 SQL-route agent 超时，因此改为本地对 v2 current/history 现有 SQL 测试和 cumulative 表定向复现重新检查。
- [x] 2026-07-23 11:45Z：写入并验证 `statement-summary-v1-v2-audit-report.md`。
- [x] 2026-07-23 11:50Z：完成 Ready 级文档验证，记录命令、风险和未验证项。

## 关键发现

- `origin/master` 在 PR #69914 创建后有推进，但 `pkg/util/stmtsummary` 相关文件在 PR base 和当前 `origin/master` 之间没有变化。
- 本地 `master` 落后于 `origin/master`，因此审查不能直接使用本地 checkout 的 revision。
- 将 PR #69914 合并到当前 `origin/master` 的 synthetic tree 可以稳定复现。
- 三个独立 reviewer 在共享详细结论前已收敛到多个相同问题：v2 filtered-file FD 泄露、容量缩小绕过 eviction accounting、`Others` 聚合字段缺失、evicted 表同步问题。
- v2 第一批定向测试确认了 cumulative 表列缺失、容量缩小、disable 后重新插入、不完整 `Others` merge、过滤文件 FD 泄露、绝对路径日志文件时间解析、NormalizedSQL 未截断、AVG 分母错误、table names 多余逗号、begin/end 时间不一致等问题。
- v1 第一批定向测试确认了保留最老 interval、history size 降低后返回最老 row、encoded plan nil deref、key 边界碰撞、容量缩小、不完整 `Others` merge 等问题。
- v1/v2 evicted 表候选问题均被 race detector 证明为实际数据竞争。
- v2 persistent 模式下 SQL 层测试确认：
  - `SELECT * FROM information_schema.tidb_statements_stats` 因 `ERRORS` 等 cumulative 表列未注册而 panic；
  - `SELECT digest FROM information_schema.tidb_statements_stats` 因 v2 executor 未初始化 cumulative rows reader 而 nil pointer；
  - 单边 history 时间谓词会误过滤。
- v2 额外持久化复现确认：超大 evicted record 写入失败后丢失、`tidb-statements2.log` 被误读、`GlobalStmtSummary=nil` 时 wrapper panic、out-of-order 历史日志导致合法旧记录被跳过。
- v2 可以在 in-flight `Add()` 更新 record 之前就 rotate 并持久化旧 window，导致一次执行统计不在 current，也不在 history。
- statement summary 用户分组和权限检查只使用 `Username`，没有使用 TiDB 认证账号中的 host 部分。

## 决策记录

- 审查 PR #69914 预期合入后的状态：把 PR 应用到当前 `origin/master`，同时对比原始 revision。
  - 原因：用户是在 review PR #69914 后要求继续查 bug。只看旧 base 会漏掉最新集成问题；不合 PR 又会重复发现 PR 已修的问题。

- 已知的 v2 `Add` / `ClearInternal` race 作为 baseline，不计入“新发现问题”，但在最终报告中保留。
  - 原因：用户明确要求查这个已知问题之外的新 bug，但最终交付仍需要完整说明当前状态。

- 优先使用定向 unit/race test，不先部署 TiUP。
  - 原因：绝大多数状态机、锁、周期、持久化问题都能在所属 package 内直接复现。只有依赖真实 TiKV、多节点聚合或进程生命周期的行为才需要 TiUP。

- 将容量缩小导致的数据丢失归类为确认 bug，同时把 evicted metric 语义另行说明。
  - 原因：v1/v2 都会在容量缩小时直接丢弃多余 row，没有合并到 `Others`，也没有其他保留路径。已有测试如果只约束 metric，不足以证明这种数据丢失是正确行为。

- 临时测试只作为审查证据，不作为生产代码修改。
  - 原因：用户要的是审查报告而不是修复。临时测试描述预期行为，并且在当前代码上预期失败；证据记录后全部删除。

## 结果回顾

本次审查产出 `statement-summary-v1-v2-audit-report.md`。报告记录了：

- 审查 revision；
- PR #69914 已知问题；
- 34 个新确认问题或紧密相关的问题组；
- 需要产品/兼容性确认的行为差异；
- 被反证或降级的候选问题；
- 可复现命令。

确认问题主要分为五类：

- v2 persistent 模式 SQL 路由/reader 缺口，尤其是 cumulative 表和时间谓词 pushdown。
- `Others` 和 eviction accounting 不一致，包括 metric merge 缺失、用户/资源组隔离、容量缩小、internal query 清理。
- v2 持久化/history reader 资源与正确性问题，包括 FD 泄露、文件匹配过宽、out-of-order 提前停止、超大写失败丢失、无界工作/内存。
- v2 在选中 record 后释放 window lock，导致 rotate/flush 或配置 clear 可以看到旧状态。
- 共享身份/键问题，包括只按 username 授权、`StmtDigestKey` 字段边界不明确。

临时诊断测试已从 synthetic worktree 删除。已在 synthetic worktree 重新运行 `make bazel_prepare` 移除临时 BUILD 变化，并删除 `/tmp` 下本次 statement summary 审查产生的 synthetic worktree。主 worktree 只保留 Markdown 审查产物。

## 上下文

v1 缓存是 `pkg/util/stmtsummary/statement_summary.go` 中的全局 `stmtSummaryByDigestMap`。它的 `SimpleLRUCache` 每个 digest key 保留一个对象，每个对象内部可以保留多个时间 interval。被淘汰的记录会在 `pkg/util/stmtsummary/evicted.go` 中合并到对应 interval 的 `Others`。SQL row 转换在 `pkg/util/stmtsummary/reader.go`。

v2 缓存是 `pkg/util/stmtsummary/v2/stmtsummary.go` 中的 `StmtSummary`。它维护当前 `stmtWindow`，按周期 rotate，把完成的 window 通过 `stmtStorage` 持久化，可选地记录单条 evicted 记录，并通过 `pkg/util/stmtsummary/v2/reader.go` 对外读出。`lockedStmtRecord` 保护可变的 `StmtRecord` 字段。

运行时变量通过这些 package 外部的兼容 wrapper 和 callback 分发，因此审查也覆盖：

- `pkg/sessionctx/variable`
- `pkg/domain`
- information_schema 表构造
- executor statement collection
- server setup/shutdown

PR #69914 包含 commit：

- `997c0e3f1f1908d4563f0c9b218dae241e4895b3`
- `46dc2b36c215a7430451e450ff879f211e98b083`

本次审查使用的当前 `origin/master`：

- `ed2376acc6e0feeff9f3e2c38db489727933aa80`

## 工作方法

1. 从当前 `origin/master` 创建临时 worktree，并应用 PR #69914，不修改用户当前 checkout 的分支。
2. 梳理所有生产文件、可变字段、goroutine、锁、cache callback、sysvar callback、reader、storage 边界。
3. 并行进行三条 review：
   - v1：add、interval rollover、history resize、cache resize、enable/disable、internal query、eviction、auth、table reader。
   - v2：add、rotate、persistence、log serialization、history reader、option change、close/flush、error path。
   - cross-cutting：锁顺序、race、goroutine/channel 生命周期、pool 对象所有权、FD/内存、hot path 成本、v1/v2 和 SQL 表一致性。
4. 中央 triage 候选问题。可信问题只在临时 worktree 中加临时诊断测试。
5. 对 race 使用 `go test -race`。对性能/资源问题尽量给出定向 workload 或可执行复现思路。
6. 互相反证：每个 reviewer 尝试推翻其他人的候选问题，检查严重性、范围、同类路径和遗漏。
7. 最终收敛：多个 reviewer 或本地窄范围复查确认没有新的高置信 actionable bug。

## 关键命令

获取和验证 revision：

```bash
git fetch origin master pull/69914/head:refs/remotes/origin/pr/69914
git rev-parse origin/master refs/remotes/origin/pr/69914
git merge-base origin/master refs/remotes/origin/pr/69914
```

发现文件和调用方：

```bash
rg --files pkg/util/stmtsummary
rg -n "StmtSummary|statement_summary|STATEMENTS_SUMMARY|SetEnableInternal|ClearInternal" pkg cmd
```

v1 package 使用 failpoint wrapper：

```bash
rg -n --fixed-strings -- "failpoint." pkg/util/stmtsummary
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary -run '<FocusedTest>' -count=1
```

v2 root package 没有 failpoint instrumentation，使用普通 Go test：

```bash
go test -run '<FocusedTest>' -count=1 -tags=intest,deadlock ./pkg/util/stmtsummary/v2
```

race 候选问题：

```bash
go test -race -run '<FocusedRaceTest>' -count=1 -tags=intest,deadlock ./pkg/util/stmtsummary/v2
```

## 验证标准

报告满足以下条件后视为完成：

1. 明确列出 source revision，并区分 known baseline、newly confirmed issue、rejected candidate。
2. 每个确认问题包含受影响版本/实现、触发条件、用户可见或运行影响、复现命令、预期和实际结果、源码路径。
3. race 和资源类问题要有 detector 输出、确定性编排、或能被 runnable test 证明的 interleaving。
4. 性能类问题不能只做定性猜测，需要 benchmark/workload 或明确资源边界证据。
5. 至少三个独立 review 角色完成初审，发现问题经过交叉反证，最终 reviewer 未发现新的 actionable bug。
6. 临时 worktree 和诊断文件已删除，failpoint 已关闭，用户主 worktree 只保留两个 Markdown 文档。

最终文档验证包括：

- Markdown 路径/链接检查；
- 命令准确性检查；
- `git diff --check`；
- repository status 检查。

由于本次交付是文档和临时诊断证据，不是生产代码修复，因此只有在能直接验证发现时才跑定向 package test；主 worktree 没有 Go/Bazel 生产代码变更时不跑全量 lint。

## 幂等和恢复

- 只读搜索和定向测试可以重复执行。
- 诊断测试只放在显式命名的临时 worktree，并在记录证据后删除。
- failpoint 测试使用 `tools/check/failpoint-go-test.sh`，该脚本会在清理时关闭 failpoint。
- 如需 TiUP 实验，遵循 RealTiKV runner 的启动和清理流程，并确认 PD endpoint 在清理后不可达。
- 不重置、不清理用户主 worktree。
- 如果 synthetic merge 冲突，只能删除明确的临时 worktree，并确认其中没有未同步证据。

## 产物

主报告：

```text
statement-summary-v1-v2-audit-report.md
```

执行记录：

```text
statement-summary-v1-v2-audit-execplan.md
```

中文版：

```text
statement-summary-v1-v2-audit-report.zh.md
statement-summary-v1-v2-audit-execplan.zh.md
```

