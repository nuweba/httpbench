package engine

import (
	"github.com/nuweba/httpbench/syncedtrace"
	"io/ioutil"
	"net/http"
	"time"
)

type TraceResultError struct {
	Message string
}

func NewTraceResultError(err error) *TraceResultError {
	if err == nil {
		return nil
	}
	return &TraceResultError{Message: err.Error()}
}

func (traceResultError *TraceResultError) Error() string {
	return traceResultError.Message
}

type TraceResult struct {
	*syncedtrace.Trace `json:"trace_hooks" yaml:"trace_hooks"`
	Summary            *TraceSummary     `json:"trace_summary" yaml:"trace_summary"`
	Response           *http.Response    `json:"-" yaml:"-"`
	Body               string            `json:"response_body,string" yaml:"response_body,flow"`
	Err                *TraceResultError `json:"error" yaml:"error"`
}

type TraceSummary struct {
	Dns          time.Duration
	TcpHandshake time.Duration
	TlsHandshake time.Duration
	GotConn      time.Duration
	WroteRequest time.Duration
	FirstByte    time.Duration
	ReadingBody  time.Duration
}

func NewTraceResult(traceHooks *syncedtrace.Trace, resp *http.Response) *TraceResult {
	return &TraceResult{
		Trace:    traceHooks,
		Response: resp,
	}
}

func (tr *TraceResult) TraceSummary() {
	sum := &TraceSummary{
		Dns:          tr.Hooks[syncedtrace.DNSDone].Duration,
		TcpHandshake: tr.Hooks[syncedtrace.ConnectDone].Duration,
		TlsHandshake: tr.Hooks[syncedtrace.TLSHandshakeDone].Duration,
		GotConn:      tr.Hooks[syncedtrace.GotConn].Duration,
		WroteRequest: tr.Hooks[syncedtrace.WroteRequest].Duration,
		FirstByte:    tr.Hooks[syncedtrace.GotFirstResponseByte].Duration,
		ReadingBody:  tr.Done.Sub(tr.Hooks[syncedtrace.GotFirstResponseByte].Time),
	}
	tr.Summary = sum
}

func (tr *TraceResult) ReadBody() {
	if tr.Response == nil {
		return
	}

	if tr.Err != nil {
		return
	}

	if tr.Body != "" {
		return
	}

	bodyBytes, err := ioutil.ReadAll(tr.Response.Body)
	tr.Response.Body.Close()
	tr.Body = string(bodyBytes)
	tr.Err = NewTraceResultError(err)
}
