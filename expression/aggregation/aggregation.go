// Copyright 2016 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package aggregation

import (
	"bytes"

	"github.com/juju/errors"
	"github.com/pingcap/tidb/ast"
	"github.com/pingcap/tidb/expression"
	"github.com/pingcap/tidb/sessionctx/stmtctx"
	"github.com/pingcap/tidb/types"
	tipb "github.com/pingcap/tipb/go-tipb"
	"github.com/pingcap/tidb/util/chunk"
	"sync/atomic"
)

// Aggregation stands for aggregate functions.
type Aggregation interface {
	// Update during executing.
	Update(evalCtx *AggEvaluateContext, sc *stmtctx.StatementContext, row types.Row) error

	// GetPartialResult will called by coprocessor to get partial results. For avg function, partial results will return
	// sum and count values at the same time.
	GetPartialResult(evalCtx *AggEvaluateContext) []types.Datum

	// GetResult will be called when all data have been processed.
	GetResult(evalCtx *AggEvaluateContext) types.Datum

	// Create a new AggEvaluateContext for the aggregation function.
	CreateContext(sc *stmtctx.StatementContext) *AggEvaluateContext

	// Reset the content of the evaluate context.
	ResetContext(sc *stmtctx.StatementContext, evalCtx *AggEvaluateContext)
}

// NewDistAggFunc creates new Aggregate function for mock tikv.
func NewDistAggFunc(expr *tipb.Expr, fieldTps []*types.FieldType, sc *stmtctx.StatementContext) (Aggregation, error) {
	args := make([]expression.Expression, 0, len(expr.Children))
	for _, child := range expr.Children {
		arg, err := expression.PBToExpr(child, fieldTps, sc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		args = append(args, arg)
	}
	switch expr.Tp {
	case tipb.ExprType_Sum:
		return &sumFunction{aggFunction: newAggFunc(ast.AggFuncSum, args, false)}, nil
	case tipb.ExprType_Count:
		return &countFunction{aggFunction: newAggFunc(ast.AggFuncCount, args, false)}, nil
	case tipb.ExprType_Avg:
		return &avgFunction{aggFunction: newAggFunc(ast.AggFuncAvg, args, false)}, nil
	case tipb.ExprType_GroupConcat:
		return &concatFunction{aggFunction: newAggFunc(ast.AggFuncGroupConcat, args, false)}, nil
	case tipb.ExprType_Max:
		return &maxMinFunction{aggFunction: newAggFunc(ast.AggFuncMax, args, false), isMax: true}, nil
	case tipb.ExprType_Min:
		return &maxMinFunction{aggFunction: newAggFunc(ast.AggFuncMin, args, false)}, nil
	case tipb.ExprType_First:
		return &firstRowFunction{aggFunction: newAggFunc(ast.AggFuncFirstRow, args, false)}, nil
	case tipb.ExprType_Agg_BitOr:
		return &bitOrFunction{aggFunction: newAggFunc(ast.AggFuncBitOr, args, false)}, nil
	case tipb.ExprType_Agg_BitXor:
		return &bitXorFunction{aggFunction: newAggFunc(ast.AggFuncBitXor, args, false)}, nil
	case tipb.ExprType_Agg_BitAnd:
		return &bitAndFunction{aggFunction: newAggFunc(ast.AggFuncBitAnd, args, false)}, nil
	}
	return nil, errors.Errorf("Unknown aggregate function type %v", expr.Tp)
}

type Shuffle interface{
	Next(srcChk *chunk.Chunk) *chunk.Chunk
}

type HashAggTask struct {
	input *chunk.Chunk
}

type StreamAggPartialTask struct {
	input *chunk.Chunk
	begin int  // idx of input chunk.
	end int
	groupIdx int // group index in StreamAggExec.resultCh
}

type StreamAggFinalTask struct {
	interResult afIntermediateResult
	groupIdx int
}

type baseAggExecutor struct{
	partitionHandler
	shuffleHandler

	partialWorker []aggWorker
	finalWorker   []aggWorker
}

type partitionHandler interface{
	NextPart() *chunk.Chunk
}

type shuffleHandler interface{
	NextGroup() []afIntermediateResult
}

func (e *StreamAggExec) Next(chk *chunk.Chunk) {
	!e.prepared{
		// read data from child
		// shuffle data to partial workers
		Shuffle()
		// start partial workers
		for ;i < n; {
			go StreamAggWorker.runMap()
		}
		for ;i < m;{
			go StreamAggWorker.runReduce()
		}
	}
	// get result from result ch
	for rCh, ok := range resultCh {
		<-rCh
	}
}

type aggWorker interface {
	runMap()
	runReduce()
}

type baseAggWorker struct {
	aggFuncs []Aggregation
	aggCtxs  []*AggEvaluateContext
}

type StreamAggWorker struct {
	baseAggWorker
	curGroupKey []byte
}

func (*StreamAggWorker) runMap() {
	// for until StreamAggExec.partialWorkerTaskCh is closed.
	for {
		// 1. get task from partialWorkerTaskCh
		// 2. check whether equals to curGroupkey

		// 3. if so,
		for _, f := range aggFuncs {
			switch f.state {
			case Dedup:
				f.evalDedup(task.input, aggEvaluateContext) // store distinct map or partial result in aggEvaluateContext
			case Partial1:
				f.evalPartial1()
			case Partial2:
				f.evalPartial2()
			}
		}
		//if not, encode intermediate result from aggEvaluateContext and pass to final worker

	}
}

func (*StreamAggWorker) runReduce(){
	// for until StreamAggExec.finalWorkerTaskCh is closed.
	for {
		// 1. get task from finalWorkerTaskCh
		// 2. check whether the parts of curGroupKey are all fetched.

		// 3. if so,
		for _, f := range aggFuncs{
			switch f.state{
			case Complete:

			case Final:
			}
		}
		// output the final result from aggEvaluateContext and pass to specific ch.
		// if not, continue
	}
}

type StreamAggExec struct{
	baseAggExecutor

	partialWorkerTaskCh chan StreamAggPartialTask
	finalWorkerTaskCh chan StreamAggFinalTask
	resultCh []chan chunk.Row  // const len, resultCh may need to be extracted as a Struct
	paritalWorkers []StreamAggWorker
	finalWorkers []StreamAggWorker
}

type AFEvaluator interface{
	evalDedup(chk *chunk.Chunk)
	evalPartial1(chk *chunk.Chunk)
	evalPartial2(chk *chunk.Chunk)
	evalFinal(interResult *afIntermediateResult)
	evalComplete(interResult *afIntermediateResult)
}

type afIntermediateResult struct{
	groupKey []byte
	intermediateResult [][]byte
}

// AggEvaluateContext is used to store intermediate result when calculating aggregate functions.
type AggEvaluateContext struct {
	DistinctChecker *distinctChecker
	Count           int64
	Value           types.Datum
	Buffer          *bytes.Buffer // Buffer is used for group_concat.
	GotFirstRow     bool          // It will check if the agg has met the first row key.
}

// AggFunctionMode stands for the aggregation function's mode.
type AggFunctionMode int

const (
	// CompleteMode function accepts origin data.
	CompleteMode AggFunctionMode = iota
	// FinalMode function accepts partial data.
	FinalMode
)

type aggFunction struct {
	*AggFuncDesc
}

func newAggFunc(funcName string, args []expression.Expression, hasDistinct bool) aggFunction {
	return aggFunction{AggFuncDesc: &AggFuncDesc{
		Name:        funcName,
		Args:        args,
		HasDistinct: hasDistinct,
	}}
}

// CreateContext implements Aggregation interface.
func (af *aggFunction) CreateContext(sc *stmtctx.StatementContext) *AggEvaluateContext {
	evalCtx := &AggEvaluateContext{}
	if af.HasDistinct {
		evalCtx.DistinctChecker = createDistinctChecker(sc)
	}
	return evalCtx
}

func (af *aggFunction) ResetContext(sc *stmtctx.StatementContext, evalCtx *AggEvaluateContext) {
	if af.HasDistinct {
		evalCtx.DistinctChecker = createDistinctChecker(sc)
	}
	evalCtx.Value.SetNull()
}

func (af *aggFunction) updateSum(sc *stmtctx.StatementContext, evalCtx *AggEvaluateContext, row types.Row) error {
	a := af.Args[0]
	value, err := a.Eval(row)
	if err != nil {
		return errors.Trace(err)
	}
	if value.IsNull() {
		return nil
	}
	if af.HasDistinct {
		d, err1 := evalCtx.DistinctChecker.Check([]types.Datum{value})
		if err1 != nil {
			return errors.Trace(err1)
		}
		if !d {
			return nil
		}
	}
	evalCtx.Value, err = calculateSum(sc, evalCtx.Value, value)
	if err != nil {
		return errors.Trace(err)
	}
	evalCtx.Count++
	return nil
}
