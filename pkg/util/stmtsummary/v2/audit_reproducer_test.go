// Copyright 2026 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build audit

package stmtsummary

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pingcap/tidb/pkg/config"
	"github.com/pingcap/tidb/pkg/meta/model"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/auth"
	"github.com/pingcap/tidb/pkg/sessionctx/stmtctx"
	"github.com/pingcap/tidb/pkg/types"
	"github.com/stretchr/testify/require"
)

func auditV2Columns(names ...string) []*model.ColumnInfo {
	columns := make([]*model.ColumnInfo, 0, len(names))
	for _, name := range names {
		columns = append(columns, &model.ColumnInfo{Name: ast.NewCIStr(name)})
	}
	return columns
}

func auditUseV2StmtLog(t *testing.T) string {
	t.Helper()
	restore := config.RestoreFunc()
	t.Cleanup(restore)
	filename := filepath.Join(t.TempDir(), "tidb-statements.log")
	config.UpdateGlobal(func(conf *config.Config) {
		conf.Instance.StmtSummaryFilename = filename
	})
	return filename
}

func auditReadAllV2HistoryRows(t *testing.T, reader *HistoryReader) [][]types.Datum {
	t.Helper()
	defer func() { require.NoError(t, reader.Close()) }()

	var allRows [][]types.Datum
	for {
		rows, err := reader.Rows()
		require.NoError(t, err)
		if rows == nil {
			return allRows
		}
		allRows = append(allRows, rows...)
	}
}

func auditSumExecCountForDigest(rows [][]types.Datum, digest string) int64 {
	var total int64
	for _, row := range rows {
		if row[0].GetString() == digest {
			total += row[1].GetInt64()
		}
	}
	return total
}

func auditV2FindRowByDigest(t *testing.T, rows [][]types.Datum, digest string) []types.Datum {
	t.Helper()
	for _, row := range rows {
		if row[0].GetString() == digest {
			return row
		}
	}
	require.Failf(t, "row not found", "digest %q not found in %v", digest, rows)
	return nil
}

func TestAuditV2SameUsernameDifferentHostsShouldNotShareVisibility(t *testing.T) {
	columns := auditV2Columns(DigestStr)
	ss := NewStmtSummary4Test(10)
	defer ss.Close()

	info := GenerateStmtExecInfo4Test("digest-owned-by-alice-host1")
	info.User = "alice"
	ss.Add(info)

	reader := NewMemReader(ss, columns, "", time.UTC,
		&auth.UserIdentity{Username: "alice", Hostname: "host2"}, false, nil, nil)

	require.Empty(t, reader.Rows(),
		"statement summary authorization should distinguish alice@host2 from alice@host1")
}

func TestAuditV2DisabledSummaryDoesNotAcceptNewStatements(t *testing.T) {
	columns := auditV2Columns(DigestStr)
	ss := NewStmtSummary4Test(10)
	defer ss.Close()
	require.NoError(t, ss.SetEnabled(false))

	ss.Add(GenerateStmtExecInfo4Test("digest-added-after-disable"))

	rows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	require.Empty(t, rows,
		"persistent statement summary should not accept new rows after SetEnabled(false)")
}

func TestAuditV2DisabledInternalQueryDoesNotAcceptNewInternalStatements(t *testing.T) {
	columns := auditV2Columns(DigestStr)
	ss := NewStmtSummary4Test(10)
	defer ss.Close()
	require.NoError(t, ss.SetEnableInternalQuery(false))

	info := GenerateStmtExecInfo4Test("digest-internal-added-while-disabled")
	info.IsInternal = true
	ss.Add(info)

	rows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	require.Empty(t, rows,
		"persistent statement summary should not accept internal rows when internal-query collection is disabled")
}

func TestAuditV2CapacityReductionIsAccountedAsEviction(t *testing.T) {
	columns := auditV2Columns(DigestStr, ExecCountStr)
	ss := NewStmtSummary4Test(3)
	defer ss.Close()

	ss.Add(GenerateStmtExecInfo4Test("digest-1"))
	ss.Add(GenerateStmtExecInfo4Test("digest-2"))
	ss.Add(GenerateStmtExecInfo4Test("digest-3"))
	require.NoError(t, ss.SetMaxStmtCount(1))

	rows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	other := auditV2FindRowByDigest(t, rows, "")
	require.Equal(t, int64(2), other[1].GetInt64(),
		"records removed by shrinking tidb_stmt_summary_max_stmt_count should be merged into Others")
}

func TestAuditV2OthersPreservesAllAggregateMetrics(t *testing.T) {
	columns := auditV2Columns(DigestStr, AvgMemArbitrationStr)
	ss := NewStmtSummary4Test(1)
	defer ss.Close()

	evicted := GenerateStmtExecInfo4Test("digest-evicted-with-mem-arbitration")
	evicted.MemArbitration = 42
	ss.Add(evicted)
	ss.Add(GenerateStmtExecInfo4Test("digest-survivor"))

	rows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	other := auditV2FindRowByDigest(t, rows, "")
	require.Equal(t, float64(42), other[1].GetFloat64(),
		"Others should preserve SQL-visible memory arbitration metrics from evicted records")
}

func TestAuditV2OthersDoesNotExposeOtherUsersMetrics(t *testing.T) {
	columns := auditV2Columns(DigestStr, ExecCountStr)
	ss := NewStmtSummary4Test(1)
	defer ss.Close()
	require.NoError(t, ss.SetGroupByUser(true))

	alice := GenerateStmtExecInfo4Test("digest-alice")
	alice.User = "alice"
	bob1 := GenerateStmtExecInfo4Test("digest-bob-1")
	bob1.User = "bob"
	bob2 := GenerateStmtExecInfo4Test("digest-bob-2")
	bob2.User = "bob"
	ss.Add(alice)
	ss.Add(bob1)
	ss.Add(bob2)

	rows := NewMemReader(ss, columns, "", time.UTC,
		&auth.UserIdentity{Username: "alice"}, false, nil, nil).Rows()
	other := auditV2FindRowByDigest(t, rows, "")
	require.Equal(t, int64(1), other[1].GetInt64(),
		"with group_by_user enabled, Alice's Others row must not include Bob's evicted executions")
}

func TestAuditAverageRequestDurationsUseExecutionCount(t *testing.T) {
	columns := auditV2Columns(DigestStr, AvgKvTimeStr)
	ss := NewStmtSummary4Test(10)
	defer ss.Close()

	info := GenerateStmtExecInfo4Test("digest-readonly-kv-wait")
	info.ExecDetail.CommitDetail = nil
	info.TiKVExecDetails.WaitKVRespDuration = int64(100 * time.Millisecond)
	ss.Add(info)

	rows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	row := auditV2FindRowByDigest(t, rows, "digest-readonly-kv-wait")
	require.Equal(t, int64(100*time.Millisecond), row[1].GetInt64(),
		"AVG_KV_TIME should divide request wait time by execution count, not commit count")
}

func TestAuditV2MemReaderNeverReturnsBeginAfterEnd(t *testing.T) {
	oldTimeNow := timeNow
	begin := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	now := begin
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = oldTimeNow })

	ss := NewStmtSummary4Test(10)
	defer ss.Close()
	ss.Add(GenerateStmtExecInfo4Test("digest-time"))
	now = begin.Add(-1 * time.Second)

	_ = NewMemReader(ss, auditV2Columns(SummaryBeginTimeStr, SummaryEndTimeStr), "", time.UTC, nil, false, nil, nil).Rows()
	record := ss.window.lru.Values()[0].(*lockedStmtRecord).StmtRecord
	require.LessOrEqual(t, record.Begin, record.End,
		"MemReader should not expose SUMMARY_BEGIN_TIME greater than SUMMARY_END_TIME if wall clock moves backward")
}

func TestAuditV2TableNamesHaveNoDanglingComma(t *testing.T) {
	info := GenerateStmtExecInfo4Test("digest-table-name")
	info.StmtCtx.Tables = []stmtctx.TableEntry{
		{DB: "db", Table: "t"},
		{DB: "db_only", Table: ""},
	}
	record := NewStmtRecord(info)
	require.Equal(t, "db.t", record.TableNames)
}

func TestAuditV2MaxSQLLengthAlsoCapsDigestText(t *testing.T) {
	ss := NewStmtSummary4Test(10)
	defer ss.Close()
	oldGlobal := GlobalStmtSummary
	GlobalStmtSummary = ss
	t.Cleanup(func() { GlobalStmtSummary = oldGlobal })
	require.NoError(t, ss.SetMaxSQLLength(4))

	info := GenerateStmtExecInfo4Test("digest-long-normalized")
	info.NormalizedSQL = strings.Repeat("x", 10)
	record := NewStmtRecord(info)
	require.Equal(t, "xxxx(len:10)", record.NormalizedSQL,
		"DIGEST_TEXT should be capped by tidb_stmt_summary_max_sql_length")
}

func TestAuditV2NewStmtSummaryReportsLoggerInitFailure(t *testing.T) {
	ss, err := NewStmtSummary(&Config{Filename: t.TempDir()})
	if ss != nil {
		defer ss.Close()
	}
	require.Error(t, err,
		"NewStmtSummary should report logger initialization errors instead of silently falling back to a nop logger")
}

func TestAuditV2RotatePersistenceShouldBeSerialized(t *testing.T) {
	storage := &auditBlockingStorage{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	ss := NewStmtSummary4Test(10)
	ss.storage = storage
	defer ss.Close()
	defer close(storage.release)

	ss.Add(GenerateStmtExecInfo4Test("digest-before-first-rotate"))
	ss.rotate(timeNow())
	<-storage.entered

	ss.Add(GenerateStmtExecInfo4Test("digest-before-second-rotate"))
	ss.rotate(timeNow())
	<-storage.entered

	require.LessOrEqual(t, storage.maxInFlight(), int32(1),
		"stmt summary persistence should serialize rotate writes instead of running concurrent unbounded persists")
}

func TestAuditV2HistoryQueryMustNotDoubleCountWindowAcrossMemAndDisk(t *testing.T) {
	filename := auditUseV2StmtLog(t)
	ss, err := NewStmtSummary(&Config{Filename: filename})
	require.NoError(t, err)
	defer ss.Close()

	columns := auditV2Columns(DigestStr, ExecCountStr)
	ss.Add(GenerateStmtExecInfo4Test("digest-snapshot"))

	// This is the same ordering as the executor: memory rows are materialized
	// before the disk history reader is created.
	memRows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	ss.flush()

	history, err := NewHistoryReader(context.Background(), columns, "", time.UTC, nil, false, nil, nil, 2)
	require.NoError(t, err)
	historyRows := auditReadAllV2HistoryRows(t, history)

	total := auditSumExecCountForDigest(memRows, "digest-snapshot") +
		auditSumExecCountForDigest(historyRows, "digest-snapshot")
	require.Equal(t, int64(1), total,
		"history query should not double count a window that moved from memory to disk during the query")
}

func TestAuditV2PersistEvictedShouldRemainVisibleInBuiltInHistory(t *testing.T) {
	filename := auditUseV2StmtLog(t)
	ss, err := NewStmtSummary(&Config{Filename: filename})
	require.NoError(t, err)

	require.NoError(t, ss.SetMaxStmtCount(1))
	require.NoError(t, ss.SetPersistEvicted(true))
	ss.Add(GenerateStmtExecInfo4Test("digest-evicted"))
	ss.Add(GenerateStmtExecInfo4Test("digest-survivor"))
	ss.Close()

	columns := auditV2Columns(DigestStr, ExecCountStr)
	history, err := NewHistoryReader(context.Background(), columns, "", time.UTC, nil, false, nil, nil, 2)
	require.NoError(t, err)
	rows := auditReadAllV2HistoryRows(t, history)

	total := auditSumExecCountForDigest(rows, "digest-evicted") +
		auditSumExecCountForDigest(rows, "digest-survivor")
	require.Equal(t, int64(2), total,
		"built-in statement summary history should include both evicted and non-evicted executions")
}

func TestAuditV2PersistDoesNotSilentlyLoseOversizedRecords(t *testing.T) {
	filename := auditUseV2StmtLog(t)
	ss, err := NewStmtSummary(&Config{
		Filename:    filename,
		FileMaxSize: 1,
	})
	require.NoError(t, err)

	info := GenerateStmtExecInfo4Test("digest-oversized-log-record")
	info.NormalizedSQL = strings.Repeat("x", 2*1024*1024)
	ss.Add(info)
	ss.Close()

	content, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Contains(t, string(content), "digest-oversized-log-record",
		"oversized statement-summary records should be persisted or surfaced as an explicit error")
}

func TestAuditV2RotatePersistsOthersWhenLRUIsEmpty(t *testing.T) {
	storage := &mockStmtStorage{}
	ss := NewStmtSummary4Test(1)
	ss.storage = storage
	defer ss.Close()

	ss.Add(GenerateStmtExecInfo4Test("digest-evicted"))
	ss.Add(GenerateStmtExecInfo4Test("digest-survivor"))
	ss.window.lru.DeleteAll()

	ss.flush()
	storage.Lock()
	defer storage.Unlock()
	require.Len(t, storage.windows, 1,
		"a window with non-empty Others should still be persisted even when the LRU is empty")
}

func TestAuditV2DisableInternalClearsInternalOthers(t *testing.T) {
	columns := auditV2Columns(DigestStr, ExecCountStr)
	ss := NewStmtSummary4Test(1)
	defer ss.Close()
	require.NoError(t, ss.SetEnableInternalQuery(true))

	internal := GenerateStmtExecInfo4Test("digest-internal")
	internal.IsInternal = true
	external := GenerateStmtExecInfo4Test("digest-external")
	ss.Add(internal)
	ss.Add(external)

	require.NoError(t, ss.SetEnableInternalQuery(false))
	rows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	for _, row := range rows {
		require.NotEqual(t, "", row[0].GetString(),
			"turning internal-query collection off should remove internal contributions already merged into Others")
	}
}

func TestAuditV2HistoryRangeFilterClosesRejectedFiles(t *testing.T) {
	filename := auditUseV2StmtLog(t)
	dir := filepath.Dir(filename)
	for i := range 5 {
		name := filepath.Join(dir, fmt.Sprintf("tidb-statements-2026-01-01T00-00-%02d.000.log", i))
		begin := int64(1767225600 + i)
		content := fmt.Sprintf("{\"begin\":%d,\"end\":%d}\n", begin, begin+1)
		require.NoError(t, os.WriteFile(name, []byte(content), 0o600))
	}

	before := auditOpenFDCount(t)
	files, err := newStmtFiles(context.Background(), []*StmtTimeRange{{Begin: 1, End: 2}})
	require.NoError(t, err)
	defer files.close()
	require.Empty(t, files.files)
	after := auditOpenFDCount(t)
	require.LessOrEqual(t, after, before+1,
		"files rejected by statement-summary history time range should be closed immediately")
}

func TestAuditV2HistoryFileSelectionDoesNotReadSiblingPrefixes(t *testing.T) {
	filename := auditUseV2StmtLog(t)
	dir := filepath.Dir(filename)
	matching := filepath.Join(dir, "tidb-statements.log")
	sibling := filepath.Join(dir, "tidb-statements-extra.log")
	require.NoError(t, os.WriteFile(matching, []byte("{\"begin\":10,\"end\":20}\n"), 0o600))
	require.NoError(t, os.WriteFile(sibling, []byte("{\"begin\":30,\"end\":40}\n"), 0o600))

	files, err := newStmtFiles(context.Background(), nil)
	require.NoError(t, err)
	defer files.close()
	require.Len(t, files.files, 1,
		"history file selection should not include sibling files that merely share the configured filename prefix")
	require.Equal(t, matching, files.files[0].file.Name())
}

func TestAuditV2AbsoluteFilenameParsesRotationEnd(t *testing.T) {
	filename := auditUseV2StmtLog(t)
	dir := filepath.Dir(filename)
	rotatedEnd := time.Date(2026, 1, 1, 0, 0, 1, 0, time.Local)
	rotated := filepath.Join(dir, "tidb-statements-2026-01-01T00-00-01.000.log")
	require.NoError(t, os.WriteFile(rotated, []byte("{\"begin\":10,\"end\":20}\n"), 0o600))

	file, err := openStmtFile(rotated)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.close()) }()
	require.Equal(t, rotatedEnd.Unix(), file.end,
		"rotation end timestamp should be parsed when tidb_stmt_summary_filename is an absolute path")
}

func TestAuditV2HistoryReaderDoesNotStopBeforeOutOfOrderOlderRecord(t *testing.T) {
	worker := stmtScanWorker{
		batchSize: batchScanSize,
		checker: &stmtChecker{timeRanges: []*StmtTimeRange{
			{Begin: 0, End: 50},
		}},
	}
	reader := bufio.NewReader(strings.NewReader(
		"{\"begin\":100,\"end\":110}\n" +
			"{\"begin\":10,\"end\":20}\n",
	))

	lines, err := worker.readlines(reader)
	require.NoError(t, err,
		"history reader should not stop at a newer out-of-range record before checking older valid records")
	require.Len(t, lines, 1)
}

func TestAuditV2HistoryReaderShouldNotOpenAllMatchingFilesAtOnce(t *testing.T) {
	filename := auditUseV2StmtLog(t)
	dir := filepath.Dir(filename)
	for i := range 10 {
		name := filepath.Join(dir, fmt.Sprintf("tidb-statements-2026-01-01T00-00-%02d.000.log", i))
		begin := int64(1767225600 + i)
		content := fmt.Sprintf("{\"begin\":%d,\"end\":%d}\n", begin, begin+1)
		require.NoError(t, os.WriteFile(name, []byte(content), 0o600))
	}

	files, err := newStmtFiles(context.Background(), nil)
	require.NoError(t, err)
	defer files.close()

	require.LessOrEqual(t, len(files.files), 2,
		"history reader should stream matching files instead of holding every matching file descriptor for the whole query")
}

func TestAuditV2EvictedKeyTrackingDefeatsCapacityBound(t *testing.T) {
	ss := NewStmtSummary4Test(1)
	defer ss.Close()
	require.NoError(t, ss.SetMaxStmtCount(1))

	for i := range 100 {
		ss.Add(GenerateStmtExecInfo4Test(fmt.Sprintf("digest-%d", i)))
	}
	require.LessOrEqual(t, ss.window.evicted.count(), int(ss.MaxStmtCount()),
		"evicted key tracking should be bounded by statement summary capacity")
}

func TestAuditV2OthersLastSeenMatchesLastExecution(t *testing.T) {
	ss := NewStmtSummary4Test(1)
	defer ss.Close()

	execTime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	evicted := GenerateStmtExecInfo4Test("digest-evicted-last-seen")
	evicted.StartTime = execTime
	ss.Add(evicted)
	ss.Add(GenerateStmtExecInfo4Test("digest-survivor"))

	ss.window.evicted.Lock()
	lastSeen := ss.window.evicted.other.LastSeen
	ss.window.evicted.Unlock()
	require.True(t, lastSeen.Equal(execTime),
		"Others LastSeen should be initialized from the evicted execution time, not wall-clock time at aggregate creation")
}

func TestAuditV2OthersResourceGroupShouldRepresentEvictedGroup(t *testing.T) {
	columns := auditV2Columns(DigestStr, ResourceGroupName)
	ss := NewStmtSummary4Test(1)
	defer ss.Close()

	first := GenerateStmtExecInfo4Test("digest-rg-1")
	first.ResourceGroupName = "rg_audit"
	second := GenerateStmtExecInfo4Test("digest-rg-2")
	second.ResourceGroupName = "rg_audit"
	ss.Add(first)
	ss.Add(second)

	rows := NewMemReader(ss, columns, "", time.UTC, nil, false, nil, nil).Rows()
	for _, row := range rows {
		if row[0].GetString() == "" {
			require.Equal(t, "rg_audit", row[1].GetString(),
				"Others should retain resource group attribution when all merged rows come from the same resource group")
			return
		}
	}
	require.Fail(t, "expected an Others row after LRU eviction")
}

func TestAuditV2EvictedCountReaderIsRaceFree(t *testing.T) {
	ss := NewStmtSummary4Test(1)
	defer ss.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range 2000 {
			w := newStmtWindow(time.Now().Add(time.Duration(i)*time.Nanosecond), 1, nil)
			w.evicted.keys["seed"] = struct{}{}
			ss.windowLock.Lock()
			ss.window = w
			ss.windowLock.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_ = ss.Evicted()
		}
	}()

	wg.Wait()
}

type auditBlockingStorage struct {
	mu       sync.Mutex
	inFlight int32
	maxSeen  int32
	entered  chan struct{}
	release  chan struct{}
}

func (s *auditBlockingStorage) persist(_ *stmtWindow, _ time.Time) {
	s.mu.Lock()
	s.inFlight++
	if s.inFlight > s.maxSeen {
		s.maxSeen = s.inFlight
	}
	s.mu.Unlock()

	s.entered <- struct{}{}
	<-s.release

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
}

func (*auditBlockingStorage) logEvicted(_ []*StmtRecord) {}

func (*auditBlockingStorage) sync() error {
	return nil
}

func (s *auditBlockingStorage) maxInFlight() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxSeen
}

func auditOpenFDCount(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	return len(entries)
}
