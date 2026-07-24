# Statement Summary v1/v2 audit runnable test matrix

Date: 2026-07-23

This document records the runnable `TestAudit*` reproducers for the statement-summary v1/v2 audit. The tests live in the TiDB workspace and are isolated behind the `audit` build tag, so normal test runs do not execute them.

Test files:

- `pkg/planner/core/audit_stmtsummary_reproducer_test.go`
- `pkg/util/stmtsummary/audit_reproducer_test.go`
- `pkg/util/stmtsummary/v2/audit_reproducer_test.go`
- `pkg/util/stmtsummary/v2/tests/audit_reproducer_test.go`

Current classification:

- 42 runnable `TestAudit*` tests are recorded.
- 39 tests are confirmed-bug reproducers.
- 2 tests are runnable but need product/trigger qualification before filing.
- 1 test is a rejected-candidate guard that currently passes.

## Command templates

Run from repository root: `/home/xhy/Development/github.com/pingcap/tidb`.

v1 statement-summary tests require the failpoint wrapper:

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -tags=intest,deadlock,audit -run '^<TEST_NAME>$' -count=1
```

v1 race tests:

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -tags=intest,deadlock,audit -race -run '^<TEST_NAME>$' -count=1
```

v2 statement-summary unit tests:

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/util/stmtsummary/v2 \
  -run '^<TEST_NAME>$' -count=1
```

v2 race tests:

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit -race ./pkg/util/stmtsummary/v2 \
  -run '^<TEST_NAME>$' -count=1
```

v2 SQL / information_schema tests require the failpoint wrapper:

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests \
  -tags=intest,deadlock,audit -run '^<TEST_NAME>$' -count=1
```

Planner extractor tests:

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/planner/core \
  -run '^<TEST_NAME>$' -count=1
```

## Confirmed bugs with runnable reproducers

| ID | Issue | Test source | Command template | Current failure | Trigger |
| --- | --- | --- | --- | --- | --- |
| S1 | v2 persistent mode panics on `SELECT * FROM INFORMATION_SCHEMA.TIDB_STATEMENTS_STATS` | `pkg/util/stmtsummary/v2/tests/audit_reproducer_test.go:26` `TestAuditV2SQLCumulativeTableDoesNotPanic` | v2 SQL | panic: `should never happen, should register new column ERRORS into columnValueFactoryMap` | Persistent statement summary enabled; querying all columns from the cumulative table |
| S2 | v2 persistent cumulative table also panics when selecting only a supported column | `pkg/util/stmtsummary/v2/tests/audit_reproducer_test.go:41` `TestAuditV2SQLCumulativeTableSupportedColumnDoesNotPanic` | v2 SQL | nil pointer in `executor.(*rowsReader).pull` | Persistent mode enabled; querying `information_schema.tidb_statements_stats` |
| P1 | Statement-summary single-sided time predicates are incorrectly narrowed to a one-hour window | `pkg/planner/core/audit_stmtsummary_reproducer_test.go:26` `TestAuditStatementsSummarySingleSidedTimePredicateStaysOpenEnded` | planner | `SUMMARY_END_TIME >= start` becomes `[start, start+1h]` | v2 history queries with only one time bound |
| V1-1 | v1 visibility checks use username only and ignore host | `pkg/util/stmtsummary/audit_reproducer_test.go:54` `TestAuditV1SameUsernameDifferentHostsShouldNotShareVisibility` | v1 | `alice@host2` can see `alice@host1` rows | Same username, different host accounts, no PROCESS privilege |
| V1-2 | v1 history clear keeps the oldest interval instead of current/newest | `pkg/util/stmtsummary/audit_reproducer_test.go:71` `TestAuditV1ClearHistoryKeepsCurrentInterval` | v1 | expected begin=200, actual begin=100 | History is disabled or cleared |
| V1-3 | v1 history-size reduction returns oldest intervals first | `pkg/util/stmtsummary/audit_reproducer_test.go:89` `TestAuditV1HistorySizeReductionKeepsNewestIntervals` | v1 | expected begin=200, actual begin=100; evicted history fails similarly | Lowering `tidb_stmt_summary_history_size` |
| V1-4 | v1 plan encoding failure can panic statement summary collection | `pkg/util/stmtsummary/audit_reproducer_test.go:108` `TestAuditV1PlanEncodingFailureDoesNotPanic` | v1 | nil pointer panic in statement-summary stats dereference | `StmtExecLazyInfo.GetEncodedPlan()` returns an error/recovered panic |
| V1-5 | v1 capacity shrink drops removed records instead of merging them into `Others` | `pkg/util/stmtsummary/audit_reproducer_test.go:128` `TestAuditV1CapacityReductionIsAccountedAsEviction` | v1 | no `Others` row after shrinking to 1 | Runtime decrease of `tidb_stmt_summary_max_stmt_count` |
| V1-6 | v1 `Others` aggregation omits SQL-visible metrics | `pkg/util/stmtsummary/audit_reproducer_test.go:148` `TestAuditV1OthersPreservesAllAggregateMetrics` | v1 | `MAX_RESULT_ROWS` expected 42, actual 0 | Digest is LRU-evicted into `Others` |
| V1-7 | v1 `group_by_user` does not isolate `Others` by user | `pkg/util/stmtsummary/audit_reproducer_test.go:169` `TestAuditV1OthersDoesNotExposeOtherUsersMetrics` | v1 | alice sees exec_count=2 instead of 1, including bob's evicted contribution | `tidb_stmt_summary_enable_group_by_user=ON` and multiple users have evicted rows |
| V1-8 | v1 request-duration AVG columns use the wrong denominator | `pkg/util/stmtsummary/audit_reproducer_test.go:197` `TestAuditV1AverageRequestDurationsUseExecutionCount` | v1 | `AVG_KV_TIME` expected 100ms, actual 0 | Reading KV/PD/backoff/request AVG columns when commit_count is 0 |
| V1-9 | v1 table names can keep a dangling comma | `pkg/util/stmtsummary/audit_reproducer_test.go:214` `TestAuditV1TableNamesHaveNoDanglingComma` | v1 | expected `db.t`, actual `db.t,` | Statement summary row has table names |
| V1-10 | v1 disabling internal-query collection does not remove internal contributions already merged into `Others` | `pkg/util/stmtsummary/audit_reproducer_test.go:225` `TestAuditV1DisableInternalClearsInternalOthers` | v1 | empty-digest `Others` row remains after disabling internal collection | Internal query was evicted into `Others` before disabling internal collection |
| V1-11 | v1 `STATEMENTS_SUMMARY_EVICTED` has a data race | `pkg/util/stmtsummary/audit_reproducer_test.go:276` `TestAuditV1EvictedCountReaderIsRaceFree` | v1 race | race between `evicted.go:189` list read and `evicted.go:100` `PushFront` write | Concurrent evicted-table reads and LRU evictions |
| V2-1 | v2 visibility checks use username only and ignore host | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:94` `TestAuditV2SameUsernameDifferentHostsShouldNotShareVisibility` | v2 | `alice@host2` can see `alice@host1` rows | Same username, different host accounts, no PROCESS privilege |
| V2-2 | v2 disabled persistent summary still accepts new statements | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:110` `TestAuditV2DisabledSummaryDoesNotAcceptNewStatements` | v2 | row `digest-added-after-disable` remains visible after `SetEnabled(false)` | Persistent statement summary disabled by sysvar/API, then a statement is added |
| V2-3 | v2 disabled internal-query collection still accepts new internal statements | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:123` `TestAuditV2DisabledInternalQueryDoesNotAcceptNewInternalStatements` | v2 | row `digest-internal-added-while-disabled` remains visible while internal collection is disabled | Persistent summary enabled, internal-query collection disabled, then an internal statement is added |
| V2-4 | v2 capacity shrink drops removed records instead of merging them into `Others` | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:138` `TestAuditV2CapacityReductionIsAccountedAsEviction` | v2 | no `Others` row after shrinking to 1 | Runtime decrease of `tidb_stmt_summary_max_stmt_count` |
| V2-5 | v2 `Others` aggregation omits SQL-visible metrics | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:154` `TestAuditV2OthersPreservesAllAggregateMetrics` | v2 | `AVG_MEM_ARBITRATION` expected 42, actual 0 | Digest is LRU-evicted into `Others` |
| V2-6 | v2 `group_by_user` does not isolate `Others` by user | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:170` `TestAuditV2OthersDoesNotExposeOtherUsersMetrics` | v2 | alice sees exec_count=2 instead of 1 | `group_by_user=ON` and multiple users have evicted rows |
| V2-7 | v2 request-duration AVG columns use the wrong denominator | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:193` `TestAuditAverageRequestDurationsUseExecutionCount` | v2 | `AVG_KV_TIME` expected 100ms, actual 0 | Reading KV/PD/backoff/request AVG columns when commit_count is 0 |
| V2-8 | v2 `MemReader` can return begin > end | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:209` `TestAuditV2MemReaderNeverReturnsBeginAfterEnd` | v2 | `SUMMARY_BEGIN_TIME` is greater than `SUMMARY_END_TIME` | Clock moves backward, or injected `timeNow` moves backward |
| V2-9 | v2 table names can keep a dangling comma | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:227` `TestAuditV2TableNamesHaveNoDanglingComma` | v2 | expected `db.t`, actual `db.t,` | Statement summary row has table names |
| V2-10 | v2 `tidb_stmt_summary_max_sql_length` does not cap `DIGEST_TEXT` | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:237` `TestAuditV2MaxSQLLengthAlsoCapsDigestText` | v2 | digest text remains `xxxxxxxxxx`, not `xxxx(len:10)` | Small `MaxSQLLength` while reading `DIGEST_TEXT` |
| V2-11 | v2 logger initialization failure is not returned to caller | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:252` `TestAuditV2NewStmtSummaryReportsLoggerInitFailure` | v2 | filename is a directory; expected error, actual nil error with nop logger | Persistent logger path cannot be created/written |
| V2-12 | v2 rotate persistence has no serialization/backpressure | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:261` `TestAuditV2RotatePersistenceShouldBeSerialized` | v2 | max concurrent persist calls is 2, expected at most 1 | Frequent rotation or slow persistence I/O |
| V2-13 | v2 history query combines memory and disk without a consistent snapshot | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:283` `TestAuditV2HistoryQueryMustNotDoubleCountWindowAcrossMemAndDisk` | v2 | total exec_count expected 1, actual 2 | Current window is flushed to disk while a query combines memory and history |
| V2-14 | v2 oversized statement-summary log writes are silently lost | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:329` `TestAuditV2PersistDoesNotSilentlyLoseOversizedRecords` | v2 | file contains only a write-error line and no recoverable digest; caller receives no error | Encoded record exceeds the maximum log line/file write size |
| V2-15 | v2 rotate/flush skips persistence when LRU is empty but `Others` has data | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:348` `TestAuditV2RotatePersistsOthersWhenLRUIsEmpty` | v2 | expected one persisted window, actual 0 | Current window only has evicted aggregate data |
| V2-16 | v2 disabling internal-query collection does not remove internal contributions already merged into `Others` | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:365` `TestAuditV2DisableInternalClearsInternalOthers` | v2 | empty-digest `Others` row remains after disabling internal collection | Internal query was evicted into `Others` before disabling internal collection |
| V2-17 | v2 history reader leaks FDs for files rejected by time range | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:385` `TestAuditV2HistoryRangeFilterClosesRejectedFiles` | v2 | FD count increases beyond baseline + 1 | Many rotated history files exist and most are rejected by time range |
| V2-18 | v2 history file selection can read sibling files with the same prefix | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:405` `TestAuditV2HistoryFileSelectionDoesNotReadSiblingPrefixes` | v2 | selected 2 files, including `tidb-statements-extra.log` | Other files share the configured filename prefix |
| V2-19 | v2 rotated filename end-time parsing fails for absolute paths | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:421` `TestAuditV2AbsoluteFilenameParsesRotationEnd` | v2 | expected parsed end timestamp, actual 0 | `stmt-summary-filename` is an absolute path |
| V2-20 | v2 history reader stops at a too-new line and skips later valid older lines | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:435` `TestAuditV2HistoryReaderDoesNotStopBeforeOutOfOrderOlderRecord` | v2 | first too-new line causes EOF; valid older row is missed | Log file lines are not strictly time-ordered |
| V2-21 | v2 history reader opens all matching files at once | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:453` `TestAuditV2HistoryReaderShouldNotOpenAllMatchingFilesAtOnce` | v2 | opens 10 matching files, expected streaming/small bounded open set | Long-running clusters with many history files |
| V2-22 | v2 evicted-key tracking is not bounded by `max_stmt_count` | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:471` `TestAuditV2EvictedKeyTrackingDefeatsCapacityBound` | v2 | evicted key count 99 with max_stmt_count=1 | Many distinct digests are evicted in a single window |
| V2-23 | v2 `Others.LastSeen` is not initialized from execution time | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:483` `TestAuditV2OthersLastSeenMatchesLastExecution` | v2 | `Others.LastSeen` uses wall-clock initialization, not evicted SQL execution time | First evicted record is merged into `Others` |
| V2-24 | v2 `Others` loses resource-group attribution | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:500` `TestAuditV2OthersResourceGroupShouldRepresentEvictedGroup` | v2 | expected `rg_audit`, actual empty string | Same resource group's digests are evicted into `Others` |
| V2-25 | v2 `STATEMENTS_SUMMARY_EVICTED` has a data race | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:523` `TestAuditV2EvictedCountReaderIsRaceFree` | v2 race | race at `stmtsummary.go:361` reading `s.window` while another goroutine writes it | Concurrent evicted-table reads and window rotation |

## Tests that need trigger/semantic qualification before filing

| ID | Issue | Test source | Current result | Filing guidance |
| --- | --- | --- | --- | --- |
| C1 | `StmtDigestKey` has ambiguous field boundaries | `pkg/util/stmtsummary/audit_reproducer_test.go:119` `TestAuditStmtDigestKeyHasUnambiguousFieldBoundaries` | Fails: two different logical tuples both hash to `digestabc` | File as key-encoding robustness issue, but state that this is a unit-level collision; developers should confirm whether real SQL digest/plan digest values can form an equivalent collision |
| C2 | v2 `persist_evicted=ON` can make built-in history miss per-record evicted executions | `pkg/util/stmtsummary/v2/audit_reproducer_test.go:307` `TestAuditV2PersistEvictedShouldRemainVisibleInBuiltInHistory` | Fails: total exec_count expected 2, actual 1 | File as a bug only if built-in history is expected to include evicted executions. If per-record evicted logs are intentionally offline-only, this is a product/doc semantics decision |

## Rejected or not recommended as confirmed bugs

| ID | Candidate | Test source | Current result | Conclusion |
| --- | --- | --- | --- | --- |
| R1 | v1 always loses resource-group attribution for single-resource-group `Others` | `pkg/util/stmtsummary/audit_reproducer_test.go:249` `TestAuditV1OthersResourceGroupShouldRepresentEvictedGroup` | Passes | Current v1 preserves resource group when all evicted rows come from one resource group. Do not file this exact case as a v1 bug. A multi-resource-group filter/display semantic issue would need a separate reproducer. |

## Observations not classified as bugs yet

Do not file these as confirmed bugs until a stable reproducer or owner confirmation exists:

- v2 `Add()` versus rotate/flush can lose one execution: the code has a lock gap, but there is no deterministic no-instrumentation test yet. Add a failpoint/hook between record selection and `record.Add` before filing.
- v2 evicted log channel is bounded by record count, not bytes: this is a resource-risk observation without an explicit byte budget/product constraint. Do not file as a correctness bug as-is.
- Evicted-table timestamp timezone semantics: confirm SQL type/display expectations first; no stable reproducer is recorded yet.

## Trigger realism classification

Use this when deciding whether to file an issue:

- File directly as confirmed bugs: all rows in "Confirmed bugs with runnable reproducers". They exercise reachable code paths through unit, reader, SQL/testkit, or race tests. Some require specific but real conditions, for example capacity changes, persistent mode, many history files, clock rollback, oversized records, or concurrent queries.
- File with qualification only: C1 and C2. They have runnable tests, but the issue text must state the limitation. C1 is a unit-level key-boundary collision until developers confirm a real SQL digest/plan digest collision. C2 depends on whether built-in history is expected to include `persist_evicted=ON` per-record evicted executions.
- Do not file as confirmed bugs: R1 and the "Observations not classified as bugs yet" section. R1 is a guard proving the exact v1 single-resource-group case currently passes. The observation list contains code-review risks or product questions that still need deterministic instrumentation or owner confirmation.

## Commands already run for new/key reproducers

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/planner/core \
  -run '^TestAuditStatementsSummarySingleSidedTimePredicateStaysOpenEnded$' -count=1
```

Result: failed because `SUMMARY_END_TIME >= start` was narrowed to `[start, start+1h]`.

```bash
PATH=/home/xhy/.gvm/gos/go1.25.8/bin:$PATH ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary \
  -tags=intest,deadlock,audit -race -run '^TestAuditV1EvictedCountReaderIsRaceFree$' -count=1
```

Result: failed with a race between `evicted.go:189` and `evicted.go:100`.

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit -race ./pkg/util/stmtsummary/v2 \
  -run '^TestAuditV2EvictedCountReaderIsRaceFree$' -count=1
```

Result: failed with a race at `stmtsummary.go:361` reading `s.window` while another goroutine writes it.

```bash
/home/xhy/.gvm/gos/go1.25.8/bin/go test -tags=intest,deadlock,audit ./pkg/util/stmtsummary/v2 \
  -run '^(TestAuditV2DisabledSummaryDoesNotAcceptNewStatements|TestAuditV2DisabledInternalQueryDoesNotAcceptNewInternalStatements)$' -count=1
```

Result: failed because rows added after `SetEnabled(false)` / `SetEnableInternalQuery(false)` remain visible.

The remaining tests were run in targeted batches; their observed failures are summarized in the matrix above.
