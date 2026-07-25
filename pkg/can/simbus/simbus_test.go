package simbus_test

import (
	"testing"
	"time"

	"github.com/isaiah-harville/automotive-flow/pkg/can"
	"github.com/isaiah-harville/automotive-flow/pkg/can/simbus"
)

func TestSendRecvRoundTrip(t *testing.T) {
	a, b := simbus.NewPair()
	want := can.Frame{ID: 0x123, Data: []byte{0x01, 0x02, 0x03}}

	if err := a.Send(want); err != nil {
		t.Fatalf("a.Send: %v", err)
	}
	got, err := b.Recv(time.Second)
	if err != nil {
		t.Fatalf("b.Recv: %v", err)
	}
	if got.ID != want.ID || string(got.Data) != string(want.Data) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRecvIsBidirectional(t *testing.T) {
	a, b := simbus.NewPair()
	want := can.Frame{ID: 0x456, Data: []byte{0xAA}}

	if err := b.Send(want); err != nil {
		t.Fatalf("b.Send: %v", err)
	}
	got, err := a.Recv(time.Second)
	if err != nil {
		t.Fatalf("a.Recv: %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("got ID 0x%X, want 0x%X", got.ID, want.ID)
	}
}

func TestRecvTimeout(t *testing.T) {
	a, _ := simbus.NewPair()
	_, err := a.Recv(50 * time.Millisecond)
	if err != can.ErrTimeout {
		t.Fatalf("got err %v, want ErrTimeout", err)
	}
}

func TestSendAfterCloseReturnsErrClosed(t *testing.T) {
	a, _ := simbus.NewPair()
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := a.Send(can.Frame{ID: 0x1, Data: []byte{0x00}}); err != can.ErrClosed {
		t.Fatalf("got err %v, want ErrClosed", err)
	}
}

func TestRecvAfterCloseReturnsErrClosed(t *testing.T) {
	a, b := simbus.NewPair()
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Closing a closes a's outbound channel, which is b's inbound channel;
	// once drained, b.Recv should observe the channel closed.
	if _, err := b.Recv(time.Second); err != can.ErrClosed {
		t.Fatalf("got err %v, want ErrClosed", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	a, _ := simbus.NewPair()
	if err := a.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
