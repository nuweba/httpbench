package tests

import (
	"fmt"
	"github.com/nuweba/httpbench"
	"github.com/nuweba/httpbench/syncedtrace"
	utils "github.com/nuweba/httpbench/tests/utils"
	"github.com/pkg/errors"
	"strings"
	"sync"
	"testing"
)

func TestRequestPerDurationTable(t *testing.T) {
	testType := utils.PerDuration

	requestTests := []utils.BenchConfig{
		{SyncedConcurrent: 1, ReqDelay: utils.BaseReqDelay, Duration: utils.BaseDuration, Type: testType},
		{SyncedConcurrent: 1, ReqDelay: utils.BaseReqDelay, Duration: utils.BaseDuration, Type: testType, DummyTraceSummary: utils.DummySleepTraceSummary},
	}

	inputValues := [][]int64{
		{utils.BaseReqDelay.Nanoseconds(), utils.BaseReqDelay.Nanoseconds() / 20},
		{int64(syncedtrace.TLSHandshakeStart), int64(syncedtrace.TLSHandshakeDone), int64(syncedtrace.WroteRequest), int64(syncedtrace.GotFirstResponseByte)},
		{int64(utils.FailRequestEvery)},
		utils.FailInRow,
		{int64(utils.Http200) /*utils.Http300,*/, int64(utils.HttpRand)},
	}

	requestTests = utils.GenerateBenchConfigs(requestTests, inputValues)

	for _, benchConfig := range requestTests {
		testName := fmt.Sprintf("RequestPerDuration: %#v\n", benchConfig)

		t.Run(testName, func(t *testing.T) {
			p := httpbench.New(utils.GetRequest(utils.GetTlsServerUrl()), benchConfig.WaitHook)
			utils.RunTest(t, p, benchConfig, func() *httpbench.PresetResult {
				return p.RequestPerDuration(benchConfig.ReqDelay, benchConfig.Duration)
			})
		})
	}
}

//bad ssl
func TestRequestPerDurationBadSSL(t *testing.T) {
	benchConfig := utils.BenchConfig{MaxConcurrentLimit: 0, SyncedConcurrent: 1, Duration: utils.BaseDuration, ReqDelay: utils.BaseReqDelay}

	url := strings.ReplaceAll(utils.GetTlsServerUrl(), "localhost", "127.0.0.1") // To generate SSL error
	p := httpbench.New(utils.GetRequest(url), benchConfig.WaitHook)

	tr := utils.TestResults{T: t, Bench: &benchConfig, TestResults: utils.NewTestResults()}
	testResults := tr.TestResults

	ready := make(chan error)
	serverCmd, err := tr.SetupAndRunServer(ready)
	if err != nil {
		t.Error(err)
		return
	}
	defer serverCmd.Stop()

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		for {
			select {
			case r, ok := <-p.ResultCh:
				if !ok {
					wg.Done()
					return
				}
				testResults.TraceResults[r.UniqueId] = r
			}
		}
	}()

	if err := <-ready; err != nil {
		t.Error(errors.Wrap(err, "Server run failed"))
		return
	}
	pRes := p.RequestPerDuration(benchConfig.ReqDelay, benchConfig.Duration)
	tr.PresetResult = pRes
	wg.Wait()

	if l1, l2, done := tr.WaitForAllResults(); !done {
		t.Error(fmt.Sprintf("not all server results received: %d vs %d, %t", l1, l2, pRes == nil))
		return
	}

	for _, res := range tr.TestResults.TraceResults {
		if !res.Error || res.Err == nil || !strings.Contains(res.Err.Error(), "certificate") {
			t.Error(fmt.Sprintf("All results should contain error about certificate, %s doesn't", res.UniqueId))
		}
	}
	for _, res := range tr.TestResults.ServerResults {
		if res.Err == "" || !strings.Contains(res.Err, "certificate") {
			t.Error(fmt.Sprintf("All results should contain error about certificate, %s doesn't", res.UniqueId))
		}
	}
}
