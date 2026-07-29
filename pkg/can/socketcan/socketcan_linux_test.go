//go:build linux

package socketcan

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/isaiah-harville/automotive/pkg/can"
)

func TestBusSendAndReceive(t *testing.T) {
	socketBus, peer := newSocketPairBus(t)

	wantSent := can.Frame{ID: 0x7E0, Data: []byte{0x02, 0x10, 0x02}}
	if err := socketBus.Send(wantSent); err != nil {
		t.Fatalf("Send: %v", err)
	}
	var sentRaw [classicFrameSize]byte
	n, err := unix.Read(peer, sentRaw[:])
	if err != nil {
		t.Fatalf("reading sent frame: %v", err)
	}
	gotSent, err := unmarshalFrame(sentRaw[:n])
	if err != nil {
		t.Fatalf("decoding sent frame: %v", err)
	}
	assertFrameEqual(t, gotSent, wantSent)

	wantReceived := can.Frame{ID: 0x18DAF110, Data: []byte{0x01, 0x3E}}
	receivedRaw, err := marshalFrame(wantReceived)
	if err != nil {
		t.Fatalf("encoding received frame: %v", err)
	}
	if _, err := unix.Write(peer, receivedRaw[:]); err != nil {
		t.Fatalf("writing received frame: %v", err)
	}
	gotReceived, err := socketBus.Recv(time.Second)
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	assertFrameEqual(t, gotReceived, wantReceived)
}

func TestBusReceiveTimeout(t *testing.T) {
	socketBus, _ := newSocketPairBus(t)

	if _, err := socketBus.Recv(time.Millisecond); !errors.Is(err, can.ErrTimeout) {
		t.Fatalf("Recv error = %v, want can.ErrTimeout", err)
	}
}

func TestBusCloseIsIdempotentAndWakesReceive(t *testing.T) {
	socketBus, _ := newSocketPairBus(t)
	receiveDone := make(chan error, 1)
	go func() {
		_, err := socketBus.Recv(time.Hour)
		receiveDone <- err
	}()

	if err := socketBus.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := socketBus.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case err := <-receiveDone:
		if !errors.Is(err, can.ErrClosed) {
			t.Fatalf("blocked Recv error = %v, want can.ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not wake blocked Recv")
	}

	if err := socketBus.Send(can.Frame{ID: 0x123}); !errors.Is(err, can.ErrClosed) {
		t.Fatalf("Send after Close error = %v, want can.ErrClosed", err)
	}
	if _, err := socketBus.Recv(time.Second); !errors.Is(err, can.ErrClosed) {
		t.Fatalf("Recv after Close error = %v, want can.ErrClosed", err)
	}
}

func TestOpenRejectsEmptyInterface(t *testing.T) {
	if bus, err := Open(""); err == nil || bus != nil {
		t.Fatalf("Open empty interface = (%v, %v), want (nil, error)", bus, err)
	}
}

func newSocketPairBus(t *testing.T) (*bus, int) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("Socketpair: %v", err)
	}
	socketBus := &bus{fd: fds[0]}
	t.Cleanup(func() {
		_ = socketBus.Close()
		_ = unix.Close(fds[1])
	})
	return socketBus, fds[1]
}

func assertFrameEqual(t *testing.T, got, want can.Frame) {
	t.Helper()
	if got.ID != want.ID || !bytes.Equal(got.Data, want.Data) {
		t.Fatalf("frame = %+v, want %+v", got, want)
	}
}
