package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/numaproj/numaflow-go/pkg/mapper"

	"github.com/isaiah-harville/automotive/components/go/flowtypes"
	"github.com/isaiah-harville/automotive/pkg/can"
	"github.com/isaiah-harville/automotive/pkg/uds"
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

	result := fl.flash(context.Background(), job)
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

	result := fl.flash(context.Background(), job)
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

	result := fl.flash(context.Background(), job)
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
	if tags := got[0].Tags(); !reflect.DeepEqual(tags, []string{flashSuccessTag}) {
		t.Fatalf("got tags %v, want [%q]", tags, flashSuccessTag)
	}
}

func TestFlashRetriesTransientFailuresWithExponentialBackoff(t *testing.T) {
	attempts := 0
	var backoffs []time.Duration
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		attempts++
		if attempts < 3 {
			return can.ErrTimeout
		}
		return nil
	})
	fl.policy = retryPolicy{maxAttempts: 3, initialBackoff: 10 * time.Millisecond}
	fl.sleep = func(_ context.Context, duration time.Duration) error {
		backoffs = append(backoffs, duration)
		return nil
	}

	result := fl.flash(context.Background(), validFlashJob())

	if result.Status != "ok" || result.Attempts != 3 {
		t.Fatalf("got %+v, want success on attempt 3", result)
	}
	if want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}; !reflect.DeepEqual(backoffs, want) {
		t.Fatalf("backoffs = %v, want %v", backoffs, want)
	}
}

func TestFlashStopsAfterRetryExhaustion(t *testing.T) {
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		return can.ErrTimeout
	})
	fl.policy = retryPolicy{maxAttempts: 2}

	result := fl.flash(context.Background(), validFlashJob())

	if result.Status != "error" || result.Attempts != 2 {
		t.Fatalf("got %+v, want error after 2 attempts", result)
	}
}

func TestFlashDoesNotRetryNegativeResponse(t *testing.T) {
	attempts := 0
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		attempts++
		return &uds.NegativeResponseError{SID: 0x27, NRC: uds.NRCInvalidKey}
	})
	fl.policy = retryPolicy{maxAttempts: 3}

	result := fl.flash(context.Background(), validFlashJob())

	if result.Status != "error" || result.Attempts != 1 || attempts != 1 {
		t.Fatalf("got result=%+v attempts=%d, want one non-retried failure", result, attempts)
	}
}

func TestFlashDoesNotRetryClosedBus(t *testing.T) {
	attempts := 0
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		attempts++
		return can.ErrClosed
	})
	fl.policy = retryPolicy{maxAttempts: 3}

	result := fl.flash(context.Background(), validFlashJob())

	if result.Status != "error" || result.Attempts != 1 || attempts != 1 {
		t.Fatalf("got result=%+v attempts=%d, want one non-retried failure", result, attempts)
	}
}

func TestFlashHonorsContextCancellationDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		cancel()
		return can.ErrTimeout
	})
	fl.policy = retryPolicy{maxAttempts: 3, initialBackoff: time.Hour}
	fl.sleep = sleepContext

	result := fl.flash(ctx, validFlashJob())

	if result.Status != "error" || result.Attempts != 1 {
		t.Fatalf("got %+v, want cancellation after first attempt", result)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("context error = %v, want context.Canceled", ctx.Err())
	}
}

func TestFlashSkipsAttemptWhenContextAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempted := false
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		attempted = true
		return nil
	})

	result := fl.flash(ctx, validFlashJob())

	if result.Status != "error" || result.Attempts != 0 || attempted {
		t.Fatalf("got result=%+v attempted=%v, want cancellation before any attempt", result, attempted)
	}
}

func TestMapRoutesExhaustedJobToDeadLetter(t *testing.T) {
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		return can.ErrTimeout
	})
	fl.policy = retryPolicy{maxAttempts: 2}
	job := validFlashJob()
	datum := mapper.NewHandlerDatum(mustJSON(t, job), time.Now(), time.Now(), nil, nil, nil)

	got := fl.Map(context.Background(), nil, datum)

	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if tags := got[0].Tags(); !reflect.DeepEqual(tags, []string{flashDeadLetterTag}) {
		t.Fatalf("got tags %v, want [%q]", tags, flashDeadLetterTag)
	}
	var deadLetter flowtypes.FlashDeadLetter
	if err := json.Unmarshal(got[0].Value(), &deadLetter); err != nil {
		t.Fatalf("unmarshaling dead letter: %v", err)
	}
	if !reflect.DeepEqual(deadLetter.Job, job) || deadLetter.Result.Attempts != 2 {
		t.Fatalf("got dead letter %+v, want original job and 2 attempts", deadLetter)
	}
}

func TestInvalidJobSkipsFlashAttempt(t *testing.T) {
	attempted := false
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		attempted = true
		return nil
	})

	result := fl.flash(context.Background(), flowtypes.FlashJob{MemAddrHex: "invalid"})

	if result.Status != "error" || result.Attempts != 0 || attempted {
		t.Fatalf("got result=%+v attempted=%v, want validation error before any attempt", result, attempted)
	}
}

func TestEvenSecurityLevelSkipsFlashAttempt(t *testing.T) {
	attempted := false
	fl := testFlasher(func(context.Context, preparedFlashJob) error {
		attempted = true
		return nil
	})
	job := validFlashJob()
	job.SecurityLevel = 0x02

	result := fl.flash(context.Background(), job)

	if result.Status != "error" || result.Attempts != 0 || attempted {
		t.Fatalf("got result=%+v attempted=%v, want validation error before any attempt", result, attempted)
	}
}

func TestRetryConfigurationValidation(t *testing.T) {
	t.Setenv("FLASH_MAX_ATTEMPTS", "0")
	if _, err := newFlasher(); err == nil {
		t.Fatal("newFlasher accepted FLASH_MAX_ATTEMPTS=0")
	}

	t.Setenv("FLASH_MAX_ATTEMPTS", "3")
	t.Setenv("FLASH_RETRY_BACKOFF", "-1s")
	if _, err := newFlasher(); err == nil {
		t.Fatal("newFlasher accepted a negative FLASH_RETRY_BACKOFF")
	}
}

func testFlasher(attempt func(context.Context, preparedFlashJob) error) *flasher {
	return &flasher{
		policy:  retryPolicy{maxAttempts: 1},
		attempt: attempt,
		sleep:   func(context.Context, time.Duration) error { return nil },
	}
}

func validFlashJob() flowtypes.FlashJob {
	return flowtypes.FlashJob{
		JobID:       "job-1",
		ECUID:       "ecu-1",
		MemAddrHex:  "100000",
		FirmwareHex: "deadbeef",
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
