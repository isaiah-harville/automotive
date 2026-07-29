package socketcan

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/isaiah-harville/automotive/pkg/can"
)

const (
	classicFrameSize = 16
	maxDataLength    = 8

	canSFFMask = 0x000007FF
	canEFFMask = 0x1FFFFFFF
	canERRFlag = 0x20000000
	canRTRFlag = 0x40000000
	canEFFFlag = 0x80000000
)

var errIgnoredFrame = errors.New("socketcan: remote or error frame ignored")

func marshalFrame(frame can.Frame) ([classicFrameSize]byte, error) {
	var raw [classicFrameSize]byte
	if frame.ID > canEFFMask {
		return raw, fmt.Errorf("socketcan: CAN ID %#x exceeds 29 bits", frame.ID)
	}
	if len(frame.Data) > maxDataLength {
		return raw, fmt.Errorf("socketcan: data length %d exceeds classic CAN maximum of %d", len(frame.Data), maxDataLength)
	}

	id := frame.ID
	if id > canSFFMask {
		id |= canEFFFlag
	}
	binary.NativeEndian.PutUint32(raw[0:4], id)
	raw[4] = byte(len(frame.Data))
	copy(raw[8:], frame.Data)
	return raw, nil
}

func unmarshalFrame(raw []byte) (can.Frame, error) {
	if len(raw) != classicFrameSize {
		return can.Frame{}, fmt.Errorf("socketcan: received %d-byte frame, want %d", len(raw), classicFrameSize)
	}

	rawID := binary.NativeEndian.Uint32(raw[0:4])
	if rawID&(canERRFlag|canRTRFlag) != 0 {
		return can.Frame{}, errIgnoredFrame
	}

	dataLength := int(raw[4])
	if dataLength > maxDataLength {
		return can.Frame{}, fmt.Errorf("socketcan: received invalid data length %d", dataLength)
	}

	idMask := uint32(canSFFMask)
	if rawID&canEFFFlag != 0 {
		idMask = canEFFMask
	}
	data := make([]byte, dataLength)
	copy(data, raw[8:8+dataLength])
	return can.Frame{ID: rawID & idMask, Data: data}, nil
}
