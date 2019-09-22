package tests

import (
	"fmt"
	"github.com/nuweba/httpbench"
	"github.com/nuweba/httpbench/syncedtrace"
	utils "github.com/nuweba/httpbench/tests/utils"
	"testing"
)

const (
	MaxConcurrent    = 20
	ConcurrentOffset = 10
)

func TestConcurrentRequestsSyncedTable(t *testing.T) {
	waitHook := syncedtrace.TLSHandshakeDone
	testType := utils.Synced

	requestTests := []utils.BenchConfig{
		{SyncedConcurrent: 60, ReqDelay: utils.BaseReqDelay / 2, Duration: utils.BaseDuration, Type: testType, WaitHook: waitHook, DummyTraceSummary: utils.DummySleepTraceSummary},
		{SyncedConcurrent: 60, ReqDelay: utils.BaseReqDelay / 2, Duration: utils.BaseDuration, Type: testType, WaitHook: waitHook, FailsEveryReq: 5, FailsInRow: 2},
	}

	templateTests := []utils.BenchConfig{
		{Type: testType},
		{Type: testType, DummyTraceSummary: utils.DummySleepTraceSummary},
	}

	inputValues := [][]int64{
		{utils.BaseReqDelay.Nanoseconds(), utils.BaseReqDelay.Nanoseconds() / 20},
		{int64(syncedtrace.TLSHandshakeStart), int64(syncedtrace.TLSHandshakeDone), int64(syncedtrace.WroteRequest), int64(syncedtrace.GotFirstResponseByte)},
		{int64(utils.FailRequestEvery)},
		utils.FailInRow,
		{int64(utils.Http200) /*utils.Http300,*/, int64(utils.HttpRand)},
	}

	var syncedConcurrents []int64
	for i := int64(1); i <= MaxConcurrent+1; i += ConcurrentOffset { // From 1 to Max (+1) with 10 jumps
		syncedConcurrents = append(syncedConcurrents, i)
	}
	inputValues = append(inputValues, syncedConcurrents)

	requestTests = append(requestTests, utils.GenerateBenchConfigs(templateTests, inputValues)...)

	for _, benchConfig := range requestTests {
		testName := fmt.Sprintf("ConcurrentRequestsSynced: %#v\n", benchConfig)

		res := t.Run(testName, func(t *testing.T) {
			p := httpbench.New(utils.GetRequest(utils.GetTlsServerUrl()), benchConfig.WaitHook)
			utils.RunTest(t, p, benchConfig, func() *httpbench.PresetResult {
				return p.ConcurrentRequestsSynced(benchConfig.SyncedConcurrent, benchConfig.ReqDelay, benchConfig.Duration)
			})
		})
		if !res {
			break
		}
	}
}
