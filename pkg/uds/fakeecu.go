package uds

import (
	"time"

	"github.com/isaiah-harville/automotive-flow/pkg/isotp"
)

// FakeECU is a minimal scriptable UDS responder used for testing Client
// without real hardware. It understands just enough of the flashing
// sequence (session control, security access, download/transfer/exit,
// reset, DTC read/clear) to exercise Client end-to-end over pkg/isotp and
// pkg/can/simbus.
type FakeECU struct {
	conn *isotp.Conn

	Seed           []byte // seed returned by SecurityAccess request-seed
	ExpectedKey    []byte // key that unlocks SecurityAccess; nil = accept any
	MaxBlockLength int    // reported by RequestDownload; default 64
	DTCs           []byte // canned ReadDTCByStatusMask response payload
	unlocked       bool

	stop chan struct{}
}

// NewFakeECU starts a responder goroutine listening on conn. Call Stop to
// shut it down.
func NewFakeECU(conn *isotp.Conn) *FakeECU {
	e := &FakeECU{
		conn:           conn,
		Seed:           []byte{0x12, 0x34},
		MaxBlockLength: 64,
		stop:           make(chan struct{}),
	}
	go e.run()
	return e
}

func (e *FakeECU) Stop() { close(e.stop) }

func (e *FakeECU) run() {
	for {
		select {
		case <-e.stop:
			return
		default:
		}
		req, err := e.conn.Recv(200 * time.Millisecond)
		if err != nil {
			continue
		}
		if len(req) == 0 {
			continue
		}
		resp := e.handle(req)
		if resp != nil {
			_ = e.conn.Send(resp)
		}
	}
}

func positive(sid byte, data ...byte) []byte {
	return append([]byte{sid + 0x40}, data...)
}

func negative(sid, nrc byte) []byte {
	return []byte{0x7F, sid, nrc}
}

func (e *FakeECU) handle(req []byte) []byte {
	sid := req[0]
	body := req[1:]

	switch sid {
	case sidDiagnosticSessionControl:
		if len(body) < 1 {
			return negative(sid, NRCRequestOutOfRange)
		}
		return positive(sid, body[0], 0x00, 0x32, 0x01, 0xF4)

	case sidECUReset:
		if len(body) < 1 {
			return negative(sid, NRCRequestOutOfRange)
		}
		return positive(sid, body[0])

	case sidTesterPresent:
		return positive(sid, 0x00)

	case sidSecurityAccess:
		if len(body) < 1 {
			return negative(sid, NRCRequestOutOfRange)
		}
		level := body[0]
		if level%2 == 1 {
			if e.unlocked {
				return positive(sid, level) // already unlocked: zero-length seed
			}
			return positive(sid, append([]byte{level}, e.Seed...)...)
		}
		key := body[1:]
		if e.ExpectedKey != nil && !bytesEqual(key, e.ExpectedKey) {
			return negative(sid, NRCInvalidKey)
		}
		e.unlocked = true
		return positive(sid, level)

	case sidRequestDownload:
		mbl := e.MaxBlockLength
		return positive(sid, 0x20, byte(mbl>>8), byte(mbl))

	case sidTransferData:
		if len(body) < 1 {
			return negative(sid, NRCRequestOutOfRange)
		}
		return positive(sid, body[0])

	case sidRequestTransferExit:
		return positive(sid)

	case sidReadDTCInformation:
		return positive(sid, append([]byte{0x02}, e.DTCs...)...)

	case sidClearDiagnosticInfo:
		return positive(sid)

	default:
		return negative(sid, NRCServiceNotSupported)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
