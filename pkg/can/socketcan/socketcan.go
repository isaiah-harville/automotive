// Package socketcan provides a can.Bus backed by a Linux SocketCAN raw
// socket. It supports classic CAN data frames; CAN-FD and remote-transmission
// request frames are outside the can.Bus contract.
package socketcan

import "errors"

// ErrUnsupported is returned by Open on non-Linux hosts.
var ErrUnsupported = errors.New("socketcan: only supported on Linux")
