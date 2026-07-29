// Command udsflasher is a Numaflow mapper UDF that takes a FlashJob message
// and drives a full UDS flashing sequence (session control -> security
// access -> download/transfer/exit -> reset) against an ECU, emitting a
// FlashResult message.
//
// CAN_MODE controls what it talks to:
//   - "sim" (default): an in-process simulated ECU (pkg/can/simbus +
//     uds.FakeECU), so the pipeline is runnable with no hardware.
//   - "socketcan": a real SocketCAN interface named by CAN_IFACE. Not
//     implemented yet (see pkg/can/socketcan) -- flashing will fail until a
//     real transport is wired up for a given plant.
//
// A single physical CAN bus only supports one flashing conversation at a
// time, so Map calls are serialized with a mutex around the shared UDS
// client regardless of Numaflow's own concurrency.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/numaproj/numaflow-go/pkg/mapper"

	"github.com/isaiah-harville/automotive/components/go/flowtypes"
	"github.com/isaiah-harville/automotive/pkg/can"
	"github.com/isaiah-harville/automotive/pkg/can/simbus"
	"github.com/isaiah-harville/automotive/pkg/can/socketcan"
	"github.com/isaiah-harville/automotive/pkg/isotp"
	"github.com/isaiah-harville/automotive/pkg/uds"
)

// keepAliveInterval is how often TesterPresent is sent during
// TransferFirmware to hold the diagnostic session open. Well under the
// typical 5s S3 timeout, with margin for jitter.
const keepAliveInterval = 2 * time.Second

type flasher struct {
	mu     sync.Mutex
	client *uds.Client
}

func newFlasher() (*flasher, error) {
	mode := os.Getenv("CAN_MODE")
	if mode == "" {
		mode = "sim"
	}
	testerID := envHexUint32("TESTER_CAN_ID", 0x7E0)
	ecuID := envHexUint32("ECU_CAN_ID", 0x7E8)

	var bus can.Bus
	switch mode {
	case "sim":
		testerBus, ecuBus := simbus.NewPair()
		bus = testerBus
		ecuConn := isotp.NewConn(ecuBus, ecuID, testerID)
		fake := uds.NewFakeECU(ecuConn)
		// Demo mode accepts any SecurityAccess key; real ECUs never do.
		fake.ExpectedKey = nil
	case "socketcan":
		iface := os.Getenv("CAN_IFACE")
		if iface == "" {
			iface = "can0"
		}
		var err error
		bus, err = socketcan.Open(iface)
		if err != nil {
			return nil, fmt.Errorf("udsflasher: opening %s: %w", iface, err)
		}
	default:
		return nil, fmt.Errorf("udsflasher: unknown CAN_MODE %q", mode)
	}

	conn := isotp.NewConn(bus, testerID, ecuID)
	return &flasher{client: uds.NewClient(conn)}, nil
}

func envHexUint32(key string, def uint32) uint32 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseUint(v, 16, 32)
	if err != nil {
		log.Printf("udsflasher: invalid hex value %q for %s, using default: %v", v, key, err)
		return def
	}
	return uint32(n)
}

func (fl *flasher) Map(ctx context.Context, keys []string, datum mapper.Datum) mapper.Messages {
	var job flowtypes.FlashJob
	if err := json.Unmarshal(datum.Value(), &job); err != nil {
		return mapper.MessagesBuilder().Append(mapper.MessageToDrop())
	}

	result := fl.flash(job)
	out, err := json.Marshal(result)
	if err != nil {
		log.Printf("udsflasher: marshaling result for job %s: %v", job.JobID, err)
		return mapper.MessagesBuilder().Append(mapper.MessageToDrop())
	}
	return mapper.MessagesBuilder().Append(mapper.NewMessage(out))
}

func (fl *flasher) flash(job flowtypes.FlashJob) flowtypes.FlashResult {
	start := time.Now()
	result := flowtypes.FlashResult{JobID: job.JobID, ECUID: job.ECUID}

	if err := fl.doFlash(job); err != nil {
		result.Status = "error"
		result.Error = err.Error()
	} else {
		result.Status = "ok"
	}
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

func (fl *flasher) doFlash(job flowtypes.FlashJob) error {
	memAddr, err := strconv.ParseUint(job.MemAddrHex, 16, 32)
	if err != nil {
		return fmt.Errorf("parsing mem_addr_hex: %w", err)
	}
	firmware, err := hex.DecodeString(job.FirmwareHex)
	if err != nil {
		return fmt.Errorf("parsing firmware_hex: %w", err)
	}
	securityLevel := job.SecurityLevel
	if securityLevel == 0 {
		securityLevel = 0x01
	}
	keyMask := byte(0xFF)
	if job.KeyMaskHex != "" {
		m, err := strconv.ParseUint(job.KeyMaskHex, 16, 8)
		if err != nil {
			return fmt.Errorf("parsing key_mask_hex: %w", err)
		}
		keyMask = byte(m)
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	c := fl.client
	if err := c.DiagnosticSessionControl(uds.SessionProgramming); err != nil {
		return fmt.Errorf("diagnostic session control: %w", err)
	}
	if err := c.SecurityAccess(securityLevel, uds.XORKeyGenerator{Mask: keyMask}); err != nil {
		return fmt.Errorf("security access: %w", err)
	}

	// A multi-block transfer of a large image can leave gaps between
	// TransferData requests longer than the ECU's S3 (session timeout)
	// window; keep the programming session alive with periodic
	// TesterPresent requests for the duration of the transfer.
	stopKeepAlive := c.StartKeepAlive(context.Background(), keepAliveInterval)
	transferErr := c.TransferFirmware(uint32(memAddr), firmware, 0x00)
	stopKeepAlive()
	if transferErr != nil {
		return fmt.Errorf("transfer firmware: %w", transferErr)
	}
	if err := c.ECUReset(uds.ResetHard); err != nil {
		return fmt.Errorf("ecu reset: %w", err)
	}
	return nil
}

func main() {
	fl, err := newFlasher()
	if err != nil {
		log.Fatalf("udsflasher: %v", err)
	}
	if err := mapper.NewServer(fl).Start(context.Background()); err != nil {
		log.Fatalf("udsflasher: server error: %v", err)
	}
}
