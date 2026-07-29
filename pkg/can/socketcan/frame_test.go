package socketcan

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/isaiah-harville/automotive/pkg/can"
)

func TestFrameRoundTrip(t *testing.T) {
	tests := []can.Frame{
		{ID: 0x7E0, Data: []byte{0x02, 0x10, 0x02}},
		{ID: 0x18DAF110, Data: []byte{0x01, 0x3E}},
		{ID: 0x123, Data: nil},
	}
	for _, want := range tests {
		raw, err := marshalFrame(want)
		if err != nil {
			t.Fatalf("marshalFrame(%+v): %v", want, err)
		}
		got, err := unmarshalFrame(raw[:])
		if err != nil {
			t.Fatalf("unmarshalFrame(%+v): %v", want, err)
		}
		if got.ID != want.ID || !bytes.Equal(got.Data, want.Data) {
			t.Fatalf("round trip got %+v, want %+v", got, want)
		}
	}
}

func TestMarshalFrameRejectsInvalidInput(t *testing.T) {
	tests := []can.Frame{
		{ID: canEFFMask + 1},
		{ID: 0x123, Data: make([]byte, maxDataLength+1)},
	}
	for _, frame := range tests {
		if _, err := marshalFrame(frame); err == nil {
			t.Fatalf("marshalFrame(%+v) unexpectedly succeeded", frame)
		}
	}
}

func TestUnmarshalFrameRejectsMalformedInput(t *testing.T) {
	if _, err := unmarshalFrame(make([]byte, classicFrameSize-1)); err == nil {
		t.Fatal("unmarshalFrame accepted a short frame")
	}

	raw := make([]byte, classicFrameSize)
	raw[4] = maxDataLength + 1
	if _, err := unmarshalFrame(raw); err == nil {
		t.Fatal("unmarshalFrame accepted an invalid data length")
	}
}

func TestUnmarshalFrameIgnoresUnsupportedKernelFrames(t *testing.T) {
	for _, flag := range []uint32{canRTRFlag, canERRFlag} {
		raw := make([]byte, classicFrameSize)
		binary.NativeEndian.PutUint32(raw[0:4], flag|0x123)
		if _, err := unmarshalFrame(raw); !errors.Is(err, errIgnoredFrame) {
			t.Fatalf("flag %#x: got %v, want errIgnoredFrame", flag, err)
		}
	}
}
