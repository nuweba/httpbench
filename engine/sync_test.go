package engine

import (
	"fmt"
	"github.com/nuweba/counter"
	"github.com/nuweba/httpbench/syncedtrace"
	"net/http"
	"testing"
	"time"
)

func generateSyncConfig() *SyncConfig {
	resChan := make(chan *TraceResult)
	return NewSyncConfig(syncedtrace.ConnectDone, 3, 100*time.Millisecond, false, resChan)
}

func TestNewSyncConfig(t *testing.T) {
	dur := 100 * time.Millisecond
	sc := NewSyncConfig(syncedtrace.ConnectDone, 3, dur, false, nil)

	if sc.reqCounter.Counter() != 0 {
		t.Error("Newly created counter in SyncConfig should be 0")
	}

	if sc.maxRequests != uint64(dur.Nanoseconds()) {
		t.Error("maxRequests after SyncConfig creation supposed to be equal to duration ns")
	}
}

func TestSyncConfig_WaitAll(t *testing.T) {
	sc := generateSyncConfig()
	n := 10
	waitTime := 100 * time.Millisecond

	sc.done.Add(n)
	go func() {
		for i := 0; i < n; i++ {
			time.Sleep(waitTime)
			sc.done.Done()
		}
	}()
	t1 := time.Now()
	sc.WaitAll()
	t2 := time.Now()

	if t2.Sub(t1) < waitTime*time.Duration(n) {
		t.Error("WaitAll didn't wait as wanted")
	}
}

func TestSyncConfig_Duration(t *testing.T) {
	sc := generateSyncConfig()
	time.Sleep(sc.maxDuration)
	select {
	case <-sc.Duration():
		return
	default:
		t.Error("Duration timer didn't pass as wanted")
	}
}

func TestSyncConfig_ReqCounter(t *testing.T) {
	sc := generateSyncConfig()
	sc.SetSyncedConcurrent(1)

	localReqCount := counter.Counter{}

	ts := runLocalServer(func(w http.ResponseWriter, r *http.Request) {
		localReqCount.Inc()
		fmt.Fprintln(w, "Hello world")
	})
	defer ts.Close()

	HttpBench(sc, time.Duration(0), getRequest(ts.URL))
	go consumeResults(sc)

	sc.WaitAll()
	close(sc.result)
	if sc.ReqCounter() != localReqCount.Counter() {
		t.Error("ReqCounter is different than the requests arrive in the server", sc.ReqCounter(), localReqCount)
	}
}

func TestSyncConfig_WaitReqDelay(t *testing.T) {
	sc := generateSyncConfig()
	reqDelay := 2 * time.Millisecond
	last := time.Time{}

	sc.SetSyncedConcurrent(1)
	ts := runLocalServer(func(w http.ResponseWriter, r *http.Request) {
		reqTime := time.Now()

		if last != (time.Time{}) && reqTime.Sub(last) < reqDelay {
			t.Error("Req delay didn't didn't work as intended", reqTime.Sub(last))
		}

		last = reqTime // In 1 concurrent request we trust

		fmt.Fprintln(w, "Hello world")
	})
	defer ts.Close()

	HttpBench(sc, reqDelay, getRequest(ts.URL))
	go consumeResults(sc)

	sc.WaitAll()
	close(sc.result)
}

func TestSyncConfig_MaxReqReached(t *testing.T) {
	resChan := make(chan *TraceResult)
	sc := NewSyncConfig(syncedtrace.ConnectDone, 0, time.Second, false, resChan)

	sc.maxRequests = 1000 // make the test faster
	for i := uint64(0); i < sc.maxRequests; i++ {
		sc.reqCounter.Inc()
	}

	if !sc.MaxReqReached() {
		t.Error("max requests reached is false although counter is  same as max requests")
	}

}

func TestSyncConfig_MaxReqReachedNoDuration(t *testing.T) {
	resChan := make(chan *TraceResult)
	sc := NewSyncConfig(syncedtrace.ConnectDone, 3, 0, false, resChan)
	if sc.MaxReqReached() {
		t.Error("max requests reached although no duration")
	}
}

func TestSyncConfig_IsDurationSetFalse(t *testing.T) {
	resChan := make(chan *TraceResult)
	sc := NewSyncConfig(syncedtrace.ConnectDone, 3, 0, false, resChan)

	if sc.IsDurationSet() {
		t.Error("Duration is not set")
	}
}

func TestSyncConfig_IsDurationSetTrue(t *testing.T) {
	resChan := make(chan *TraceResult)
	sc := NewSyncConfig(syncedtrace.ConnectDone, 3, time.Millisecond, false, resChan)

	if !sc.IsDurationSet() {
		t.Error("Duration is set")
	}
}
