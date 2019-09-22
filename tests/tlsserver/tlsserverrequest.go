package main

import (
	"github.com/nuweba/httpbench/syncedtrace"
	tests "github.com/nuweba/httpbench/tests/utils"
	"time"
)

type ServerRequestResult tests.ServerRequestResult // Avoid import cycle

func NewRequest(Name string) *ServerRequestResult {
	req := ServerRequestResult{UniqueId: Name}

	hooks := make(map[syncedtrace.TraceHookType]*syncedtrace.Hook, syncedtrace.HooksCount)
	for i := syncedtrace.TraceHookType(0); i < syncedtrace.HooksCount; i++ {
		hooks[i] = &syncedtrace.Hook{}
	}
	req.Hooks = hooks

	return &req
}

func (req *ServerRequestResult) LogHookTime(hookType syncedtrace.TraceHookType, presetHookType syncedtrace.TraceHookType) {
	if hookType >= syncedtrace.HooksCount {
		return
	}
	req.Hooks[hookType].Time = time.Now()
}
