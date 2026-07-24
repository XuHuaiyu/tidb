# Statement Summary v1/v2 audit report

Date: 2026-07-23

> Update: runnable `TestAudit*` reproducers are now recorded in the current workspace, not only in the old temporary `/tmp` worktree. For filing issues or sending findings to developers, use `statement-summary-v1-v2-audit-test-matrix.md` and `statement-summary-v1-v2-audit-test-matrix.zh.md` as the source of truth. They record each test's source path, line number, run command, current failure summary, and whether it should be filed as a confirmed bug. The synthetic worktree paths and some older test names in this report are retained only as historical audit notes.

Audited code:

- Base: `origin/master` = `ed2376acc6e0feeff9f3e2c38db489727933aa80`
- PR under review: pingcap/tidb#69914, head = `46dc2b36c215a7430451e450ff879f211e98b083`
- Synthetic audit tree: PR #69914 merged over current `origin/master`, temporary worktree `/tmp/tidb-stmtsummary-audit.hdkvdB`

Scope:

- v1: `pkg/util/stmtsummary`
- v2 persistent statement summary: `pkg/util/stmtsummary/v2`
- SQL routing and information-schema readers: `pkg/executor/stmtsummary.go`, `pkg/planner/core/memtable_predicate_extractor.go`, startup/sysvar wiring

The original audit separated:

- Known PR #69914 issue: already reported in GitHub review comments.
- Newly confirmed issues: actionable bugs found in this audit.
- Compatibility/product decisions: real behavioral differences or risks, but may need owner decision before filing as bugs.
- Rejected candidates: checked and not treated as bugs.

Temporary audit tests were added only in the synthetic worktree. They are intentionally failing tests that assert the expected behavior. They should not be committed as-is; use them as reproducer sketches when filing issues or writing regression tests.

## Reviewer convergence

Review passes completed:

- v1 semantics pass: history, eviction, `Others`, authorization, capacity changes.
- v2 semantics/persistence pass: add/rotate, file persistence, history reader, options.
- cross-cutting pass: races, goroutines, memory/FD ownership, key encoding, SQL-visible parity.
- final persistence/resource sweep: no additional independent root cause found; it independently confirmed the v2 `Add` versus rotate/persist data-loss path and the reader/resource risks.

The final SQL-route sweep agent did not return in time. I replaced it with local targeted checks over `pkg/executor/stmtsummary.go`, `pkg/util/stmtsummary/v2/reader.go`, `pkg/util/stmtsummary/v2/column.go`, and existing v2 SQL tests. Current/history `SELECT *` coverage already exists and passes; the cumulative-table route remains the isolated SQL-route breakage.

## Known issue already reported on PR #69914

### K1. v2 `Add()` can repopulate internal-query records after `ClearInternal()`

Status: already reported in PR review comments:

- https://github.com/pingcap/tidb/pull/69914#discussion_r3637062583
- https://github.com/pingcap/tidb/pull/69914#discussion_r3637062594

Root cause:

- `Add()` checks whether a statement is internal before selecting the record.
- It releases `windowLock` before `record.Add(info)`.
- `SetEnableInternalQuery(false)` can clear current internal rows in the middle.
- The in-flight `Add()` can then update the old record after the clear.

This report does not count K1 as a new finding, but several new issues below have the same lock-gap shape.

## Historical issue notes from the original audit

Do not use this section as the current filing list. It predates the restored runnable tests and still contains some old temporary test names and candidate analyses. The current source of truth for filing is the confirmed table in `statement-summary-v1-v2-audit-test-matrix.md`; candidates without stable tests are explicitly downgraded there.

### 1. v2 persistent mode breaks `INFORMATION_SCHEMA.TIDB_STATEMENTS_STATS`

Affected:

- `pkg/executor/stmtsummary.go`
- `pkg/util/stmtsummary/v2/column.go`

Impact:

- When `tidb_stmt_summary_enable_persistent` is enabled, querying `information_schema.tidb_statements_stats` can panic.
- There are two independent failure modes:
  - `SELECT *` asks v2 to build columns such as `ERRORS`, but v2 has no factory for cumulative-table-only columns.
  - `SELECT digest` uses only a supported column, but the executor never initializes a rows reader for cumulative tables in the v2 path, so it hits a nil pointer.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -run '^TestAuditV2SQLCumulativeTable(SupportedColumn)?DoesNotPanic$' -count=1
```

Observed:

- `SELECT * FROM information_schema.tidb_statements_stats LIMIT 1` panics with `should never happen, should register new column ERRORS into columnValueFactoryMap`.
- `SELECT digest FROM information_schema.tidb_statements_stats LIMIT 1` panics with `runtime error: invalid memory address or nil pointer dereference`.

Expected:

- v2 should either implement cumulative table semantics, route cumulative tables to v1-compatible data, or return a controlled unsupported error. It should not panic.

### 2. v2 single-sided time predicates can incorrectly drop valid history rows

Affected:

- `pkg/planner/core/memtable_predicate_extractor.go`
- `pkg/executor/stmtsummary.go`
- `pkg/util/stmtsummary/v2/reader.go`

Impact:

- Queries such as:
  - `WHERE summary_end_time >= <time>`
  - `WHERE summary_begin_time <= <time>`
- can miss valid rows because the extractor invents a one-hour missing bound. The pushed-down coarse range becomes narrower than the SQL predicate.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -run '^TestAuditV2SingleSided(Lower|Upper)TimePredicateDoesNotLoseRows$' -count=1
```

Observed:

- Lower-bound-only query expected both `old` and `new`, actual only `old`.
- Upper-bound-only query expected both `old` and `new`, actual only `new`.

Expected:

- A predicate with only one side must not synthesize a one-hour opposite bound unless that bound is also logically implied by the SQL.

### 3. v1 history clearing keeps the oldest interval instead of the current/newest interval

Affected:

- `pkg/util/stmtsummary/statement_summary.go:476-489`

Impact:

- Disabling history should leave only the current interval.
- Current code uses `history.Front()`, but history stores newest at `Back()`.
- Result: after history is disabled, the visible retained row can be the oldest interval.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1ClearHistoryKeepsCurrentInterval$' -count=1
```

Expected:

- Keep current/newest interval.

Observed:

- Oldest interval remains.

### 4. v1 history-size reduction returns oldest rows first

Affected:

- `pkg/util/stmtsummary/statement_summary.go:702-719`
- `pkg/util/stmtsummary/evicted.go:212-219`

Impact:

- When `tidb_stmt_summary_history_size` is reduced, `STATEMENTS_SUMMARY_HISTORY` should keep the most recent intervals.
- Current collection starts from `Front()`, so it returns oldest intervals.
- Evicted history uses the same direction.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1HistorySizeReductionKeepsNewestIntervals$' -count=1
```

Expected:

- History size `1` keeps newest interval.

Observed:

- It keeps oldest interval.

### 5. v1 plan encoding failure can panic statement summary

Affected:

- `pkg/util/stmtsummary/statement_summary.go:617-635`
- `pkg/util/stmtsummary/statement_summary.go:725-731`
- `pkg/util/stmtsummary/statement_summary.go:764-768`

Impact:

- `StmtExecLazyInfo.GetEncodedPlan()` returns a third value for recovered panic/error.
- `newStmtSummaryStats()` returns `nil` on that error.
- Callers immediately dereference the result.
- Statement summary collection can panic instead of skipping plan text.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1PlanEncodingFailureDoesNotPanic$' -count=1
```

Expected:

- Record should still be added with empty/discarded plan fields, or the failing statement should be skipped without panic.

Observed:

- Nil pointer panic.

### 6. Statement summary key serialization has ambiguous field boundaries

Affected:

- `pkg/util/stmtsummary/statement_summary.go:62-82`
- Shared by v1 and v2 because both use `StmtDigestKey`.

Impact:

- `StmtDigestKey` concatenates `digest`, `schemaName`, `prevDigest`, `planDigest`, and `resourceGroupName` without length boundaries. Only `user` is length-prefixed.
- `SimpleLRUCache` indexes solely by `string(key.Hash())`.
- Different logical keys can become byte-identical and merge into one summary row.

Concrete collision pattern:

- Same SQL digest.
- Case A: non-point-get, `schema=""`, `planDigest=<64-char string>`.
- Case B: point-get, `schema=<same 64-char string>`, `planDigest=""`.
- With same `prevDigest`, resource group, and user, the serialized key bytes are identical.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditStmtDigestKeyHasUnambiguousFieldBoundaries$' -count=1
```

Expected:

- Field boundaries must be unambiguous for all variable/optional fields.

Observed:

- Distinct logical tuples can produce the same LRU key.

### 7. v1/v2 capacity shrink drops statement statistics instead of merging them into `Others`

Affected:

- `pkg/util/kvcache/simple_lru.go:213-225`
- `pkg/util/stmtsummary/statement_summary.go:548-551`
- `pkg/util/stmtsummary/v2/stmtsummary.go:229-239`

Impact:

- `tidb_stmt_summary_max_stmt_count` shrink deletes excess LRU entries via `SimpleLRUCache.SetCapacity`.
- `SetCapacity` does not call `onEvict`.
- Deleted records disappear instead of being merged into `Others`.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1CapacityReductionIsAccountedAsEviction$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2CapacityReductionIsAccountedAsEviction$' -count=1
```

Expected:

- Shrink-induced removals should be accounted consistently with normal LRU eviction.

Observed:

- Rows are removed without `Others` aggregation.

### 8. v1 and v2 `Others` aggregation omits SQL-visible metrics

Affected:

- v1: `pkg/util/stmtsummary/evicted.go:221-407`
- v2: `pkg/util/stmtsummary/v2/record.go:471-642`

Impact:

- `Others` is supposed to preserve aggregate metrics for evicted rows.
- Several SQL-visible fields are not merged, so `Others` under-reports or misreports them.

Confirmed examples:

- Both v1 and v2: result rows, network traffic, storage flags are not preserved.
- v2: memory arbitration is not preserved.
- v1: plan-cache-unqualified count and last reason are not preserved.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1OthersPreservesAllAggregateMetrics$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2OthersPreservesAllAggregateMetrics$' -count=1
```

Expected:

- Every additive/min/max SQL-visible metric should be merged into `Others`, or the table should explicitly document fields that are undefined for `Others`.

Observed:

- Multiple fields remain zero/default after merge.

### 9. v1 and v2 `group_by_user` do not isolate `Others` by user

Affected:

- v1: `pkg/util/stmtsummary/evicted.go`
- v2: `pkg/util/stmtsummary/v2/stmtsummary.go`, `pkg/util/stmtsummary/v2/reader.go`

Impact:

- With `tidb_stmt_summary_group_by_user=ON`, regular rows are keyed by user.
- Evicted records are still merged into one global `Others`.
- Authorization checks use an auth-user set on that single aggregate. If Alice and Bob both contributed to `Others`, Alice can see the total latency/count that includes Bob's evicted statements as soon as Alice is in the aggregate's auth set.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1OthersDoesNotExposeOtherUsersMetrics$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2OthersDoesNotExposeOtherUsersMetrics$' -count=1
```

Expected:

- If user grouping is enabled, `Others` should also be per-user or should not be visible to non-`PROCESS` users.

Observed:

- A non-`PROCESS` user can see aggregate metrics that include another user's statements.

### 10. Statement summary user identity ignores host

Affected:

- `pkg/executor/adapter.go:2192-2198`, `pkg/executor/adapter.go:2262-2274`
- v1: `pkg/util/stmtsummary/reader.go:142-146`
- v2: `pkg/util/stmtsummary/v2/reader.go:403-410`

Impact:

- TiDB account identity is user plus host, but statement summary records only `sessVars.User.Username`.
- Accounts such as `'app'@'10.%'` and `'app'@'192.%'` can be merged and can pass authorization checks as the same `app` user.
- This matters more after `group_by_user` because that feature is intended to isolate summaries by user.

Reproducer sketch:

1. Create two accounts with the same username but different host patterns.
2. Run distinct statements from each account.
3. Query statement summary as one non-`PROCESS` account.
4. Observe that statement summary identity and auth checks do not distinguish the host part.

Expected:

- Use authenticated user identity, not only username, for grouping and auth.

### 11. v1/v2 request-duration average columns use the wrong denominator

Affected:

- v1/v2 column factories for:
  - `AVG_KV_TIME`
  - `AVG_PD_TIME`
  - `AVG_BACKOFF_TOTAL_TIME`
  - `AVG_WRITE_SQL_RESP_TIME`

Impact:

- These sums are accumulated per execution.
- The AVG columns divide by `CommitCount`.
- Read-only statements or statements without commit detail can show zero even when the sum is non-zero.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1AverageRequestDurationsUseExecutionCount$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditAverageRequestDurationsUseExecutionCount$' -count=1
```

Expected:

- Divide by `ExecCount`.

Observed:

- Divide by `CommitCount`.

### 12. v1/v2 table-name formatting can leave a dangling comma

Affected:

- v1: `pkg/util/stmtsummary/statement_summary.go:619-632`
- v2: `pkg/util/stmtsummary/v2/record.go:181-196`

Impact:

- Table names are joined using the original index from `StmtCtx.Tables`.
- Entries with empty table name are skipped, but the comma decision still uses the original slice length.
- Mixed database-only and table entries can produce strings such as `db.t,`.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2TableNamesHaveNoDanglingComma$' -count=1
```

Expected:

- `db.t`

Observed:

- `db.t,`

### 13. v2 `tidb_stmt_summary_max_sql_length` does not cap `DIGEST_TEXT`

Affected:

- `pkg/util/stmtsummary/v2/record.go:213-224`
- `pkg/util/stmtsummary/v2/record.go:644-660`

Impact:

- v1 formats/truncates normalized SQL when initializing summary.
- v2 stores `info.NormalizedSQL` directly in `StmtRecord.NormalizedSQL`.
- `DIGEST_TEXT` can exceed `tidb_stmt_summary_max_sql_length`, increasing memory and persistent-log size.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2MaxSQLLengthAlsoCapsDigestText$' -count=1
```

Expected:

- `DIGEST_TEXT` is truncated in the same way as `QUERY_SAMPLE_TEXT`.

Observed:

- Full normalized SQL is retained.

### 14. v2 `Add()` versus rotate/flush can lose an execution

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:311-345`
- `pkg/util/stmtsummary/v2/stmtsummary.go:414-464`
- `pkg/util/stmtsummary/v2/logger.go:53-70`

Impact:

- `Add()` releases `windowLock` after selecting/creating the record but before `record.Add(info)`.
- `rotate()` or `flush()` can swap and persist the old window before that record update.
- The in-flight execution is then not in new current data and may not be in persisted history.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2AddCannotLoseExecutionAcrossRotate$' -count=1
```

Observed:

- Expected persisted `exec_count=1`, actual `exec_count=0`.

Expected:

- Once an execution reaches `Add()`, it should appear in exactly one of current or persisted history.

### 15. v2 `SetEnabled(false)` and `SetEnableInternalQuery(false)` can be bypassed by in-flight `Add()`

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:196-220`
- `pkg/util/stmtsummary/v2/stmtsummary.go:311-345`

Impact:

- v1 rechecks enabled/internal state under the map lock.
- v2 `Add()` does not recheck these flags under `windowLock`.
- An execution that passed the caller-side check before the sysvar change can repopulate the cleared in-memory window.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2(DisabledSummaryCannotBeRepopulatedByInflightAdd|InternalSummaryCannotBeRepopulatedAfterDisable)$' -count=1
```

Expected:

- After disable/clear completes, old in-flight statements should not repopulate the window.

Observed:

- The window can contain a post-clear record.

### 16. Disabling internal-query collection does not remove internal contributions already in `Others`

Affected:

- v1: `pkg/util/stmtsummary/statement_summary.go:459-473`
- v2: `pkg/util/stmtsummary/v2/stmtsummary.go:374-388`

Impact:

- `ClearInternal()` scans current LRU records.
- Internal queries already evicted into `Others` remain there.
- A later `SET GLOBAL tidb_stmt_summary_internal_query=OFF` does not remove those internal-query aggregates.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -run '^TestAuditV1DisableInternalClearsInternalOthers$' -count=1

go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2DisableInternalClearsInternalOthers$' -count=1
```

Expected:

- Internal contributions should be cleared from all in-memory summary surfaces, including `Others`.

Observed:

- `Others` retains them.

### 17. v1 `STATEMENTS_SUMMARY_EVICTED` has a data race

Affected:

- `pkg/util/stmtsummary/evicted.go:187-195`

Impact:

- `ToEvictedCountDatum()` iterates evicted history and reads counts without locking `stmtSummaryByDigestEvicted`.
- Concurrent eviction mutates the same history and count.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -race -run '^TestAuditV1EvictedCountConcurrentRead$' -count=1
```

Observed:

- Go race detector reports concurrent read/write on evicted-count state.

Expected:

- Evicted table reads should lock or snapshot the evicted history.

### 18. v2 `STATEMENTS_SUMMARY_EVICTED` has a data race/inconsistent snapshot

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:350-365`

Impact:

- `Evicted()` locks to read the count, unlocks, then reads `s.window.begin`.
- Concurrent rotation can replace `s.window`.
- Race detector reports access to `s.window`.
- Even without race detector, count and begin time can come from different windows.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -race -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2EvictedReadIsRaceFree$' -count=1
```

Expected:

- One locked snapshot of window begin/count.

### 19. v2 `MemReader` can return `SUMMARY_BEGIN_TIME > SUMMARY_END_TIME`

Affected:

- `pkg/util/stmtsummary/v2/reader.go:88-116`

Impact:

- `MemReader.Rows()` captures `end := timeNow().Unix()` before taking `windowLock`.
- It then reads `w.begin` after acquiring the lock.
- If time/window begin changes between those operations, rows can have begin later than end.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2MemReaderNeverReturnsBeginAfterEnd$' -count=1
```

Expected:

- Begin time should not be later than end time.

### 20. v2 setup failures are not handled safely

Affected:

- `cmd/tidb-server/main.go:1345-1357`
- `pkg/util/stmtsummary/v2/stmtsummary.go:101-150`
- `pkg/util/stmtsummary/v2/stmtsummary.go:740-803`
- `pkg/util/stmtsummary/v2/logger.go:38-45`

Impact:

- If persistent statement summary setup fails, server startup only logs the error and continues.
- Public wrappers such as `Enabled()` and `Add()` dereference `GlobalStmtSummary` when persistent mode is enabled.
- `newStmtLogStorage()` logs logger initialization errors and returns a NOP logger, so setup can appear successful while nothing is persisted.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2(PersistentWrapperHandlesSetupFailure|PersistentWrappersDoNotPanicWhenSetupFailed|NewStmtSummaryReportsLoggerInitFailure)$' -count=1
```

Expected:

- Startup should fail fast or persistent wrappers should degrade safely.
- Logger initialization failure should be returned to setup.

Observed:

- Nil pointer panic is possible; bad logger setup can be hidden behind a NOP logger.

### 21. v2 history reader leaks file descriptors for files rejected by time range

Affected:

- `pkg/util/stmtsummary/v2/reader.go:549-599`

Impact:

- `newStmtFiles()` opens a candidate file.
- If the file does not overlap the pushed-down time range, the function returns without closing it.
- Repeated history queries can leak FDs until the process reaches OS limits.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2HistoryRangeFilterClosesRejectedFiles$' -count=1
```

Observed:

- 32 excluded-file scans leak 32 descriptors in the focused test.

Expected:

- Close every opened file that is not appended to `files`.

### 22. v2 history file selection can read wrong files

Affected:

- `pkg/util/stmtsummary/v2/reader.go:499-535`
- `pkg/util/stmtsummary/v2/reader.go:549-599`

Impact:

- Matching uses `strings.HasPrefix(path, prefix)` on full paths.
- Rotation-end parsing compares base file names with a prefix that may include directories.
- Confirmed consequences:
  - With an absolute configured filename, rotated-file end time can parse as `0`, disabling pruning.
  - Sibling files such as `tidb-statements2.log` can be read as statement summary history for `tidb-statements.log`.
  - Extensionless backup names can be misparsed because a timestamp suffix such as `.000` is treated as the extension.
  - Empty or invalid filename can make prefix matching too broad.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2(AbsoluteFilenameParsesRotationEnd|HistoryFileSelectionDoesNotReadSiblingPrefixes)$' -count=1
```

Expected:

- Match only the configured active file and lumberjack-style backups for that file.
- Parse timestamps against the base prefix, not a full-path prefix.

### 23. v2 history reader can stop before valid older records

Affected:

- `pkg/util/stmtsummary/v2/reader.go:670-714`
- `pkg/util/stmtsummary/v2/reader.go:431-443`

Impact:

- The scanner stops a file when the first parsed line's `begin` is greater than all requested range ends.
- That assumes records in a file are monotonically ordered by begin time.
- v2 persist writes are asynchronous and can become out of order; malformed/manual files can also be out of order.
- A newer line at the top can cause the reader to skip later valid older lines.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2HistoryReaderDoes(NotLoseOutOfOrderLines|StopBeforeOutOfOrderOlderRecord)$' -count=1
```

Expected:

- Either guarantee write ordering, or do not use a stop optimization that depends on ordering.

Observed:

- The older valid record is skipped.

### 24. v2 rotate persistence has no serialization/backpressure

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:423-464`

Impact:

- Every rotation starts a new goroutine to persist the completed window.
- There is no serialization or queue bound for slow disk writes.
- Under low refresh interval, slow IO, or large windows, many windows and goroutines can accumulate.
- This also contributes to out-of-order file records, which makes issue 23 user-visible.

Reproducer sketch:

1. Set a very small refresh interval.
2. Use a slow/blocking `stmtStorage.persist` in a package-level test.
3. Trigger several rotations.
4. Observe multiple persist goroutines/windows outstanding.

Expected:

- Bound outstanding persistence work or serialize writes.

### 25. v2 history query combines memory and disk without an atomic snapshot

Affected:

- `pkg/executor/stmtsummary.go:257-272`
- `pkg/util/stmtsummary/v2/stmtsummary.go:454-464`

Impact:

- `STATEMENTS_SUMMARY_HISTORY` in v2 reads memory first, then opens persistent files.
- Rotation can happen between those two reads.
- A window can be missed or duplicated depending on timing.

Reproducer sketch:

1. Block a query after `mem.Rows()` and before `NewHistoryReader()`.
2. Rotate and persist the current window.
3. Resume the query.
4. Check whether that window appears zero/one/two times.

Expected:

- History query should use a consistent cutover point between memory and disk.

### 26. v2 rotate/flush skips `Others` if the LRU is empty

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:414-464`

Impact:

- `rotate()` and `flush()` persist only when `window.lru.Size() > 0`.
- They ignore `window.evicted.otherForPersist.ExecCount`.
- If current LRU is empty but `Others` contains records that need persistence, the window is skipped.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2RotatePersistsOthersWhenLRUIsEmpty$' -count=1
```

Expected:

- Persist a window if either regular rows or `otherForPersist` has data.

### 27. v2 `persist_evicted=ON` can make history miss evicted executions

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:469-486`
- `pkg/util/stmtsummary/v2/stmtsummary.go:660-670`
- `pkg/util/stmtsummary/v2/reader.go:765-774`

Impact:

- When a per-record eviction is successfully queued, v2 does not merge it into `otherForPersist`.
- The history reader skips records with `"evicted": true`.
- Therefore built-in `STATEMENTS_SUMMARY_HISTORY` can miss evicted executions when per-record evicted logging is enabled.

Reproducer sketch:

1. Enable persistent mode and `tidb_stmt_summary_persist_evicted=ON`.
2. Set statement summary capacity to `1`.
3. Run two different digests to evict the first.
4. Rotate/flush.
5. Query `STATEMENTS_SUMMARY_HISTORY`.
6. The evicted digest is not returned by the built-in history table.

Expected:

- Either history should include per-record evicted logs, or the feature should clearly document that per-record evicted files are not part of the built-in history table.

### 28. v2 oversized statement-summary log writes are silently lost

Affected:

- `pkg/util/stmtsummary/v2/logger.go:53-118`
- `gopkg.in/natefinch/lumberjack.v2` behavior: a single write larger than `MaxSize` returns an error.

Impact:

- A single statement record can contain up to 1 MiB encoded plan plus 1 MiB binary plan, before other fields.
- `logEvicted()` batches up to 64 records into one zap message.
- If the write exceeds lumberjack's file max size, zap reports the write error internally; this code cannot observe it.
- The metric is incremented as “persisted” before the write result is known.
- If `persist_evicted=ON`, the record may also be excluded from `otherForPersist`, so it is lost from built-in history too.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2PersistEvictedDoesNotLoseRecordsWhenLogWriteExceedsMaxSize$' -count=1
```

Observed:

- With file max size set to 1 MiB, the large evicted record is absent.
- The file contains zap/lumberjack write-error text and later records.

Expected:

- The code should split records, avoid batching beyond file limits, or fall back to `Others` when durable write fails.

### 29. v2 evicted-key tracking is unbounded by `max_stmt_count`

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:642-674`

Impact:

- v2 `stmtEvicted.keys` stores every distinct evicted key in the refresh window.
- This is not bounded by `tidb_stmt_summary_max_stmt_count`.
- A workload with many distinct digests can keep a large map even when summary capacity is small.
- This appears to regress the intent of previous v1 fixes around `Others` memory growth.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2EvictedKeyTrackingDefeatsCapacityBound$' -count=1
```

Observed:

- Capacity `1`, 10,000 distinct digests -> one LRU row plus 9,999 evicted keys retained.

Expected:

- `Others` count/memory should be bounded or approximate, or explicitly configured.

### 30. v2 evicted log channel is bounded by record count, not bytes

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:42-49`
- `pkg/util/stmtsummary/v2/stmtsummary.go:469-486`

Impact:

- `evictedCh` capacity is 1024 records.
- Each cloned record can hold large SQL text, encoded plan, and binary plan.
- Worst-case queued memory can be very large even though the channel length is bounded.

Reproducer sketch:

1. Enable `persist_evicted`.
2. Use a blocking `stmtStorage.logEvicted`.
3. Add >1024 large-plan distinct statements with capacity 1.
4. Observe memory retained by queued `StmtRecord` clones.

Expected:

- Bound queue by bytes, drop/shed based on serialized size, or avoid queuing very large payloads.

### 31. v2 history reader can use excessive memory and file descriptors per query

Affected:

- `pkg/util/stmtsummary/v2/reader.go:192-223`
- `pkg/util/stmtsummary/v2/reader.go:320-360`
- `pkg/util/stmtsummary/v2/reader.go:549-599`
- `pkg/util/stmtsummary/v2/reader.go:856-861`

Impact:

- `NewHistoryReader()` opens all matched files at construction.
- SQL `LIMIT` cannot reduce that upfront FD cost.
- `maxLineSize` is 1 GiB.
- `linesCh` buffers batches of 64 lines, with concurrency-based channel capacity.
- Large persisted records can therefore cause high transient memory use.

Reproducer sketch:

1. Create many rotated statement log files.
2. Query `STATEMENTS_SUMMARY_HISTORY LIMIT 1`.
3. Observe many files opened before any row is returned.
4. Use large JSON lines to observe high transient memory.

Expected:

- Open files lazily, cap row size closer to the writer's actual maximum, and make `LIMIT`/cancellation effective earlier.

### 32. v2 `Others` first/last seen timestamps are not initialized from execution times

Affected:

- `pkg/util/stmtsummary/v2/stmtsummary.go:676-684`
- `pkg/util/stmtsummary/v2/record.go:626-630`

Impact:

- `newEvictedAggregateRecord()` initializes `FirstSeen` and `LastSeen` to `time.Now()`.
- When merging old evicted records, `LastSeen` can remain the aggregate creation/eviction wall-clock time instead of the last execution time.

Reproducer:

```bash
source /home/xhy/.gvm/environments/go1.25.8
go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2OthersLastSeenMatchesLastExecution$' -count=1
```

Expected:

- First/last seen for `Others` should be min/max over merged records' execution times.

### 33. v1/v2 evicted-table timestamps ignore session timezone

Affected:

- v1: `pkg/util/stmtsummary/evicted.go:198-205`
- v2: `pkg/util/stmtsummary/v2/stmtsummary.go:350-365`

Impact:

- Normal statement summary timestamp columns are converted using the reader's session timezone.
- Evicted-count rows construct timestamps directly from `time.Unix(...)` / `time.Now()` without the reader timezone.
- Sessions in non-system time zones can see inconsistent timestamp interpretation between summary and evicted tables.

Reproducer sketch:

1. Set session `time_zone` to a value different from server local time.
2. Trigger an eviction.
3. Compare `STATEMENTS_SUMMARY_EVICTED.BEGIN_TIME` with `STATEMENTS_SUMMARY.SUMMARY_BEGIN_TIME`.

Expected:

- Use the same timezone conversion policy across statement summary tables.

### 34. Resource group attribution is broken for `Others`

Affected:

- v1: `pkg/util/stmtsummary/evicted.go:404-406`
- v2: `pkg/util/stmtsummary/v2/record.go:471-642`

Impact:

- Regular rows are keyed by resource group.
- `Others` aggregates across evicted records.
- v1 overwrites `resourceGroupName` with the last merged evicted record.
- v2 leaves the aggregate resource group empty.
- Resource-group filters can therefore miss or misattribute evicted stats.

Reproducer sketch:

1. Run distinct statements in two resource groups with capacity 1.
2. Force evictions.
3. Query statement summary with a resource group filter.
4. Compare regular rows and `Others`.

Expected:

- Either keep per-resource-group `Others` or define the aggregate as cross-resource-group and make filters behave consistently.

## Compatibility/product decisions to clarify

These are not listed above as hard bugs unless the owner confirms the expected contract.

### C1. v2 `STATEMENTS_SUMMARY_EVICTED` returns only the current period

v1 returns historical evicted-count rows up to history size. v2 returns one current-window row. This is a compatibility difference in an information-schema table.

### C2. v2 `tidb_stmt_summary_history_size` is a no-op in persistent mode

`SetHistorySize()` returns `nil` when persistent mode is enabled. The sysvar can still be set, but v2 history retention is governed by log file retention settings. This may be acceptable, but it should be documented clearly.

### C3. v2 window boundaries are not wall-clock aligned like v1

v1 interval begin is aligned to a multiple of refresh interval. v2 uses `timeNow()` when the window is created/rotated. Multi-node cluster windows can be offset by process start/rotation time. This may affect dashboards that compare intervals across nodes.

### C4. `persist_evicted=ON` may intentionally produce raw evicted logs outside built-in history

If the intended design is “raw evicted records are for offline consumers only,” then issue 27 should be documented rather than fixed as a built-in history bug. If users expect `STATEMENTS_SUMMARY_HISTORY` to remain complete, it is a correctness bug.

## Rejected or downgraded candidates

- `digest IS NULL` queries: existing tests cover this path. The digest extractor does not incorrectly remove `IS NULL`; final SQL filtering handles it.
- Inclusive time-range overlap at exact boundaries: although `StmtTimeRange` comments say `[Begin, End)`, SQL extractor logic uses `summary_begin_time <= end AND summary_end_time >= start`, so closed-overlap prefiltering is consistent with current SQL predicate shape. It may over-read but does not by itself prove wrong rows.
- `CacheStmtExecInfo` reuse: the only obvious non-reset field is `ExecRetryTime`, but it is consumed only when `ExecRetryCount > 0`, so I did not find a concrete cross-statement pollution bug there.
- zap/lumberjack byte interleaving: no evidence of interleaved JSON bytes. The real issue is silent write failure/order/oversized writes, covered above.
- `HistoryReader.Close()` order: closing files before cancel looks rough, but I did not find a deterministic deadlock. The FD leak is in rejected-file handling, not normal close.

## Historical reproduction command summary

All commands below were run from `/tmp/tidb-stmtsummary-audit.hdkvdB`. The `TestAudit*` files were temporary and intentionally fail against the audited code.

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

Additional targeted sanity run:

```bash
source /home/xhy/.gvm/environments/go1.25.8
./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -run '^TestStmtSummary' -count=1
```

Result: existing v2 current/history SQL tests pass. This supports scoping the SQL panic to cumulative table routing rather than all v2 statement summary tables.

## Historical suggested filing order

If filing GitHub issues, I would split them by owner/root cause:

1. v2 cumulative table route panic.
2. v2 time predicate false negatives.
3. shared key serialization collision.
4. v1 history ordering/clearHistory defects.
5. capacity shrink dropping rows in `SimpleLRU.SetCapacity`.
6. `Others` aggregation completeness plus user/resource-group isolation.
7. v2 `Add`/rotate/flush data-loss race.
8. v1/v2 evicted-table races.
9. v2 history reader FD/file matching/out-of-order issues.
10. v2 persistence error handling and oversized-write loss.
11. v2 resource bounds: unbounded persist goroutines, evicted queue bytes, history-reader memory/FD.
