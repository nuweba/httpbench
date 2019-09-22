package httpbench

import (
	"github.com/nuweba/httpbench/engine"
	"github.com/nuweba/httpbench/syncedtrace"
	"math/rand"
	"net/http"
	"time"
)

type Preset struct {
	ResultCh   chan *engine.TraceResult
	NewRequest func(uniqueId string) (*http.Request, error)
	Hook       syncedtrace.TraceHookType
}

type PresetResult struct {
	Start         time.Time `json:"start_time"`
	Done          time.Time `json:"done_time"`
	ReqCount      uint64    `json:"request_count"`
	MaxConcurrent uint64    `json:"max_concurrent"`
}

type PresetType int

const (
	RequestPerDuration PresetType = iota
	ConcurrentRequestsUnsynced
	ConcurrentRequestsSynced
	ConcurrentRequestsSyncedOnce
	RequestsForTimeGraph
	ConcurrentForTimeGraph
	PresetCount
)

func (pt PresetType) String() string {
	return [...]string{
		"RequestPerDuration",
		"ConcurrentRequestsUnsynced",
		"ConcurrentRequestsSynced",
		"ConcurrentRequestsSyncedOnce",
		"RequestsForTimeGraph",
		"ConcurrentForTimeGraph",
	}[pt]
}

func New(newRequest func(uniqueId string) (*http.Request, error), hook syncedtrace.TraceHookType) *Preset {
	rand.Seed(time.Now().UTC().UnixNano())
	return &Preset{ResultCh: make(chan *engine.TraceResult), NewRequest: newRequest, Hook: hook}
}

func (p *Preset) RequestPerDuration(reqDelay time.Duration, duration time.Duration) *PresetResult {

	if reqDelay < time.Millisecond {
		panic("Less then a millisecond request delay is not allowed in RequestPerDuration mode. this can flood the remote server.")
	}

	syncConfig := engine.NewSyncConfig(p.Hook, 0, duration, false, p.ResultCh)
	syncConfig.SetSyncedConcurrent(1)

	start := time.Now()

	engine.HttpBench(syncConfig, reqDelay, p.NewRequest)
	syncConfig.WaitAll()

	close(p.ResultCh)
	return &PresetResult{
		Start:         start,
		Done:          time.Now(),
		ReqCount:      syncConfig.ReqCounter(),
		MaxConcurrent: syncConfig.Concurrency.ConcurrentCount.Max(),
	}
}

func (p *Preset) ConcurrentRequestsUnsynced(maxConcurrencyLimit uint64, reqDelay time.Duration, duration time.Duration) *PresetResult {
	if maxConcurrencyLimit == 0 {
		panic("unlimited concurrency is not allowed in ConcurrentRequestsUnsynced mode. this can flood the remote server.")
	}

	syncConfig := engine.NewSyncConfig(p.Hook, maxConcurrencyLimit, duration, false, p.ResultCh)
	syncConfig.SetSyncedConcurrent(1)

	start := time.Now()

	engine.HttpBench(syncConfig, reqDelay, p.NewRequest)

	syncConfig.WaitAll()

	close(p.ResultCh)
	return &PresetResult{
		Start:         start,
		Done:          time.Now(),
		ReqCount:      syncConfig.ReqCounter(),
		MaxConcurrent: syncConfig.Concurrency.ConcurrentCount.Max(),
	}
}

func (p *Preset) ConcurrentRequestsSynced(concurrencyLimit uint64, reqDelay time.Duration, duration time.Duration) *PresetResult {
	if concurrencyLimit == 0 {
		panic("Unlimited concurrency is not allowed in ConcurrentRequestsSynced mode. this can flood the remote server.")
	}

	syncConfig := engine.NewSyncConfig(p.Hook, concurrencyLimit, duration, true, p.ResultCh)
	syncConfig.SetSyncedConcurrent(concurrencyLimit)

	start := time.Now()

	engine.HttpBench(syncConfig, reqDelay, p.NewRequest)

	syncConfig.WaitAll()

	close(p.ResultCh)
	return &PresetResult{
		Start:         start,
		Done:          time.Now(),
		ReqCount:      syncConfig.ReqCounter(),
		MaxConcurrent: syncConfig.Concurrency.ConcurrentCount.Max(),
	}
}

func (p *Preset) ConcurrentRequestsSyncedOnce(concurrencyLimit uint64, reqDelay time.Duration) *PresetResult {
	if concurrencyLimit == 0 {
		panic("Unlimited concurrency is not allowed in ConcurrentRequestsSynced mode. this can flood the remote server.")
	}

	syncConfig := engine.NewSyncConfig(p.Hook, concurrencyLimit, 0, true, p.ResultCh)
	syncConfig.SetSyncedConcurrent(concurrencyLimit)

	start := time.Now()

	engine.HttpBench(syncConfig, reqDelay, p.NewRequest)

	syncConfig.WaitAll()

	close(p.ResultCh)
	return &PresetResult{
		Start:         start,
		Done:          time.Now(),
		ReqCount:      syncConfig.ReqCounter(),
		MaxConcurrent: syncConfig.Concurrency.ConcurrentCount.Max(),
	}
}

type RequestsPerTime struct {
	Concurrent uint64        `json:"concurrent"`
	Time       time.Duration `json:"time"`
}

type HitsGraph []RequestsPerTime
type ConcurrentGraph []RequestsPerTime

func (p *Preset) RequestsForTimeGraph(hitsGraph HitsGraph) *PresetResult {
	start := time.Now()

	syncConcurrent := engine.NewSyncConcurrent()
	for _, point := range hitsGraph {
		syncConfig := syncConcurrent.NewSyncConfig(p.Hook, 0, 0, false, p.ResultCh)
		syncConfig.SetSyncedConcurrent(point.Concurrent)
		go engine.HttpBench(syncConfig, 0, p.NewRequest)
		time.Sleep(point.Time)
	}

	syncConcurrent.WaitAll()

	close(p.ResultCh)
	return &PresetResult{
		Start:         start,
		Done:          time.Now(),
		ReqCount:      syncConcurrent.ReqCounter(),
		MaxConcurrent: syncConcurrent.MaxConcurrent(),
	}
}

/*
Unlike RequestsForTimeGraph, in this preset Time represents the absolute time in which the concurrent amount changes,
and not the time difference between one point and the next.
*/
func (p *Preset) ConcurrentForTimeGraph(concurrentGraph ConcurrentGraph) *PresetResult {
	start := time.Now()
	graphLen := len(concurrentGraph)
	syncConfig := engine.NewSyncConfig(p.Hook, concurrentGraph[0].Concurrent, concurrentGraph[graphLen-1].Time, false, p.ResultCh)
	syncConfig.SetSyncedConcurrent(1)
	go func() {
		for idx, point := range concurrentGraph {
			if point.Concurrent == 0 {
				panic("zero Concurrent is not allowed in ConcurrentForTimeGraph")
			}
			if idx < graphLen-1 {
				time.Sleep(concurrentGraph[idx+1].Time - point.Time)
			}

			syncConfig.Concurrency.ConcurrencySem.Resize(point.Concurrent)

		}
	}()

	engine.HttpBench(syncConfig, 0, p.NewRequest)
	syncConfig.WaitAll()

	close(p.ResultCh)
	return &PresetResult{
		Start:         start,
		Done:          time.Now(),
		ReqCount:      syncConfig.ReqCounter(),
		MaxConcurrent: syncConfig.Concurrency.ConcurrentCount.Max(),
	}
}
