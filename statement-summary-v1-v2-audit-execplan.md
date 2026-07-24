# Audit statement summary v1 and v2 for correctness and operational risks

This ExecPlan is a living document. Keep `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` up to date as work proceeds.

Reference: `PLANS.md` at the repository root. This plan is maintained according to that file and the repository-wide `AGENTS.md` instructions supplied for this workspace.

## Purpose / Big Picture

TiDB has two statement-summary implementations. The legacy in-memory implementation is referred to as v1 and lives in `pkg/util/stmtsummary`. The persistent implementation is referred to as v2 and lives in `pkg/util/stmtsummary/v2`. Both aggregate executions by a statement digest and expose the result through statement-summary information-schema tables.

This audit looks for defects beyond the already identified v2 `Add` versus `ClearInternal` race in pull request #69914. The audit covers SQL-visible correctness, interval and history behavior, eviction and `Others`, persistence, concurrent access, shutdown and rotation, memory/resource ownership, and performance paths. Each confirmed defect must have reproducible evidence whenever practical: a focused Go test, a race-detector report, a benchmark, or a TiUP/RealTiKV scenario.

The user-visible outcome is `statement-summary-v1-v2-audit-report.md`, which records the audited revision, every confirmed issue, impact, evidence, and copy-pasteable reproduction steps. Acceptance requires independent v1, v2, and concurrency/performance reviewers, followed by cross-review, to report no additional actionable findings after the documented set is rechecked.

## Progress

- [x] (2026-07-23 09:49Z) Read repository instructions, `PLANS.md`, the TiDB test-guidelines skill, failpoint test runner, and WIP verification profile.
- [x] (2026-07-23 09:49Z) Selected the audit baseline: current `origin/master` plus the two commits in PR #69914, while retaining the current `origin/master` implementation as a comparison point.
- [x] (2026-07-23 10:15Z) Built the subsystem and caller map for v1, v2, sysvar dispatch, information-schema readers, persistence, and shutdown.
- [x] (2026-07-23 10:15Z) Completed three independent first-pass reviews: v1 semantics; v2 semantics/persistence; concurrency, performance, memory, and cross-version parity.
- [x] (2026-07-23 11:10Z) Turn every credible candidate into a minimal reproducer or document why deterministic local reproduction is not practical. Focused failure tests confirm the main history, panic, aggregation, FD, column-routing, truncation, timing, and persistence candidates; focused race tests confirm both evicted-table races.
- [x] (2026-07-23 11:20Z) Run cross-review in which reviewers attempt to disprove one another's findings and search the areas missed in the first pass.
- [x] (2026-07-23 11:35Z) Run a final convergence pass in which multiple reviewers independently state whether any new actionable issue remains. Persistence/resource final reviewers found no additional independent root cause. A narrow SQL-route final agent timed out, so the SQL-route scope was rechecked locally against existing v2 current/history tests and focused cumulative-table reproducers.
- [x] (2026-07-23 11:45Z) Write and validate `statement-summary-v1-v2-audit-report.md`.
- [x] (2026-07-23 11:50Z) Complete Ready-profile documentation validation and report exact commands, risks, and unverified areas.

## Surprises & Discoveries

- Observation: `origin/master` advanced after PR #69914 was opened, but files under `pkg/util/stmtsummary` did not change between the PR base and current `origin/master`.
  Evidence: `git diff --name-status 66be7a615be13b90a22fa36606e0258de76d95f7..origin/master -- pkg/util/stmtsummary` produced no entries.

- Observation: the local branch was behind `origin/master`, so the audit must not silently use the checked-out `master` revision.
  Evidence: local `HEAD` was `5db8029fe080f28b01f3929bcd4fc6b89c1cb7a9`; fetched `origin/master` was `ed2376acc6e0feeff9f3e2c38db489727933aa80`.

- Observation: the post-PR synthetic tree is reproducible and cleanly merges over current `origin/master`.
  Evidence: temporary worktree `/tmp/tidb-stmtsummary-audit.hdkvdB`; staged synthetic tree `271f8a2974a5d7fd7e0d7cb67fe90b1bd05e4c32`; `make bazel_prepare` completed successfully without adding unstaged generated changes.

- Observation: three independent reviewers converged on four defects before sharing detailed notes: the v2 filtered-file descriptor leak, capacity-shrink bypass of eviction accounting, missing aggregate fields in `Others`, and evicted-table synchronization defects.
  Evidence: independent `/root/audit_v1`, `/root/audit_v2`, and `/root/audit_crosscut` first-pass reports plus locally authored failing tests.

- Observation: the first focused v2 batch produced distinct expected failures for cumulative-table columns, capacity shrink, post-disable insertion, incomplete `Others` merge, 32 leaked descriptors after 32 excluded-file scans, absolute-path rotation parsing, normalized-SQL truncation, average denominators, table-name formatting, and inconsistent begin/end timestamps.
  Evidence: `go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 -run '^TestAudit' -count=1`.

- Observation: the first focused v1 batch produced distinct expected failures for retaining the oldest interval, returning the oldest row after history-size reduction, encoded-plan nil dereference, key-boundary collision, capacity shrink, and incomplete `Others` merge.
  Evidence: `./tools/check/failpoint-go-test.sh pkg/util/stmtsummary -run '^TestAudit' -count=1`.

- Observation: both evicted-table synchronization candidates are actual Go data races.
  Evidence: v2 `go test -race ... -run '^TestAuditV2EvictedReadIsRaceFree$'` reports `Evicted` reading `s.window` after unlock; v1 failpoint-wrapped `-race` reports `ToEvictedCountDatum` reading `count` concurrently with `AddEvicted`.

- Observation: SQL-level v2 tests confirm persistent-mode statement summary regressions beyond unit-level reasoning.
  Evidence: `./tools/check/failpoint-go-test.sh pkg/util/stmtsummary/v2/tests -run '^TestAuditV2SQLCumulativeTable(SupportedColumn)?DoesNotPanic$' -count=1` fails in two ways: `SELECT * FROM information_schema.tidb_statements_stats` panics with `should never happen, should register new column ERRORS into columnValueFactoryMap`, and `SELECT digest FROM information_schema.tidb_statements_stats` panics with a nil pointer because the v2 executor path never initializes a cumulative-table rows reader. Single-sided history predicates fail with `expected: "[new]\n[old]\n"`, but actual rows are only `[old]` for lower-bound and only `[new]` for upper-bound cases.

- Observation: additional focused v2 reproductions confirmed persistence and lifecycle defects.
  Evidence: `go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 -run '^TestAuditV2(PersistEvictedDoesNotLoseRecordsWhenLogWriteExceedsMaxSize|HistoryFileSelectionDoesNotReadSiblingPrefixes|PersistentWrappersDoNotPanicWhenSetupFailed|HistoryReaderDoesNotStopBeforeOutOfOrderOlderRecord)$' -count=1` fails as expected: the large evicted record is absent after lumberjack rejects a 2,100,551-byte write against a 1,048,576-byte file limit; `tidb-statements2.log` is read as a sibling of `tidb-statements.log`; persistent wrappers panic when `GlobalStmtSummary` is nil; and an out-of-order older record is skipped after a newer `begin` line.

- Observation: v2 can persist a rotated window before an in-flight `Add()` has applied the execution to the selected record.
  Evidence: `go test -tags=intest,deadlock ./pkg/util/stmtsummary/v2 -run '^TestAuditV2AddCannotLoseExecutionAcrossRotate$' -count=1` fails with expected `[]int64{1}` and actual `[]int64{0}`. The reproducer models the real interleaving where `Add()` releases `windowLock` after selecting or creating the record, rotate swaps and persists the old window, and only then does `record.Add(info)` update the old object that is no longer current and has already been written.

- Observation: statement-summary user grouping and privilege checks reduce account identity to `Username` only, while TiDB's authenticated account identity includes host matching.
  Evidence: `pkg/executor/adapter.go` assigns `StmtExecInfo.User = sessVars.User.Username`; v1 `reader.go` and v2 `reader.go` check `authUsers[user.Username]`; `auth.UserIdentity` carries `AuthUsername` and `AuthHostname`, and privilege code uses both.

## Decision Log

- Decision: Audit the expected post-PR state by applying PR #69914 to current `origin/master`, and compare suspicious behavior with both raw revisions.
  Rationale: the request follows review of PR #69914, so excluding its changes would rediscover already fixed bugs, while auditing only the older PR base would miss integration with the latest branch.
  Date/Author: 2026-07-23, Codex.

- Decision: Treat the previously confirmed v2 `Add`/`ClearInternal` race as a known baseline issue, not a new audit discovery, but retain it in the final report for completeness.
  Rationale: the user explicitly asked for bugs beyond this issue, while the deliverable must still explain the full tested state.
  Date/Author: 2026-07-23, Codex.

- Decision: Prefer focused unit/race tests before starting TiUP.
  Rationale: most state-machine, lock, interval, and persistence behavior is directly testable in the owning packages. A real cluster is reserved for behavior that depends on SQL routing, real TiKV, multi-node aggregation, or process lifecycle.
  Date/Author: 2026-07-23, Codex.

- Decision: Classify capacity shrink data loss as a confirmed behavior defect, while noting the metric-count compatibility wrinkle separately.
  Rationale: both implementations demonstrably discard excess rows without merging them into `Others` or preserving the executions anywhere. Existing tests that expect no eviction metric on resize do not justify silent loss of SQL-visible summary data; the metric semantics can be fixed separately from preserving the data.
  Date/Author: 2026-07-23, Codex.

- Decision: Treat temporary tests as executable audit evidence, not production changes.
  Rationale: the user requested a report rather than fixes. Assertions describe intended behavior and intentionally fail on the audited tree; all diagnostic files will be removed after exact commands and output are preserved.
  Date/Author: 2026-07-23, Codex.

## Outcomes & Retrospective

The audit produced `statement-summary-v1-v2-audit-report.md`. The report records the audited revisions, the known PR #69914 issue, 34 newly confirmed issues or tightly related issue groups, compatibility/product decisions, rejected candidates, and reproduction commands.

Most confirmed issues fall into five root-cause clusters:

- v2 persistent-mode SQL route/reader gaps, especially cumulative-table routing and time predicate pushdown.
- `Others` and eviction accounting inconsistencies, including incomplete metric merge, user/resource-group isolation, capacity shrink, and internal-query clearing.
- v2 persistence/history reader resource and correctness problems, including FD leaks, broad file matching, out-of-order early stop, oversized-write loss, and unbounded work/memory.
- concurrency gaps where v2 releases the window lock before the selected record update, allowing rotate/flush or option clear to observe stale state.
- shared identity/key bugs, including username-only authorization and ambiguous `StmtDigestKey` field boundaries.

Temporary diagnostic tests were removed from the synthetic worktree. `make bazel_prepare` was rerun there to remove temporary BUILD metadata, and both statement-summary synthetic worktrees under `/tmp` were deleted after evidence was copied into the report. The main worktree now contains only the two Markdown audit artifacts.

## Context and Orientation

The v1 cache is the global `stmtSummaryByDigestMap` in `pkg/util/stmtsummary/statement_summary.go`. Its `SimpleLRUCache` retains one object per digest key, while each object may retain multiple time intervals. Evicted records are merged into interval-specific `Others` data in `pkg/util/stmtsummary/evicted.go`. SQL row conversion occurs in `pkg/util/stmtsummary/reader.go`.

The v2 cache is `StmtSummary` in `pkg/util/stmtsummary/v2/stmtsummary.go`. It retains one current `stmtWindow`, rotates windows, persists completed windows through `stmtStorage`, optionally logs individual evictions, and exposes data through the readers in `pkg/util/stmtsummary/v2/reader.go`. `lockedStmtRecord` protects mutable `StmtRecord` fields.

Runtime variables dispatch through compatibility wrappers and callbacks outside these packages. The audit must therefore trace callers under `pkg/sessionctx/variable`, `pkg/domain`, information-schema table construction, executor statement collection, and server setup/shutdown. A digest is the normalized statement identity used as part of the cache key. `Others` is the aggregate row containing summaries removed by LRU capacity eviction.

PR #69914 consists of commits `997c0e3f1f1908d4563f0c9b218dae241e4895b3` and `46dc2b36c215a7430451e450ff879f211e98b083`. Current `origin/master` for this audit is `ed2376acc6e0feeff9f3e2c38db489727933aa80`.

## Plan of Work

First, create a temporary worktree from current `origin/master` and apply PR #69914 without modifying the user's checked-out branch. Inventory every production file, mutable field, goroutine, lock, cache callback, variable callback, reader, and storage boundary. Record the ownership and lock rules in working notes.

Second, run three independent reviews in parallel. The v1 reviewer follows add, interval rollover, history resizing, cache resizing, enable/disable, internal-query clearing, eviction, authorization, and all table readers. The v2 reviewer follows add, rotation, persistence, log serialization, history readers, option changes, close/flush, and error paths. The cross-cutting reviewer checks lock order, race potential, goroutine/channel lifecycle, pooled-object ownership, file-descriptor and memory behavior, hot-path cost, and parity between v1/v2 and SQL-visible tables.

Third, triage candidates centrally. For each credible candidate, add only a temporary diagnostic test in the temporary worktree, using nearby test helpers. Run it against the affected revision and, when useful, against the comparison revision. Use `go test -race` for unsynchronized access and benchmarks or allocation measurements for performance claims. Remove temporary diagnostics after preserving exact source and output in the report. If a behavior requires a real cluster, read the official TiUP workflow and use the repository RealTiKV/TiUP skill before starting it.

Fourth, exchange the candidate list among reviewers. Each reviewer must attempt to falsify the proposed issue, check severity and scope, inspect for the same pattern elsewhere, and identify missing cases. A candidate becomes confirmed only when its code path and reproduction agree. Rejected candidates remain in working notes but only material false positives are summarized in the final report.

Finally, ask multiple reviewers for independent final passes over the post-PR tree and report draft. The audit converges when they find no additional actionable issue and agree that every confirmed issue has adequate evidence and accurately bounded impact.

## Concrete Steps

Run all commands from the repository root unless a command names another working directory.

Fetch and verify revisions:

    git fetch origin master pull/69914/head:refs/remotes/origin/pr/69914
    git rev-parse origin/master refs/remotes/origin/pr/69914
    git merge-base origin/master refs/remotes/origin/pr/69914

Create a disposable integration worktree with `mktemp -d`, add `origin/master`, and merge the PR head without committing. Record the resulting tree and remove the worktree after the audit.

Discover files and callers:

    rg --files pkg/util/stmtsummary
    rg -n "StmtSummary|statement_summary|STATEMENTS_SUMMARY|SetEnableInternal|ClearInternal" pkg cmd

For v1 package tests, first confirm failpoint use and use the wrapper:

    rg -n --fixed-strings -- "failpoint." pkg/util/stmtsummary
    ./tools/check/failpoint-go-test.sh pkg/util/stmtsummary -run '<FocusedTest>' -count=1

For v2 root-package tests, confirm the root package itself has no failpoint instrumentation, then run:

    go test -run '<FocusedTest>' -count=1 -tags=intest,deadlock ./pkg/util/stmtsummary/v2

For race candidates, run a focused temporary diagnostic:

    go test -race -run '<FocusedRaceTest>' -count=1 -tags=intest,deadlock ./pkg/util/stmtsummary/v2

Expected evidence is either `PASS`, or a focused failure/race report pointing at the hypothesized code path. Do not treat broad unrelated failures as confirmation.

## Validation and Acceptance

The report is accepted when:

1. It names exact source revisions and clearly separates known baseline issues, newly confirmed issues, and rejected candidates.
2. Every confirmed issue includes affected implementation/version, trigger, user-visible or operational impact, exact reproduction commands, expected versus actual result, and source links or repository-relative locations.
3. Race and resource claims include detector output, deterministic orchestration, or a narrowly reasoned interleaving with a runnable test.
4. Performance claims include a benchmark/workload and measured output rather than qualitative speculation.
5. At least three independent review roles have completed an initial pass, the findings have been cross-reviewed, and multiple final reviewers report no additional actionable bug.
6. Temporary worktrees and diagnostic files are removed, failpoints are disabled, and the user's original worktree contains only the two intended Markdown documents.

The final documentation validation checks Markdown links/paths, command accuracy, `git diff --check`, and the repository status. Since the deliverable is documentation and temporary diagnostics rather than a code fix, full package sweeps are run only when they materially validate a finding; `make lint` is run at Ready stage as required for repository changes.

## Idempotence and Recovery

All read-only searches and focused tests are safe to rerun. Diagnostic tests live only in an explicitly named temporary worktree and are removed with `apply_patch` after evidence is captured. Failpoint-enabled tests use `tools/check/failpoint-go-test.sh`, which disables failpoints during cleanup. A failed TiUP experiment, if needed, follows the RealTiKV runner cleanup contract and verifies that the PD endpoint is unreachable afterward.

Never reset or clean the user's main worktree. If a synthetic merge conflicts, discard only the explicit temporary worktree with `git worktree remove --force <exact-temp-path>` after confirming it contains no evidence that has not been copied into the report, then recreate it.

## Artifacts and Notes

Primary deliverable:

    statement-summary-v1-v2-audit-report.md

Living execution record:

    statement-summary-v1-v2-audit-execplan.md

The final report will include concise race-detector output and test failure excerpts rather than raw full logs.

## Interfaces and Dependencies

The audit does not change production interfaces. Important interfaces and functions include:

- `pkg/util/stmtsummary.(*stmtSummaryByDigestMap).AddStatement`
- `pkg/util/stmtsummary.(*stmtSummaryByDigestMap).SetEnabledInternalQuery`
- `pkg/util/stmtsummary.(*stmtSummaryReader).GetStmtSummaryCurrentRows`
- `pkg/util/stmtsummary.(*stmtSummaryReader).GetStmtSummaryHistoryRows`
- `pkg/util/stmtsummary/v2.(*StmtSummary).Add`
- `pkg/util/stmtsummary/v2.(*StmtSummary).ClearInternal`
- `pkg/util/stmtsummary/v2.(*StmtSummary).rotate`
- `pkg/util/stmtsummary/v2.(*StmtSummary).Close`
- the v2 `stmtStorage` implementations and readers
- runtime sysvar callbacks that select v1 or v2 behavior

Revision note: this initial plan established the audit scope, parallel review roles, reproduction standard, and convergence criteria before production-code review began.
