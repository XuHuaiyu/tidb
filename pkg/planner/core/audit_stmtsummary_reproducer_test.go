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

package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuditStatementsSummarySingleSidedTimePredicateStaysOpenEnded(t *testing.T) {
	start := time.Date(2026, 7, 18, 16, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	extractor := &StatementsSummaryExtractor{}

	lowerOnly := extractor.buildTimeRange(start.UnixNano(), 0)
	require.True(t, lowerOnly.EndTime.IsZero(),
		"a predicate such as SUMMARY_END_TIME >= start must not be narrowed to [start, start+1h]")

	upperOnly := extractor.buildTimeRange(0, end.UnixNano())
	require.True(t, upperOnly.StartTime.IsZero(),
		"a predicate such as SUMMARY_BEGIN_TIME <= end must not be narrowed to [end-1h, end]")
}
