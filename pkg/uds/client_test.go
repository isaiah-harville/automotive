package uds_test

import (
	"errors"
	"testing"

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
