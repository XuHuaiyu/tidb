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

package tests

import (
	"testing"

	"github.com/pingcap/tidb/pkg/testkit"
	"github.com/stretchr/testify/require"
)

func TestAuditV2SQLCumulativeTableDoesNotPanic(t *testing.T) {
	setupStmtSummary()
	defer closeStmtSummary()

	store := testkit.CreateMockStore(t)
	tk := newTestKitWithRoot(t, store)
	tk.MustExec("set global tidb_enable_stmt_summary = 0")
	tk.MustExec("set global tidb_enable_stmt_summary = 1")
	tk.MustQuery("select 1")

	require.NotPanics(t, func() {
		tk.MustQuery("select * from information_schema.tidb_statements_stats").Rows()
	}, "v2 persistent mode should not panic when querying TIDB_STATEMENTS_STATS with SELECT *")
}

func TestAuditV2SQLCumulativeTableSupportedColumnDoesNotPanic(t *testing.T) {
	setupStmtSummary()
	defer closeStmtSummary()

	store := testkit.CreateMockStore(t)
	tk := newTestKitWithRoot(t, store)
	tk.MustExec("set global tidb_enable_stmt_summary = 0")
	tk.MustExec("set global tidb_enable_stmt_summary = 1")
	tk.MustQuery("select 1")

	require.NotPanics(t, func() {
		tk.MustQuery("select digest from information_schema.tidb_statements_stats").Rows()
	}, "v2 persistent mode should initialize a rows reader for supported cumulative-table columns")
}
