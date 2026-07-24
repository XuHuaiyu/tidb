# Statement Summary v1/v2 审查报告

日期：2026-07-23

英文原版：`statement-summary-v1-v2-audit-report.md`

> 更新：可运行的 `TestAudit*` 复现已经整理到当前工作区，不再只存在于旧的临时 `/tmp` worktree。提交 issue / 发给开发者时，以 `statement-summary-v1-v2-audit-test-matrix.zh.md` 和 `statement-summary-v1-v2-audit-test-matrix.md` 为准；这两份矩阵记录了每个测试的源码路径、行号、运行命令、当前失败摘要和是否适合作为 confirmed bug 提交。本文早期章节里引用的 synthetic worktree 和部分旧测试名仅保留为历史审查记录。

审查代码：

- Base：`origin/master` = `ed2376acc6e0feeff9f3e2c38db489727933aa80`
- 被审查 PR：pingcap/tidb#69914，head = `46dc2b36c215a7430451e450ff879f211e98b083`
- Synthetic audit tree：将 PR #69914 合并到当前 `origin/master` 后的临时 worktree `/tmp/tidb-stmtsummary-audit.hdkvdB`

审查范围：

- v1：`pkg/util/stmtsummary`
- v2 persistent statement summary：`pkg/util/stmtsummary/v2`
- SQL 路由和 information_schema reader：`pkg/executor/stmtsummary.go`、`pkg/planner/core/memtable_predicate_extractor.go`、启动和 sysvar wiring

原始审查将问题分为四类：

- PR #69914 已知问题：已通过 GitHub review comment 反馈。
- 新确认问题：本次审查发现的 actionable bug。
- 需要产品/兼容性确认的问题：确实存在行为差异或风险，但可能需要 owner 判断预期语义。
- 被反证或降级的候选问题。

临时 audit 测试只写入 synthetic worktree。它们是用于证明当前代码不满足预期行为的失败测试，不应原样提交；可以作为后续创建 issue 或写 regression test 的复现草稿。

## Reviewer 收敛情况

已完成的 review 视角：

- v1 语义：history、eviction、`Others`、权限、容量变更。
- v2 语义/持久化：add/rotate、文件持久化、history reader、配置项。
- cross-cutting：race、goroutine、内存/FD 所有权、key encoding、SQL 可见一致性。
- final persistence/resource sweep：没有发现新的独立根因；独立确认了 v2 `Add` 与 rotate/persist 数据丢失路径，以及 reader/资源边界风险。

最终 SQL-route sweep agent 未及时返回，因此改为本地定向检查：

- `pkg/executor/stmtsummary.go`
- `pkg/util/stmtsummary/v2/reader.go`
- `pkg/util/stmtsummary/v2/column.go`
- 现有 v2 SQL tests

结论：v2 current/history 的 `SELECT *` 已有测试覆盖并通过；cumulative 表路由仍是独立破坏点。

## PR #69914 已知问题

### K1. v2 `Add()` 可在 `ClearInternal()` 后重新写入 internal query 记录

状态：已在 PR review comment 中反馈：

- https://github.com/pingcap/tidb/pull/69914#discussion_r3637062583
- https://github.com/pingcap/tidb/pull/69914#discussion_r3637062594

根因：

- `Add()` 在选中 record 前检查 statement 是否为 internal。
- 它在 `record.Add(info)` 前释放 `windowLock`。
- `SetEnableInternalQuery(false)` 可能在中间清理 current internal rows。
- in-flight `Add()` 随后仍能更新旧 record。

这个问题不计入本次“新发现问题”，但报告保留它作为当前状态说明。

## 原始审查中的历史问题记录

不要把本节作为当前提单清单。本节早于可运行测试恢复，仍保留了一些旧的临时测试名和候选分析。当前提单请以 `statement-summary-v1-v2-audit-test-matrix.zh.md` 的 confirmed 表为准；没有稳定测试的候选项已经在矩阵中明确降级。

### 1. v2 persistent 模式下 `INFORMATION_SCHEMA.TIDB_STATEMENTS_STATS` 会 panic

受影响路径：

- `pkg/executor/stmtsummary.go`
- `pkg/util/stmtsummary/v2/column.go`

影响：

- 开启 `tidb_stmt_summary_enable_persistent` 后，查询 `information_schema.tidb_statements_stats` 可能 panic。
- 两种独立失败模式：
  - `SELECT *` 会要求 v2 构造 `ERRORS` 等 cumulative 表专有列，但 v2 没有对应 column factory。
  - `SELECT digest` 只用 v2 支持列，但 executor 的 v2 路径没有初始化 cumulative table rows reader，导致 nil pointer。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -run '^TestAuditV2SQLCumulativeTable(SupportedColumn)?DoesNotPanic$' -count=1
```

实际结果：

- `SELECT * FROM information_schema.tidb_statements_stats LIMIT 1` panic：`should never happen, should register new column ERRORS into columnValueFactoryMap`
- `SELECT digest FROM information_schema.tidb_statements_stats LIMIT 1` panic：nil pointer

预期：

- v2 应实现 cumulative table、路由到 v1 兼容数据，或返回受控 unsupported error，不能 panic。

### 2. v2 单边时间谓词会错误丢失 history 行

受影响路径：

- `pkg/planner/core/memtable_predicate_extractor.go`
- `pkg/executor/stmtsummary.go`
- `pkg/util/stmtsummary/v2/reader.go`

影响：

以下查询可能漏掉合法记录：

- `WHERE summary_end_time >= <time>`
- `WHERE summary_begin_time <= <time>`

原因是 extractor 会给缺失的一侧合成 1 小时默认边界，使 pushdown 的粗粒度时间范围比 SQL 谓词更窄。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -run '^TestAuditV2SingleSided(Lower|Upper)TimePredicateDoesNotLoseRows$' -count=1
```

实际结果：

- 只有下界的查询期望返回 `old` 和 `new`，实际只返回 `old`。
- 只有上界的查询期望返回 `old` 和 `new`，实际只返回 `new`。

预期：

- 单边谓词不能凭空合成另一侧边界，除非 SQL 本身能推出该边界。

### 3. v1 清理 history 时保留了最老 interval，而不是 current/newest interval

受影响路径：

- `pkg/util/stmtsummary/statement_summary.go:476-489`

影响：

- 关闭 history 时，按语义应只保留当前 interval。
- 当前代码使用 `history.Front()`，但 history 最新元素在 `Back()`。
- 结果是保留了最老 interval。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1ClearHistoryKeepsCurrentInterval$' -count=1
```

### 4. v1 降低 history size 后返回最老 rows

受影响路径：

- `pkg/util/stmtsummary/statement_summary.go:702-719`
- `pkg/util/stmtsummary/evicted.go:212-219`

影响：

- `tidb_stmt_summary_history_size` 降低后，`STATEMENTS_SUMMARY_HISTORY` 应保留最近 interval。
- 当前从 `Front()` 开始收集，返回最老 interval。
- evicted history 也有相同方向问题。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1HistorySizeReductionKeepsNewestIntervals$' -count=1
```

### 5. v1 plan encoding 失败会导致 statement summary panic

受影响路径：

- `pkg/util/stmtsummary/statement_summary.go:617-635`
- `pkg/util/stmtsummary/statement_summary.go:725-731`
- `pkg/util/stmtsummary/statement_summary.go:764-768`

影响：

- `StmtExecLazyInfo.GetEncodedPlan()` 的第三个返回值表示 recover 到的 panic/error。
- `newStmtSummaryStats()` 在有 error 时返回 nil。
- 调用方立即解引用这个 nil。
- 结果是 statement summary 收集路径 panic。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1PlanEncodingFailureDoesNotPanic$' -count=1
```

预期：

- 应跳过 plan 字段、使用空/丢弃标记，或跳过该条 summary；不能 panic。

### 6. statement summary key 序列化缺少字段边界，可能合并不同 SQL 统计

受影响路径：

- `pkg/util/stmtsummary/statement_summary.go:62-82`
- v1/v2 共享 `StmtDigestKey`

影响：

- `StmtDigestKey` 直接拼接 `digest`、`schemaName`、`prevDigest`、`planDigest`、`resourceGroupName`，只有 `user` 做了长度前缀。
- `SimpleLRUCache` 只按 `string(key.Hash())` 做索引。
- 不同逻辑 key 可以生成完全相同的字节串，最终合并成一条 summary。

具体碰撞模式：

- 相同 SQL digest；
- A：非 point-get，`schema=""`，`planDigest=<64 字符串>`；
- B：point-get，`schema=<同一个 64 字符串>`，`planDigest=""`；
- 如果 `prevDigest`、resource group、user 相同，则 key 字节完全相同。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditStmtDigestKeyHasUnambiguousFieldBoundaries$' -count=1
```

### 7. v1/v2 缩小容量会直接丢 statement 统计，而不是合并到 `Others`

受影响路径：

- `pkg/util/kvcache/simple_lru.go:213-225`
- `pkg/util/stmtsummary/statement_summary.go:548-551`
- `pkg/util/stmtsummary/v2/stmtsummary.go:229-239`

影响：

- 调小 `tidb_stmt_summary_max_stmt_count` 时，`SimpleLRUCache.SetCapacity` 会删除超出容量的 entry。
- `SetCapacity` 不触发 `onEvict`。
- 被删记录不会合并到 `Others`，统计直接消失。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1CapacityReductionIsAccountedAsEviction$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2CapacityReductionIsAccountedAsEviction$' -count=1
```

### 8. v1/v2 `Others` 聚合遗漏 SQL 可见指标

受影响路径：

- v1：`pkg/util/stmtsummary/evicted.go:221-407`
- v2：`pkg/util/stmtsummary/v2/record.go:471-642`

影响：

`Others` 应保留被淘汰 row 的聚合指标，但多个 SQL 可见字段没有 merge，导致 `Others` 低报或显示默认值。

已确认示例：

- v1/v2：result rows、network traffic、storage flags 未保留。
- v2：memory arbitration 未保留。
- v1：plan-cache-unqualified count 和 last reason 未保留。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1OthersPreservesAllAggregateMetrics$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2OthersPreservesAllAggregateMetrics$' -count=1
```

### 9. v1/v2 `group_by_user` 不隔离 `Others`

受影响路径：

- v1：`pkg/util/stmtsummary/evicted.go`
- v2：`pkg/util/stmtsummary/v2/stmtsummary.go`、`pkg/util/stmtsummary/v2/reader.go`

影响：

- 开启 `tidb_stmt_summary_group_by_user=ON` 后，普通 row 会按 user 作为 key 的一部分。
- 但被淘汰的记录仍合并进全局 `Others`。
- 如果 Alice 和 Bob 都贡献过 `Others`，Alice 只要出现在 `Others` 的 auth set 中，就可能看到包含 Bob statement 的聚合延迟/次数。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1OthersDoesNotExposeOtherUsersMetrics$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2OthersDoesNotExposeOtherUsersMetrics$' -count=1
```

### 10. statement summary 用户身份忽略 host

受影响路径：

- `pkg/executor/adapter.go:2192-2198`
- `pkg/executor/adapter.go:2262-2274`
- v1：`pkg/util/stmtsummary/reader.go:142-146`
- v2：`pkg/util/stmtsummary/v2/reader.go:403-410`

影响：

- TiDB 账号身份是 user + host。
- statement summary 只记录 `sessVars.User.Username`。
- 例如 `'app'@'10.%'` 和 `'app'@'192.%'` 可能被合并，并在权限检查中被当成同一个 `app`。
- 开启 `group_by_user` 后影响更明显，因为该功能本意是按用户隔离 summary。

复现思路：

1. 创建两个 username 相同但 host pattern 不同的账号。
2. 分别执行不同 SQL。
3. 用其中一个非 `PROCESS` 账号查询 statement summary。
4. 观察 summary 身份和权限检查没有区分 host。

### 11. v1/v2 多个 AVG 请求耗时列使用错误分母

受影响列：

- `AVG_KV_TIME`
- `AVG_PD_TIME`
- `AVG_BACKOFF_TOTAL_TIME`
- `AVG_WRITE_SQL_RESP_TIME`

影响：

- 这些 sum 按每次执行累加。
- AVG 却除以 `CommitCount`。
- 只读语句或没有 commit detail 的语句，即使 sum 非零，也可能显示 0。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1AverageRequestDurationsUseExecutionCount$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditAverageRequestDurationsUseExecutionCount$' -count=1
```

预期：

- 使用 `ExecCount` 作为分母。

### 12. v1/v2 table names 格式可能留下多余逗号

受影响路径：

- v1：`pkg/util/stmtsummary/statement_summary.go:619-632`
- v2：`pkg/util/stmtsummary/v2/record.go:181-196`

影响：

- table names 使用原始 `StmtCtx.Tables` 的 index 决定是否加逗号。
- 空 table name 的 entry 会被跳过，但逗号判断仍基于原始 slice 长度。
- 混合 database-only entry 和 table entry 时，可能产生 `db.t,`。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2TableNamesHaveNoDanglingComma$' -count=1
```

### 13. v2 `tidb_stmt_summary_max_sql_length` 不限制 `DIGEST_TEXT`

受影响路径：

- `pkg/util/stmtsummary/v2/record.go:213-224`
- `pkg/util/stmtsummary/v2/record.go:644-660`

影响：

- v1 初始化 summary 时会格式化/截断 normalized SQL。
- v2 直接保存 `info.NormalizedSQL` 到 `StmtRecord.NormalizedSQL`。
- `DIGEST_TEXT` 可能超过 `tidb_stmt_summary_max_sql_length`，增加内存和 persistent log 大小。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2MaxSQLLengthAlsoCapsDigestText$' -count=1
```

### 14. v2 `Add()` 与 rotate/flush 之间可能丢一次执行统计

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:311-345`
- `pkg/util/stmtsummary/v2/stmtsummary.go:414-464`
- `pkg/util/stmtsummary/v2/logger.go:53-70`

影响：

- `Add()` 在选中/创建 record 后、执行 `record.Add(info)` 前释放 `windowLock`。
- `rotate()` 或 `flush()` 可在这期间替换并持久化旧 window。
- in-flight 这次执行既不在新 current，也可能不在已写出的 history。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2AddCannotLoseExecutionAcrossRotate$' -count=1
```

实际结果：

- 预期 persisted `exec_count=1`，实际 `exec_count=0`。

### 15. v2 `SetEnabled(false)` / `SetEnableInternalQuery(false)` 可被 in-flight `Add()` 绕过

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:196-220`
- `pkg/util/stmtsummary/v2/stmtsummary.go:311-345`

影响：

- v1 会在 map lock 下重新检查 enabled/internal 状态。
- v2 `Add()` 没有在 `windowLock` 下重新检查这些 flag。
- 在 sysvar 变更前已通过 caller-side check 的执行，可以在 disable/clear 后重新写入 window。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2(DisabledSummaryCannotBeRepopulatedByInflightAdd|InternalSummaryCannotBeRepopulatedAfterDisable)$' -count=1
```

### 16. 关闭 internal query 统计不会清掉已进入 `Others` 的 internal 贡献

受影响路径：

- v1：`pkg/util/stmtsummary/statement_summary.go:459-473`
- v2：`pkg/util/stmtsummary/v2/stmtsummary.go:374-388`

影响：

- `ClearInternal()` 只扫描当前 LRU。
- 已经进入 `Others` 的 internal query 聚合不会被清理。
- 之后 `SET GLOBAL tidb_stmt_summary_internal_query=OFF`，这些聚合仍保留。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1DisableInternalClearsInternalOthers$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2DisableInternalClearsInternalOthers$' -count=1
```

### 17. v1 `STATEMENTS_SUMMARY_EVICTED` 存在 data race

受影响路径：

- `pkg/util/stmtsummary/evicted.go:187-195`

影响：

- `ToEvictedCountDatum()` 遍历 evicted history 并读取 count 时，没有锁住 `stmtSummaryByDigestEvicted`。
- 并发 eviction 会修改同一份 history/count。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -race -run '^TestAuditV1EvictedCountConcurrentRead$' -count=1
```

### 18. v2 `STATEMENTS_SUMMARY_EVICTED` 存在 data race / 不一致 snapshot

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:350-365`

影响：

- `Evicted()` 加锁读 count 后释放锁，再读 `s.window.begin`。
- 并发 rotate 可替换 `s.window`。
- race detector 会报告 `s.window` 访问竞争。
- 即使不看 race，count 和 begin 也可能来自不同 window。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -race -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2EvictedReadIsRaceFree$' -count=1
```

### 19. v2 `MemReader` 可能返回 `SUMMARY_BEGIN_TIME > SUMMARY_END_TIME`

受影响路径：

- `pkg/util/stmtsummary/v2/reader.go:88-116`

影响：

- `MemReader.Rows()` 在拿 `windowLock` 前先计算 `end := timeNow().Unix()`。
- 然后加锁读取 `w.begin`。
- 如果时间或 window begin 在这之间变化，row 可出现 begin 晚于 end。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2MemReaderNeverReturnsBeginAfterEnd$' -count=1
```

### 20. v2 setup 失败处理不安全

受影响路径：

- `cmd/tidb-server/main.go:1345-1357`
- `pkg/util/stmtsummary/v2/stmtsummary.go:101-150`
- `pkg/util/stmtsummary/v2/stmtsummary.go:740-803`
- `pkg/util/stmtsummary/v2/logger.go:38-45`

影响：

- persistent statement summary setup 失败时，server 只打日志并继续启动。
- persistent 模式下，`Enabled()` / `Add()` 等 public wrapper 会直接解引用 `GlobalStmtSummary`。
- `newStmtLogStorage()` 遇到 logger 初始化错误时返回 NOP logger，setup 可能看起来成功，但实际不落盘。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2(PersistentWrapperHandlesSetupFailure|PersistentWrappersDoNotPanicWhenSetupFailed|NewStmtSummaryReportsLoggerInitFailure)$' -count=1
```

### 21. v2 history reader 对被 time range 排除的文件泄露 FD

受影响路径：

- `pkg/util/stmtsummary/v2/reader.go:549-599`

影响：

- `newStmtFiles()` 会先打开 candidate file。
- 如果文件与 pushdown time range 不重叠，函数返回时没有 close 该 file。
- 多次 history 查询会逐步泄露 FD，直到进程触达 OS 限制。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2HistoryRangeFilterClosesRejectedFiles$' -count=1
```

### 22. v2 history 文件选择可能读错文件

受影响路径：

- `pkg/util/stmtsummary/v2/reader.go:499-535`
- `pkg/util/stmtsummary/v2/reader.go:549-599`

影响：

- 文件匹配使用 `strings.HasPrefix(path, prefix)`，而且基于 full path。
- rotation end 解析时，用 base file name 和可能包含目录的 prefix 比较。
- 已确认后果：
  - 配置绝对路径时，rotated-file end time 可能解析为 `0`，导致剪枝失效。
  - `tidb-statements2.log` 这类 sibling 文件会被误读成 `tidb-statements.log` 的 history。
  - 无扩展名 backup name 可能把 timestamp suffix `.000` 当成 extension。
  - 空或非法 filename 会使 prefix 匹配过宽。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2(AbsoluteFilenameParsesRotationEnd|HistoryFileSelectionDoesNotReadSiblingPrefixes)$' -count=1
```

### 23. v2 history reader 可能在遇到新记录后提前停止，跳过合法旧记录

受影响路径：

- `pkg/util/stmtsummary/v2/reader.go:670-714`
- `pkg/util/stmtsummary/v2/reader.go:431-443`

影响：

- scanner 如果看到第一条 parsed line 的 `begin` 大于所有查询 range end，就停止读取该文件。
- 这个优化假设文件中的记录按 begin time 单调递增。
- v2 persist 是异步写，可能乱序；手工/异常文件也可能乱序。
- 文件顶部一条较新的记录可能导致后续合法旧记录被跳过。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2HistoryReaderDoes(NotLoseOutOfOrderLines|StopBeforeOutOfOrderOlderRecord)$' -count=1
```

### 24. v2 rotate 持久化没有序列化和背压

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:423-464`

影响：

- 每次 rotate 都创建一个 goroutine 持久化完成的 window。
- 没有序列化，也没有队列上限。
- refresh interval 很小、IO 慢或 window 很大时，goroutine 和待写 window 会堆积。
- 也会加剧 issue 23 中 out-of-order 历史日志的问题。

复现思路：

1. 设置很小的 refresh interval。
2. 在 package test 中使用慢/阻塞的 `stmtStorage.persist`。
3. 触发多次 rotate。
4. 观察多个 persist goroutine/window outstanding。

### 25. v2 history 查询合并 memory 和 disk 时没有一致性 snapshot

受影响路径：

- `pkg/executor/stmtsummary.go:257-272`
- `pkg/util/stmtsummary/v2/stmtsummary.go:454-464`

影响：

- v2 `STATEMENTS_SUMMARY_HISTORY` 先读 memory，再打开 persistent files。
- 这两步之间可能发生 rotate。
- 某个 window 可能漏掉，也可能重复。

复现思路：

1. 在 `mem.Rows()` 后、`NewHistoryReader()` 前阻塞查询。
2. 触发 rotate 并持久化当前 window。
3. 恢复查询。
4. 检查该 window 出现 0/1/2 次。

### 26. v2 rotate/flush 在 LRU 为空但 `Others` 有数据时跳过持久化

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:414-464`

影响：

- `rotate()` / `flush()` 只在 `window.lru.Size() > 0` 时持久化。
- 它们忽略 `window.evicted.otherForPersist.ExecCount`。
- 如果 current LRU 为空，但 `Others` 中仍有需要持久化的数据，该 window 会被跳过。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2RotatePersistsOthersWhenLRUIsEmpty$' -count=1
```

### 27. v2 `persist_evicted=ON` 可能导致 built-in history 丢失被淘汰执行

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:469-486`
- `pkg/util/stmtsummary/v2/stmtsummary.go:660-670`
- `pkg/util/stmtsummary/v2/reader.go:765-774`

影响：

- 单条 eviction 成功入队时，v2 不会把它 merge 到 `otherForPersist`。
- history reader 会跳过 `"evicted": true` 的记录。
- 因此开启 per-record evicted logging 后，内置 `STATEMENTS_SUMMARY_HISTORY` 可能看不到这些被淘汰执行。

复现思路：

1. 开启 persistent mode 和 `tidb_stmt_summary_persist_evicted=ON`。
2. 设置 capacity 为 1。
3. 执行两个不同 digest，淘汰第一个。
4. rotate/flush。
5. 查询 `STATEMENTS_SUMMARY_HISTORY`。
6. 被淘汰 digest 不返回。

### 28. v2 超大 statement-summary log write 会被静默丢失

受影响路径：

- `pkg/util/stmtsummary/v2/logger.go:53-118`
- `gopkg.in/natefinch/lumberjack.v2`

影响：

- 单条 statement record 可包含最多 1 MiB encoded plan 和 1 MiB binary plan。
- `logEvicted()` 最多把 64 条 record 合并成一个 zap message。
- 如果单次 write 超过 lumberjack 的 file max size，zap 内部记录错误，但当前代码无法感知。
- metric 仍按 persisted 增加。
- 如果 `persist_evicted=ON`，该 record 也可能被排除在 `otherForPersist` 之外，导致 built-in history 也丢失。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2PersistEvictedDoesNotLoseRecordsWhenLogWriteExceedsMaxSize$' -count=1
```

### 29. v2 evicted-key tracking 不受 `max_stmt_count` 限制

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:642-674`

影响：

- v2 `stmtEvicted.keys` 保存当前 refresh window 中所有 distinct evicted key。
- 这个 map 不受 `tidb_stmt_summary_max_stmt_count` 限制。
- 大量 distinct digest 工作负载下，即使 summary capacity 很小，也会保留很大的 map。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2EvictedKeyTrackingDefeatsCapacityBound$' -count=1
```

实际结果：

- capacity=1，10,000 个 distinct digest -> 1 条 LRU row + 9,999 个 evicted key。

### 30. v2 evicted log channel 只按 record 数限流，不按字节限流

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:42-49`
- `pkg/util/stmtsummary/v2/stmtsummary.go:469-486`

影响：

- `evictedCh` capacity 是 1024 records。
- 每个 clone record 可能包含大 SQL、encoded plan、binary plan。
- 即使 channel 长度有上限，排队内存仍可能很大。

复现思路：

1. 开启 `persist_evicted`。
2. 使用阻塞的 `stmtStorage.logEvicted`。
3. capacity=1，持续加入超过 1024 条大 plan distinct statement。
4. 观察 queued `StmtRecord` clone 占用的内存。

### 31. v2 history reader 单次查询可能占用过多内存和 FD

受影响路径：

- `pkg/util/stmtsummary/v2/reader.go:192-223`
- `pkg/util/stmtsummary/v2/reader.go:320-360`
- `pkg/util/stmtsummary/v2/reader.go:549-599`
- `pkg/util/stmtsummary/v2/reader.go:856-861`

影响：

- `NewHistoryReader()` 构造时会打开所有匹配文件。
- SQL `LIMIT` 不能降低这部分 upfront FD 成本。
- `maxLineSize` 是 1 GiB。
- `linesCh` 按 batch=64 缓冲，channel 容量和 concurrency 相关。
- 大 persisted record 可造成高瞬时内存占用。

复现思路：

1. 创建大量 rotated statement log 文件。
2. 查询 `STATEMENTS_SUMMARY_HISTORY LIMIT 1`。
3. 观察返回第一行前已打开大量文件。
4. 使用大 JSON line 观察瞬时内存。

### 32. v2 `Others` 的 first/last seen 时间不是从执行时间初始化

受影响路径：

- `pkg/util/stmtsummary/v2/stmtsummary.go:676-684`
- `pkg/util/stmtsummary/v2/record.go:626-630`

影响：

- `newEvictedAggregateRecord()` 把 `FirstSeen` / `LastSeen` 初始化为 `time.Now()`。
- merge 较旧的 evicted record 时，`LastSeen` 可能仍是 aggregate 创建/eviction 的墙钟时间，而不是最后一次执行时间。

复现：

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2OthersLastSeenMatchesLastExecution$' -count=1
```

### 33. v1/v2 evicted 表时间戳忽略 session timezone

受影响路径：

- v1：`pkg/util/stmtsummary/evicted.go:198-205`
- v2：`pkg/util/stmtsummary/v2/stmtsummary.go:350-365`

影响：

- 普通 statement summary 时间列会按 reader 的 session timezone 转换。
- evicted-count row 直接使用 `time.Unix(...)` / `time.Now()` 构造 timestamp。
- 非系统时区 session 中，summary 表和 evicted 表时间显示策略不一致。

复现思路：

1. 设置 session `time_zone` 为不同于 server local time 的值。
2. 触发 eviction。
3. 比较 `STATEMENTS_SUMMARY_EVICTED.BEGIN_TIME` 和 `STATEMENTS_SUMMARY.SUMMARY_BEGIN_TIME`。

### 34. `Others` 的 resource group 归属/filter 行为不正确

受影响路径：

- v1：`pkg/util/stmtsummary/evicted.go:404-406`
- v2：`pkg/util/stmtsummary/v2/record.go:471-642`

影响：

- 普通 row 以 resource group 作为 key 的一部分。
- `Others` 会聚合被淘汰记录。
- v1 merge 时用最后一个 evicted record 覆盖 `resourceGroupName`。
- v2 aggregate resource group 为空。
- 按 resource group 查询时，会漏掉或误归属 evicted stats。

复现思路：

1. 在两个 resource group 中运行不同 SQL，capacity=1。
2. 强制 eviction。
3. 带 resource group filter 查询 statement summary。
4. 对比普通 row 和 `Others`。

## 需要产品/兼容性确认的问题

### C1. v2 `STATEMENTS_SUMMARY_EVICTED` 只返回当前周期

v1 会返回最多 history size 个历史 evicted-count row。v2 只返回当前 window 一行。这是 information_schema 表的兼容性差异。

### C2. v2 persistent 模式下 `tidb_stmt_summary_history_size` 是 no-op

`SetHistorySize()` 在 persistent mode 下直接返回 nil。sysvar 仍可以被设置，但 v2 history 保留由日志文件保留配置控制。这个行为可能合理，但需要明确文档化。

### C3. v2 window 边界不像 v1 一样按墙钟对齐

v1 interval begin 会按 refresh interval 对齐。v2 使用 window 创建/rotate 时的 `timeNow()`。多 TiDB 节点的 cluster summary window 可能按进程启动/rotate 时间错开，会影响 dashboard 对比。

### C4. `persist_evicted=ON` 也许设计上就是给离线 raw evicted logs 使用

如果设计预期是“raw evicted record 只供离线消费者使用”，那么 issue 27 应转为文档问题。如果用户预期 `STATEMENTS_SUMMARY_HISTORY` 始终完整，则这是正确性 bug。

## 被反证或降级的问题

- `digest IS NULL` 查询：已有测试覆盖。digest extractor 不会错误移除 `IS NULL`；最终 SQL filter 会处理。
- 精确边界上的 inclusive time overlap：虽然 `StmtTimeRange` 注释写 `[Begin, End)`，但 SQL extractor 使用的是 `summary_begin_time <= end AND summary_end_time >= start` 这种闭区间重叠语义。它可能多读文件，但没有证明会返回错误 row。
- `CacheStmtExecInfo` 复用污染：明显未重置的 `ExecRetryTime` 只有在 `ExecRetryCount > 0` 时才被消费，未找到具体跨 statement 污染 bug。
- zap/lumberjack 字节交错：未发现 JSON 字节交错证据。真实问题是写失败、顺序和超大 write，已在 issue 28 覆盖。
- `HistoryReader.Close()` 顺序：看起来不理想，但未发现确定性 deadlock。FD 泄露点在 time range reject 文件未 close，不在正常 close。

## 历史复现命令汇总

以下命令均在 synthetic worktree `/tmp/tidb-stmtsummary-audit.hdkvdB` 中执行。`TestAudit*` 是临时测试，在被审查代码上预期失败。

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAudit' -count=1
```

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -race -run '^TestAuditV1EvictedCountConcurrentRead$' -count=1
```

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAudit' -count=1
```

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -race -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2EvictedReadIsRaceFree$' -count=1
```

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -run '^TestAuditV2(SQLCumulativeTable(SupportedColumn)?DoesNotPanic|SingleSided(Lower|Upper)TimePredicateDoesNotLoseRows)$' -count=1
```

额外 sanity check：

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -run '^TestStmtSummary' -count=1
```

结果：现有 v2 current/history SQL tests 通过，因此 SQL panic 范围可以限定为 cumulative 表路由，而不是全部 v2 statement summary 表。

## 历史建议 issue 拆分顺序

如果要创建 GitHub issue，建议按根因拆分：

1. v2 cumulative 表路由 panic。
2. v2 时间谓词 false negative。
3. 共享 key 序列化碰撞。
4. v1 history 顺序 / clearHistory 缺陷。
5. `SimpleLRU.SetCapacity` 缩容丢 row。
6. `Others` 聚合完整性、用户隔离、resource group 隔离。
7. v2 `Add` / rotate / flush 数据丢失 race。
8. v1/v2 evicted 表 race。
9. v2 history reader FD、文件匹配、out-of-order 问题。
10. v2 persistence error handling 和超大 write 丢失。
11. v2 资源边界：无界 persist goroutine、evicted queue 字节数、history reader 内存/FD。
