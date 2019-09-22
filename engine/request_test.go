package engine

import (
	"errors"
	"fmt"
	"github.com/nuweba/counter"
	"github.com/nuweba/httpbench/syncedtrace"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func getRequest(url string) func(string) (*http.Request, error) {
	return func(uniqueId string) (*http.Request, error) {
		return http.NewRequest(http.MethodGet, url, nil)
	}
}

func getBadRequest(url string) func(string) (*http.Request, error) {
	return func(uniqueId string) (*http.Request, error) {
		r, _ := http.NewRequest(http.MethodGet, url, nil)
		return r, errors.New("test")
	}
}

func runLocalServer(f func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(f))
	return ts
}

func consumeResults(sc *SyncConfig) {
	for {
		_, ok := <-sc.result
		if !ok {
			return
		}
	}
}

func TestHttpBench_RequestIsBeingReceived(t *testing.T) {
	resChan := make(chan *TraceResult)
	sc := NewSyncConfig(syncedtrace.ConnectDone, 1, 5*time.Millisecond, false, resChan)
	sc.SetSyncedConcurrent(1)

	gotRequest := false
	ts := runLocalServer(func(w http.ResponseWriter, r *http.Request) {
		gotRequest = true
		fmt.Fprintln(w, "Hello world")
	})
	defer ts.Close()

	go consumeResults(sc)
	HttpBench(sc, time.Duration(0), getRequest(ts.URL))
	sc.WaitAll()

	if !gotRequest {
		t.Error("Server didn't get any request from HttpBench")
	}
}

func TestHttpBench_NumberOfRequestIsCorrect(t *testing.T) {
	resChan := make(chan *TraceResult)
	wg := sync.WaitGroup{}
	duration := 5 * time.Millisecond
	sc := NewSyncConfig(syncedtrace.ConnectDone, 5, duration, false, resChan)

	sc.SetSyncedConcurrent(5)
	wg.Add(1)

	localReqCount := counter.Counter{}

	ts := runLocalServer(func(w http.ResponseWriter, r *http.Request) {
		localReqCount.Inc()
		fmt.Fprintln(w, "Hello world")
	})
	defer ts.Close()

	HttpBench(sc, time.Duration(0), getRequest(ts.URL))

	gotResults := uint64(0)
	go func() {
		for {
			select {
			case _, ok := <-resChan:
				if !ok {
					if gotResults != localReqCount.Counter() {
						t.Error("Got different number of requests and results", localReqCount, gotResults)
					}
					wg.Done()
					return
				}
				gotResults++
			}
		}
	}()
	sc.WaitAll()
	close(resChan)
	wg.Wait()
}

func TestHttpBench_MaxReqReached(t *testing.T) {
	resChan := make(chan *TraceResult)
	sc := NewSyncConfig(syncedtrace.ConnectDone, 0, 10*time.Second, false, resChan)
	maxReqs := uint64(100)
	asyncDrift := uint64(1) // Because requests being launched in an async way to number of requests, might be some more requests launched than the max.
	sc.maxRequests = maxReqs
	sc.SetSyncedConcurrent(maxReqs + asyncDrift)

	ts := runLocalServer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello world")
	})
	defer ts.Close()

	go consumeResults(sc)
	HttpBench(sc, time.Millisecond, getRequest(ts.URL))
	sc.WaitAll()

	close(sc.result)

	if sc.ReqCounter() > maxReqs+asyncDrift {
		t.Error("max requests wasn't enforced by HttpBench", sc.ReqCounter(), maxReqs)
	}
}

func TestHttpBench_PanicNoSyncedConcurrency(t *testing.T) {
	resChan := make(chan *TraceResult)

	syncConfig := NewSyncConfig(1, 2, time.Millisecond, false, resChan)
	defer func() {
		if r := recover(); r == nil {
			t.Error("HttpBench shouldn't allow no synced concurrency")
		}
	}()
	HttpBench(syncConfig, 0, getRequest("http://Sheker.com"))
}

func TestHttpBench_PanicConcurrency(t *testing.T) {
	resChan := make(chan *TraceResult)

	syncConfig := NewSyncConfig(1, 2, time.Millisecond, false, resChan)
	syncConfig.SetSyncedConcurrent(3)

	defer func() {
		if r := recover(); r == nil {
			t.Error("HttpBench shouldn't allow to synced concurrency be higher than concurrency limit")
		}

	}()
	HttpBench(syncConfig, 0, getRequest("http://Sheker.com"))
}

func TestHttpBench_PanicReqDelayTooShort(t *testing.T) {
	resChan := make(chan *TraceResult)

	syncConfig := NewSyncConfig(1, 0, time.Millisecond, false, resChan)
	syncConfig.SetSyncedConcurrent(1)
	defer func() {
		if r := recover(); r == nil {
			t.Error("HttpBench shouldn't allow no reqDelay to be less than 1 millisecond")
		}
	}()
	HttpBench(syncConfig, time.Millisecond-time.Nanosecond, getRequest("http://Sheker.com"))
}

func TestHttpBench_WaitReqWorks(t *testing.T) {
	resChan := make(chan *TraceResult) // Relying on blocking res channel while not being consumed
	dur := 100 * time.Millisecond      // In general, without any block this should generate more than 1 request

	sc := NewSyncConfig(syncedtrace.ConnectDone, 0, dur, true, resChan)
	sc.SetSyncedConcurrent(1)

	localReqCount := counter.Counter{}
	ts := runLocalServer(func(w http.ResponseWriter, r *http.Request) {
		localReqCount.Inc()
		fmt.Fprintln(w, "Hello world")
	})
	defer ts.Close()

	go func() {
		time.Sleep(dur)
		consumeResults(sc)
	}()
	HttpBench(sc, time.Millisecond, getRequest(ts.URL))
	sc.WaitAll()

	if localReqCount.Counter() != 1 {
		t.Error("HttpBench should wait for request to end", localReqCount.Counter())
	}
}

func TestHttpBench_PanicOnBadRequest(t *testing.T) {
	resChan := make(chan *TraceResult)

	syncConfig := NewSyncConfig(1, 2, time.Millisecond, false, resChan)
	defer func() {
		if r := recover(); r == nil {
			t.Error("request shouldn't proceed if request error is not nil")
		}
	}()
	HttpBench(syncConfig, 0, getBadRequest("http://Sheker.com"))
}

func TestHttpBench_SyncedConcurrent(t *testing.T) {
	resChan := make(chan *TraceResult)
	sc := NewSyncConfig(syncedtrace.ConnectDone, 0, 0, true, resChan)

	localReqCount := counter.Counter{}
	totalLocalReqCount := counter.Counter{}
	maxConcurrentToTest := uint64(100)
	ts := runLocalServer(func(w http.ResponseWriter, r *http.Request) {
		localReqCount.Inc()
		totalLocalReqCount.Inc()
		fmt.Fprintln(w, "Hello world")
	})
	defer ts.Close()

	for i := uint64(1); i <= maxConcurrentToTest; i++ {
		t.Run(fmt.Sprintf("%d_concurrent\n", i), func(t *testing.T) {
			sc.SetSyncedConcurrent(i)

			// Since duration is nothing, it should end quickly and we should get only the one iteration concurrent requests.
			go consumeResults(sc)

			HttpBench(sc, 0, getRequest(ts.URL))
			if localReqCount.Counter() != i {
				t.Error("Synced concurrent failed", localReqCount.Counter())
			} else {
			}
		})
		localReqCount = counter.Counter{}
	}
}

func TestDoRequest_GetResult(t *testing.T) {
	resChan := make(chan *TraceResult)
	syncConfig := NewSyncConfig(1, 0, time.Millisecond, false, resChan)

	gotRes := false
	syncConfig.traceSync.ReadyWg.Add(1)
	syncConfig.done.Add(1)
	go doRequest(getRequest("https://www.nuweba.com"), syncConfig)
	go func() {
		<-resChan
		gotRes = true
	}()
	syncConfig.traceSync.Cond.L.Lock()
	syncConfig.traceSync.Cond.Signal()
	syncConfig.traceSync.Cond.L.Unlock()

	timer := time.NewTimer(1 * time.Second)
	<-timer.C
	t.SkipNow() // TODO: fix, test is not working
	if !gotRes {
		t.Error("Didn't get anything on result channel")
	}
}

func TestDoRequest_PanicCantCreateRequest(t *testing.T) {
	resChan := make(chan *TraceResult)

	syncConfig := NewSyncConfig(1, 0, time.Millisecond, false, resChan)

	defer func() {
		if r := recover(); r == nil {
			t.Error("HttpBench shouldn't panic when request cannot be created")
		}
	}()

	f := func(str string) (*http.Request, error) { return nil, nil }
	doRequest(f, syncConfig)
}
