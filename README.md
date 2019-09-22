[TOC]

#### Purpose
Fine tuned http/s benchmark framework with synchronization across requests and detailed trace.
#### Motivation
In order to reliably test invocation latency and benchmark FaaS providers, a benchmark tool with fine grained control over the request and the results.
The major issue with current benchmarking tools is that they didn't allow us to synchronize between the stages of request while testing.

For example, a simple test of 3 concurrent requests with a delay of 20ms to an endpoint resulted in an unreliable invocation latency results, because it took into affect the dns resolve time, the tcp handshake and tlshandshake time, and the delay didn't have the desired effect, requests invoked the endpoint in random times.

This was due to the fact that the invocation accord AFTER the tlshandshake.

httpbench allowed us to wait for the 3 concurrent requests to finished the tls handshake, and then release each one with 20ms delay between them.
#### Hooks
![Hooks](https://github.com/nuweba/httpbench/blob/master/_assets/the_hooks.svg)
```
	GetConn
	DNSStart
	DNSDone
	ConnectStart
	ConnectDone
	TLSHandshakeStart
	TLSHandshakeDone
	GotConn
	WroteHeaders
	WroteRequest
	Got100Continue
	GotFirstResponseByte
	Wait100Continue
  ```
#### Presets
Over time presets where added to enable more test types:

##### RequestPerDuration
![RequestPerDuration](https://github.com/nuweba/httpbench/blob/master/_assets/RequestPerDuration.svg)
##### ConcurrentRequestsUnsynced
![ConcurrentRequestsUnsynced](https://github.com/nuweba/httpbench/blob/master/_assets/ConcurrentRequestsUnSynced.svg)
##### ConcurrentRequestsSynced
![ConcurrentRequestsSynced](https://github.com/nuweba/httpbench/blob/master/_assets/ConcurrentRequestsSynced.svg)
##### ConcurrentRequestsSyncedOnce
![ConcurrentRequestsSyncedOnce](https://github.com/nuweba/httpbench/blob/master/_assets/ConcurrentRequestsOnce.svg)
##### RequestsForTimeGraph
![RequestsForTimeGraph](https://github.com/nuweba/httpbench/blob/master/_assets/RequestForTimeGraph.svg)
##### ConcurrentForTimeGraph
![ConcurrentForTimeGraph](https://github.com/nuweba/httpbench/blob/master/_assets/ConcurrentForTimeGraph.svg)

#### Test
Should trust the included private key in order to run locally (or add your own trusted one)
Integration tests cannot run parallel at the moment (TODO: add different server process for each test in order to run parallel)
