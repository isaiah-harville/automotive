// Package simbus provides an in-process can.Bus implementation for testing
// and local development without any real CAN hardware.
package simbus

import (
	"sync"
	"time"

	"github.com/isaiah-harville/automotive-flow/pkg/can"
)

// bus is one end of a simulated point-to-point CAN link.
type bus struct {
	out chan can.Frame

	mu     sync.Mutex
	in     chan can.Frame
	closed bool
}

// NewPair returns two connected buses: frames sent on a arrive on b, and
// frames sent on b arrive on a. This models a tester and an ECU sharing one
// CAN segment where every frame is visible to both sides.
func NewPair() (a can.Bus, b can.Bus) {
	ab := make(chan can.Frame, 64)
	ba := make(chan can.Frame, 64)
	return &bus{out: ab, in: ba}, &bus{out: ba, in: ab}
}

func (b *bus) Send(f can.Frame) error {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return can.ErrClosed
	}
	b.out <- f
	return nil
}

func (b *bus) Recv(deadline time.Duration) (can.Frame, error) {
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case f, ok := <-b.in:
		if !ok {
			return can.Frame{}, can.ErrClosed
		}
		return f, nil
	case <-timer.C:
		return can.Frame{}, can.ErrTimeout
	}
}

func (b *bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	close(b.out)
	return nil
}
