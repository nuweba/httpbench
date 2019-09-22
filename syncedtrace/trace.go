package syncedtrace

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"net/http/httptrace"
	"strconv"
	"sync"
	"time"
)

type SyncConfig struct {
	Cond       *sync.Cond      `json:"-" yaml:"-"`
	ReadyWg    *sync.WaitGroup `json:"-" yaml:"-"`
	WaitOnHook TraceHookType   `json:"-" yaml:"-"`
}

type Trace struct {
	*SyncConfig `json:"-" yaml:"-"`
	Id          uint64                  `json:"id" yaml:"id"`
	UniqueId    string                  `json:"unique_id" yaml:"unique_id"`
	TotalDrift  time.Duration           `json:"total_drift" yaml:"total_drift"`
	Start       time.Time               `json:"start_time" yaml:"start_time"`
	Latest      time.Time               `json:"latest" yaml:"latest"`
	Done        time.Time               `json:"done" yaml:"done"`
	Total       time.Duration           `json:"total" yaml:"total"`
	Error       bool                    `json:"had_error" yaml:"had_error"`
	LocalAddr   string                  `json:"local_addr" yaml:"local_addr"`
	Hooks       map[TraceHookType]*Hook `json:"hooks" yaml:"hooks"`
}

func New(id uint64, syncConfig *SyncConfig) *Trace {
	hooks := make(map[TraceHookType]*Hook, HooksCount)
	for i := TraceHookType(0); i < HooksCount; i++ {
		hooks[i] = &Hook{}
	}

	hooks[syncConfig.WaitOnHook].SetWait()

	return &Trace{
		SyncConfig: syncConfig,
		Hooks:      hooks,
		Id:         id,
		UniqueId:   strconv.FormatUint(rand.Uint64(), 10),
	}
}

func (t *Trace) LogTime(hook TraceHookType) {
	now := time.Now()
	t.SetStart(now)
	t.Hooks[hook].LogTime(now, t.Latest)
	t.UpdateLatest(now)
}

func (t *Trace) UpdateLatest(latest time.Time) {
	t.Latest = latest
}

func (t *Trace) WaitForSignal(hook TraceHookType) {
	if !t.Hooks[hook].NeedToWait() {
		return
	}
	start := time.Now()
	t.Cond.L.Lock()
	t.ReadyWg.Done()
	t.Cond.Wait()
	t.Cond.L.Unlock()
	t.IncreaseDrift(start)
	t.Hooks[hook].SetLocalDrift(t.GetDrift())
}

func (t *Trace) LogError(hook TraceHookType, err error) {
	t.Hooks[hook].LogError(err)
	t.Error = true
	//t.SetDone()
	//todo: fix wg.ready deadlock
	//panic(Err)
}

func (t *Trace) IncreaseDrift(start time.Time) {
	//in case we set the Wait to the first hook (GetConn),
	//the first time we set the time is only AFTER the first Wait, so no drift
	if t.Start.IsZero() {
		return
	}
	t.TotalDrift += time.Since(start)
}

func (t *Trace) GetDrift() time.Duration {
	return t.TotalDrift
}

func (t *Trace) SetDone() {
	t.Done = time.Now()
	t.Total = t.Done.Sub(t.Start) - t.TotalDrift

	//release the wait group in case we didnt get to the hook
	if !t.Hooks[t.WaitOnHook].NeedToWait() {
		return
	}

	if t.Hooks[t.WaitOnHook].Time.IsZero() {
		t.ReadyWg.Done()
	}

	return
}

func (t *Trace) SetStart(now time.Time) {
	if !t.Start.IsZero() {
		return
	}
	t.Start = now
	t.UpdateLatest(t.Start)
}

func GetTraceHooks(t *Trace) *httptrace.ClientTrace {

	trace := &httptrace.ClientTrace{
		PutIdleConn: func(err error) {
			panic("PutIdleConn - connection reuse should not be used")
		},

		GetConn: func(_ string) {
			t.WaitForSignal(GetConn)
			t.LogTime(GetConn)
		},

		DNSStart: func(_ httptrace.DNSStartInfo) {
			t.WaitForSignal(DNSStart)
			t.LogTime(DNSStart)
		},

		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			if dnsInfo.Err != nil {
				t.LogError(DNSDone, dnsInfo.Err)
				return
			}

			if dnsInfo.Coalesced {
				//fmt.Println("used same dns")
			}

			t.WaitForSignal(DNSDone)
			t.LogTime(DNSDone)
		},

		ConnectStart: func(_, addr string) {
			if addr[:len("[::1]")] == "[::1]" { // localhost ipv6
				return
			}
			t.WaitForSignal(ConnectStart)
			t.LogTime(ConnectStart)
		},

		ConnectDone: func(_, _ string, err error) {
			if err != nil {
				t.LogError(ConnectDone, err)
				return
			}
			t.WaitForSignal(ConnectDone)
			t.LogTime(ConnectDone)
		},

		TLSHandshakeStart: func() {
			t.WaitForSignal(TLSHandshakeStart)
			t.LogTime(TLSHandshakeStart)
		},

		TLSHandshakeDone: func(tlsState tls.ConnectionState, err error) {
			if err != nil {
				t.LogError(TLSHandshakeDone, err)
				return
			}

			if tlsState.DidResume {
				fmt.Println("TLSHandshakeDone - resumed")
			}
			t.WaitForSignal(TLSHandshakeDone)
			t.LogTime(TLSHandshakeDone)
		},

		GotConn: func(gotConnInfo httptrace.GotConnInfo) {
			if gotConnInfo.Reused || gotConnInfo.WasIdle {
				panic("GotConn - connection reuse should not be used")
			}
			if gotConnInfo.Conn != nil {
				t.LocalAddr = gotConnInfo.Conn.LocalAddr().String()
			}
			t.WaitForSignal(GotConn)
			t.LogTime(GotConn)
		},

		WroteHeaders: func() {
			t.WaitForSignal(WroteHeaders)
			t.LogTime(WroteHeaders)
		},

		WroteRequest: func(rInfo httptrace.WroteRequestInfo) {
			//WroteRequest is non-blocking!
			if rInfo.Err != nil {
				t.LogError(WroteRequest, rInfo.Err)
				return
			}
			t.WaitForSignal(WroteRequest)
			t.LogTime(WroteRequest)
		},

		Wait100Continue: func() {
			t.WaitForSignal(Wait100Continue)
			t.LogTime(Wait100Continue)
		},

		GotFirstResponseByte: func() {
			t.WaitForSignal(GotFirstResponseByte)
			t.LogTime(GotFirstResponseByte)
		},
		Got100Continue: func() {
			t.WaitForSignal(Got100Continue)
			t.LogTime(Got100Continue)
		},
	}

	return trace
}
