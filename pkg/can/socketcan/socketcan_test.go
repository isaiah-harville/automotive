package socketcan_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/isaiah-harville/automotive/pkg/can/socketcan"
)

func TestOpenUnavailableInterfaceErrors(t *testing.T) {
	bus, err := socketcan.Open("definitely-not-a-can-interface")
	if bus != nil {
		t.Fatalf("got non-nil bus %v for unavailable interface", bus)
	}
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if runtime.GOOS != "linux" && !errors.Is(err, socketcan.ErrUnsupported) {
		t.Fatalf("got %v, want ErrUnsupported on %s", err, runtime.GOOS)
	}
}
