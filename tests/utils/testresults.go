package tests

import (
	"fmt"
	"github.com/fatih/structs"
	"github.com/nuweba/httpbench"
	"github.com/nuweba/httpbench/engine"
	"github.com/nuweba/httpbench/syncedtrace"
	"github.com/pkg/errors"
	"sort"
	"strings"
	"testing"
	"time"
)

type TestResults struct {
	T                      *testing.T
	Bench                  *BenchConfig
	TestResults            *CombinedTestResults
	PresetResult           *httpbench.PresetResult
	controlSock            *SockIpc
	scopeCounter           int
	latestRequestStartTime time.Time
	latestRequestDoneTime  time.Time
}

func getNextServerHookTime(res *ServerRequestResult, th syncedtrace.TraceHookType) (syncedtrace.TraceHookType, time.Time) { //todo: only time
	//TODO: Check res.Hooks boundaries
	t := syncedtrace.Hook{} //empty struct
	for i := th + 1; i < syncedtrace.HooksCount; i++ {
		if *res.Hooks[i] != t {
			return i, res.Hooks[i].Time
		}
	}
	return syncedtrace.HooksCount, res.EndTime
}

func getPrevServerHookTime(res *ServerRequestResult, th syncedtrace.TraceHookType) (syncedtrace.TraceHookType, time.Time) {
	//TODO: Check res.Hooks boundaries
	t := syncedtrace.Hook{} //empty struct
	for i := th - 1; i >= syncedtrace.TraceHookType(0); i-- {
		if *res.Hooks[i] != t {
			return i, res.Hooks[i].Time
		}
	}
	return syncedtrace.HooksCount, res.EndTime
}

func getPrevTraceHook(res *engine.TraceResult, th syncedtrace.TraceHookType) *syncedtrace.Hook {
	//TODO: Check res.Hooks boundaries
	t := syncedtrace.Hook{} //empty struct
	for i := th - 1; i >= syncedtrace.TraceHookType(0); i-- {
		if *res.Hooks[i] != t {
			return res.Hooks[i]
		}
	}
	return &t
}

func (tr *TestResults) getSortedIdsByReqDoneTime() []string {
	type sortValues struct {
		id       string
		doneTime int64
	}

	values := make([]sortValues, 0)
	for k, v := range tr.TestResults.TraceResults {
		values = append(values, sortValues{k, v.Done.UnixNano()})
	}

	sort.Slice(values, func(i, j int) bool {
		return values[i].doneTime < values[j].doneTime
	})

	keys := make([]string, 0)
	for valIndex := range values {
		keys = append(keys, values[valIndex].id)
	}
	return keys
}

func (tr *TestResults) getSortedIdsByHookDoneTime(hookType syncedtrace.TraceHookType) []string {
	type sortValues struct {
		id           string
		hookDoneTime int64
	}

	values := make([]sortValues, 0)
	for k, v := range tr.TestResults.TraceResults {
		values = append(values, sortValues{k, v.Hooks[hookType].Time.UnixNano()})
	}

	sort.Slice(values, func(i, j int) bool {
		return values[i].hookDoneTime < values[j].hookDoneTime
	})

	keys := make([]string, 0)
	for valIndex := range values {
		keys = append(keys, values[valIndex].id)
	}
	return keys
}

func (tr *TestResults) getLatestWaitHookTime(results []*engine.TraceResult) time.Time {
	var times []int64
	for i := 0; i < len(results); i++ {
		hookDone := results[i].Hooks[tr.Bench.WaitHook].Time.Add(-1 * results[i].Hooks[tr.Bench.WaitHook].LocalDrift)
		times = append(times, hookDone.UnixNano())
	}
	return time.Unix(0, max(times...))
}

func (tr *TestResults) verifyReqDelay(results []*engine.TraceResult, latestWaitHookDoneTime time.Time) error {
	clientSideTrace := []syncedtrace.TraceHookType{syncedtrace.WroteRequest}
	if len(results) < int(tr.Bench.SyncedConcurrent) || tr.Bench.SyncedConcurrent <= 1 {
		return nil
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Hooks[tr.Bench.WaitHook].Time.UnixNano() < results[j].Hooks[tr.Bench.WaitHook].Time.UnixNano()
	})

	var prevHookTimes []int64
	var waitHookDoneTimes []int64
	var expectedServerDelay time.Duration // should be the dummy sleep while in range, than should be ~reqDelay
	var expectedReqDelay time.Duration
	var lastDelay time.Duration

	for _, TraceRes := range results {
		serverRes := tr.TestResults.ServerResults[TraceRes.UniqueId]
		_, nextHookTime := getNextServerHookTime(serverRes, tr.Bench.WaitHook)
		reqDelay := nextHookTime.Sub(serverRes.Hooks[tr.Bench.WaitHook].Time)

		if contains(clientSideTrace, tr.Bench.WaitHook) { // Client side traces cannot me measure with the next hook on server, but with the previous
			_, prevServerHook := getPrevServerHookTime(serverRes, tr.Bench.WaitHook)
			reqDelay = serverRes.Hooks[tr.Bench.WaitHook].Time.Sub(prevServerHook)
		}

		hookDoneTime := TraceRes.Hooks[tr.Bench.WaitHook].Time
		hookDoneWoDriftTime := hookDoneTime.Add(-1 * TraceRes.Hooks[tr.Bench.WaitHook].LocalDrift)
		prevHook := getPrevTraceHook(TraceRes, TraceRes.WaitOnHook)

		prevHookTimes = append(prevHookTimes, prevHook.Time.Add(-1*prevHook.LocalDrift).UnixNano())
		waitHookDoneTimes = append(waitHookDoneTimes, hookDoneTime.UnixNano())

		waitedForAllHooks := latestWaitHookDoneTime.Sub(hookDoneWoDriftTime) // How long did this request waited before all got to hook, without reqDelay!
		currentDelay := reqDelay                                             // use this to later set to lastDelay, without changes

		// Handle dummy sleep
		if tr.Bench.DummyTraceSummary != nil && reqDelay < tr.Bench.DummyTraceSummary.WroteRequest+3*ReqDelayAddition {
			lastDelay = time.Duration(0) // Not relevat the last one, as all should be same
			expectedServerDelay = tr.Bench.DummyTraceSummary.WroteRequest
		} else {
			if tr.Bench.DummyTraceSummary != nil && expectedServerDelay == tr.Bench.DummyTraceSummary.WroteRequest {
				lastDelay = time.Duration(0)
			} else {
				expectedReqDelay = tr.Bench.ReqDelay + ReqDelayAddition
			}
			expectedServerDelay = hookDoneTime.Sub(hookDoneWoDriftTime)

			if tr.Bench.WaitHook == syncedtrace.TLSHandshakeStart { // We should calc the handshake time, cause it take time to shake!!
				expectedServerDelay += TraceRes.Summary.TlsHandshake
			}
		}

		if err := IsValueInRange(reqDelay, expectedServerDelay); err != nil {
			return errors.Wrapf(err, "server request ID %s delay isn't the same as the server delay",
				serverRes.UniqueId)
		}

		if tr.Bench.WaitHook == syncedtrace.TLSHandshakeStart || lastDelay == time.Duration(0) { // On TLSHandshakeStart server waits synchronously on TLS handshake
			lastDelay = currentDelay - waitedForAllHooks
			continue
		}

		if err := IsValueInRange(reqDelay-waitedForAllHooks-lastDelay, expectedReqDelay); err != nil {
			return errors.Wrapf(err, "server request ID %s delay doesn't match", serverRes.UniqueId)
		}
		lastDelay = currentDelay - waitedForAllHooks
	}

	minHookDone := min(waitHookDoneTimes...)
	maxPrevHook := max(prevHookTimes...)
	if maxPrevHook > minHookDone {
		return errors.New(fmt.Sprintf("the latest previous hook was ended after at least 1 request ended the wait hook: %s > %s", time.Unix(0, maxPrevHook), time.Unix(0, minHookDone)))
	}
	return nil
}

func (tr *TestResults) verifySingleRequest(id string) error {
	clientSideTrace := []syncedtrace.TraceHookType{syncedtrace.WroteRequest}
	serverRes := tr.TestResults.ServerResults[id]
	traceRes := tr.TestResults.TraceResults[id]

	_, nextServerHookTime := getNextServerHookTime(serverRes, tr.Bench.WaitHook)
	serverReqDelay := nextServerHookTime.Sub(serverRes.Hooks[tr.Bench.WaitHook].Time)

	expectedServerDelay := tr.Bench.ReqDelay
	dummySleep := time.Duration(0)

	if tr.Bench.DummyTraceSummary != nil {
		dummySleep = tr.Bench.DummyTraceSummary.WroteRequest
		if tr.Bench.DummyTraceSummary.WroteRequest > tr.Bench.ReqDelay {
			expectedServerDelay = dummySleep
		}
	}

	if tr.Bench.WaitHook == syncedtrace.TLSHandshakeStart {
		expectedServerDelay = tr.Bench.ReqDelay + traceRes.Summary.TlsHandshake // dummySleep can be ignored, cause even if its is greater, client handshake will wait for it
	}

	if contains(clientSideTrace, tr.Bench.WaitHook) {
		expectedServerDelay = dummySleep
	}

	if err := IsValueInRange(serverReqDelay, expectedServerDelay); err != nil { // Check it from the server side
		return errors.Wrap(err, fmt.Sprintf("Req delay ID %s from server differ from config", id))
	}

	drift := traceRes.Hooks[tr.Bench.WaitHook].LocalDrift
	if err := IsValueInRange(drift, tr.Bench.ReqDelay); err != nil { // Check it from the bench side, where on requestPerDuration LocalDrift == serverReqDelay
		return errors.Wrap(err, fmt.Sprintf("Req delay ID %s from bench differ from config", id))
	}

	if traceRes.Start.Sub(tr.PresetResult.Start) > tr.Bench.Duration+3*ReqDelayAddition { // Request started after duration ended, race is possible so let's add some limit
		return errors.New(fmt.Sprintf("Requst %s got connection after duration has ended: %s/%s", id, traceRes.Start.Sub(tr.PresetResult.Start), tr.Bench.Duration))
	}
	return nil
}

func (tr *TestResults) verifyTrace(traceResult *engine.TraceResult, serverResult *ServerRequestResult) error {
	if traceResult == nil {
		return errors.New(fmt.Sprintf("no trace given for ID: %s", traceResult.UniqueId))
	}

	if traceResult.UniqueId != serverResult.UniqueId {
		return errors.New(fmt.Sprintf("verify trace got two different IDS: %s, %s", traceResult.UniqueId, serverResult.UniqueId))
	}

	benchMap := structs.Map(traceResult.Summary) // TODO: structs?!
	inputMap := structs.Map(GenerateTestTraceSummary(0))
	if tr.Bench.DummyTraceSummary != nil {
		inputMap = structs.Map(tr.Bench.DummyTraceSummary)
	}
	for trace, hook := range benchMap {
		if _, ok := TraceHooksServerTypes[trace]; !ok { // Not a server trace, so we can't control it
			continue
		}

		baseValue := hook.(time.Duration)
		refValue := inputMap[trace].(time.Duration)

		switch trace {
		case TlsHandshakeTraceName:
			refValue = serverResult.Hooks[syncedtrace.TLSHandshakeDone].Time.Sub(serverResult.Hooks[syncedtrace.TLSHandshakeStart].Time)
		case FirstByteTraceName:
			baseValue += traceResult.Hooks[syncedtrace.TLSHandshakeDone].LocalDrift + traceResult.Hooks[syncedtrace.WroteRequest].LocalDrift // depends if any is the wait hook
			//depends if there is dummy sleep
			serverWithSleep := serverResult.Hooks[syncedtrace.GotFirstResponseByte].Time.Sub(serverResult.Hooks[syncedtrace.WroteRequest].Time).Nanoseconds()
			serverWithReqDrift := serverResult.Hooks[syncedtrace.WroteRequest].Time.Sub(serverResult.Hooks[syncedtrace.TLSHandshakeDone].Time).Nanoseconds()
			refValue += time.Duration(max(serverWithSleep, serverWithReqDrift))
		case ReadingBodyTraceName:
			drift := traceResult.Hooks[syncedtrace.GotFirstResponseByte].LocalDrift
			if drift < refValue {
				baseValue += drift
				refValue += serverResult.EndTime.Sub(serverResult.Hooks[syncedtrace.GotFirstResponseByte].Time) - refValue
			} else {
				refValue = 0 // Waited too long for all hooks that the dummy sleep isn't relevant
			}
		}

		if err := IsValueInRange(baseValue, refValue); err != nil {
			return errors.Wrap(err, fmt.Sprintf("trace duration doesn't match for request ID %s(hook %s)", traceResult.UniqueId, trace))
		}
	}
	return nil
}

func (tr *TestResults) verifyConcurrency(results []*engine.TraceResult, latestRequestDone time.Time) (time.Time, error) {
	if len(results) != int(tr.Bench.SyncedConcurrent) {
		return time.Time{}, errors.New(fmt.Sprintf("got %d trace results, while synced concurrency is %d", len(results), tr.Bench.SyncedConcurrent))
	}

	newLatestDone := time.Time{}
	for _, res := range results {
		if res.Done.Sub(latestRequestDone) < 0 {
			return time.Time{}, errors.New(fmt.Sprintf("Req %s is done (%s) before the latest done time %s", res.UniqueId, res.Done, latestRequestDone))
		}

		if res.Done.Sub(newLatestDone) > 0 {
			newLatestDone = res.Done
		}
	}
	if newLatestDone == (time.Time{}) {
		return newLatestDone, errors.New("Couldn't set new latest done time")
	}
	return newLatestDone, nil
}

func (tr *TestResults) verifyGraphConcurrency(results []*engine.TraceResult) error {
	if len(results) != int(tr.Bench.SyncedConcurrent) || len(results) == 0 {
		return errors.New(fmt.Sprintf("got %d trace results, while synced concurrency is %d", len(results), tr.Bench.SyncedConcurrent))
	}

	newLatestStart := results[0].Hooks[tr.Bench.WaitHook].Time
	for i, res := range results {
		startedRequestTime := res.Hooks[tr.Bench.WaitHook].Time

		err := IsValueInRange(startedRequestTime.Sub(newLatestStart), time.Duration(0))
		if i > 0 && err != nil {
			return errors.Wrap(err, fmt.Sprintf("Req %s started before another request in the previous graph point", res.UniqueId))
		}

		if startedRequestTime.Sub(newLatestStart) > 0 {
			newLatestStart = startedRequestTime
		}
	}
	if newLatestStart == (time.Time{}) {
		return errors.New("Couldn't set new latest start time")
	}
	tr.latestRequestStartTime = newLatestStart
	return nil
}

func (tr *TestResults) verifyGraphTotalRequestsNumber() error {
	totalReqByBench := uint64(0)
	for _, graph := range tr.Bench.HitsGraph {
		totalReqByBench += graph.Concurrent
	}

	if len(tr.TestResults.TraceResults) != int(totalReqByBench) {
		return errors.New("number of graph requests doesn't match bench config")
	}
	return nil
}

func (tr *TestResults) verifyScope(scopedTraceResults []*engine.TraceResult) {
	latestWaitHookDoneTime := tr.getLatestWaitHookTime(scopedTraceResults)

	switch tr.Bench.Type {
	case PerDuration, UnSynced, SyncedOnce:
		if err := tr.verifySingleRequest(scopedTraceResults[0].UniqueId); err != nil {
			tr.T.Error(err)
		}

		latestRequestDoneTime, err := tr.verifyConcurrency(scopedTraceResults, tr.latestRequestDoneTime)
		if err != nil {
			tr.T.Error(errors.Wrapf(err, fmt.Sprintf("verifyConcurrency: latest request done time %s", latestRequestDoneTime)))
		}

		if tr.Bench.Type != SyncedOnce || tr.scopeCounter != 1 {
			break
		}

		if err := tr.verifyReqDelay(scopedTraceResults, latestWaitHookDoneTime); err != nil {
			tr.T.Error(errors.Wrapf(err, "Failed to verify scoped concurrency no.%d: ", tr.scopeCounter))
		}
	case Synced:
		sort.Slice(scopedTraceResults, func(i, j int) bool {
			return scopedTraceResults[i].Summary.FirstByte.Nanoseconds() > scopedTraceResults[j].Summary.FirstByte.Nanoseconds()
		})

		for _, bRes := range scopedTraceResults {
			if err := tr.verifyTrace(bRes, tr.TestResults.ServerResults[bRes.UniqueId]); err != nil {
				tr.T.Error(err)
				return
			}
		}

		latestWaitHookDoneTime := tr.getLatestWaitHookTime(scopedTraceResults)
		if err := tr.verifyReqDelay(scopedTraceResults, latestWaitHookDoneTime); err != nil {
			tr.T.Error(errors.Wrapf(err, "Failed to verify scoped concurrency no.%d: ", tr.scopeCounter))
			return
		}

	case Graph:
		var err error // We don't want to re-init latestRequestStartTime
		err = tr.verifyGraphConcurrency(scopedTraceResults)
		if err != nil {
			tr.T.Error(errors.Wrap(err, "verifyGraphConcurrency:"))
			return
		}
	default:
		tr.T.Error("Couldn't get bench config type")
	}
}

func (tr *TestResults) VerifyServerResults() {
	var scopedTraceResults []*engine.TraceResult

	if tr.TestResults == nil || tr.TestResults.TraceResults == nil || tr.TestResults.ServerResults == nil {
		tr.T.Error(fmt.Sprintf("No results for test: %+v", tr.TestResults))
	}

	if tr.PresetResult == nil {
		tr.T.Error("Couldn't retrieve preset result")
	}

	if tr.PresetResult != nil && int(tr.PresetResult.ReqCount) != len(tr.TestResults.TraceResults) {
		tr.T.Error(fmt.Sprintf("Number of results different between trace and preset res: %d != %d (Serv %d)", tr.PresetResult.ReqCount, len(tr.TestResults.TraceResults), len(tr.TestResults.ServerResults)))
	}

	if len(tr.TestResults.ServerResults) != len(tr.TestResults.TraceResults) || len(tr.TestResults.ServerResults) == 0 {
		tr.T.Error(fmt.Sprintf("Number of benchConf results and server results differ: %d != %d", len(tr.TestResults.ServerResults), len(tr.TestResults.TraceResults)))
	}

	var sortedUniqueIds []string
	if tr.Bench.Type == Graph {
		if err := tr.verifyGraphTotalRequestsNumber(); err != nil {
			tr.T.Error(errors.Wrapf(err, "verifyGraphTotalRequestsNumber: "))
		}
		sortedUniqueIds = tr.getSortedIdsByHookDoneTime(tr.Bench.WaitHook)
	} else {
		sortedUniqueIds = tr.getSortedIdsByReqDoneTime()
	}

	tr.scopeCounter = 1
	for _, k := range sortedUniqueIds {
		serverRes := tr.TestResults.ServerResults[k]
		if serverRes == nil {
			tr.T.Error(fmt.Sprintf("Couldn't find server result with ID %s", k))
			return
		}

		traceRes := tr.TestResults.TraceResults[serverRes.UniqueId]

		if serverRes.Err != "" {
			tr.T.Error(fmt.Sprintf("Server result with UniqueId %s failed with error: %s\n", serverRes.UniqueId, serverRes.Err))
		}

		if traceRes.Error {
			var hookErrs []error
			for hook := range traceRes.Hooks {
				if traceRes.Hooks[hook].Err != nil && !strings.Contains(traceRes.Hooks[hook].Err.Error(), IpV6Localhost) { // We don't listen on IPv6 localhost, so most probably it'll raise an error
					hookErrs = append(hookErrs, traceRes.Hooks[hook].Err)
				}
			}
			if len(hookErrs) > 0 {
				tr.T.Error(fmt.Sprintf("Trace result with UniqueId %s failed with error: %s\n", traceRes.UniqueId, hookErrs))
			}
		}
		scopedTraceResults = append(scopedTraceResults, traceRes) // Should maintain the same order as server results

		if tr.Bench.HitsGraph != nil && tr.scopeCounter <= len(tr.Bench.HitsGraph) {
			tr.Bench.SyncedConcurrent = tr.Bench.HitsGraph[tr.scopeCounter-1].Concurrent
		}

		if uint64(len(scopedTraceResults)) < tr.Bench.SyncedConcurrent {
			continue
		}

		tr.verifyScope(scopedTraceResults)
		scopedTraceResults = nil
		tr.scopeCounter++
	}
}
