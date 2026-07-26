// Package uds implements a client for the subset of ISO 14229 (UDS) needed
// to drive an ECU flashing sequence: session control, security access,
// request download / transfer data / transfer exit, ECU reset, tester
// present, and DTC read/clear. It is built entirely on pkg/isotp, so it
// works unmodified against a simulated ECU (pkg/can/simbus) or real hardware
// once a can.Bus implementation exists for it.
package uds

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/isaiah-harville/automotive-flow/pkg/isotp"
)

// Negative response codes relevant to the services implemented here.
const (
	NRCRequestCorrectlyReceivedResponsePending = 0x78
	NRCGeneralReject                           = 0x10
	NRCServiceNotSupported                     = 0x11
	NRCConditionsNotCorrect                    = 0x22
	NRCRequestOutOfRange                       = 0x31
	NRCInvalidKey                              = 0x35
	NRCExceedNumberOfAttempts                  = 0x36
)

// NegativeResponseError is returned when the ECU replies with a 0x7F
// negative response.
type NegativeResponseError struct {
	SID byte
	NRC byte
}

func (e *NegativeResponseError) Error() string {
	return fmt.Sprintf("uds: negative response to SID 0x%02X: NRC 0x%02X", e.SID, e.NRC)
}

var ErrShortResponse = errors.New("uds: response shorter than expected")

// Client is a UDS session over a single ISO-TP connection.
type Client struct {
	conn *isotp.Conn
	// ResponseTimeout bounds each individual wait for a response frame.
	ResponseTimeout time.Duration
	// PendingTimeout bounds total time spent retrying while the ECU
	// replies with NRC 0x78 (response pending).
	PendingTimeout time.Duration

	// wireMu serializes access to the underlying ISO-TP connection across
	// concurrent callers of Request, e.g. a StartKeepAlive goroutine
	// racing with the goroutine driving a multi-block transfer. It is
	// held only for the duration of a single request/response exchange,
	// not across a whole service call, so a keep-alive tick only ever
	// lands in the gap between two other requests.
	wireMu sync.Mutex
}

// NewClient wraps an ISO-TP connection already addressed to a specific ECU.
func NewClient(conn *isotp.Conn) *Client {
	return &Client{
		conn:            conn,
		ResponseTimeout: 1 * time.Second,
		PendingTimeout:  10 * time.Second,
	}
}

// Request sends a UDS service request (sid + payload) and returns the
// response payload with the echoed SID+0x40 stripped off. It transparently
// retries while the ECU reports "response pending".
func (c *Client) Request(sid byte, payload []byte) ([]byte, error) {
	c.wireMu.Lock()
	defer c.wireMu.Unlock()

	req := make([]byte, 1+len(payload))
	req[0] = sid
	copy(req[1:], payload)
	if err := c.conn.Send(req); err != nil {
		return nil, fmt.Errorf("uds: sending SID 0x%02X: %w", sid, err)
	}

	deadline := time.Now().Add(c.PendingTimeout)
	for {
		resp, err := c.conn.Recv(c.ResponseTimeout)
		if err != nil {
			return nil, fmt.Errorf("uds: awaiting response to SID 0x%02X: %w", sid, err)
		}
		if len(resp) < 1 {
			return nil, ErrShortResponse
		}

		if resp[0] == 0x7F {
			if len(resp) < 3 {
				return nil, ErrShortResponse
			}
			nrc := resp[2]
			if nrc == NRCRequestCorrectlyReceivedResponsePending {
				if time.Now().After(deadline) {
					return nil, &NegativeResponseError{SID: sid, NRC: nrc}
				}
				continue
			}
			return nil, &NegativeResponseError{SID: sid, NRC: nrc}
		}

		if resp[0] != sid+0x40 {
			return nil, fmt.Errorf("uds: unexpected response SID 0x%02X for request SID 0x%02X", resp[0], sid)
		}
		return resp[1:], nil
	}
}
