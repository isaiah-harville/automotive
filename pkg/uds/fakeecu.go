package uds

import (
	"sync/atomic"
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

	// RoutineResults maps a routine identifier to the routineStatusRecord
	// bytes StartRoutine/StopRoutine/RequestRoutineResults should return
	// for it. A routine with no entry returns a positive response with
	// zero-length results.
	RoutineResults map[uint16][]byte

	// DataRecords backs ReadDataByIdentifier/WriteDataByIdentifier: reads
	// return whatever is stored under a data identifier (zero-length if
	// absent), and writes overwrite it. Only the responder goroutine
	// touches this map, so it's safe to pre-populate before starting a
	// test but not to read/write concurrently from a test goroutine.
	DataRecords map[uint16][]byte

	// TesterPresentCount counts received TesterPresent (0x3E) requests,
	// for tests asserting on keep-alive behavior.
	TesterPresentCount atomic.Int32

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
		e.TesterPresentCount.Add(1)
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

	case sidRoutineControl:
		if len(body) < 3 {
			return negative(sid, NRCRequestOutOfRange)
		}
		subFunction, routineID := body[0], uint16(body[1])<<8|uint16(body[2])
		result := e.RoutineResults[routineID]
		return positive(sid, append([]byte{subFunction, body[1], body[2]}, result...)...)

	case sidReadDataByIdentifier:
		if len(body) < 2 {
			return negative(sid, NRCRequestOutOfRange)
		}
		dataID := uint16(body[0])<<8 | uint16(body[1])
		return positive(sid, append([]byte{body[0], body[1]}, e.DataRecords[dataID]...)...)

	case sidWriteDataByIdentifier:
		if len(body) < 2 {
			return negative(sid, NRCRequestOutOfRange)
		}
		dataID := uint16(body[0])<<8 | uint16(body[1])
		if e.DataRecords == nil {
			e.DataRecords = make(map[uint16][]byte)
		}
		e.DataRecords[dataID] = append([]byte(nil), body[2:]...)
		return positive(sid, body[0], body[1])

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
