// Package can defines a transport-agnostic CAN bus abstraction. Concrete
// implementations (simulated, SocketCAN, vendor pass-thru devices) live in
// subpackages and are interchangeable behind the Bus interface.
package can

import (
	"errors"
	"time"
)

// ErrClosed is returned by Send/Recv once the bus has been closed.
var ErrClosed = errors.New("can: bus closed")

// ErrTimeout is returned by Recv when no frame arrives within the deadline.
var ErrTimeout = errors.New("can: receive timeout")

// Frame is a single CAN data frame. ID is an 11-bit or 29-bit arbitration ID;
// Data is 0-8 bytes (classic CAN; CAN-FD is out of scope for now).
type Frame struct {
	ID   uint32
	Data []byte
}

// Bus is the minimum surface every CAN transport must implement. isotp and
// uds are built entirely against this interface so they work unmodified
// against a simulated bus, SocketCAN, or a future vendor pass-thru driver.
type Bus interface {
	Send(f Frame) error
	// Recv blocks until a frame arrives or deadline elapses, returning
	// ErrTimeout on expiry and ErrClosed if the bus has been closed.
	Recv(deadline time.Duration) (Frame, error)
	Close() error
}
