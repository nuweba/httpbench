package syncedtrace

import (
	"github.com/pkg/errors"
	"strconv"
	"strings"
	"time"
)

type Hook struct {
	Time       time.Time     `json:"time"`
	Duration   time.Duration `json:"duration"`
	Wait       bool          `json:"did_wait"`
	LocalDrift time.Duration `json:"local_drift"`
	Err        error         `json:"error"`
}

type TraceHookType int

const (
	GetConn TraceHookType = iota
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
	HooksCount
)

var ThtStrings = [...]string{
	"GetConn",
	"DNSStart",          //ServerSide
	"DNSDone",           //ServerSide
	"ConnectStart",      //ServerSide
	"ConnectDone",       //ServerSide
	"TLSHandshakeStart", //ServerSide
	"TLSHandshakeDone",  //ServerSide
	"GotConn",
	"WroteHeaders",
	"WroteRequest",
	"Got100Continue",       //ServerSide
	"GotFirstResponseByte", //ServerSide
	"Wait100Continue",      //ServerSide
}

func (tht TraceHookType) String() string {
	return ThtStrings[tht]
}

func Unstring(name string) (TraceHookType, error) {
	for i, th := range ThtStrings {
		a := strings.Trim(name, "\"")
		if a == th {
			return TraceHookType(i), nil
		}
	}
	return HooksCount, errors.New("no trace hook type match found")
}

func (tht TraceHookType) MarshalText() (text []byte, err error) {
	return []byte(tht.String()), nil
}

func (tht *TraceHookType) UnmarshalText(text []byte) (err error) {
	thi, err := strconv.Atoi(string(text))
	if err == nil {
		*tht = TraceHookType(thi)
	} else {
		*tht, err = Unstring(string(text))
	}
	return err
}

func (th *Hook) LogError(err error) {
	if err != nil {
		th.Err = err
	}
}

func (th *Hook) LogTime(now time.Time, latest time.Time) {
	th.Time = now
	if now == latest {
		return
	}
	th.Duration = th.Time.Sub(latest) - th.LocalDrift
}

func (th *Hook) SetWait() {
	th.Wait = true
}

func (th *Hook) UnsetWait() {
	th.Wait = false
}

func (th *Hook) SetLocalDrift(localDrift time.Duration) {
	th.LocalDrift = localDrift
}

func (th *Hook) NeedToWait() bool {
	return th.Wait
}
