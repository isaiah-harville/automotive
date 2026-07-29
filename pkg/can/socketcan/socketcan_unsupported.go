//go:build !linux

package socketcan

import (
	"fmt"

	"github.com/isaiah-harville/automotive/pkg/can"
)

// Open returns ErrUnsupported outside Linux, where SocketCAN is unavailable.
func Open(iface string) (can.Bus, error) {
	return nil, fmt.Errorf("%w (cannot open %q)", ErrUnsupported, iface)
}
