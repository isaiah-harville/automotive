package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/numaproj/numaflow-go/pkg/mapper"

	"github.com/isaiah-harville/automotive/components/go/flowtypes"
)

func TestNewFlasherUnknownModeErrors(t *testing.T) {
	t.Setenv("CAN_MODE", "carrier-pigeon")

	if _, err := newFlasher(); err == nil {
		t.Fatal("expected an error for an unknown CAN_MODE, got nil")
	}
}

func TestNewFlasherSocketCANModeErrorsForUnavailableInterface(t *testing.T) {
	t.Setenv("CAN_MODE", "socketcan")
	t.Setenv("CAN_IFACE", "definitely-not-a-can-interface")

	if _, err := newFlasher(); err == nil {
		t.Fatal("expected an error for an unavailable SocketCAN interface")
	}
}

func TestNewFlasherDefaultsToSimMode(t *testing.T) {
	fl, err := newFlasher()
	if err != nil {
		t.Fatalf("newFlasher with no CAN_MODE set: %v", err)
	}
	if fl == nil || fl.client == nil {
		t.Fatal("expected a flasher with a client wired up in sim mode")
	}
}

func TestEnvHexUint32InvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("TESTER_CAN_ID", "not-hex")

	got := envHexUint32("TESTER_CAN_ID", 0x7E0)
	if got != 0x7E0 {
		t.Fatalf("got 0x%X, want the default 0x7E0 for an invalid hex value", got)
	}
}

func TestEnvHexUint32UsesDefaultWhenUnset(t *testing.T) {
	if got := envHexUint32("NOT_A_REAL_ENV_VAR_XYZ", 0x7E8); got != 0x7E8 {
		t.Fatalf("got 0x%X, want default 0x7E8", got)
	}
}

func TestMapDropsMessageOnMalformedJSON(t *testing.T) {
	fl, err := newFlasher()
	if err != nil {
		t.Fatalf("newFlasher: %v", err)
	}

	datum := mapper.NewHandlerDatum([]byte("not json"), time.Now(), time.Now(), nil, nil, nil)
	got := fl.Map(context.Background(), nil, datum)

	if len(got) != 1 {
		t.Fatalf("got %d messages, want exactly 1 (the dropped message)", len(got))
	}
	tags := got[0].Tags()
	if len(tags) != 1 || tags[0] != mapper.DROP {
		t.Fatalf("got tags %v, want [%q]", tags, mapper.DROP)
	}
}

func TestDoFlashRejectsBadMemAddrHex(t *testing.T) {
	fl, err := newFlasher()
	if err != nil {
		t.Fatalf("newFlasher: %v", err)
	}
	job := flowtypes.FlashJob{JobID: "1", ECUID: "ecu", MemAddrHex: "zz", FirmwareHex: "00"}

	result := fl.flash(job)
	if result.Status != "error" {
		t.Fatalf("got status %q, want error for a malformed mem_addr_hex", result.Status)
	}
}

func TestDoFlashRejectsBadFirmwareHex(t *testing.T) {
	fl, err := newFlasher()
	if err != nil {
		t.Fatalf("newFlasher: %v", err)
	}
	job := flowtypes.FlashJob{JobID: "1", ECUID: "ecu", MemAddrHex: "100000", FirmwareHex: "not-hex"}

	result := fl.flash(job)
	if result.Status != "error" {
		t.Fatalf("got status %q, want error for malformed firmware_hex", result.Status)
	}
}

func TestDoFlashRejectsBadKeyMaskHex(t *testing.T) {
	fl, err := newFlasher()
	if err != nil {
		t.Fatalf("newFlasher: %v", err)
	}
	job := flowtypes.FlashJob{
		JobID: "1", ECUID: "ecu", MemAddrHex: "100000", FirmwareHex: "00",
		KeyMaskHex: "not-hex",
	}

	result := fl.flash(job)
	if result.Status != "error" {
		t.Fatalf("got status %q, want error for malformed key_mask_hex", result.Status)
	}
}

func TestFlashResultMarshalsToValidFlashResultJSON(t *testing.T) {
	fl, err := newFlasher()
	if err != nil {
		t.Fatalf("newFlasher: %v", err)
	}
	job := flowtypes.FlashJob{JobID: "abc", ECUID: "ecu-1", MemAddrHex: "100000", FirmwareHex: "deadbeef"}

	datum := mapper.NewHandlerDatum(mustJSON(t, job), time.Now(), time.Now(), nil, nil, nil)
	got := fl.Map(context.Background(), nil, datum)
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}

	var result flowtypes.FlashResult
	if err := json.Unmarshal(got[0].Value(), &result); err != nil {
		t.Fatalf("unmarshaling FlashResult: %v", err)
	}
	if result.JobID != "abc" || result.Status != "ok" {
		t.Fatalf("got %+v, want a successful result for job abc", result)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling: %v", err)
	}
	return b
}
