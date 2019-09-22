package tests

import (
	"fmt"
	"github.com/nuweba/httpbench"
	"github.com/nuweba/httpbench/syncedtrace"
	utils "github.com/nuweba/httpbench/tests/utils"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"
)

const (
	MaxGraphConcurrent    = 15
	ConcurrentGraphOffset = 2
	TimeOffset            = 10 * time.Millisecond
	StartTime             = 10 * time.Millisecond
	MaxTime               = 100 * time.Millisecond
)

func zipRequestsPerTime(concurrents, times []int64) []httpbench.RequestsPerTime {
	maxLen := int(math.Max(float64(len(concurrents)), float64(len(times))))
	graph := make([]httpbench.RequestsPerTime, maxLen, maxLen)

	var lastCurr, lastTime int64
	for i := 0; i < maxLen; i++ {
		if i >= len(concurrents) {
			graph[i].Concurrent = uint64(lastCurr)
		} else {
			graph[i].Concurrent = uint64(concurrents[i])
			lastCurr = concurrents[i]
		}

		if i >= len(times) {
			graph[i].Time = time.Duration(lastTime)
		} else {
			graph[i].Time = time.Duration(times[i])
			lastTime = times[i]
		}
	}
	return graph
}

// Todo: Graph test still flaky. To be fixed.
func TestRequestsForTimeGraph(t *testing.T) {
	testType := utils.Graph

	var ConcurrentOrder, TimeDurations []int64
	for i := int64(1); i <= MaxGraphConcurrent; i += ConcurrentGraphOffset {
		ConcurrentOrder = append(ConcurrentOrder, i)
	}

	for i := StartTime.Nanoseconds(); i < MaxTime.Nanoseconds(); i += TimeOffset.Nanoseconds() {
		TimeDurations = append(TimeDurations, i)
	}

	shuffledConcurrentOrder := make([]int64, len(ConcurrentOrder))
	reversedConcurrentOrder := make([]int64, len(ConcurrentOrder))
	shuffledTimeDurations := make([]int64, len(TimeDurations))
	reversedTimeDurations := make([]int64, len(TimeDurations))

	copy(shuffledConcurrentOrder, ConcurrentOrder)
	copy(shuffledTimeDurations, TimeDurations)

	rand.Shuffle(len(shuffledConcurrentOrder), func(i, j int) {
		shuffledConcurrentOrder[i], shuffledConcurrentOrder[j] = shuffledConcurrentOrder[j], shuffledConcurrentOrder[i]
	})
	rand.Shuffle(len(shuffledTimeDurations), func(i, j int) {
		shuffledTimeDurations[i], shuffledTimeDurations[j] = shuffledTimeDurations[j], shuffledTimeDurations[i]
	})

	copy(reversedConcurrentOrder, ConcurrentOrder)
	copy(reversedTimeDurations, TimeDurations)

	sort.Slice(reversedConcurrentOrder, func(i, j int) bool {
		return reversedConcurrentOrder[i] > reversedConcurrentOrder[j]
	})
	sort.Slice(reversedTimeDurations, func(i, j int) bool {
		return reversedTimeDurations[i] > reversedTimeDurations[j]
	})

	var graphs []httpbench.HitsGraph
	graphs = append(graphs, zipRequestsPerTime(ConcurrentOrder, TimeDurations))

	for _, t := range TimeDurations {
		graphs = append(graphs, zipRequestsPerTime(ConcurrentOrder, []int64{t}))
	}
	for _, c := range ConcurrentOrder {
		graphs = append(graphs, zipRequestsPerTime([]int64{c}, TimeDurations))
	}

	graphs = append(graphs, zipRequestsPerTime(ConcurrentOrder, reversedTimeDurations))         // reverse
	graphs = append(graphs, zipRequestsPerTime(reversedConcurrentOrder, TimeDurations))         // reverse
	graphs = append(graphs, zipRequestsPerTime(reversedConcurrentOrder, reversedTimeDurations)) // reverse

	graphs = append(graphs, zipRequestsPerTime(ConcurrentOrder, shuffledTimeDurations))         // shuffle
	graphs = append(graphs, zipRequestsPerTime(shuffledConcurrentOrder, TimeDurations))         // shuffle
	graphs = append(graphs, zipRequestsPerTime(shuffledConcurrentOrder, shuffledTimeDurations)) // shuffle

	// Todo: add more permutation values
	inputValues := [][]int64{ // By BenchConfig.SetPermutation
		{},
		{int64(syncedtrace.TLSHandshakeStart), int64(syncedtrace.TLSHandshakeDone), int64(syncedtrace.WroteRequest), int64(syncedtrace.GotFirstResponseByte)},
		{},
		{},
		{},
		{},
	}

	var requestTests []utils.BenchConfig

	for _, graph := range graphs { // add graphs
		bench := utils.BenchConfig{Type: testType, HitsGraph: graph}
		requestTests = append(requestTests, utils.GenerateBenchConfigs([]utils.BenchConfig{bench}, inputValues)...)
	}

	for _, benchConfig := range requestTests {
		testName := fmt.Sprintf("HitsGraph: %#v\n", benchConfig)

		t.Run(testName, func(t *testing.T) {
			p := httpbench.New(utils.GetRequest(utils.GetTlsServerUrl()), benchConfig.WaitHook)
			utils.RunTest(t, p, benchConfig, func() *httpbench.PresetResult {
				return p.RequestsForTimeGraph(benchConfig.HitsGraph)
			})
		})
	}
}
