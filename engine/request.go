package engine

import (
	"context"
	"errors"
	"fmt"
	"github.com/nuweba/httpbench/syncedtrace"
	"net"
	"net/http"
	"net/http/httptrace"
	"syscall"
	"time"
)

func updateLocalAddrToPort(th *syncedtrace.Trace, fd int) error {
	if th.LocalAddr != "" {
		return nil
	}

	sa, err := syscall.Getsockname(fd)
	if err != nil {
		return err
	}

	switch t := sa.(type) {
	case *syscall.SockaddrInet4:
		th.LocalAddr = fmt.Sprintf(":%d", t.Port)
		return nil
	case *syscall.SockaddrInet6:
		th.LocalAddr = fmt.Sprintf(":%d", t.Port)
		return nil
	default:
		return errors.New("unknown socket addr struct type")
	}

}

func wrapError(old, new error) error {
	if old == nil {
		return new
	}
	if new == nil {
		return old
	}
	return fmt.Errorf("%s;; previous error: %s", new.Error(), old.Error())
}

func doRequest(newReq func(uniqueId string) (*http.Request, error), syncConfig *SyncConfig) {
	defer syncConfig.done.Done()

	th := syncedtrace.New(syncConfig.reqCounter.Inc(), syncConfig.traceSync)

	req, err := newReq(th.UniqueId)
	if err != nil {
		panic("cant create a new request " + err.Error())
	}
	if req == nil {
		panic("got nil request")
	}
	traceHooks := syncedtrace.GetTraceHooks(th)
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), traceHooks))

	var duplicatedSocket int
	//creating new client and new transport to eliminate tcp reuse
	client := &http.Client{Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Control: func(network, address string, c syscall.RawConn) error {
				var err error
				if ctrlErr := c.Control(func(fd uintptr) {
					if duplicatedSocket != 0 {
						return
					}
					duplicatedSocket, err = syscall.Dup(int(fd))
				}); ctrlErr != nil {
					return ctrlErr
				}
				return err
			},
			Timeout:   20 * time.Second,
			KeepAlive: 20 * time.Second,
		}).DialContext,
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          0,
		IdleConnTimeout:       0,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     true,
	}}

	req.Close = true

	syncConfig.Concurrency.AddConcurrent()
	resp, err := client.Do(req)

	if duplicatedSocket != 0 {
		err = wrapError(err, updateLocalAddrToPort(th, duplicatedSocket))
		syscall.Close(duplicatedSocket)
	}

	result := NewTraceResult(th, resp)
	result.Err = NewTraceResultError(err)
	result.ReadBody()
	result.SetDone()
	syncConfig.Concurrency.DecConcurrent()

	result.TraceSummary()

	//todo: fix blocking
	syncConfig.result <- result
}

func HttpBench(sc *SyncConfig, reqDelay time.Duration, newReq func(uniqueId string) (*http.Request, error)) {
	if !sc.Concurrency.IsConcurrencyUnlimited() && sc.syncedConcurrent > sc.concurrencyLimit {
		panic("synced concurrent cannot be bigger then the concurrency limit")
	}

	if reqDelay < time.Millisecond && sc.Concurrency.IsConcurrencyUnlimited() && sc.IsDurationSet() {
		panic("Less then a millisecond request delay with unlimited concurrency and Duration is not allowed. this can flood the remote server.")
	}

	if sc.syncedConcurrent == 0 {
		panic("synced concurrent is not set! need to call SetSyncedConcurrent")
	}

	sc.SetReqDelay(reqDelay)

outer:
	for !sc.MaxReqReached() {
		for i := uint64(0); i < sc.syncedConcurrent && !sc.MaxReqReached(); i++ {
			sc.Concurrency.AcquireConcurrencySlot()
			sc.traceSync.ReadyWg.Add(1)
			sc.done.Add(1)
			go doRequest(newReq, sc)
		}

		sc.traceSync.ReadyWg.Wait()
		if sc.reqDelay >= time.Millisecond {
			for i := sc.syncedConcurrent; i > 0; i-- {
				sc.WaitReqDelay()
				sc.traceSync.Cond.L.Lock()
				sc.traceSync.Cond.Signal()
				sc.traceSync.Cond.L.Unlock()
			}
		} else {
			sc.traceSync.Cond.L.Lock()
			sc.traceSync.Cond.Broadcast()
			sc.traceSync.Cond.L.Unlock()
		}

		if sc.waitReq {
			sc.WaitAll()
		}

		if !sc.IsDurationSet() {
			break outer
		}

		select {
		case <-sc.Duration():
			break outer
		default:
		}
	}
}
