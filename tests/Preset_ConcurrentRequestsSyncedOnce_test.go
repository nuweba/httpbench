package tests

import (
	"fmt"
	"github.com/nuweba/httpbench"
	"github.com/nuweba/httpbench/syncedtrace"
	utils "github.com/nuweba/httpbench/tests/utils"
	"testing"
)

func TestConcurrentRequestsSyncedOnceTable(t *testing.T) {
	testType := utils.SyncedOnce

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
		testName := fmt.Sprintf("ConcurrentRequestsUnsynced: %#v\n", benchConfig)

		t.Run(testName, func(t *testing.T) {
			p := httpbench.New(utils.GetRequest(utils.GetTlsServerUrl()), benchConfig.WaitHook)
			utils.RunTest(t, p, benchConfig, func() *httpbench.PresetResult {
				return p.RequestPerDuration(benchConfig.ReqDelay, benchConfig.Duration)
			})
		})
	}
}
