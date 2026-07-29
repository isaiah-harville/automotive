package socketcan_test

import (
	"testing"

	"github.com/isaiah-harville/automotive/pkg/can/socketcan"
)

func TestOpenReturnsNotImplementedError(t *testing.T) {
	bus, err := socketcan.Open("can0")
	if bus != nil {
		t.Fatalf("got non-nil bus %v, want nil until socketcan is implemented", bus)
	}
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}
