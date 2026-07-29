package isotp_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/isaiah-harville/automotive/pkg/can/simbus"
	"github.com/isaiah-harville/automotive/pkg/isotp"
)

const (
	testerID = 0x7E0
	ecuID    = 0x7E8
)

func TestSingleFrameRoundTrip(t *testing.T) {
	a, b := simbus.NewPair()
	tester := isotp.NewConn(a, testerID, ecuID)
	ecu := isotp.NewConn(b, ecuID, testerID)

	payload := []byte{0x10, 0x03}
	errc := make(chan error, 1)
	go func() { errc <- tester.Send(payload) }()

	got, err := ecu.Recv(time.Second)
	if err != nil {
		t.Fatalf("ecu.Recv: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("tester.Send: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %x, want %x", got, payload)
	}
}

func TestMultiFrameRoundTrip(t *testing.T) {
	a, b := simbus.NewPair()
	tester := isotp.NewConn(a, testerID, ecuID)
	ecu := isotp.NewConn(b, ecuID, testerID)

	payload := make([]byte, 200)
	for i := range payload {
		payload[i] = byte(i)
	}

	errc := make(chan error, 1)
	go func() { errc <- tester.Send(payload) }()

	got, err := ecu.Recv(time.Second)
	if err != nil {
		t.Fatalf("ecu.Recv: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("tester.Send: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %d bytes, want %d bytes; mismatch", len(got), len(payload))
	}
}

func TestExactlySingleFrameBoundary(t *testing.T) {
	a, b := simbus.NewPair()
	tester := isotp.NewConn(a, testerID, ecuID)
	ecu := isotp.NewConn(b, ecuID, testerID)

	payload := bytes.Repeat([]byte{0xAB}, 7) // exactly the single-frame max
	errc := make(chan error, 1)
	go func() { errc <- tester.Send(payload) }()

	got, err := ecu.Recv(time.Second)
	if err != nil {
		t.Fatalf("ecu.Recv: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("tester.Send: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %x, want %x", got, payload)
	}
}

func TestTooLarge(t *testing.T) {
	a, _ := simbus.NewPair()
	tester := isotp.NewConn(a, testerID, ecuID)
	if err := tester.Send(make([]byte, 4096)); err != isotp.ErrTooLarge {
		t.Fatalf("got err %v, want ErrTooLarge", err)
	}
}
