package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/numaproj/numaflow-go/pkg/sourcer"
)

func writeJobsFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing jobs file: %v", err)
	}
	return path
}

func TestNewSourceLoadsNonBlankLines(t *testing.T) {
	path := writeJobsFile(t, "{\"job_id\":\"1\"}\n\n{\"job_id\":\"2\"}\n   \n{\"job_id\":\"3\"}\n")

	src, err := newSource(path)
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	if len(src.lines) != 4 {
		t.Fatalf("got %d lines, want 4 (blank lines skipped, whitespace-only line kept)", len(src.lines))
	}
}

func TestNewSourceMissingFile(t *testing.T) {
	if _, err := newSource(filepath.Join(t.TempDir(), "does-not-exist.jsonl")); err == nil {
		t.Fatal("expected an error opening a missing jobs file, got nil")
	}
}

func TestNewSourceEmptyFile(t *testing.T) {
	path := writeJobsFile(t, "")

	src, err := newSource(path)
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	if len(src.lines) != 0 {
		t.Fatalf("got %d lines from an empty file, want 0", len(src.lines))
	}
	if got := src.Pending(context.Background()); got != 0 {
		t.Fatalf("Pending() = %d, want 0", got)
	}
}

type fakeReadRequest struct {
	count   uint64
	timeout time.Duration
}

func (r fakeReadRequest) Count() uint64          { return r.count }
func (r fakeReadRequest) TimeOut() time.Duration { return r.timeout }

func TestReadEmitsMessagesInOrderAndAdvancesIndex(t *testing.T) {
	src, err := newSource(writeJobsFile(t, "{\"job_id\":\"1\"}\n{\"job_id\":\"2\"}\n{\"job_id\":\"3\"}\n"))
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}

	messageCh := make(chan sourcer.Message, 3)
	src.Read(context.Background(), fakeReadRequest{count: 2, timeout: time.Second}, messageCh)
	close(messageCh)

	var got []string
	for m := range messageCh {
		got = append(got, string(m.Value()))
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2 (count was capped at 2)", len(got))
	}
	if got[0] != `{"job_id":"1"}` || got[1] != `{"job_id":"2"}` {
		t.Fatalf("got messages %v, want the first two lines in file order", got)
	}
	if src.idx != 2 {
		t.Fatalf("src.idx = %d, want 2 after reading 2 messages", src.idx)
	}
	if got := src.Pending(context.Background()); got != 1 {
		t.Fatalf("Pending() = %d, want 1 (one job left unread)", got)
	}
}

func TestReadStopsAtEndOfFileEvenIfCountNotReached(t *testing.T) {
	src, err := newSource(writeJobsFile(t, "{\"job_id\":\"1\"}\n"))
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}

	messageCh := make(chan sourcer.Message, 10)
	src.Read(context.Background(), fakeReadRequest{count: 10, timeout: time.Second}, messageCh)
	close(messageCh)

	var n int
	for range messageCh {
		n++
	}
	if n != 1 {
		t.Fatalf("got %d messages, want 1 (only one job in the file)", n)
	}
}

func TestReadIsExhaustedAfterAllJobsSent(t *testing.T) {
	src, err := newSource(writeJobsFile(t, "{\"job_id\":\"1\"}\n"))
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}

	drain := func() int {
		messageCh := make(chan sourcer.Message, 10)
		src.Read(context.Background(), fakeReadRequest{count: 10, timeout: time.Second}, messageCh)
		close(messageCh)
		n := 0
		for range messageCh {
			n++
		}
		return n
	}

	if n := drain(); n != 1 {
		t.Fatalf("first Read: got %d messages, want 1", n)
	}
	if n := drain(); n != 0 {
		t.Fatalf("second Read after exhaustion: got %d messages, want 0", n)
	}
}

func TestActiveAndTotalPartitions(t *testing.T) {
	src, err := newSource(writeJobsFile(t, "{}\n"))
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}

	if got := src.ActivePartitions(context.Background()); len(got) != 1 {
		t.Fatalf("ActivePartitions() = %v, want exactly one partition", got)
	}
	total := src.TotalPartitions(context.Background())
	if total == nil || *total != 1 {
		t.Fatalf("TotalPartitions() = %v, want a pointer to 1", total)
	}
}

// Ack and Nack are no-ops (jobs come from a static file, no upstream to
// acknowledge); this just documents that calling them never panics.
func TestAckAndNackAreNoOps(t *testing.T) {
	src, err := newSource(writeJobsFile(t, "{}\n"))
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	src.Ack(context.Background(), nil)
	src.Nack(context.Background(), nil)
}
