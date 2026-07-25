// Command cansource is a Numaflow user-defined source that emits ECU flash
// jobs read from a newline-delimited JSON file. It exists so the example
// pipeline is runnable end-to-end without any external queue: point JOBS_FILE
// at a file of FlashJob records and each line becomes one pipeline message.
package main

import (
	"bufio"
	"context"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/numaproj/numaflow-go/pkg/sourcer"
)

type source struct {
	mu    sync.Mutex
	lines [][]byte
	idx   int
}

func newSource(path string) (*source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines [][]byte
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		lines = append(lines, cp)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &source{lines: lines}, nil
}

func (s *source) Read(ctx context.Context, req sourcer.ReadRequest, messageCh chan<- sourcer.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deadline := time.Now().Add(req.TimeOut())
	var n uint64
	for n < req.Count() && s.idx < len(s.lines) && time.Now().Before(deadline) {
		line := s.lines[s.idx]
		offset := sourcer.NewOffsetWithDefaultPartitionId([]byte(strconv.Itoa(s.idx)))
		messageCh <- sourcer.NewMessage(line, offset, time.Now())
		s.idx++
		n++
	}
}

func (s *source) Ack(ctx context.Context, req sourcer.AckRequest) {
	// Jobs are read from a static file; nothing to acknowledge upstream.
}

func (s *source) Nack(ctx context.Context, req sourcer.NackRequest) {
	log.Printf("cansource: nack received for offsets, no redelivery mechanism configured")
}

func (s *source) Pending(ctx context.Context) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.lines) - s.idx)
}

func (s *source) ActivePartitions(ctx context.Context) []int32 {
	return sourcer.DefaultPartitions()
}

func (s *source) TotalPartitions(ctx context.Context) *int32 {
	total := int32(1)
	return &total
}

func main() {
	path := os.Getenv("JOBS_FILE")
	if path == "" {
		path = "/etc/automotive-flow/jobs.jsonl"
	}
	src, err := newSource(path)
	if err != nil {
		log.Fatalf("cansource: loading %s: %v", path, err)
	}
	log.Printf("cansource: loaded %d job(s) from %s", len(src.lines), path)

	if err := sourcer.NewServer(src).Start(context.Background()); err != nil {
		log.Fatalf("cansource: server error: %v", err)
	}
}
