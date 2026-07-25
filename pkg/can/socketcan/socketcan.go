// Package socketcan will provide a can.Bus backed by a real Linux SocketCAN
// interface (e.g. can0) for use once a plant has physical CAN hardware wired
// up. Not implemented yet — components should be built against can.Bus and
// can/simbus in the meantime so swapping this in later requires no changes
// above the transport layer.
package socketcan

import (
	"errors"

	"github.com/isaiah-harville/automotive-flow/pkg/can"
)

// Open will dial a SocketCAN interface (e.g. "can0") once hardware is
// available. TODO: implement using golang.org/x/sys/unix AF_CAN sockets, or
// swap in a vendor SDK for contracted flashing tools.
func Open(iface string) (can.Bus, error) {
	return nil, errors.New("socketcan: not implemented, no hardware target configured yet")
}
