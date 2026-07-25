// Package isotp implements enough of ISO 15765-2 (ISO-TP) to carry UDS
// (ISO 14229) request/response payloads over classic 8-byte CAN frames:
// single-frame, first-frame/consecutive-frame segmentation and reassembly,
// and flow control. Functional (broadcast) addressing and CAN-FD are not
// implemented.
package isotp

import (
	"errors"
	"fmt"
	"time"

	"github.com/isaiah-harville/automotive-flow/pkg/can"
)

const (
	pciSingleFrame      = 0x0
	pciFirstFrame       = 0x1
	pciConsecutiveFrame = 0x2
	pciFlowControl      = 0x3

	fcContinue = 0x0
	fcWait     = 0x1
	fcOverflow = 0x2

	maxSingleFrameLen = 7
	maxPayloadLen     = 4095 // 12-bit length field
)

var (
	ErrTooLarge       = errors.New("isotp: payload exceeds 4095 bytes")
	ErrProtocol       = errors.New("isotp: protocol violation")
	ErrFlowControl    = errors.New("isotp: flow control aborted transfer")
	ErrSequenceNumber = errors.New("isotp: unexpected consecutive frame sequence number")
)

// Conn is a point-to-point ISO-TP connection over a can.Bus, addressed by a
// fixed pair of CAN arbitration IDs (physical addressing only).
type Conn struct {
	Bus   can.Bus
	TxID  uint32 // arbitration ID we send frames on
	RxID  uint32 // arbitration ID we expect frames on
	STmin time.Duration
}

// NewConn constructs a Conn with no consecutive-frame delay (suitable for
// simbus and most bench setups).
func NewConn(bus can.Bus, txID, rxID uint32) *Conn {
	return &Conn{Bus: bus, TxID: txID, RxID: rxID}
}

// Send segments payload into single or first/consecutive frames as needed.
func (c *Conn) Send(payload []byte) error {
	if len(payload) > maxPayloadLen {
		return ErrTooLarge
	}
	if len(payload) <= maxSingleFrameLen {
		frame := make([]byte, 8)
		frame[0] = byte(pciSingleFrame<<4) | byte(len(payload))
		copy(frame[1:], payload)
		return c.Bus.Send(can.Frame{ID: c.TxID, Data: trimPadding(frame, 1+len(payload))})
	}

	first := make([]byte, 8)
	first[0] = byte(pciFirstFrame<<4) | byte((len(payload)>>8)&0x0F)
	first[1] = byte(len(payload) & 0xFF)
	copy(first[2:], payload[:6])
	if err := c.Bus.Send(can.Frame{ID: c.TxID, Data: first}); err != nil {
		return err
	}

	bs, stmin, err := c.readFlowControl()
	if err != nil {
		return err
	}

	rest := payload[6:]
	seq := byte(1)
	sinceFC := 0
	for len(rest) > 0 {
		n := 7
		if n > len(rest) {
			n = len(rest)
		}
		cf := make([]byte, 8)
		cf[0] = byte(pciConsecutiveFrame<<4) | (seq & 0x0F)
		copy(cf[1:], rest[:n])
		if err := c.Bus.Send(can.Frame{ID: c.TxID, Data: trimPadding(cf, 1+n)}); err != nil {
			return err
		}
		rest = rest[n:]
		seq++
		sinceFC++

		if stmin > 0 {
			time.Sleep(stmin)
		}
		if bs > 0 && sinceFC == int(bs) && len(rest) > 0 {
			bs, stmin, err = c.readFlowControl()
			if err != nil {
				return err
			}
			sinceFC = 0
		}
	}
	return nil
}

func (c *Conn) readFlowControl() (blockSize byte, stmin time.Duration, err error) {
	f, err := c.Bus.Recv(2 * time.Second)
	if err != nil {
		return 0, 0, fmt.Errorf("isotp: waiting for flow control: %w", err)
	}
	if f.ID != c.RxID || len(f.Data) < 3 {
		return 0, 0, ErrProtocol
	}
	if f.Data[0]>>4 != pciFlowControl {
		return 0, 0, ErrProtocol
	}
	switch f.Data[0] & 0x0F {
	case fcContinue:
		return f.Data[1], stminFromByte(f.Data[2]), nil
	case fcWait:
		return c.readFlowControl()
	default:
		return 0, 0, ErrFlowControl
	}
}

func stminFromByte(b byte) time.Duration {
	switch {
	case b <= 0x7F:
		return time.Duration(b) * time.Millisecond
	case b >= 0xF1 && b <= 0xF9:
		return time.Duration(b-0xF0) * 100 * time.Microsecond
	default:
		return 0
	}
}

// Recv reassembles the next full payload, sending flow control frames as
// needed for multi-frame transfers. It blocks up to the given deadline for
// the first frame; subsequent consecutive frames each get their own
// deadline-length wait.
func (c *Conn) Recv(deadline time.Duration) ([]byte, error) {
	f, err := c.recvFrom(deadline)
	if err != nil {
		return nil, err
	}
	if len(f.Data) == 0 {
		return nil, ErrProtocol
	}

	pci := f.Data[0] >> 4
	switch pci {
	case pciSingleFrame:
		n := int(f.Data[0] & 0x0F)
		if n == 0 || 1+n > len(f.Data) {
			return nil, ErrProtocol
		}
		return append([]byte(nil), f.Data[1:1+n]...), nil

	case pciFirstFrame:
		if len(f.Data) < 2 {
			return nil, ErrProtocol
		}
		total := (int(f.Data[0]&0x0F) << 8) | int(f.Data[1])
		if total > maxPayloadLen {
			return nil, ErrTooLarge
		}
		payload := make([]byte, 0, total)
		payload = append(payload, f.Data[2:min(8, len(f.Data))]...)

		if err := c.Bus.Send(can.Frame{ID: c.TxID, Data: []byte{byte(pciFlowControl<<4) | fcContinue, 0x00, 0x00}}); err != nil {
			return nil, err
		}

		seq := byte(1)
		for len(payload) < total {
			cf, err := c.recvFrom(deadline)
			if err != nil {
				return nil, err
			}
			if len(cf.Data) == 0 || cf.Data[0]>>4 != pciConsecutiveFrame {
				return nil, ErrProtocol
			}
			if cf.Data[0]&0x0F != seq&0x0F {
				return nil, ErrSequenceNumber
			}
			need := total - len(payload)
			chunk := cf.Data[1:]
			if len(chunk) > need {
				chunk = chunk[:need]
			}
			payload = append(payload, chunk...)
			seq++
		}
		return payload, nil

	default:
		return nil, ErrProtocol
	}
}

func (c *Conn) recvFrom(deadline time.Duration) (can.Frame, error) {
	for {
		f, err := c.Bus.Recv(deadline)
		if err != nil {
			return can.Frame{}, err
		}
		if f.ID == c.RxID {
			return f, nil
		}
		// Frame for a different arbitration ID on the shared bus; ignore.
	}
}

func trimPadding(frame []byte, n int) []byte {
	if n >= len(frame) {
		return frame
	}
	return frame[:n]
}
