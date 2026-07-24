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
	"container/list"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pingcap/tidb/pkg/meta/model"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/auth"
	"github.com/pingcap/tidb/pkg/sessionctx/stmtctx"
	"github.com/pingcap/tidb/pkg/types"
	"github.com/stretchr/testify/require"
)

func auditV1Columns(names ...string) []*model.ColumnInfo {
	columns := make([]*model.ColumnInfo, 0, len(names))
	for _, name := range names {
		columns = append(columns, &model.ColumnInfo{Name: ast.NewCIStr(name)})
	}
	return columns
}

func auditV1FindRowByDigest(t *testing.T, rows [][]types.Datum, digest string) []types.Datum {
	t.Helper()
	for _, row := range rows {
		if row[0].GetString() == digest {
			return row
		}
	}
	require.Failf(t, "row not found", "digest %q not found in %v", digest, rows)
	return nil
}

func TestAuditV1SameUsernameDifferentHostsShouldNotShareVisibility(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()

	info := generateAnyExecInfo()
	info.Digest = "digest-owned-by-alice-host1"
	info.User = "alice"
	ssMap.AddStatement(info)

	reader := NewStmtSummaryReader(&auth.UserIdentity{Username: "alice", Hostname: "host2"},
		false, auditV1Columns(DigestStr), "", time.UTC)
	reader.ssMap = ssMap

	require.Empty(t, reader.GetStmtSummaryCurrentRows(),
		"statement summary authorization should distinguish alice@host2 from alice@host1")
}

func TestAuditV1ClearHistoryKeepsCurrentInterval(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()

	key := &StmtDigestKey{}
	key.Init("schema", "digest", "", "plan", "", "")
	ssbd := &stmtSummaryByDigest{initialized: true, history: list.New()}
	ssbd.history.PushBack(newInduceSsbde(100, 200))
	ssbd.history.PushBack(newInduceSsbde(200, 300))
	ssMap.summaryMap.Put(key, ssbd)

	ssMap.clearHistory()
	require.Equal(t, 1, ssbd.history.Len())
	kept := ssbd.history.Back().Value.(*stmtSummaryByDigestElement)
	require.Equal(t, int64(200), kept.beginTime,
		"clearing history should keep the current/newest interval")
}

func TestAuditV1HistorySizeReductionKeepsNewestIntervals(t *testing.T) {
	ssbd := &stmtSummaryByDigest{initialized: true, history: list.New()}
	ssbd.history.PushBack(newInduceSsbde(100, 200))
	ssbd.history.PushBack(newInduceSsbde(200, 300))

	rows := ssbd.collectHistorySummaries(nil, 1)
	require.Len(t, rows, 1)
	require.Equal(t, int64(200), rows[0].beginTime,
		"history collection should return the newest intervals when history size is reduced")

	evicted := newStmtSummaryByDigestEvicted()
	evicted.history.PushBack(newStmtSummaryByDigestEvictedElement(100, 200))
	evicted.history.PushBack(newStmtSummaryByDigestEvictedElement(200, 300))
	evictedRows := evicted.collectHistorySummaries(1)
	require.Len(t, evictedRows, 1)
	require.Equal(t, int64(200), evictedRows[0].beginTime,
		"evicted history collection should also keep the newest intervals")
}

func TestAuditV1PlanEncodingFailureDoesNotPanic(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()

	info := generateAnyExecInfo()
	info.LazyInfo = auditFailingPlanLazyInfo{}
	require.NotPanics(t, func() {
		ssMap.AddStatement(info)
	}, "statement summary should not panic when plan encoding fails")
}

func TestAuditStmtDigestKeyHasUnambiguousFieldBoundaries(t *testing.T) {
	var left, right StmtDigestKey
	left.Init("", "digest", "", "abc", "", "")
	right.Init("a", "digest", "", "bc", "", "")

	require.NotEqual(t, string(left.Hash()), string(right.Hash()),
		"different logical digest/schema/plan/resource-group tuples must not serialize to the same LRU key")
}

func TestAuditV1CapacityReductionIsAccountedAsEviction(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()
	require.NoError(t, ssMap.SetMaxStmtCount(3))

	for _, digest := range []string{"digest-1", "digest-2", "digest-3"} {
		info := generateAnyExecInfo()
		info.Digest = digest
		ssMap.AddStatement(info)
	}
	require.NoError(t, ssMap.SetMaxStmtCount(1))

	reader := NewStmtSummaryReader(nil, false, auditV1Columns(DigestStr, ExecCountStr), "", time.UTC)
	reader.ssMap = ssMap
	rows := reader.GetStmtSummaryCurrentRows()
	other := auditV1FindRowByDigest(t, rows, "")
	require.Equal(t, int64(2), other[1].GetInt64(),
		"records removed by shrinking tidb_stmt_summary_max_stmt_count should be merged into Others")
}

func TestAuditV1OthersPreservesAllAggregateMetrics(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()
	require.NoError(t, ssMap.SetMaxStmtCount(1))

	evicted := generateAnyExecInfo()
	evicted.Digest = "digest-evicted-result-rows"
	evicted.ResultRows = 42
	ssMap.AddStatement(evicted)
	survivor := generateAnyExecInfo()
	survivor.Digest = "digest-survivor"
	ssMap.AddStatement(survivor)

	reader := NewStmtSummaryReader(nil, false, auditV1Columns(DigestStr, MaxResultRowsStr), "", time.UTC)
	reader.ssMap = ssMap
	rows := reader.GetStmtSummaryCurrentRows()
	other := auditV1FindRowByDigest(t, rows, "")
	require.Equal(t, int64(42), other[1].GetInt64(),
		"Others should preserve SQL-visible result row metrics from evicted records")
}

func TestAuditV1OthersDoesNotExposeOtherUsersMetrics(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()
	require.NoError(t, ssMap.SetMaxStmtCount(1))
	require.NoError(t, ssMap.SetGroupByUser(true))

	alice := generateAnyExecInfo()
	alice.Digest = "digest-alice"
	alice.User = "alice"
	bob1 := generateAnyExecInfo()
	bob1.Digest = "digest-bob-1"
	bob1.User = "bob"
	bob2 := generateAnyExecInfo()
	bob2.Digest = "digest-bob-2"
	bob2.User = "bob"
	ssMap.AddStatement(alice)
	ssMap.AddStatement(bob1)
	ssMap.AddStatement(bob2)

	reader := NewStmtSummaryReader(&auth.UserIdentity{Username: "alice"},
		false, auditV1Columns(DigestStr, ExecCountStr), "", time.UTC)
	reader.ssMap = ssMap
	rows := reader.GetStmtSummaryCurrentRows()
	other := auditV1FindRowByDigest(t, rows, "")
	require.Equal(t, int64(1), other[1].GetInt64(),
		"with group_by_user enabled, Alice's Others row must not include Bob's evicted executions")
}

func TestAuditV1AverageRequestDurationsUseExecutionCount(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()

	info := generateAnyExecInfo()
	info.Digest = "digest-readonly-kv-wait"
	info.ExecDetail.CommitDetail = nil
	info.TiKVExecDetails.WaitKVRespDuration = int64(100 * time.Millisecond)
	ssMap.AddStatement(info)

	reader := NewStmtSummaryReader(nil, false, auditV1Columns(DigestStr, AvgKvTimeStr), "", time.UTC)
	reader.ssMap = ssMap
	row := auditV1FindRowByDigest(t, reader.GetStmtSummaryCurrentRows(), "digest-readonly-kv-wait")
	require.Equal(t, int64(100*time.Millisecond), row[1].GetInt64(),
		"AVG_KV_TIME should divide request wait time by execution count, not commit count")
}

func TestAuditV1TableNamesHaveNoDanglingComma(t *testing.T) {
	info := generateAnyExecInfo()
	info.StmtCtx.Tables = []stmtctx.TableEntry{
		{DB: "db", Table: "t"},
		{DB: "db_only", Table: ""},
	}
	ssbd := &stmtSummaryByDigest{}
	ssbd.init(info, 0, 60, 1)
	require.Equal(t, "db.t", ssbd.tableNames)
}

func TestAuditV1DisableInternalClearsInternalOthers(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()
	require.NoError(t, ssMap.SetMaxStmtCount(1))
	require.NoError(t, ssMap.SetEnabledInternalQuery(true))

	internal := generateAnyExecInfo()
	internal.Digest = "digest-internal"
	internal.IsInternal = true
	external := generateAnyExecInfo()
	external.Digest = "digest-external"
	ssMap.AddStatement(internal)
	ssMap.AddStatement(external)

	require.NoError(t, ssMap.SetEnabledInternalQuery(false))
	reader := NewStmtSummaryReader(nil, false, auditV1Columns(DigestStr, ExecCountStr), "", time.UTC)
	reader.ssMap = ssMap
	rows := reader.GetStmtSummaryCurrentRows()
	for _, row := range rows {
		require.NotEqual(t, "", row[0].GetString(),
			"turning internal-query collection off should remove internal contributions already merged into Others")
	}
}

func TestAuditV1OthersResourceGroupShouldRepresentEvictedGroup(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()
	require.NoError(t, ssMap.SetMaxStmtCount(1))

	first := generateAnyExecInfo()
	first.Digest = "digest-rg-1"
	first.ResourceGroupName = "rg_audit"
	second := generateAnyExecInfo()
	second.Digest = "digest-rg-2"
	second.ResourceGroupName = "rg_audit"
	ssMap.AddStatement(first)
	ssMap.AddStatement(second)

	reader := NewStmtSummaryReader(nil, false, auditV1Columns(DigestStr, ResourceGroupName), "", time.UTC)
	reader.ssMap = ssMap
	rows := reader.GetStmtSummaryCurrentRows()
	for _, row := range rows {
		if row[0].GetString() == "" {
			require.Equal(t, "rg_audit", row[1].GetString(),
				"Others should retain resource group attribution when all merged rows come from the same resource group")
			return
		}
	}
	require.Fail(t, "expected an Others row after LRU eviction")
}

func TestAuditV1EvictedCountReaderIsRaceFree(t *testing.T) {
	ssMap := newStmtSummaryByDigestMap()
	ssMap.Clear()
	require.NoError(t, ssMap.SetMaxStmtCount(1))

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range 2000 {
			info := generateAnyExecInfo()
			info.Digest = fmt.Sprintf("audit-v1-race-digest-%d", i)
			ssMap.AddStatement(info)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_ = ssMap.ToEvictedCountDatum()
		}
	}()

	wg.Wait()
}

type auditFailingPlanLazyInfo struct{}

func (auditFailingPlanLazyInfo) GetOriginalSQL() string {
	return "select 1"
}

func (auditFailingPlanLazyInfo) GetEncodedPlan() (string, string, any) {
	return "", "", errors.New("audit plan encoding failure")
}

func (auditFailingPlanLazyInfo) GetBinaryPlan() string {
	return ""
}

func (auditFailingPlanLazyInfo) GetPlanDigest() string {
	return ""
}

func (auditFailingPlanLazyInfo) GetBindingSQLAndDigest() (string, string) {
	return "", ""
}
