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
	"errors"
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
const (
	keepAliveInterval     = 2 * time.Second
	defaultMaxAttempts    = 3
	defaultRetryBackoff   = time.Second
	maxConfiguredAttempts = 10

	flashSuccessTag    = "flash-success"
	flashDeadLetterTag = "flash-dead-letter"
)

type preparedFlashJob struct {
	memAddr       uint32
	firmware      []byte
	securityLevel byte
	keyMask       byte
}

type retryPolicy struct {
	maxAttempts    int
	initialBackoff time.Duration
}

type flasher struct {
	mu      sync.Mutex
	client  *uds.Client
	policy  retryPolicy
	attempt func(context.Context, preparedFlashJob) error
	sleep   func(context.Context, time.Duration) error
}

func newFlasher() (*flasher, error) {
	mode := os.Getenv("CAN_MODE")
	if mode == "" {
		mode = "sim"
	}
	testerID := envHexUint32("TESTER_CAN_ID", 0x7E0)
	ecuID := envHexUint32("ECU_CAN_ID", 0x7E8)
	maxAttempts, err := envBoundedPositiveInt("FLASH_MAX_ATTEMPTS", defaultMaxAttempts, maxConfiguredAttempts)
	if err != nil {
		return nil, err
	}
	retryBackoff, err := envNonNegativeDuration("FLASH_RETRY_BACKOFF", defaultRetryBackoff)
	if err != nil {
		return nil, err
	}

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
	fl := &flasher{
		client: uds.NewClient(conn),
		policy: retryPolicy{
			maxAttempts:    maxAttempts,
			initialBackoff: retryBackoff,
		},
		sleep: sleepContext,
	}
	fl.attempt = fl.attemptFlash
	return fl, nil
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

func envBoundedPositiveInt(key string, def, max int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return def, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > max {
		return 0, fmt.Errorf("udsflasher: %s must be an integer from 1 to %d, got %q", key, max, value)
	}
	return parsed, nil
}

func envNonNegativeDuration(key string, def time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return def, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("udsflasher: %s must be a non-negative duration, got %q", key, value)
	}
	return parsed, nil
}

func (fl *flasher) Map(ctx context.Context, keys []string, datum mapper.Datum) mapper.Messages {
	var job flowtypes.FlashJob
	if err := json.Unmarshal(datum.Value(), &job); err != nil {
		return mapper.MessagesBuilder().Append(mapper.MessageToDrop())
	}

	result := fl.flash(ctx, job)
	var (
		payload any = result
		tag         = flashSuccessTag
	)
	if result.Status == "error" {
		payload = flowtypes.FlashDeadLetter{
			Job:      job,
			Result:   result,
			FailedAt: time.Now().UTC(),
		}
		tag = flashDeadLetterTag
	}
	out, err := json.Marshal(payload)
	if err != nil {
		log.Printf("udsflasher: marshaling result for job %s: %v", job.JobID, err)
		return mapper.MessagesBuilder().Append(mapper.MessageToDrop())
	}
	return mapper.MessagesBuilder().Append(mapper.NewMessage(out).WithTags([]string{tag}))
}

func (fl *flasher) flash(ctx context.Context, job flowtypes.FlashJob) flowtypes.FlashResult {
	start := time.Now()
	result := flowtypes.FlashResult{JobID: job.JobID, ECUID: job.ECUID}

	prepared, err := prepareFlashJob(job)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()
	for attempt := 1; attempt <= fl.policy.maxAttempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			result.Status = "error"
			result.Error = fmt.Sprintf("flash canceled before attempt %d: %v", attempt, ctxErr)
			break
		}
		result.Attempts = attempt
		err = fl.attempt(ctx, prepared)
		if err == nil {
			result.Status = "ok"
			break
		}
		if !isRetryableFlashError(err) || attempt == fl.policy.maxAttempts {
			result.Status = "error"
			result.Error = err.Error()
			break
		}

		backoff := fl.policy.backoff(attempt)
		log.Printf(
			"udsflasher: job %s attempt %d/%d failed: %v; retrying in %s",
			job.JobID, attempt, fl.policy.maxAttempts, err, backoff,
		)
		if sleepErr := fl.sleep(ctx, backoff); sleepErr != nil {
			result.Status = "error"
			result.Error = fmt.Sprintf("retry backoff interrupted after attempt %d: %v", attempt, sleepErr)
			break
		}
	}
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

func prepareFlashJob(job flowtypes.FlashJob) (preparedFlashJob, error) {
	memAddr, err := strconv.ParseUint(job.MemAddrHex, 16, 32)
	if err != nil {
		return preparedFlashJob{}, fmt.Errorf("parsing mem_addr_hex: %w", err)
	}
	firmware, err := hex.DecodeString(job.FirmwareHex)
	if err != nil {
		return preparedFlashJob{}, fmt.Errorf("parsing firmware_hex: %w", err)
	}
	if len(firmware) == 0 {
		return preparedFlashJob{}, errors.New("firmware image is empty")
	}
	securityLevel := job.SecurityLevel
	if securityLevel == 0 {
		securityLevel = 0x01
	}
	if securityLevel%2 == 0 {
		return preparedFlashJob{}, fmt.Errorf(
			"security_level 0x%02X must be an odd request-seed sub-function",
			securityLevel,
		)
	}
	keyMask := byte(0xFF)
	if job.KeyMaskHex != "" {
		m, err := strconv.ParseUint(job.KeyMaskHex, 16, 8)
		if err != nil {
			return preparedFlashJob{}, fmt.Errorf("parsing key_mask_hex: %w", err)
		}
		keyMask = byte(m)
	}
	return preparedFlashJob{
		memAddr:       uint32(memAddr),
		firmware:      firmware,
		securityLevel: securityLevel,
		keyMask:       keyMask,
	}, nil
}

func (fl *flasher) attemptFlash(ctx context.Context, job preparedFlashJob) error {
	c := fl.client
	if err := c.DiagnosticSessionControl(uds.SessionProgramming); err != nil {
		return fmt.Errorf("diagnostic session control: %w", err)
	}
	if err := c.SecurityAccess(job.securityLevel, uds.XORKeyGenerator{Mask: job.keyMask}); err != nil {
		return fmt.Errorf("security access: %w", err)
	}

	// A multi-block transfer of a large image can leave gaps between
	// TransferData requests longer than the ECU's S3 (session timeout)
	// window; keep the programming session alive with periodic
	// TesterPresent requests for the duration of the transfer.
	stopKeepAlive := c.StartKeepAlive(ctx, keepAliveInterval)
	transferErr := c.TransferFirmware(job.memAddr, job.firmware, 0x00)
	stopKeepAlive()
	if transferErr != nil {
		return fmt.Errorf("transfer firmware: %w", transferErr)
	}
	if err := c.ECUReset(uds.ResetHard); err != nil {
		return fmt.Errorf("ecu reset: %w", err)
	}
	return nil
}

func isRetryableFlashError(err error) bool {
	var negativeResponse *uds.NegativeResponseError
	return !errors.As(err, &negativeResponse) && !errors.Is(err, can.ErrClosed)
}

func (policy retryPolicy) backoff(failedAttempt int) time.Duration {
	backoff := policy.initialBackoff
	for i := 1; i < failedAttempt; i++ {
		if backoff > time.Duration(1<<63-1)/2 {
			return time.Duration(1<<63 - 1)
		}
		backoff *= 2
	}
	return backoff
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if duration == 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
