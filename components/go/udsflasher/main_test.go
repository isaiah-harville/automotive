package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/isaiah-harville/automotive/components/go/flowtypes"
)

func TestFlashSampleJobs(t *testing.T) {
	f, err := os.Open("../../../pipelines/examples/jobs.jsonl")
	if err != nil {
		t.Fatalf("opening sample jobs: %v", err)
	}
	defer f.Close()

	fl, err := newFlasher()
	if err != nil {
		t.Fatalf("newFlasher: %v", err)
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	count := 0
	for scanner.Scan() {
		var job flowtypes.FlashJob
		if err := json.Unmarshal(scanner.Bytes(), &job); err != nil {
			t.Fatalf("unmarshaling job: %v", err)
		}
		result := fl.flash(context.Background(), job)
		if result.Status != "ok" {
			t.Fatalf("job %s: got status %q, want ok (error: %s)", job.JobID, result.Status, result.Error)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning sample jobs: %v", err)
	}
	if count == 0 {
		t.Fatal("no jobs found in sample fixture")
	}
}
