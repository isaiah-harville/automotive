//go:build linux

package socketcan

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/isaiah-harville/automotive/pkg/can"
)

type bus struct {
	fd int

	stateMu  sync.Mutex
	closed   bool
	active   sync.WaitGroup
	once     sync.Once
	closeErr error
}

// Open binds a raw CAN socket to iface, such as "can0" or "vcan0".
func Open(iface string) (can.Bus, error) {
	if iface == "" {
		return nil, errors.New("socketcan: interface name is empty")
	}
	networkInterface, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("socketcan: finding interface %q: %w", iface, err)
	}

	fd, err := unix.Socket(unix.AF_CAN, unix.SOCK_RAW|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, unix.CAN_RAW)
	if err != nil {
		return nil, fmt.Errorf("socketcan: creating raw socket: %w", err)
	}
	if err := unix.Bind(fd, &unix.SockaddrCAN{Ifindex: networkInterface.Index}); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("socketcan: binding to interface %q: %w", iface, err)
	}
	return &bus{fd: fd}, nil
}

func (b *bus) Send(frame can.Frame) error {
	raw, err := marshalFrame(frame)
	if err != nil {
		return err
	}
	fd, ok := b.beginOperation()
	if !ok {
		return can.ErrClosed
	}
	defer b.active.Done()

	written, err := unix.Write(fd, raw[:])
	if err != nil {
		if b.isClosed() {
			return can.ErrClosed
		}
		return fmt.Errorf("socketcan: sending frame: %w", err)
	}
	if written != len(raw) {
		return fmt.Errorf("socketcan: short write: wrote %d of %d bytes", written, len(raw))
	}
	return nil
}

func (b *bus) Recv(deadline time.Duration) (can.Frame, error) {
	if deadline <= 0 {
		return can.Frame{}, can.ErrTimeout
	}
	fd, ok := b.beginOperation()
	if !ok {
		return can.Frame{}, can.ErrClosed
	}
	defer b.active.Done()

	expires := time.Now().Add(deadline)
	for {
		remaining := time.Until(expires)
		if remaining <= 0 {
			return can.Frame{}, can.ErrTimeout
		}
		timeoutMillis := int((remaining + time.Millisecond - 1) / time.Millisecond)
		pollFDs := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		ready, err := unix.Poll(pollFDs, timeoutMillis)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if b.isClosed() {
				return can.Frame{}, can.ErrClosed
			}
			return can.Frame{}, fmt.Errorf("socketcan: waiting for frame: %w", err)
		}
		if ready == 0 {
			return can.Frame{}, can.ErrTimeout
		}
		if pollFDs[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			if b.isClosed() {
				return can.Frame{}, can.ErrClosed
			}
			return can.Frame{}, fmt.Errorf("socketcan: socket poll failed with events %#x", pollFDs[0].Revents)
		}

		var raw [classicFrameSize]byte
		n, err := unix.Read(fd, raw[:])
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			continue
		}
		if err != nil {
			if b.isClosed() {
				return can.Frame{}, can.ErrClosed
			}
			return can.Frame{}, fmt.Errorf("socketcan: receiving frame: %w", err)
		}
		frame, err := unmarshalFrame(raw[:n])
		if errors.Is(err, errIgnoredFrame) {
			continue
		}
		if err != nil {
			return can.Frame{}, err
		}
		return frame, nil
	}
}

func (b *bus) Close() error {
	b.once.Do(func() {
		b.stateMu.Lock()
		b.closed = true
		fd := b.fd
		b.stateMu.Unlock()

		// Shutdown wakes any goroutine blocked in Poll or Write. Keep the file
		// descriptor open until all active operations exit so it cannot be
		// reused for an unrelated socket while an operation still references it.
		_ = unix.Shutdown(fd, unix.SHUT_RDWR)
		b.active.Wait()
		if err := unix.Close(fd); err != nil && !errors.Is(err, unix.EBADF) {
			b.closeErr = fmt.Errorf("socketcan: closing socket: %w", err)
		}
	})
	return b.closeErr
}

func (b *bus) beginOperation() (int, bool) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.closed {
		return 0, false
	}
	b.active.Add(1)
	return b.fd, true
}

func (b *bus) isClosed() bool {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	return b.closed
}
