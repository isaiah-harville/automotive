package uds_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/isaiah-harville/automotive-flow/pkg/can/simbus"
	"github.com/isaiah-harville/automotive-flow/pkg/isotp"
	"github.com/isaiah-harville/automotive-flow/pkg/uds"
)

const (
	testerID = 0x7E0
	ecuID    = 0x7E8
)

func newTestClient(t *testing.T) (*uds.Client, *uds.FakeECU) {
	t.Helper()
	a, b := simbus.NewPair()
	testerConn := isotp.NewConn(a, testerID, ecuID)
	ecuConn := isotp.NewConn(b, ecuID, testerID)

	ecu := uds.NewFakeECU(ecuConn)
	t.Cleanup(ecu.Stop)

	return uds.NewClient(testerConn), ecu
}

func TestDiagnosticSessionControl(t *testing.T) {
	c, _ := newTestClient(t)
	if err := c.DiagnosticSessionControl(uds.SessionProgramming); err != nil {
		t.Fatalf("DiagnosticSessionControl: %v", err)
	}
}

func TestSecurityAccessSuccess(t *testing.T) {
	c, ecu := newTestClient(t)
	kg := uds.XORKeyGenerator{Mask: 0xFF}
	seed := ecu.Seed
	expected := make([]byte, len(seed))
	for i, b := range seed {
		expected[i] = b ^ 0xFF
	}
	ecu.ExpectedKey = expected

	if err := c.SecurityAccess(0x01, kg); err != nil {
		t.Fatalf("SecurityAccess: %v", err)
	}
}

func TestSecurityAccessInvalidKey(t *testing.T) {
	c, ecu := newTestClient(t)
	ecu.ExpectedKey = []byte{0xDE, 0xAD}

	err := c.SecurityAccess(0x01, uds.XORKeyGenerator{Mask: 0x00})
	var nre *uds.NegativeResponseError
	if !errors.As(err, &nre) {
		t.Fatalf("got err %v (%T), want *NegativeResponseError", err, err)
	}
	if nre.NRC != uds.NRCInvalidKey {
		t.Fatalf("got NRC 0x%02X, want 0x%02X", nre.NRC, uds.NRCInvalidKey)
	}
}

func TestTransferFirmware(t *testing.T) {
	c, _ := newTestClient(t)
	image := make([]byte, 500)
	for i := range image {
		image[i] = byte(i)
	}
	if err := c.TransferFirmware(0x00100000, image, 0x00); err != nil {
		t.Fatalf("TransferFirmware: %v", err)
	}
}

func TestStartKeepAliveSendsTesterPresentUntilStopped(t *testing.T) {
	c, ecu := newTestClient(t)

	stop := c.StartKeepAlive(context.Background(), 10*time.Millisecond)
	time.Sleep(55 * time.Millisecond)
	stop()

	count := ecu.TesterPresentCount.Load()
	if count < 3 {
		t.Fatalf("got %d TesterPresent requests in 55ms at a 10ms interval, want at least 3", count)
	}

	// No further ticks should arrive after stop returns.
	time.Sleep(30 * time.Millisecond)
	if got := ecu.TesterPresentCount.Load(); got != count {
		t.Fatalf("TesterPresentCount changed after stop: %d -> %d", count, got)
	}
}

func TestStartKeepAliveInterleavesWithConcurrentRequests(t *testing.T) {
	c, ecu := newTestClient(t)
	// Small blocks over many round trips give a fast ticker real wall-clock
	// gaps to land in, since each block is its own channel round trip
	// through simbus.
	ecu.MaxBlockLength = 8

	stop := c.StartKeepAlive(context.Background(), 200*time.Microsecond)
	defer stop()

	image := make([]byte, 50000)
	for i := range image {
		image[i] = byte(i)
	}
	if err := c.TransferFirmware(0x00100000, image, 0x00); err != nil {
		t.Fatalf("TransferFirmware with concurrent keep-alive: %v", err)
	}
	if ecu.TesterPresentCount.Load() == 0 {
		t.Fatal("expected at least one TesterPresent to interleave with the transfer")
	}
}

func TestStartRoutine(t *testing.T) {
	c, ecu := newTestClient(t)
	const eraseRoutineID = 0xFF00
	ecu.RoutineResults = map[uint16][]byte{eraseRoutineID: {0x00}} // 0x00 = success

	result, err := c.StartRoutine(eraseRoutineID, nil)
	if err != nil {
		t.Fatalf("StartRoutine: %v", err)
	}
	if result.RoutineID != eraseRoutineID {
		t.Fatalf("got routine ID 0x%04X, want 0x%04X", result.RoutineID, eraseRoutineID)
	}
	if len(result.Data) != 1 || result.Data[0] != 0x00 {
		t.Fatalf("got result data %x, want [0x00]", result.Data)
	}
}

func TestRequestRoutineResultsUnknownRoutineReturnsEmptyData(t *testing.T) {
	c, _ := newTestClient(t)

	result, err := c.RequestRoutineResults(0x0203)
	if err != nil {
		t.Fatalf("RequestRoutineResults: %v", err)
	}
	if result.RoutineID != 0x0203 {
		t.Fatalf("got routine ID 0x%04X, want 0x0203", result.RoutineID)
	}
	if len(result.Data) != 0 {
		t.Fatalf("got result data %x, want none", result.Data)
	}
}

func TestStopRoutine(t *testing.T) {
	c, _ := newTestClient(t)

	if _, err := c.StopRoutine(0x1234, []byte{0xAB}); err != nil {
		t.Fatalf("StopRoutine: %v", err)
	}
}

func TestReadDataByIdentifier(t *testing.T) {
	c, ecu := newTestClient(t)
	const vinDataID = 0xF190
	ecu.DataRecords = map[uint16][]byte{vinDataID: []byte("1FA6P8CF0")}

	got, err := c.ReadDataByIdentifier(vinDataID)
	if err != nil {
		t.Fatalf("ReadDataByIdentifier: %v", err)
	}
	if string(got) != "1FA6P8CF0" {
		t.Fatalf("got %q, want %q", got, "1FA6P8CF0")
	}
}

func TestReadDataByIdentifierUnknownReturnsEmpty(t *testing.T) {
	c, _ := newTestClient(t)

	got, err := c.ReadDataByIdentifier(0x0102)
	if err != nil {
		t.Fatalf("ReadDataByIdentifier: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %x, want none", got)
	}
}

func TestWriteThenReadDataByIdentifier(t *testing.T) {
	c, _ := newTestClient(t)
	const calDataID = 0xF1A0

	if err := c.WriteDataByIdentifier(calDataID, []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("WriteDataByIdentifier: %v", err)
	}
	got, err := c.ReadDataByIdentifier(calDataID)
	if err != nil {
		t.Fatalf("ReadDataByIdentifier: %v", err)
	}
	if string(got) != "\x01\x02\x03" {
		t.Fatalf("got %x, want 010203", got)
	}
}

func TestReadAndClearDTCs(t *testing.T) {
	c, ecu := newTestClient(t)
	ecu.DTCs = []byte{0x01, 0x23, 0x45, 0x08}

	resp, err := c.ReadDTCByStatusMask(0xFF)
	if err != nil {
		t.Fatalf("ReadDTCByStatusMask: %v", err)
	}
	if len(resp) < 1 || resp[0] != 0x02 {
		t.Fatalf("unexpected response: %x", resp)
	}

	if err := c.ClearDiagnosticInformation(0xFFFFFF); err != nil {
		t.Fatalf("ClearDiagnosticInformation: %v", err)
	}
}

func TestFullFlashSequence(t *testing.T) {
	c, ecu := newTestClient(t)
	kg := uds.XORKeyGenerator{Mask: 0xAA}
	expected := make([]byte, len(ecu.Seed))
	for i, b := range ecu.Seed {
		expected[i] = b ^ 0xAA
	}
	ecu.ExpectedKey = expected

	if err := c.DiagnosticSessionControl(uds.SessionProgramming); err != nil {
		t.Fatalf("session control: %v", err)
	}
	if err := c.SecurityAccess(0x01, kg); err != nil {
		t.Fatalf("security access: %v", err)
	}
	image := make([]byte, 1024)
	if err := c.TransferFirmware(0x00100000, image, 0x00); err != nil {
		t.Fatalf("transfer firmware: %v", err)
	}
	if err := c.ECUReset(uds.ResetHard); err != nil {
		t.Fatalf("ecu reset: %v", err)
	}
}
