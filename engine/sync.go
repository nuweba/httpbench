package engine

import (
	"github.com/nuweba/counter"
	"github.com/nuweba/httpbench/concurrency"
	"github.com/nuweba/httpbench/syncedtrace"
	"sync"
	"time"
)

type SyncConfig struct {
	done             *sync.WaitGroup
	result           chan *TraceResult
	concurrencyLimit uint64
	syncedConcurrent uint64
	reqCounter       *counter.Counter
	Concurrency      *concurrency.Manager
	maxRequests      uint64
	durationTimer    *time.Timer
	maxDuration      time.Duration
	reqDelay         time.Duration
	waitReq          bool
	traceSync        *syncedtrace.SyncConfig
}

//concurrencyLimit == 0 for unlimited concurrency
//Duration == 0 for one time loop
func NewSyncConfig(hook syncedtrace.TraceHookType, concurrencyLimit uint64, duration time.Duration, waitReq bool, result chan *TraceResult) *SyncConfig {
	traceSync := &syncedtrace.SyncConfig{
		Cond:       sync.NewCond(&sync.Mutex{}),
		ReadyWg:    &sync.WaitGroup{},
		WaitOnHook: hook,
	}

	sc := &SyncConfig{
		done:             &sync.WaitGroup{},
		result:           result,
		concurrencyLimit: concurrencyLimit,
		Concurrency:      concurrency.New(concurrencyLimit),
		reqCounter:       new(counter.Counter),
		durationTimer:    time.NewTimer(duration),
		maxDuration:      duration,
		waitReq:          waitReq,
		traceSync:        traceSync,
	}
	if sc.maxRequests = uint64(sc.maxDuration.Nanoseconds()); sc.reqDelay != time.Duration(0) {
		sc.maxRequests = uint64(sc.maxDuration.Nanoseconds() / sc.reqDelay.Nanoseconds())
	}

	return sc
}

func (sc *SyncConfig) WaitAll() {
	sc.done.Wait()
}

func (sc *SyncConfig) Duration() <-chan time.Time {
	return sc.durationTimer.C
}

func (sc *SyncConfig) SetSyncedConcurrent(syncedCount uint64) {
	sc.syncedConcurrent = syncedCount
}

func (sc *SyncConfig) SetReqDelay(reqDelay time.Duration) {
	sc.reqDelay = reqDelay
}

func (sc *SyncConfig) WaitReqDelay() {
	time.Sleep(sc.reqDelay)
}

func (sc *SyncConfig) ReqCounter() uint64 {
	return sc.reqCounter.Counter()
}

func (sc *SyncConfig) IsDurationSet() bool {
	if sc.maxDuration == 0 {
		return false
	}

	return true
}

func (sc *SyncConfig) MaxReqReached() bool {
	if !sc.IsDurationSet() {
		return false
	}

	if sc.ReqCounter() < sc.maxRequests {
		return false
	}
	return true
}
