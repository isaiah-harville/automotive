package uds

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// Diagnostic session types (ISO 14229-1 Table 24).
const (
	SessionDefault      = 0x01
	SessionProgramming  = 0x02
	SessionExtendedDiag = 0x03
	SessionSafetySystem = 0x04
)

// ECU reset types (ISO 14229-1 Table 102).
const (
	ResetHard         = 0x01
	ResetKeyOffOn     = 0x02
	ResetSoft         = 0x03
	ResetEnableRapid  = 0x04
	ResetDisableRapid = 0x05
)

const (
	sidDiagnosticSessionControl = 0x10
	sidECUReset                 = 0x11
	sidSecurityAccess           = 0x27
	sidRoutineControl           = 0x31
	sidRequestDownload          = 0x34
	sidTransferData             = 0x36
	sidRequestTransferExit      = 0x37
	sidTesterPresent            = 0x3E
	sidReadDTCInformation       = 0x19
	sidClearDiagnosticInfo      = 0x14
	sidReadDataByIdentifier     = 0x22
	sidWriteDataByIdentifier    = 0x2E
)

// RoutineControl sub-functions (ISO 14229-1 Table 320).
const (
	RoutineControlStart          = 0x01
	RoutineControlStop           = 0x02
	RoutineControlRequestResults = 0x03
)

// DiagnosticSessionControl requests a diagnostic session (SessionDefault,
// SessionProgramming, etc).
func (c *Client) DiagnosticSessionControl(session byte) error {
	_, err := c.Request(sidDiagnosticSessionControl, []byte{session})
	return err
}

// ECUReset requests an ECU reset of the given type.
func (c *Client) ECUReset(resetType byte) error {
	_, err := c.Request(sidECUReset, []byte{resetType})
	return err
}

// TesterPresent should be sent periodically (typically every 2s) to keep a
// non-default diagnostic session alive.
func (c *Client) TesterPresent() error {
	_, err := c.Request(sidTesterPresent, []byte{0x00})
	return err
}

// StartKeepAlive sends TesterPresent every interval until ctx is canceled,
// to hold a non-default diagnostic session open across a long-running
// operation (e.g. TransferFirmware) that may otherwise leave gaps longer
// than the ECU's S3 timeout between requests. It runs in its own goroutine
// and returns a stop function that blocks until that goroutine has exited;
// callers should defer stop() around the operation being kept alive.
//
// It is safe to call concurrently with other Client methods: Request
// serializes access to the underlying connection, so a keep-alive tick
// simply queues behind whatever request is already in flight.
func (c *Client) StartKeepAlive(ctx context.Context, interval time.Duration) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = c.TesterPresent()
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

// SecurityAccess performs a seed/key exchange to unlock the given security
// level (an odd "request seed" sub-function; the "send key" sub-function is
// level+1). If the ECU is already unlocked it returns a zero-length seed and
// no key exchange is required.
func (c *Client) SecurityAccess(level byte, kg KeyGenerator) error {
	if level%2 == 0 {
		return fmt.Errorf("uds: security access level 0x%02X must be an odd request-seed sub-function", level)
	}

	seedResp, err := c.Request(sidSecurityAccess, []byte{level})
	if err != nil {
		return fmt.Errorf("uds: requesting security seed: %w", err)
	}
	if len(seedResp) < 1 {
		return ErrShortResponse
	}
	seed := seedResp[1:]

	allZero := true
	for _, b := range seed {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return nil // already unlocked
	}

	key, err := kg.ComputeKey(seed)
	if err != nil {
		return fmt.Errorf("uds: computing security key: %w", err)
	}

	payload := append([]byte{level + 1}, key...)
	if _, err := c.Request(sidSecurityAccess, payload); err != nil {
		return fmt.Errorf("uds: sending security key: %w", err)
	}
	return nil
}

// DownloadHandle describes the block-transfer parameters the ECU returned
// from RequestDownload, needed to drive TransferData correctly.
type DownloadHandle struct {
	MaxBlockLength int
}

// RequestDownload requests permission to download memSize bytes starting at
// memAddr, using 32-bit address/size encoding (addrLenFormatId 0x44).
func (c *Client) RequestDownload(memAddr, memSize uint32, dataFormatID byte) (DownloadHandle, error) {
	payload := make([]byte, 0, 10)
	payload = append(payload, dataFormatID)
	payload = append(payload, 0x44) // addressAndLengthFormatIdentifier: 4 bytes addr, 4 bytes size
	addrBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(addrBuf, memAddr)
	payload = append(payload, addrBuf...)
	sizeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBuf, memSize)
	payload = append(payload, sizeBuf...)

	resp, err := c.Request(sidRequestDownload, payload)
	if err != nil {
		return DownloadHandle{}, err
	}
	if len(resp) < 2 {
		return DownloadHandle{}, ErrShortResponse
	}
	lenFormatSize := int(resp[0] >> 4)
	if lenFormatSize == 0 || 1+lenFormatSize > len(resp) {
		return DownloadHandle{}, ErrShortResponse
	}
	var maxBlockLen uint64
	for _, b := range resp[1 : 1+lenFormatSize] {
		maxBlockLen = maxBlockLen<<8 | uint64(b)
	}
	return DownloadHandle{MaxBlockLength: int(maxBlockLen)}, nil
}

// TransferData sends one block of firmware data with the given block
// sequence counter (starts at 1 and wraps 0x00-0xFF per ISO 14229-1).
func (c *Client) TransferData(blockSequenceCounter byte, data []byte) error {
	payload := append([]byte{blockSequenceCounter}, data...)
	_, err := c.Request(sidTransferData, payload)
	return err
}

// RequestTransferExit signals that the data transfer is complete.
func (c *Client) RequestTransferExit() error {
	_, err := c.Request(sidRequestTransferExit, nil)
	return err
}

// TransferFirmware drives the full RequestDownload -> TransferData* ->
// RequestTransferExit sequence for a complete firmware image, chunking it
// per the ECU-reported max block length. Assumes the caller has already
// completed DiagnosticSessionControl and SecurityAccess.
func (c *Client) TransferFirmware(memAddr uint32, image []byte, dataFormatID byte) error {
	if len(image) == 0 {
		return errors.New("uds: empty firmware image")
	}
	handle, err := c.RequestDownload(memAddr, uint32(len(image)), dataFormatID)
	if err != nil {
		return fmt.Errorf("uds: request download: %w", err)
	}
	chunkSize := handle.MaxBlockLength - 2 // minus the TransferData SID + block counter byte
	if chunkSize <= 0 {
		chunkSize = 4094
	}

	seq := byte(1)
	for offset := 0; offset < len(image); offset += chunkSize {
		end := offset + chunkSize
		if end > len(image) {
			end = len(image)
		}
		if err := c.TransferData(seq, image[offset:end]); err != nil {
			return fmt.Errorf("uds: transfer data block %d: %w", seq, err)
		}
		seq++
	}

	if err := c.RequestTransferExit(); err != nil {
		return fmt.Errorf("uds: request transfer exit: %w", err)
	}
	return nil
}

// RoutineResult is the ECU's response to a RoutineControl request.
type RoutineResult struct {
	RoutineID uint16
	// Data is the routineStatusRecord, if any - format is
	// routine-specific (e.g. a checksum pass/fail byte).
	Data []byte
}

func (c *Client) routineControl(subFunction byte, routineID uint16, optionRecord []byte) (RoutineResult, error) {
	payload := make([]byte, 0, 3+len(optionRecord))
	payload = append(payload, subFunction, byte(routineID>>8), byte(routineID))
	payload = append(payload, optionRecord...)

	resp, err := c.Request(sidRoutineControl, payload)
	if err != nil {
		return RoutineResult{}, err
	}
	if len(resp) < 3 {
		return RoutineResult{}, ErrShortResponse
	}
	return RoutineResult{
		RoutineID: uint16(resp[1])<<8 | uint16(resp[2]),
		Data:      resp[3:],
	}, nil
}

// StartRoutine starts the routine identified by routineID (RoutineControl
// subfunction 0x01) - e.g. an erase-before-flash or post-flash checksum
// verification routine. optionRecord carries any routine-specific input
// parameters and may be nil.
func (c *Client) StartRoutine(routineID uint16, optionRecord []byte) (RoutineResult, error) {
	return c.routineControl(RoutineControlStart, routineID, optionRecord)
}

// StopRoutine stops a previously started routine.
func (c *Client) StopRoutine(routineID uint16, optionRecord []byte) (RoutineResult, error) {
	return c.routineControl(RoutineControlStop, routineID, optionRecord)
}

// RequestRoutineResults retrieves the current results of a routine (e.g.
// whether a checksum verification passed) without stopping it.
func (c *Client) RequestRoutineResults(routineID uint16) (RoutineResult, error) {
	return c.routineControl(RoutineControlRequestResults, routineID, nil)
}

// ReadDataByIdentifier reads the data record for a data identifier (e.g.
// VIN, ECU part number, calibration data) outside of a flash sequence.
func (c *Client) ReadDataByIdentifier(dataID uint16) ([]byte, error) {
	payload := []byte{byte(dataID >> 8), byte(dataID)}
	resp, err := c.Request(sidReadDataByIdentifier, payload)
	if err != nil {
		return nil, err
	}
	if len(resp) < 2 {
		return nil, ErrShortResponse
	}
	return resp[2:], nil
}

// WriteDataByIdentifier writes record as the data for a data identifier.
func (c *Client) WriteDataByIdentifier(dataID uint16, record []byte) error {
	payload := make([]byte, 0, 2+len(record))
	payload = append(payload, byte(dataID>>8), byte(dataID))
	payload = append(payload, record...)
	_, err := c.Request(sidWriteDataByIdentifier, payload)
	return err
}

// ReadDTCByStatusMask reads stored DTCs matching a status mask (subfunction
// 0x02), the most common ReadDTCInformation use case.
func (c *Client) ReadDTCByStatusMask(statusMask byte) ([]byte, error) {
	return c.Request(sidReadDTCInformation, []byte{0x02, statusMask})
}

// ClearDiagnosticInformation clears DTCs matching the given group (0xFFFFFF
// clears all groups).
func (c *Client) ClearDiagnosticInformation(group uint32) error {
	payload := []byte{byte(group >> 16), byte(group >> 8), byte(group)}
	_, err := c.Request(sidClearDiagnosticInfo, payload)
	return err
}
