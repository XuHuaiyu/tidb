# Statement Summary v1/v2 audit 可运行测试记录

日期：2026-07-23

这份文档用于补足 audit 报告里的 `TestAudit*` 复现信息。当前这些测试都放在 TiDB 工作区源码里，并使用 `audit` build tag 隔离；普通测试不会默认执行它们。

测试文件：

- `pkg/planner/core/audit_stmtsummary_reproducer_test.go`
- `pkg/util/stmtsummary/audit_reproducer_test.go`
- `pkg/util/stmtsummary/v2/audit_reproducer_test.go`
- `pkg/util/stmtsummary/v2/tests/audit_reproducer_test.go`

当前分类：

- 已记录 42 个可运行 `TestAudit*` 测试。
- 39 个是 confirmed bug 复现测试。
- 2 个有可运行测试，但提交前需要标注产品语义或触发限制。
- 1 个是反证/降级用例，当前通过。

## 运行命令模板

从仓库根目录 `/home/xhy/Development/github.com/pingcap/tidb` 执行。

v1 statement summary 测试需要 failpoint wrapper：

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -tags=intest,deadlock,audit -run '^<TEST_NAME>$' -count=1
```

v1 race 测试：

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -tags=intest,deadlock,audit -race -run '^<TEST_NAME>$' -count=1
```

v2 statement summary 单元测试：

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/util/stmtsummary/v2 \
  -run '^<TEST_NAME>$' -count=1
```

v2 race 测试：

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit -race ./pkg/util/stmtsummary/v2 \
  -run '^<TEST_NAME>$' -count=1
```

v2 SQL / information_schema 测试需要 failpoint wrapper：

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -tags=intest,deadlock,audit -run '^<TEST_NAME>$' -count=1
```

planner extractor 测试：

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/planner/core \
  -run '^<TEST_NAME>$' -count=1
```

## 可直接作为 confirmed bug 提交的问题

| ID | 问题 | 测试源码 | 命令模板 | 当前失败摘要 | 触发条件 |
| --- | --- | --- | --- | --- | --- |
| S1 | v2 persistent 模式下 `INFORMATION_SCHEMA.TIDB_STATEMENTS_STATS` 的 `SELECT *` panic | `pkg/util/stmtsummary/v2/tests/audit_reproducer_test.go:26` `TestAuditV2SQLCumulativeTableDoesNotPanic` | v2 SQL | panic：`should never happen, should register new column ERRORS into columnValueFactoryMap` | 开启 persistent statement summary 后查询 cumulative 表全列 |
| S2 | v2 persistent 模式下 cumulative 表即使只查支持列也 panic | `pkg/util/stmtsummary/v2/tests/audit_reproducer_test.go:41` `TestAuditV2SQLCumulativeTableSupportedColumnDoesNotPanic` | v2 SQL | nil pointer：`executor.(*rowsReader).pull` | 开启 persistent 后查询 `information_schema.tidb_statements_stats` 的普通列 |
| P1 | statement summary 单边时间谓词被错误压成 1 小时窗口 | `pkg/planner/core/audit_stmtsummary_reproducer_test.go:26` `TestAuditStatementsSummarySingleSidedTimePredicateStaysOpenEnded` | planner | `SUMMARY_END_TIME >= start` 被收窄为 `[start, start+1h]` | v2 history 查询只带上界或下界谓词 |
| V1-1 | v1 用户可见性只按 username，不区分 host | `pkg/util/stmtsummary/audit_reproducer_test.go:54` `TestAuditV1SameUsernameDifferentHostsShouldNotShareVisibility` | v1 | `alice@host2` 能看到 `alice@host1` 的 summary | 同 username 不同 host 的账号，且无 PROCESS 权限 |
| V1-2 | v1 clear history 保留最老周期而不是当前/最新周期 | `pkg/util/stmtsummary/audit_reproducer_test.go:71` `TestAuditV1ClearHistoryKeepsCurrentInterval` | v1 | 期望 begin=200，实际 begin=100 | 关闭 history 或清理 history |
| V1-3 | v1 降低 history size 后返回最老周期 | `pkg/util/stmtsummary/audit_reproducer_test.go:89` `TestAuditV1HistorySizeReductionKeepsNewestIntervals` | v1 | 期望保留 begin=200，实际保留 begin=100；evicted history 同样失败 | 下调 `tidb_stmt_summary_history_size` |
| V1-4 | v1 plan encoding 失败会 panic | `pkg/util/stmtsummary/audit_reproducer_test.go:108` `TestAuditV1PlanEncodingFailureDoesNotPanic` | v1 | nil pointer panic，调用栈落在 `statement_summary.go` 的 summary stats 解引用 | `StmtExecLazyInfo.GetEncodedPlan()` 返回 recover/error |
| V1-5 | v1 缩小容量时被移除记录没有合并到 `Others` | `pkg/util/stmtsummary/audit_reproducer_test.go:128` `TestAuditV1CapacityReductionIsAccountedAsEviction` | v1 | 缩到 1 后找不到 `Others` 行，只剩 survivor digest | 运行中下调 `tidb_stmt_summary_max_stmt_count` |
| V1-6 | v1 `Others` 聚合遗漏 SQL 可见指标 | `pkg/util/stmtsummary/audit_reproducer_test.go:148` `TestAuditV1OthersPreservesAllAggregateMetrics` | v1 | `MAX_RESULT_ROWS` 期望 42，实际 0 | digest 被 LRU 淘汰并合并到 `Others` |
| V1-7 | v1 `group_by_user` 下 `Others` 不按用户隔离 | `pkg/util/stmtsummary/audit_reproducer_test.go:169` `TestAuditV1OthersDoesNotExposeOtherUsersMetrics` | v1 | alice 只应看到 1 次，实际看到 2 次，包含 bob 的 evicted 贡献 | 开启 `tidb_stmt_summary_enable_group_by_user` 且不同用户 digest 被淘汰 |
| V1-8 | v1 请求耗时 AVG 列分母错误 | `pkg/util/stmtsummary/audit_reproducer_test.go:197` `TestAuditV1AverageRequestDurationsUseExecutionCount` | v1 | `AVG_KV_TIME` 期望 100ms，实际 0 | 查询 KV/PD/backoff/request 相关 AVG 列，且 commit_count 为 0 |
| V1-9 | v1 table names 字符串会残留尾逗号 | `pkg/util/stmtsummary/audit_reproducer_test.go:214` `TestAuditV1TableNamesHaveNoDanglingComma` | v1 | 期望 `db.t`，实际 `db.t,` | statement summary 行里包含 table name |
| V1-10 | v1 关闭 internal query 后，已进入 `Others` 的 internal 贡献仍可见 | `pkg/util/stmtsummary/audit_reproducer_test.go:225` `TestAuditV1DisableInternalClearsInternalOthers` | v1 | 关闭 internal 后仍有空 digest 的 `Others` 行 | internal query 被淘汰进 `Others` 后再关闭 internal collection |
| V1-11 | v1 `STATEMENTS_SUMMARY_EVICTED` 读取存在 data race | `pkg/util/stmtsummary/audit_reproducer_test.go:276` `TestAuditV1EvictedCountReaderIsRaceFree` | v1 race | race：`evicted.go:189` 读 list 长度，与 `evicted.go:100` `PushFront` 写冲突 | 并发查询 evicted 表和 statement summary LRU 淘汰 |
| V2-1 | v2 用户可见性只按 username，不区分 host | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:94` `TestAuditV2SameUsernameDifferentHostsShouldNotShareVisibility` | v2 | `alice@host2` 能看到 `alice@host1` 的 summary | 同 username 不同 host 的账号，且无 PROCESS 权限 |
| V2-2 | v2 persistent summary 关闭后仍接受新 statement | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:110` `TestAuditV2DisabledSummaryDoesNotAcceptNewStatements` | v2 | `SetEnabled(false)` 后仍能看到 `digest-added-after-disable` | 通过变量/API 关闭 persistent statement summary 后又新增 statement |
| V2-3 | v2 internal query collection 关闭后仍接受新 internal statement | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:123` `TestAuditV2DisabledInternalQueryDoesNotAcceptNewInternalStatements` | v2 | internal collection 关闭时仍能看到 `digest-internal-added-while-disabled` | persistent summary 开启、internal collection 关闭后新增 internal statement |
| V2-4 | v2 缩小容量时被移除记录没有合并到 `Others` | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:138` `TestAuditV2CapacityReductionIsAccountedAsEviction` | v2 | 缩到 1 后找不到 `Others` 行 | 运行中下调 `tidb_stmt_summary_max_stmt_count` |
| V2-5 | v2 `Others` 聚合遗漏 SQL 可见指标 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:154` `TestAuditV2OthersPreservesAllAggregateMetrics` | v2 | `AVG_MEM_ARBITRATION` 期望 42，实际 0 | digest 被 LRU 淘汰并合并到 `Others` |
| V2-6 | v2 `group_by_user` 下 `Others` 不按用户隔离 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:170` `TestAuditV2OthersDoesNotExposeOtherUsersMetrics` | v2 | alice 只应看到 1 次，实际看到 2 次 | 开启 group_by_user 且不同用户 digest 被淘汰 |
| V2-7 | v2 请求耗时 AVG 列分母错误 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:193` `TestAuditAverageRequestDurationsUseExecutionCount` | v2 | `AVG_KV_TIME` 期望 100ms，实际 0 | 查询 KV/PD/backoff/request 相关 AVG 列，且 commit_count 为 0 |
| V2-8 | v2 `MemReader` 可能返回 begin > end | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:209` `TestAuditV2MemReaderNeverReturnsBeginAfterEnd` | v2 | `SUMMARY_BEGIN_TIME` 大于 `SUMMARY_END_TIME` | 系统时间回拨或测试注入 `timeNow` 倒退 |
| V2-9 | v2 table names 字符串会残留尾逗号 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:227` `TestAuditV2TableNamesHaveNoDanglingComma` | v2 | 期望 `db.t`，实际 `db.t,` | statement summary 行里包含 table name |
| V2-10 | v2 `tidb_stmt_summary_max_sql_length` 不限制 `DIGEST_TEXT` | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:237` `TestAuditV2MaxSQLLengthAlsoCapsDigestText` | v2 | digest text 仍为完整 `xxxxxxxxxx`，没有截断为 `xxxx(len:10)` | 设置较小 `MaxSQLLength` 后读取 `DIGEST_TEXT` |
| V2-11 | v2 logger 初始化失败没有向调用方返回错误 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:252` `TestAuditV2NewStmtSummaryReportsLoggerInitFailure` | v2 | filename 是目录时，期望返回 error，实际 nil error + nop logger | persistent logger 文件路径不可创建/不可写 |
| V2-12 | v2 rotate 持久化没有序列化/背压 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:261` `TestAuditV2RotatePersistenceShouldBeSerialized` | v2 | 并发 persist 最大 in-flight 为 2，期望不超过 1 | rotate 很频繁或持久化 I/O 慢 |
| V2-13 | v2 history 查询 memory+disk 没有一致性 snapshot，可能双计 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:283` `TestAuditV2HistoryQueryMustNotDoubleCountWindowAcrossMemAndDisk` | v2 | 同一窗口 total exec_count 期望 1，实际 2 | 查询过程中 current window 被 flush 到历史文件 |
| V2-14 | v2 超大 statement summary log 写失败被静默丢失 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:329` `TestAuditV2PersistDoesNotSilentlyLoseOversizedRecords` | v2 | 文件只留下 write error 行，没有可恢复 digest；调用方也拿不到错误 | 单条 encoded record 超过最大 log line size/文件写入限制 |
| V2-15 | v2 rotate/flush 在 LRU 空但 `Others` 有数据时跳过持久化 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:348` `TestAuditV2RotatePersistsOthersWhenLRUIsEmpty` | v2 | 期望持久化 1 个窗口，实际 0 | 当前窗口只有 evicted aggregate，没有 live LRU record |
| V2-16 | v2 关闭 internal query 后，已进入 `Others` 的 internal 贡献仍可见 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:365` `TestAuditV2DisableInternalClearsInternalOthers` | v2 | 关闭 internal 后仍有空 digest 的 `Others` 行 | internal query 被淘汰进 `Others` 后再关闭 internal collection |
| V2-17 | v2 history reader 对被时间范围排除的文件泄露 FD | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:385` `TestAuditV2HistoryRangeFilterClosesRejectedFiles` | v2 | FD 数量明显增加，超过基线 + 1 | history 查询扫描到很多 rotated files，但大部分被 time range 排除 |
| V2-18 | v2 history 文件选择会读到同前缀 sibling 文件 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:405` `TestAuditV2HistoryFileSelectionDoesNotReadSiblingPrefixes` | v2 | 选择了 2 个文件，包括 `tidb-statements-extra.log` | 配置文件名旁边存在相同前缀的其他文件 |
| V2-19 | v2 使用绝对路径时 rotated filename 时间解析失败 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:421` `TestAuditV2AbsoluteFilenameParsesRotationEnd` | v2 | 期望解析 end timestamp，实际 0 | `stmt-summary-filename` 是绝对路径 |
| V2-20 | v2 history reader 遇到过新的记录会提前 EOF，跳过后续合法旧记录 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:435` `TestAuditV2HistoryReaderDoesNotStopBeforeOutOfOrderOlderRecord` | v2 | 第一行 too-new 后直接 EOF，后续合法 older row 没被读到 | log 文件内部记录不是严格按时间排序 |
| V2-21 | v2 history reader 一次打开所有匹配文件 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:453` `TestAuditV2HistoryReaderShouldNotOpenAllMatchingFilesAtOnce` | v2 | 10 个匹配文件全部打开，期望流式打开少量文件 | history 文件很多的长期运行集群 |
| V2-22 | v2 evicted key tracking 不受 `max_stmt_count` 约束 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:471` `TestAuditV2EvictedKeyTrackingDefeatsCapacityBound` | v2 | evicted key 数 99，大于 max_stmt_count=1 | 一个周期内大量不同 digest 被淘汰进 `Others` |
| V2-23 | v2 `Others.LastSeen` 不是执行时间 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:483` `TestAuditV2OthersLastSeenMatchesLastExecution` | v2 | `Others.LastSeen` 用当前墙钟初始化，不等于 evicted SQL 执行时间 | evicted record 第一次合并到 `Others` |
| V2-24 | v2 `Others` resource group 归属丢失 | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:500` `TestAuditV2OthersResourceGroupShouldRepresentEvictedGroup` | v2 | 期望 `rg_audit`，实际空字符串 | 同一 resource group 的 digest 被淘汰进 `Others` |
| V2-25 | v2 `STATEMENTS_SUMMARY_EVICTED` 读取存在 data race | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:523` `TestAuditV2EvictedCountReaderIsRaceFree` | v2 race | race：`stmtsummary.go:361` 读 `s.window`，与测试中模拟 rotate 写 `s.window` 冲突 | 并发查询 evicted 表和窗口 rotate |

## 有测试，但提交前需要标注触发限制或产品语义

| ID | 问题 | 测试源码 | 当前结果 | 提交建议 |
| --- | --- | --- | --- | --- |
| C1 | `StmtDigestKey` 字段边界不明确 | `pkg/util/stmtsummary/audit_reproducer_test.go:119` `TestAuditStmtDigestKeyHasUnambiguousFieldBoundaries` | 当前失败：两个不同逻辑 tuple 的 `Hash()` 都是 `digestabc` | 可以提交为 key encoding 健壮性问题；但 issue 里需要说明这是 unit-level collision，真实 SQL digest/plan digest 是否能构造出同等碰撞需要开发者确认 |
| C2 | v2 `persist_evicted=ON` 时 built-in history 看不到 per-record evicted execution | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:307` `TestAuditV2PersistEvictedShouldRemainVisibleInBuiltInHistory` | 当前失败：期望 total exec_count=2，实际 1 | 只有当产品语义要求 built-in history 覆盖 evicted execution 时才作为 bug；如果设计是“per-record evicted log 仅供离线采集”，应改为文档/语义确认 |

## 已反证或不建议按 confirmed bug 提交

| ID | 候选问题 | 测试源码 | 当前结果 | 结论 |
| --- | --- | --- | --- | --- |
| R1 | v1 单一 resource group 下 `Others` resource group 一定丢失 | `pkg/util/stmtsummary/audit_reproducer_test.go:249` `TestAuditV1OthersResourceGroupShouldRepresentEvictedGroup` | 当前通过 | 当前代码下，v1 在“所有 evicted rows 都来自同一 resource group”的条件下可以保留 resource group。不能按这个条件提交 v1 bug；若要继续查，需要构造多 resource group filter/展示语义问题。 |

## 尚未升级为 bug 的观察项

这些不要按 confirmed bug 提交，除非补上稳定复现测试或开发者确认产品语义：

- v2 `Add()` 与 rotate/flush 之间丢一次执行统计：代码上存在 lock gap，但目前没有无需插桩的稳定失败测试。建议如果要提交，先加 failpoint/hook 让 `Add` 卡在选中 record 后、`record.Add` 前。
- v2 evicted log channel 只按 record 数限流、不按字节限流：这是资源风险，缺少明确字节预算/产品约束；不建议按 correctness bug 提交。
- evicted 表时间戳是否应使用 session timezone：需要先确认 SQL 类型/展示语义；当前没有稳定复现测试。

## 触发现实性分类

提 issue 时按这个分类使用：

- 可以直接按 confirmed bug 提交：上面 “可直接作为 confirmed bug 提交的问题” 表里的所有行。这些测试都走到了可达代码路径，包括 unit、reader、SQL/testkit 或 race test。部分问题需要特定但真实的条件，例如调整容量、开启 persistent mode、history 文件很多、系统时间回拨、单条记录过大、并发查询等。
- 可以提交，但必须标注限制：C1 和 C2。它们都有可运行测试，但 issue 里必须说明限制。C1 当前是 unit-level key-boundary collision，真实 SQL digest/plan digest 是否能形成同等碰撞需要开发者确认。C2 取决于产品语义：built-in history 是否应该包含 `persist_evicted=ON` 的 per-record evicted execution。
- 不要按 confirmed bug 提交：R1 和 “尚未升级为 bug 的观察项”。R1 是反证用例，说明 v1 单一 resource group 的这个精确场景当前通过。观察项是代码审查风险或产品问题，需要补确定性插桩测试或 owner 确认后才能升级。

## 已实际运行并确认的命令

新增/关键用例已经在当前工作区验证为预期失败：

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/planner/core \
  -run '^TestAuditStatementsSummarySingleSidedTimePredicateStaysOpenEnded$' -count=1
```

结果：失败，`SUMMARY_END_TIME >= start` 被收窄到 `[start, start+1h]`。

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -tags=intest,deadlock,audit -race -run '^TestAuditV1EvictedCountReaderIsRaceFree$' -count=1
```

结果：失败，race detector 报告 `evicted.go:189` 与 `evicted.go:100` 并发读写。

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit -race ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2EvictedCountReaderIsRaceFree$' -count=1
```

结果：失败，race detector 报告 `stmtsummary.go:361` 读取 `s.window` 与并发写冲突。

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/util/stmtsummary/v2 \
  -run '^(TestAuditV2DisabledSummaryDoesNotAcceptNewStatements|TestAuditV2DisabledInternalQueryDoesNotAcceptNewInternalStatements)$' -count=1
```

结果：失败，`SetEnabled(false)` / `SetEnableInternalQuery(false)` 之后新增的行仍然可见。

其他测试已按上面的分组命令批量运行过，失败摘要记录在表格的“当前失败摘要”列中。
