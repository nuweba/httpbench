package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/nuweba/counter"
	"github.com/nuweba/httpbench/syncedtrace"
	"github.com/nuweba/httpbench/tests/utils"
	"github.com/pkg/errors"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	emptyFunc = func() {}
)

type TlsServer struct {
	commSock          *tests.SockIpc
	First             time.Time
	Latest            time.Time
	Duration          time.Duration
	CurrentReqCounter *counter.Counter
	ReqCounter        *counter.Counter
	BenchConfig       tests.BenchConfig
	MaxRequests       uint64
}

func (s *TlsServer) LogRequest() {
	t := time.Now()
	if s.First.IsZero() {
		s.First = t
	} else {
		s.Latest = t
		s.Duration = s.Latest.Sub(s.First)
	}
}

func (s *TlsServer) sendResult(res *ServerRequestResult) {
	data, err := json.Marshal(res)

	if err != nil {
		fmt.Println("Couldn't marshal object", err)
		return
	}

	data = append(data, []byte(tests.CommunicationSeparator)...)
	if s.commSock != nil {
		s.commSock.SendData(data)
	}
}

func (s *TlsServer) incReqCounter() (tests.HttpResponseType, error) {
	c := s.CurrentReqCounter.Inc()
	reqs := s.ReqCounter.Inc()

	if s.BenchConfig.Type != tests.Graph && s.BenchConfig.MaxConcurrentLimit != 0 && (c > s.BenchConfig.SyncedConcurrent || c > s.BenchConfig.MaxConcurrentLimit) {
		return tests.Http200, errors.New(fmt.Sprintf("requests counter exceeded over %d: %d", s.BenchConfig.SyncedConcurrent, s.CurrentReqCounter.Counter()))
	}

	if s.BenchConfig.FailsEveryReq == 0 {
		return tests.Http200, nil
	}

	n := s.BenchConfig.FailsEveryReq
	m := s.BenchConfig.FailsInRow
	if reqs%n == 0 || (reqs/n >= 1 && reqs%n < m) {
		if s.BenchConfig.FailType == tests.HttpRand {
			return tests.HttpResponseType(rand.Intn(int(tests.HttpRand-1) + 1)), nil //Exclude Http200 from rand
		}
		return s.BenchConfig.FailType, nil
	}

	return tests.Http200, nil
}

func (s *TlsServer) sleepTraceSummary(hookType syncedtrace.TraceHookType) {
	if s.BenchConfig.DummyTraceSummary == nil {
		return
	}
	// For every hook, sleep before next hook to increase its duration
	switch hookType {
	case syncedtrace.TLSHandshakeStart:
		time.Sleep(s.BenchConfig.DummyTraceSummary.TlsHandshake)
	case syncedtrace.TLSHandshakeDone:
		time.Sleep(s.BenchConfig.DummyTraceSummary.WroteRequest)
	case syncedtrace.WroteRequest:
		time.Sleep(s.BenchConfig.DummyTraceSummary.FirstByte)
	case syncedtrace.GotFirstResponseByte:
		time.Sleep(s.BenchConfig.DummyTraceSummary.ReadingBody)
	}
}

func (s *TlsServer) LogHookAndWait(req *ServerRequestResult, hookType syncedtrace.TraceHookType) error {
	req.LogHookTime(hookType, s.BenchConfig.WaitHook)
	s.sleepTraceSummary(hookType) // Stall next hook trace

	return nil
}

func (s *TlsServer) communicationSetup() (err error) {
	sock := tests.SockIpc{}
	err = sock.Connect()
	if err != nil {
		return
	}
	s.commSock = &sock

	rawConfig := sock.ReadConfig()
	var bench tests.BenchConfig

	err = json.Unmarshal(rawConfig, &bench)
	if err != nil {
		return err
	}

	s.BenchConfig = bench
	if s.BenchConfig.WaitHook == syncedtrace.GetConn { // So no way error will be generated for non-synced (GetConn is a client-side hook)
		s.BenchConfig.WaitHook = syncedtrace.HooksCount
	}
	return
}

func (s *TlsServer) setup() (*tls.Config, error) {

	rand.Seed(time.Now().UnixNano())

	err := s.communicationSetup()
	if err != nil {
		return nil, errors.Wrap(err, "setup communication")
	}

	s.CurrentReqCounter = new(counter.Counter)
	s.ReqCounter = new(counter.Counter)

	cer, err := tls.LoadX509KeyPair(tests.CertFile, tests.KeyFile)
	config := tls.Config{Certificates: []tls.Certificate{cer}}
	if err != nil {
		return nil, errors.Wrap(err, "loading certificates")
	}

	roughRequestsNumber := uint64(s.BenchConfig.Duration.Nanoseconds())
	if s.BenchConfig.ReqDelay >= time.Millisecond {
		roughRequestsNumber /= uint64(s.BenchConfig.ReqDelay.Nanoseconds() / 2) // Dividing by 2 to leave some spare
	}
	roughRequestsNumber += s.BenchConfig.SyncedConcurrent // in case where there is only 1 loop while ignoring the duration

	if s.BenchConfig.HitsGraph != nil {
		roughRequestsNumber = 0
		for _, hits := range s.BenchConfig.HitsGraph {
			roughRequestsNumber += hits.Concurrent
		}
	}

	s.MaxRequests = roughRequestsNumber

	return &config, nil
}

func (s *TlsServer) setStatusReady() {
	s.setStatusReadyError(nil)
}

func (s *TlsServer) setStatusReadyError(err error) {
	if s.commSock != nil {
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		s.commSock.SendData([]byte(tests.ReadyString + errStr))
	}
}

func (s *TlsServer) Run() func() {
	config, err := s.setup()
	if err != nil {
		fmt.Println(err)
		return emptyFunc
	}

	ln, err := tls.Listen("tcp", fmt.Sprintf(tests.GetTlsServerAddress()), config)
	if err != nil {
		log.Fatal("Error listening: ", err)
	}

	serverRunning := false
	ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	go func() {
		for {
			_, err := http.Get(tests.GetTlsServerUrl()) // Wait for server socket to listen
			if err == nil {
				serverRunning = true
				s.setStatusReady()
				return
			}

			if !strings.Contains(err.Error(), "timeout") {
				s.setStatusReadyError(err)
			}

			select {
			case <-ctx.Done():
				s.setStatusReadyError(errors.New("server didn't respond in time"))
				return
			}
		}
	}()

	for {
		conn, err := ln.Accept()
		if !serverRunning {
			go func() {
				conn.Write(tests.Http200.Get())
				conn.Close()
			}()
			continue
		}
		ctxCancel()

		if err != nil {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				fmt.Println(err)
			}
			conn.Close()
			return func() { ln.Close() }
		}
		go s.handleConnection(conn)
		if s.ReqCounter.Counter() > s.MaxRequests {
			// Channel is full
			fmt.Println("Server results channel is full. Stop server.", s.MaxRequests)
			return func() { ln.Close() }
		}
	}

	return func() { ln.Close() }
}

func (s *TlsServer) getRequestId(msg string) string {
	start := "GET /"
	end := "HTTP/"
	return strings.TrimSpace(msg[strings.Index(msg, start)+len(start) : strings.Index(msg, end)])
}

func (s *TlsServer) sendResultWithError(req *ServerRequestResult, err error) {
	if err != nil {
		req.Err = err.Error()
	}

	req.AggregatedDuration = s.Duration
	s.CurrentReqCounter.Dec()
	req.EndTime = time.Now()
	s.sendResult(req)
}

func (s *TlsServer) readByte(r *bufio.Reader, req *ServerRequestResult) error {
	req.BeforeRead = time.Now()
	if _, err := r.ReadByte(); err != nil {
		s.sendResultWithError(req, err)
		return err
	}
	if err := s.LogHookAndWait(req, syncedtrace.WroteRequest); err != nil { // Make sure no request has gotten here without all the rest synced passed their hooks
		s.sendResultWithError(req, err)
		return err
	}
	return r.UnreadByte()
}

func (s *TlsServer) handleRequest(req *ServerRequestResult, conn net.Conn, responseType tests.HttpResponseType) {
	r := bufio.NewReader(conn)
	readByte := false

	for {
		for {
			if !readByte {
				// Read only first byte in order to log read time efficiently (read all may take more time)
				if err := s.readByte(r, req); err != nil {
					return
				}
				readByte = true
			}
			msg, err := r.ReadString('\n')

			if err != nil {
				if err == io.EOF { // Connection was closed gracefully
					s.sendResultWithError(req, nil)
				} else {
					s.sendResultWithError(req, err)
				}
				return
			}

			if strings.HasPrefix(msg, "GET /") {
				req.UniqueId = s.getRequestId(strings.TrimSpace(msg))
			}

			if msg == "\r\n" { // End of request
				break
			}
		}

		serverResponse := responseType.Get()                                         // Send response
		for i, respBytes := range [][]byte{serverResponse[:1], serverResponse[1:]} { // To get closer to the client GotFirstByte, send only 1 byte, log and then send the rest
			n, err := conn.Write(respBytes)
			if i == 0 {
				s.LogHookAndWait(req, syncedtrace.GotFirstResponseByte)
			}
			if err != nil {
				fmt.Println(n, err)
				return
			}
		}
	}
}

func (s *TlsServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	req := NewRequest(fmt.Sprintf("%s%d", tests.NonSeededRequest, s.ReqCounter.Counter()))

	err := conn.SetReadDeadline(time.Now().Add(tests.ServerTimeout)) // Server timeout
	if err != nil {
		s.sendResultWithError(req, err)
		conn.Close()
		return
	}

	responseType, err := s.incReqCounter()
	if err != nil {
		s.sendResultWithError(req, err)
		return
	}

	s.LogRequest()
	if err := s.LogHookAndWait(req, syncedtrace.ConnectDone); err != nil {
		return
	}

	s.LogHookAndWait(req, syncedtrace.TLSHandshakeStart)
	err = conn.(*tls.Conn).Handshake()
	s.LogHookAndWait(req, syncedtrace.TLSHandshakeDone)

	if err != nil {
		s.sendResultWithError(req, err)
		conn.Close()
		return
	}

	s.handleRequest(req, conn, responseType)
}

func main() {
	s := TlsServer{}
	s.Run()
}
