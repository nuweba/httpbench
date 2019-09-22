package engine

import (
	"github.com/nuweba/counter"
	"github.com/nuweba/httpbench/concurrency"
	"github.com/nuweba/httpbench/syncedtrace"
	"time"
)

type SyncConcurrent struct {
	syncs       []*SyncConfig
	reqCounter  *counter.Counter
	concurrency *concurrency.Manager
}

func NewSyncConcurrent() *SyncConcurrent {
	return &SyncConcurrent{reqCounter: new(counter.Counter), concurrency: concurrency.New(0)}
}

func (syncC *SyncConcurrent) NewSyncConfig(hook syncedtrace.TraceHookType, concurrencyLimit uint64, duration time.Duration, waitReq bool, result chan *TraceResult) *SyncConfig {
	sc := NewSyncConfig(hook, concurrencyLimit, duration, waitReq, result)
	sc.reqCounter = syncC.reqCounter
	sc.Concurrency.ConcurrentCount = syncC.concurrency.ConcurrentCount
	syncC.syncs = append(syncC.syncs, sc)

	return sc
}

func (syncC *SyncConcurrent) MaxConcurrent() uint64 {
	return syncC.concurrency.ConcurrentCount.Max()
}

func (syncC *SyncConcurrent) WaitAll() {
	for _, sync := range syncC.syncs {
		sync.done.Wait()
	}
}

func (syncC *SyncConcurrent) ReqCounter() uint64 {
	return syncC.reqCounter.Counter()
}
