package tests

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nuweba/httpbench"
	"github.com/nuweba/httpbench/engine"
	"github.com/pkg/errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const NumberOfTasksToFinish = 2 /* If these two tasks doesn't finish with a reasonable time- we declare test timeout
a. The http bench process
b. The result channel is closed
*/

type TlsServerCmd struct {
	Cmd        *exec.Cmd
	ctx        context.Context
	cancelFunc context.CancelFunc
	StdoutPipe io.ReadCloser
	StderrPipe io.ReadCloser
}

type CombinedTestResults struct {
	TraceResults  map[string]*engine.TraceResult
	ServerResults map[string]*ServerRequestResult
}

type CombinedTestResult struct {
	TraceResults  *engine.TraceResult  `json:"bench_result"`
	ServerResults *ServerRequestResult `json:"server_result"`
}

func NewTestResults() *CombinedTestResults {
	trace := make(map[string]*engine.TraceResult)
	server := make(map[string]*ServerRequestResult)
	tr := CombinedTestResults{
		TraceResults:  trace,
		ServerResults: server,
	}
	return &tr
}

func (s *TlsServerCmd) Done() <-chan struct{} {
	return s.ctx.Done()
}

func (s *TlsServerCmd) Stop() {
	s.StdoutPipe.Close()
	s.StderrPipe.Close()
	s.cancelFunc()
	s.Done()
}

func runServer(serverReadyChan chan error) (*TlsServerCmd, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, ServerPath)

	outputFromServer := make(chan string)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	server := TlsServerCmd{
		Cmd:        cmd,
		ctx:        ctx,
		cancelFunc: cancel,
		StdoutPipe: stdoutPipe,
		StderrPipe: stderrPipe,
	}

	readFromPipe := func(pipe io.ReadCloser) {
		reader := bufio.NewReader(pipe)
		for {
			output, _, err := reader.ReadLine()
			if err == nil {
				outputFromServer <- string(output)
				continue
			}

			if !strings.Contains(err.Error(), os.ErrClosed.Error()) {
				fmt.Println("Exiting pipe", err)
			}
			serverReadyChan <- err
			return
		}
	}

	go readFromPipe(stdoutPipe)
	go readFromPipe(stderrPipe)

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			select { // TODO: If server exited. should we do something about it? (stdout is enough..?)
			case <-ctx.Done():
				serverReadyChan <- errors.New("context done")
				return
			case output := <-outputFromServer:
				fmt.Println("From TLS server: ", output)
			default:

			}
		}
	}()

	return &server, nil
}

func parseServerResults(data []byte) ([]*ServerRequestResult, []byte, error) {
	parsedRes := make([]*ServerRequestResult, 0)

	splitStrings := bytes.Split(data, []byte(CommunicationSeparator))
	for i, sp := range splitStrings {
		if len(sp) == 0 {
			continue
		}

		res := ServerRequestResult{}
		err := json.Unmarshal(sp, &res)
		if err != nil {
			if i == len(splitStrings)-1 { //Last item may be truncated data
				return parsedRes, []byte(sp), nil
			} else {
				return parsedRes, nil, err
			}
		}
		parsedRes = append(parsedRes, &res)
	}

	return parsedRes, nil, nil
}

func (tr *TestResults) WaitForAllResults() (int, int, bool) {
	timeout := time.After(10 * time.Second)
	tick := time.Tick(200 * time.Millisecond)

	for {
		select {
		case <-timeout:
			l1, l2 := len(tr.TestResults.TraceResults), len(tr.TestResults.ServerResults)
			gotAllRes := l1 == l2 && l2 != 0
			return l1, l2, gotAllRes
		case <-tick:
			l1, l2 := len(tr.TestResults.TraceResults), len(tr.TestResults.ServerResults)
			if l1 == l2 && l2 != 0 {
				return l1, l2, true
			}
		}
	}
}

func GenerateTestTraceSummary(sleepDuration time.Duration) *engine.TraceSummary {
	return &engine.TraceSummary{
		Dns:          sleepDuration,
		TcpHandshake: sleepDuration,
		TlsHandshake: sleepDuration,
		GotConn:      sleepDuration,
		WroteRequest: sleepDuration,
		FirstByte:    sleepDuration,
		ReadingBody:  sleepDuration,
	}
}

func GetRequest(url string) func(string) (*http.Request, error) {
	return func(uniqueId string) (*http.Request, error) {
		newUrl := fmt.Sprintf("%s/%s", url, uniqueId)
		r, _ := http.NewRequest(http.MethodGet, newUrl, nil)
		return r, nil
	}
}

func WrapTestTimeout(bench BenchConfig, finish <-chan bool) error {
	dur := bench.Duration

	//At least delay * concurrent once
	dur += time.Duration(bench.SyncedConcurrent) * bench.ReqDelay
	if bench.DummyTraceSummary != nil {
		dur += bench.DummyTraceSummary.TlsHandshake * 7 //Should be same for all 7 members in struct
	}

	if bench.HitsGraph != nil {
		for _, graph := range bench.HitsGraph {
			dur += graph.Time
		}
	}

	// Not too much, not too little
	if dur < time.Second {
		dur += 500 * time.Millisecond
	} else {
		dur += time.Second
	}

	ticker := time.NewTicker(dur)
	finishCount := 0
	for {
		select {
		case <-ticker.C:
			return errors.New("Test timed out")
		case <-finish:
			finishCount++
			if finishCount == NumberOfTasksToFinish {
				return nil
			}
		}
	}
	return nil
}

func (tr *TestResults) SetupAndRunServer(ServReadyChan chan error) (*TlsServerCmd, error) {
	sock := SockIpc{}
	tr.controlSock = &sock

	benchData, err := json.Marshal(tr.Bench)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to marshal bench config")
	}

	carry := make([]byte, 4096)
	sock.ListenAndServe(benchData, func(data []byte) {
		if strings.HasPrefix(string(data), ReadyString) {
			if string(data) == ReadyString {
				ServReadyChan <- nil
				return
			}
			ServReadyChan <- errors.New(strings.Trim(string(data), ReadyString))
		}

		data = append(bytes.Trim(carry, "\x00"), data...)
		receivedRes, carryData, err := parseServerResults(data)
		if err != nil {
			tr.T.Error(errors.Wrapf(err, "server request result(%d): %s", len(receivedRes), data))
			return
		}
		carry = carryData

		for _, res := range receivedRes {
			if res == nil {
				continue
			}
			tr.TestResults.ServerResults[strings.TrimSpace(res.UniqueId)] = res
		}
	})

	tlsServerCmd, err := runServer(ServReadyChan)

	if err != nil {
		return nil, errors.Wrap(err, "runServer")
	}

	return tlsServerCmd, nil
}

func RunTest(t *testing.T, p *httpbench.Preset, benchConfig BenchConfig, f func() *httpbench.PresetResult) {
	serverReadyChan := make(chan error, 1)

	tr := TestResults{T: t, Bench: &benchConfig, TestResults: NewTestResults()}
	testResults := tr.TestResults

	tlsServerCmd, err := tr.SetupAndRunServer(serverReadyChan)
	if err != nil {
		t.Error(err)
		return
	}

	defer tlsServerCmd.Stop()

	finishTest := make(chan bool)
	if err := <-serverReadyChan; err != nil { // Wait for server to be ready
		t.Error(errors.Wrap(err, "Server failed to run"))
		return
	}

	go func() {
		tr.PresetResult = f()
		finishTest <- true
	}()

	go func() {
		for {
			select {
			case r, ok := <-p.ResultCh:
				if !ok {
					finishTest <- true
					return
				}
				testResults.TraceResults[r.UniqueId] = r
			}
		}
	}()

	err = WrapTestTimeout(benchConfig, finishTest)
	if err != nil && t != nil {
		t.Error(err)
		return
	}

	if traceLen, serverLen, gotAll := tr.WaitForAllResults(); !gotAll {
		tr.T.Error(fmt.Sprintf("Couldn't get all results: trace len %d(%d), server len (%d)%d", traceLen, len(tr.TestResults.TraceResults), serverLen, len(tr.TestResults.ServerResults)))
		return
	}

	if tr.controlSock != nil {
		tr.controlSock.Close() //avoid data race
	}

	if tr.PresetResult == nil {
		tr.T.Error("Preset result is nil")
		return
	}
	tr.VerifyServerResults()
}
