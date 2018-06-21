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

package executor

import (
	"sync"

	"github.com/juju/errors"
	"github.com/pingcap/tidb/expression"
	"github.com/pingcap/tidb/expression/aggregation"
	"github.com/pingcap/tidb/mysql"
	"github.com/pingcap/tidb/sessionctx"
	"github.com/pingcap/tidb/sessionctx/stmtctx"
	"github.com/pingcap/tidb/types"
	"github.com/pingcap/tidb/util/chunk"
	"github.com/pingcap/tidb/util/codec"
	"github.com/pingcap/tidb/util/mvmap"
	"github.com/spaolacci/murmur3"
	"golang.org/x/net/context"
)

type aggCtxsMapper map[string][]*aggregation.AggEvaluateContext

// baseHashAggWorker stores the common attributes of HashAggFinalWorker and HashAggPartialWorker.
type baseHashAggWorker struct {
	finishCh       <-chan struct{}
	aggFuncs       []aggregation.Aggregation
	groupValDatums []types.Datum
	groupKey       []byte
	groupVals      [][]byte
	maxChunkSize   int
}

// HashAggPartialWorker does the following things:
// todo: graph to be added.
type HashAggPartialWorker struct {
	baseHashAggWorker

	inputCh        chan *chunk.Chunk
	outputChs      []chan *HashAggIntermData
	globalOutputCh chan *AfFinalResult
	giveBackCh     chan<- *HashAggInput
	aggCtxsMap     aggCtxsMapper
	groupByItems   []expression.Expression
	chk            *chunk.Chunk
}

// HashAggFinalWorker does the following things:
// todo: graph to be added.
type HashAggFinalWorker struct {
	baseHashAggWorker

	rowBuffer            []types.Datum
	mutableRow           chunk.MutRow
	aggCtxsMap           aggCtxsMapper
	groupSet             *mvmap.MVMap
	intermDataRowsBuffer []types.DatumRow
	inputCh              chan *HashAggIntermData
	outputCh             chan *AfFinalResult
	finalResultHolderCh  chan *chunk.Chunk
}

// AfFinalResult indicates aggregation functions final result.
type AfFinalResult struct {
	chk *chunk.Chunk
	err error

	giveBackCh chan *chunk.Chunk
}

// HashAggExec deals with all the aggregate functions.
// It is built from the Aggregate Plan. When Next() is called, it reads all the data from Src
// and updates all the items in AggFuncs.
type HashAggExec struct {
	baseExecutor

	prepared      bool
	sc            *stmtctx.StatementContext
	AggFuncs      []aggregation.Aggregation
	aggCtxsMap    aggCtxsMapper
	groupMap      *mvmap.MVMap
	groupIterator *mvmap.Iterator
	mutableRow    chunk.MutRow
	rowBuffer     []types.Datum
	GroupByItems  []expression.Expression
	groupKey      []byte
	groupVals     [][]byte

	// After we support parallel execution for aggregation functions with distinct,
	// we can remove this attribute.
	doesUnparallelExec bool

	finishCh         chan struct{}
	finalOutputCh    chan *AfFinalResult
	partialOutputChs []chan *HashAggIntermData
	inputCh          chan *HashAggInput
	partialWorkers   []HashAggPartialWorker
	finalWorkers     []HashAggFinalWorker
	defaultVal       *chunk.Chunk
	// isInputNull indicates whether the child only returns empty input.
	isInputNull bool
}

// HashAggInput indicates the input of hash agg exec.
type HashAggInput struct {
	chk        *chunk.Chunk
	giveBackCh chan<- *chunk.Chunk
}

// HashAggIntermData indicates the intermediate data of aggregation execution.
type HashAggIntermData struct {
	groupSet    *chunk.Chunk
	iter        *chunk.Iterator4Chunk
	groupCtxMap aggCtxsMapper
}

// ToRows converts HashAggInterData to Rows.
func (d *HashAggIntermData) ToRows(sc *stmtctx.StatementContext, rows []types.DatumRow, aggFuncs []aggregation.Aggregation, maxChunkSize int) (_ []types.DatumRow, reachEnd bool) {
	var chunkRow chunk.Row
	if d.iter == nil {
		d.iter = chunk.NewIterator4Chunk(d.groupSet)
		chunkRow = d.iter.Begin()
	}
	for ; chunkRow != d.iter.End(); chunkRow = d.iter.Next() {
		groupKey := chunkRow.GetString(0)
		row := make(types.DatumRow, 0, len(aggFuncs)*2)
		aggCtxs := d.groupCtxMap[groupKey]
		for i, f := range aggFuncs {
			for _, d := range f.GetPartialResult(aggCtxs[i]) {
				row = append(row, d)
			}
		}
		// Append groupKey as the last element.
		row = append(row, types.NewStringDatum(groupKey))
		rows = append(rows, row)
		if len(rows) == maxChunkSize {
			return rows, false
		}
	}
	return rows, true
}

// Close implements the Executor Close interface.
func (e *HashAggExec) Close() error {
	if e.doesUnparallelExec {
		e.groupMap = nil
		e.groupIterator = nil
		e.aggCtxsMap = nil
	} else {
		// `Close` may be called after `Open` without calling `Next` in test.
		if !e.prepared {
			close(e.inputCh)
			for _, ch := range e.partialOutputChs {
				close(ch)
			}
			close(e.finalOutputCh)
		}
		close(e.finishCh)
		for _, ch := range e.partialOutputChs {
			for range ch {
			}
		}
		for range e.finalOutputCh {
		}
	}
	return errors.Trace(e.baseExecutor.Close())
}

// Open implements the Executor Open interface.
func (e *HashAggExec) Open(ctx context.Context) error {
	if err := e.baseExecutor.Open(ctx); err != nil {
		return errors.Trace(err)
	}
	e.prepared = false

	if e.doesUnparallelExec {
		e.initForUnparallelExec()
		return nil
	}
	e.initForParallelExec()
	return nil
}

func (e *HashAggExec) initForUnparallelExec() {
	e.groupMap = mvmap.NewMVMap()
	e.groupIterator = e.groupMap.NewIterator()
	e.aggCtxsMap = make(aggCtxsMapper, 0)
	e.mutableRow = chunk.MutRowFromTypes(e.retTypes())
	e.rowBuffer = make([]types.Datum, 0, e.Schema().Len())
	e.groupKey = make([]byte, 0, 8)
	e.groupVals = make([][]byte, 0, 8)
}

func (e *HashAggExec) initForParallelExec() {
	sessionVars := e.ctx.GetSessionVars()
	finalConcurrency := sessionVars.HashAggFinalConcurrency
	partialConcurrency := sessionVars.HashAggPartialConcurrency

	e.isInputNull = true
	e.finalOutputCh = make(chan *AfFinalResult, finalConcurrency)
	e.inputCh = make(chan *HashAggInput, partialConcurrency)
	e.finishCh = make(chan struct{}, 1)

	e.partialOutputChs = make([]chan *HashAggIntermData, finalConcurrency)
	for i := range e.partialOutputChs {
		e.partialOutputChs[i] = make(chan *HashAggIntermData, partialConcurrency)
	}
	e.partialWorkers = make([]HashAggPartialWorker, partialConcurrency)
	e.finalWorkers = make([]HashAggFinalWorker, finalConcurrency)
	// Init partial workers.
	for i := 0; i < partialConcurrency; i++ {
		baseAggWorker := baseHashAggWorker{
			finishCh:     e.finishCh,
			aggFuncs:     e.AggFuncs,
			maxChunkSize: e.maxChunkSize,
			groupVals:    make([][]byte, 0, 8),
		}

		w := HashAggPartialWorker{
			baseHashAggWorker: baseAggWorker,
			inputCh:           make(chan *chunk.Chunk, 1),
			outputChs:         e.partialOutputChs,
			giveBackCh:        e.inputCh,
			globalOutputCh:    e.finalOutputCh,
			aggCtxsMap:        make(aggCtxsMapper, 0),
			groupByItems:      e.GroupByItems,
			chk:               e.children[0].newChunk(),
		}

		e.partialWorkers[i] = w
		e.inputCh <- &HashAggInput{
			chk:        e.children[0].newChunk(),
			giveBackCh: w.inputCh,
		}
	}

	// Init final workers.
	finalAggFuncs := e.newFinalAggFuncs()
	for i := 0; i < finalConcurrency; i++ {
		baseAggWorker := baseHashAggWorker{
			finishCh:     e.finishCh,
			aggFuncs:     finalAggFuncs,
			maxChunkSize: e.maxChunkSize,
			groupVals:    make([][]byte, 0, 8),
		}
		e.finalWorkers[i] = HashAggFinalWorker{
			baseHashAggWorker:   baseAggWorker,
			aggCtxsMap:          make(aggCtxsMapper, 0),
			groupSet:            mvmap.NewMVMap(),
			inputCh:             e.partialOutputChs[i],
			outputCh:            e.finalOutputCh,
			finalResultHolderCh: make(chan *chunk.Chunk, 1),
			rowBuffer:           make([]types.Datum, 0, e.Schema().Len()),
			mutableRow:          chunk.MutRowFromTypes(e.retTypes()),
		}
		e.finalWorkers[i].finalResultHolderCh <- e.newChunk()
	}
}

func (e *HashAggExec) newFinalAggFuncs() (newAggFuncs []aggregation.Aggregation) {
	newAggFuncs = make([]aggregation.Aggregation, 0, len(e.AggFuncs))
	idx := 0
	for _, af := range e.AggFuncs {
		var aggFunc aggregation.Aggregation
		idx, aggFunc = af.GetFinalAggFunc(idx)
		newAggFuncs = append(newAggFuncs, aggFunc)
	}
	return newAggFuncs
}

// HashAggPartialWorker gets and handles origin data or partial data from inputCh,
// then shuffle the intermediate results to corresponded final workers.
func (w *HashAggPartialWorker) run(ctx sessionctx.Context, waitGroup *sync.WaitGroup, finalConcurrency int) {
	needShuffle, sc := false, ctx.GetSessionVars().StmtCtx
	defer func() {
		if needShuffle {
			w.shuffleIntermData(sc, finalConcurrency)
		}
		waitGroup.Done()
	}()
	for {
		select {
		case <-w.finishCh:
			return
		case chk, ok := <-w.inputCh:
			if !ok {
				return
			}
			w.chk.SwapColumns(chk)
			w.giveBackCh <- &HashAggInput{
				chk:        chk,
				giveBackCh: w.inputCh,
			}
		}

		if err := w.updateIntermData(ctx, sc, w.chk, len(w.aggCtxsMap)); err != nil {
			w.globalOutputCh <- &AfFinalResult{err: errors.Trace(err)}
			return
		}
		needShuffle = true
	}
}

func (w *HashAggPartialWorker) updateIntermData(ctx sessionctx.Context, sc *stmtctx.StatementContext, chk *chunk.Chunk, finalConcurrency int) (err error) {
	inputIter := chunk.NewIterator4Chunk(chk)
	for row := inputIter.Begin(); row != inputIter.End(); row = inputIter.Next() {
		groupKey, err := w.getGroupKey(sc, row)
		if err != nil {
			return errors.Trace(err)
		}
		aggEvalCtxs := w.getContext(sc, groupKey, w.aggCtxsMap)
		for i, af := range w.aggFuncs {
			if af.Update(aggEvalCtxs[i], sc, row) != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// shuffleIntermData shuffles the intermediate data of partial workers to corresponded final workers.
// We only support parallel execution for single-machine, so process of encode and decode can be skipped.
func (w *HashAggPartialWorker) shuffleIntermData(sc *stmtctx.StatementContext, finalConcurrency int) {
	groupKeyChks := make([]*chunk.Chunk, finalConcurrency)
	for groupKey := range w.aggCtxsMap {
		finalWorkerIdx := int(murmur3.Sum32([]byte(groupKey))) % finalConcurrency
		if groupKeyChks[finalWorkerIdx] == nil {
			groupKeyChks[finalWorkerIdx] = chunk.NewChunkWithCapacity([]*types.FieldType{types.NewFieldType(mysql.TypeVarString)}, len(w.aggCtxsMap)/finalConcurrency)
		}
		groupKeyChks[finalWorkerIdx].AppendString(0, groupKey)
	}

	for i := range groupKeyChks {
		if groupKeyChks[i] == nil {
			continue
		}
		w.outputChs[i] <- &HashAggIntermData{
			groupSet:    groupKeyChks[i],
			groupCtxMap: w.aggCtxsMap,
		}
	}
}

// getGroupKey evaluates the group items and args of aggregate functions.
func (w *HashAggPartialWorker) getGroupKey(sc *stmtctx.StatementContext, row chunk.Row) ([]byte, error) {
	w.groupValDatums = w.groupValDatums[:0]
	for _, item := range w.groupByItems {
		v, err := item.Eval(row)
		if item.GetType().Tp == mysql.TypeNewDecimal {
			v.SetLength(0)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		w.groupValDatums = append(w.groupValDatums, v)
	}
	var err error
	w.groupKey, err = codec.EncodeValue(sc, w.groupKey[:0], w.groupValDatums...)
	return w.groupKey, errors.Trace(err)
}

func (w baseHashAggWorker) getContext(sc *stmtctx.StatementContext, groupKey []byte, mapper aggCtxsMapper) []*aggregation.AggEvaluateContext {
	aggCtxs, ok := mapper[string(groupKey)]
	if !ok {
		aggCtxs = make([]*aggregation.AggEvaluateContext, 0, len(w.aggFuncs))
		for _, af := range w.aggFuncs {
			aggCtxs = append(aggCtxs, af.CreateContext(sc))
		}
		mapper[string(groupKey)] = aggCtxs
	}
	return aggCtxs
}

func (w *HashAggFinalWorker) fetchIntermData(sc *stmtctx.StatementContext) (err error) {
	var (
		input *HashAggIntermData
		ok    bool
	)
	for {
		if input == nil {
			select {
			case <-w.finishCh:
				return
			case input, ok = <-w.inputCh:
				if ok && w.intermDataRowsBuffer == nil {
					w.intermDataRowsBuffer = make([]types.DatumRow, 0, w.maxChunkSize)
				}
			}
		}
		if !ok || len(w.intermDataRowsBuffer) == w.maxChunkSize {
			for _, row := range w.intermDataRowsBuffer {
				groupKey := row.GetBytes(row.Len() - 1)
				if len(w.groupSet.Get(groupKey, w.groupVals[:0])) == 0 {
					w.groupSet.Put(groupKey, []byte{})
				}
				aggEvalCtxs := w.getContext(sc, groupKey, w.aggCtxsMap)
				for i, af := range w.aggFuncs {
					if err = af.Update(aggEvalCtxs[i], sc, row); err != nil {
						return errors.Trace(err)
					}
				}
			}
			w.intermDataRowsBuffer = w.intermDataRowsBuffer[:0]
			if !ok {
				return
			}
		}
		var reachEnd bool
		if w.intermDataRowsBuffer, reachEnd = input.ToRows(sc, w.intermDataRowsBuffer, w.aggFuncs, w.maxChunkSize); reachEnd {
			input = nil
		}
	}
	return
}

func (w *HashAggFinalWorker) getFinalResult(sc *stmtctx.StatementContext) {
	groupIter := w.groupSet.NewIterator()
	result, ok := <-w.finalResultHolderCh
	if !ok {
		return
	}
	result.Reset()
	for {
		groupKey, _ := groupIter.Next()
		if groupKey == nil {
			if result.NumRows() > 0 {
				w.outputCh <- &AfFinalResult{chk: result, giveBackCh: w.finalResultHolderCh}
			}
			return
		}
		aggCtxs := w.getContext(sc, groupKey, w.aggCtxsMap)
		w.rowBuffer = w.rowBuffer[:0]
		for i, af := range w.aggFuncs {
			w.rowBuffer = append(w.rowBuffer, af.GetResult(aggCtxs[i]))
		}
		w.mutableRow.SetDatums(w.rowBuffer...)
		result.AppendRow(w.mutableRow.ToRow())
		if result.NumRows() == w.maxChunkSize {
			w.outputCh <- &AfFinalResult{chk: result, giveBackCh: w.finalResultHolderCh}
			result, ok = <-w.finalResultHolderCh
			if !ok {
				return
			}
			result.Reset()
		}
	}
}

func (w *HashAggFinalWorker) run(ctx sessionctx.Context, waitGroup *sync.WaitGroup) {
	defer func() {
		waitGroup.Done()
	}()
	sc := ctx.GetSessionVars().StmtCtx
	if err := w.fetchIntermData(sc); err != nil {
		w.outputCh <- &AfFinalResult{err: errors.Trace(err)}
	}
	w.getFinalResult(sc)
}

// Next implements the Executor Next interface.
func (e *HashAggExec) Next(ctx context.Context, chk *chunk.Chunk) error {
	chk.Reset()
	if e.doesUnparallelExec {
		return errors.Trace(e.unparallelExec(ctx, chk))
	}
	return errors.Trace(e.parallelExec(ctx, chk))
}

func (e *HashAggExec) fetchChildData(ctx context.Context) {
	var (
		input *HashAggInput
		chk   *chunk.Chunk
		ok    bool
		err   error
	)
	defer func() {
		for _, w := range e.partialWorkers {
			close(w.inputCh)
		}
	}()
	for {
		select {
		case <-e.finishCh:
			return
		case input, ok = <-e.inputCh:
			if !ok {
				return
			}
			chk = input.chk
		}
		err = e.children[0].Next(ctx, chk)
		if err != nil {
			e.finalOutputCh <- &AfFinalResult{err: errors.Trace(err)}
			return
		}
		if chk.NumRows() == 0 {
			return
		}
		input.giveBackCh <- chk
	}
}

func (e *HashAggExec) waitPartialWorkerAndCloseOutputChs(waitGroup *sync.WaitGroup) {
	waitGroup.Wait()
	for _, ch := range e.partialOutputChs {
		close(ch)
	}
}

func (e *HashAggExec) waitFinalWorkerAndCloseFinalOutput(waitGroup *sync.WaitGroup) {
	waitGroup.Wait()
	close(e.finalOutputCh)
}

// parallelExec executes hash aggregate algorithm parallelly.
func (e *HashAggExec) parallelExec(ctx context.Context, chk *chunk.Chunk) error {
	if !e.prepared {
		go e.fetchChildData(ctx)

		partialWorkerWaitGroup := &sync.WaitGroup{}
		partialWorkerWaitGroup.Add(len(e.partialWorkers))
		for i := range e.partialWorkers {
			go e.partialWorkers[i].run(e.ctx, partialWorkerWaitGroup, len(e.finalWorkers))
		}
		go e.waitPartialWorkerAndCloseOutputChs(partialWorkerWaitGroup)

		finalWorkerWaitGroup := &sync.WaitGroup{}
		finalWorkerWaitGroup.Add(len(e.finalWorkers))
		for i := range e.finalWorkers {
			go e.finalWorkers[i].run(e.ctx, finalWorkerWaitGroup)
		}
		go e.waitFinalWorkerAndCloseFinalOutput(finalWorkerWaitGroup)

		e.prepared = true
	}
	for {
		result, ok := <-e.finalOutputCh
		if !ok || result.err != nil || result.chk.NumRows() == 0 {
			if result != nil {
				return errors.Trace(result.err)
			}
			if e.isInputNull && e.defaultVal != nil {
				chk.Append(e.defaultVal, 0, 1)
			}
			e.isInputNull = false
			return nil
		}
		e.isInputNull = false
		chk.SwapColumns(result.chk)
		// Put result.chk back to the corresponded final worker's finalResultHolderCh.
		result.giveBackCh <- result.chk
		// todo: store the result that chk.numrows() < e.maxChunkSize
		if chk.NumRows() > 0 {
			break
		}
	}
	return nil
}

// unparallelExec executes hash aggregation algorithm in single thread.
func (e *HashAggExec) unparallelExec(ctx context.Context, chk *chunk.Chunk) error {
	// In this stage we consider all data from src as a single group.
	if !e.prepared {
		err := e.execute(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		if (e.groupMap.Len() == 0) && len(e.GroupByItems) == 0 {
			// If no groupby and no data, we should add an empty group.
			// For example:
			// "select count(c) from t;" should return one row [0]
			// "select count(c) from t group by c1;" should return empty result set.
			e.groupMap.Put([]byte{}, []byte{})
		}
		e.prepared = true
	}
	chk.Reset()
	for {
		groupKey, _ := e.groupIterator.Next()
		if groupKey == nil {
			return nil
		}
		aggCtxs := e.getContexts(groupKey)
		e.rowBuffer = e.rowBuffer[:0]
		for i, af := range e.AggFuncs {
			e.rowBuffer = append(e.rowBuffer, af.GetResult(aggCtxs[i]))
		}
		e.mutableRow.SetDatums(e.rowBuffer...)
		chk.AppendRow(e.mutableRow.ToRow())
		if chk.NumRows() == e.maxChunkSize {
			return nil
		}
	}
}

// execute fetches Chunks from src and update each aggregate function for each row in Chunk.
func (e *HashAggExec) execute(ctx context.Context) (err error) {
	inputIter := chunk.NewIterator4Chunk(e.childrenResults[0])
	for {
		err := e.children[0].Next(ctx, e.childrenResults[0])
		if err != nil {
			return errors.Trace(err)
		}
		// no more data.
		if e.childrenResults[0].NumRows() == 0 {
			return nil
		}
		for row := inputIter.Begin(); row != inputIter.End(); row = inputIter.Next() {
			groupKey, err := e.getGroupKey(row)
			if err != nil {
				return errors.Trace(err)
			}
			if len(e.groupMap.Get(groupKey, e.groupVals[:0])) == 0 {
				e.groupMap.Put(groupKey, []byte{})
			}
			aggCtxs := e.getContexts(groupKey)
			for i, af := range e.AggFuncs {
				err = af.Update(aggCtxs[i], e.sc, row)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (e *HashAggExec) getGroupKey(row chunk.Row) ([]byte, error) {
	vals := make([]types.Datum, 0, len(e.GroupByItems))
	for _, item := range e.GroupByItems {
		v, err := item.Eval(row)
		if item.GetType().Tp == mysql.TypeNewDecimal {
			v.SetLength(0)
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		vals = append(vals, v)
	}
	var err error
	e.groupKey, err = codec.EncodeValue(e.sc, e.groupKey[:0], vals...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return e.groupKey, nil
}

func (e *HashAggExec) getContexts(groupKey []byte) []*aggregation.AggEvaluateContext {
	groupKeyString := string(groupKey)
	aggCtxs, ok := e.aggCtxsMap[groupKeyString]
	if !ok {
		aggCtxs = make([]*aggregation.AggEvaluateContext, 0, len(e.AggFuncs))
		for _, af := range e.AggFuncs {
			aggCtxs = append(aggCtxs, af.CreateContext(e.ctx.GetSessionVars().StmtCtx))
		}
		e.aggCtxsMap[groupKeyString] = aggCtxs
	}
	return aggCtxs
}

// StreamAggExec deals with all the aggregate functions.
// It assumes all the input data is sorted by group by key.
// When Next() is called, it will return a result for the same group.
type StreamAggExec struct {
	baseExecutor

	executed     bool
	hasData      bool
	StmtCtx      *stmtctx.StatementContext
	AggFuncs     []aggregation.Aggregation
	aggCtxs      []*aggregation.AggEvaluateContext
	GroupByItems []expression.Expression
	curGroupKey  []types.Datum
	tmpGroupKey  []types.Datum

	// for chunk execution.
	inputIter  *chunk.Iterator4Chunk
	inputRow   chunk.Row
	mutableRow chunk.MutRow
	rowBuffer  []types.Datum
}

// Open implements the Executor Open interface.
func (e *StreamAggExec) Open(ctx context.Context) error {
	if err := e.baseExecutor.Open(ctx); err != nil {
		return errors.Trace(err)
	}

	e.executed = false
	e.hasData = false
	e.inputIter = chunk.NewIterator4Chunk(e.childrenResults[0])
	e.inputRow = e.inputIter.End()
	e.mutableRow = chunk.MutRowFromTypes(e.retTypes())
	e.rowBuffer = make([]types.Datum, 0, e.Schema().Len())

	e.aggCtxs = make([]*aggregation.AggEvaluateContext, 0, len(e.AggFuncs))
	for _, agg := range e.AggFuncs {
		e.aggCtxs = append(e.aggCtxs, agg.CreateContext(e.ctx.GetSessionVars().StmtCtx))
	}

	return nil
}

// Next implements the Executor Next interface.
func (e *StreamAggExec) Next(ctx context.Context, chk *chunk.Chunk) error {
	chk.Reset()

	for !e.executed && chk.NumRows() < e.maxChunkSize {
		err := e.consumeOneGroup(ctx, chk)
		if err != nil {
			e.executed = true
			return errors.Trace(err)
		}
	}
	return nil
}

func (e *StreamAggExec) consumeOneGroup(ctx context.Context, chk *chunk.Chunk) error {
	for !e.executed {
		if err := e.fetchChildIfNecessary(ctx, chk); err != nil {
			return errors.Trace(err)
		}
		for ; e.inputRow != e.inputIter.End(); e.inputRow = e.inputIter.Next() {
			meetNewGroup, err := e.meetNewGroup(e.inputRow)
			if err != nil {
				return errors.Trace(err)
			}
			if meetNewGroup {
				e.appendResult2Chunk(chk)
			}
			for i, af := range e.AggFuncs {
				err := af.Update(e.aggCtxs[i], e.StmtCtx, e.inputRow)
				if err != nil {
					return errors.Trace(err)
				}
			}
			if meetNewGroup {
				e.inputRow = e.inputIter.Next()
				return nil
			}
		}
	}
	return nil
}

func (e *StreamAggExec) fetchChildIfNecessary(ctx context.Context, chk *chunk.Chunk) error {
	if e.inputRow != e.inputIter.End() {
		return nil
	}

	err := e.children[0].Next(ctx, e.childrenResults[0])
	if err != nil {
		return errors.Trace(err)
	}
	// No more data.
	if e.childrenResults[0].NumRows() == 0 {
		if e.hasData || len(e.GroupByItems) == 0 {
			e.appendResult2Chunk(chk)
		}
		e.executed = true
		return nil
	}

	// Reach here, "e.childrenResults[0].NumRows() > 0" is guaranteed.
	e.inputRow = e.inputIter.Begin()
	e.hasData = true
	return nil
}

// appendResult2Chunk appends result of all the aggregation functions to the
// result chunk, and reset the evaluation context for each aggregation.
func (e *StreamAggExec) appendResult2Chunk(chk *chunk.Chunk) {
	e.rowBuffer = e.rowBuffer[:0]
	for i, af := range e.AggFuncs {
		e.rowBuffer = append(e.rowBuffer, af.GetResult(e.aggCtxs[i]))
		af.ResetContext(e.ctx.GetSessionVars().StmtCtx, e.aggCtxs[i])
	}
	e.mutableRow.SetDatums(e.rowBuffer...)
	chk.AppendRow(e.mutableRow.ToRow())
}

// meetNewGroup returns a value that represents if the new group is different from last group.
func (e *StreamAggExec) meetNewGroup(row chunk.Row) (bool, error) {
	if len(e.GroupByItems) == 0 {
		return false, nil
	}
	e.tmpGroupKey = e.tmpGroupKey[:0]
	matched, firstGroup := true, false
	if len(e.curGroupKey) == 0 {
		matched, firstGroup = false, true
	}
	for i, item := range e.GroupByItems {
		v, err := item.Eval(row)
		if err != nil {
			return false, errors.Trace(err)
		}
		if matched {
			c, err := v.CompareDatum(e.StmtCtx, &e.curGroupKey[i])
			if err != nil {
				return false, errors.Trace(err)
			}
			matched = c == 0
		}
		e.tmpGroupKey = append(e.tmpGroupKey, v)
	}
	if matched {
		return false, nil
	}
	e.curGroupKey = e.curGroupKey[:0]
	for _, v := range e.tmpGroupKey {
		e.curGroupKey = append(e.curGroupKey, *((&v).Copy()))
	}
	return !firstGroup, nil
}
