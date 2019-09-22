package tests

import (
	"bytes"
	"fmt"
	"github.com/nuweba/httpbench"
	"github.com/nuweba/httpbench/engine"
	"github.com/nuweba/httpbench/syncedtrace"
	"net/http/httptest"
	"net/http/httputil"
	"time"
)

type BenchConfig struct {
	MaxConcurrentLimit uint64                    `json: MaxConcurrentLimit`
	SyncedConcurrent   uint64                    `json: SyncedConcurrent`
	MaxRequests        uint64                    `json: MaxRequests`
	FailsEveryReq      uint64                    `json: FailsEveryReq`
	FailsInRow         uint64                    `json: FailsInRow`
	ReqDelay           time.Duration             `json: ReqDelay`
	Duration           time.Duration             `json: Duration`
	DummyTraceSummary  *engine.TraceSummary      `json: DummyTraceSummary`
	WaitHook           syncedtrace.TraceHookType `json: WaitHook`
	FailType           HttpResponseType          `json: FailType`
	Type               HttpBenchType             `json: Type`
	HitsGraph          httpbench.HitsGraph       `json: HitsGraph`
}

type ServerRequestResult struct {
	Hooks              map[syncedtrace.TraceHookType]*syncedtrace.Hook `json:"hooks"`
	UniqueId           string                                          `json:"unique_id"`
	EndTime            time.Time                                       `json:"start_time"`
	BeforeRead         time.Time                                       `json:"waited_for_hook"`
	AggregatedDuration time.Duration                                   `json:"aggregated_duration"`
	Err                string                                          `json:"error"`
}

const (
	ListenPort             = 8000
	IpV6Localhost          = "::1"
	ServerTimeout          = 3 * time.Second
	CertFile               = "utils/localhost.crt"
	KeyFile                = "utils/localhost.key"
	ServerPath             = "tlsserver/tlsserver" // The test could also build the server
	SockAddr               = "/tmp/comm.sock"
	ReadyString            = "ServerIsReady :)"
	CommunicationSeparator = "\n,,,,\n"

	ReqDelayAddition = 3 * time.Millisecond
	BaseDuration     = 300 * time.Millisecond
	BaseReqDelay     = 20 * time.Millisecond
	FailRequestEvery = 4

	TlsHandshakeTraceName = "TlsHandshakeDone"
	FirstByteTraceName    = "FirstByte"
	ReadingBodyTraceName  = "ReadingBody"
	NonSeededRequest      = "PreSeeded"
)

var (
	DummySleepTraceSummary = GenerateTestTraceSummary(100 * time.Millisecond)
	FailInRow              = []int64{1, 3} // Not relevant if response fail type is Http200
)

var TraceHooksServerTypes = map[string]bool{
	TlsHandshakeTraceName: true,
	FirstByteTraceName:    true,
	ReadingBodyTraceName:  true,
}

type HttpResponseType int

const (
	Http200 HttpResponseType = iota
	//Http300 // No supported for now. raises panic
	Http400
	Http500
	HttpRand
)

type HttpBenchType int

const (
	PerDuration HttpBenchType = iota
	Synced
	SyncedOnce
	UnSynced
	Graph
)

func (bench *BenchConfig) SetPermutation(permutation []int64) {
	// Each permutation: (ReqDelay, WaitHook, FailsEveryReq, FailsInRow, FailType, SyncedConcurrent)
	for i, k := range permutation {
		switch i {
		case 0:
			bench.ReqDelay = time.Duration(k)
		case 1:
			bench.WaitHook = syncedtrace.TraceHookType(k)
		case 2:
			bench.FailsEveryReq = uint64(k)
		case 3:
			bench.FailsInRow = uint64(k)
		case 4:
			bench.FailType = HttpResponseType(k)
		case 5:
			bench.SyncedConcurrent = uint64(k)
			//No default cause any number about 5 is not known in the permutation
		}
	}
}

func GenerateResponse(code int, body string, headers map[string][]string) []byte {
	respRec := httptest.ResponseRecorder{
		Code: code,
		Body: bytes.NewBuffer([]byte(body)),
	}

	respRec.WriteHeader(code)

	resp := respRec.Result()
	for k, v := range headers {
		resp.Header[k] = v
	}
	resp.ContentLength = int64(len(body))

	respByteArr, err := httputil.DumpResponse(resp, true)
	if err != nil {
		fmt.Printf("Failed to create response %d: %s", code, err)
		return nil
	}
	return respByteArr
}

// Used httpstat.us for errors
func (ht HttpResponseType) Get() []byte {
	headersMap := map[string][]string{
		"Set-Cookie":                  {"ARRAffinity=0285cfbea9f2ee78f69010c84850bd5b73ee05f1ff7f634b0b6b20c1291ca357;Path=/;HttpOnly;Domain=httpstat.us"},
		"X-AspNet-Version":            {"4.0.30319"},
		"Content-Type":                {"text/plain; charset=utf-8"},
		"X-Powered-By":                {"ASP.NET"},
		"Server":                      {"Microsoft-IIS/10.0"},
		"Cache-Control":               {"private"},
		"Date":                        {"Wed, 31 Jul 2019 11"},
		"Access-Control-Allow-Origin": {"*"},
		"Content-Length":              {"6"},
		"X-AspNetMvc-Version":         {"5.1"},
		"Location":                    {GetTlsServerUrl() + "/test"}, // suppose to be only for 3XX
	}

	return [][]byte{
		GenerateResponse(200, "200 OK", headersMap),
		//GenerateResponse(301, "301 Moved Permanently", headersMap),
		GenerateResponse(403, "403 Forbidden", headersMap),
		GenerateResponse(500, "500 Internal Server Error", headersMap),
	}[ht]
}

func GetTlsServerAddress() string {
	return fmt.Sprintf("localhost:%d", ListenPort)
}

func GetTlsServerUrl() string {
	return fmt.Sprintf("https://%s", GetTlsServerAddress())
}
