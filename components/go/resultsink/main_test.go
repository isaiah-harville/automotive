package main

import (
	"context"
	"testing"
	"time"

	"github.com/numaproj/numaflow-go/pkg/sinker"
)

type fakeDatum struct {
	id    string
	value []byte
}

func (d fakeDatum) Keys() []string                         { return nil }
func (d fakeDatum) Value() []byte                          { return d.value }
func (d fakeDatum) EventTime() time.Time                   { return time.Time{} }
func (d fakeDatum) Watermark() time.Time                   { return time.Time{} }
func (d fakeDatum) ID() string                             { return d.id }
func (d fakeDatum) Headers() map[string]string             { return nil }
func (d fakeDatum) UserMetadata() *sinker.UserMetadata     { return nil }
func (d fakeDatum) SystemMetadata() *sinker.SystemMetadata { return nil }

func TestSinkAcknowledgesEveryDatum(t *testing.T) {
	datumCh := make(chan sinker.Datum, 3)
	datumCh <- fakeDatum{id: "1", value: []byte("[OK] job=1 ecu=A flashed in 10ms")}
	datumCh <- fakeDatum{id: "2", value: []byte("[FAIL] job=2 ecu=B after 5ms: timeout")}
	datumCh <- fakeDatum{id: "3", value: []byte("")}
	close(datumCh)

	responses := sink{}.Sink(context.Background(), datumCh)

	if len(responses) != 3 {
		t.Fatalf("got %d responses, want 3 (one per datum)", len(responses))
	}
	wantIDs := map[string]bool{"1": true, "2": true, "3": true}
	for _, r := range responses {
		if !wantIDs[r.ID] {
			t.Fatalf("unexpected response ID %q", r.ID)
		}
		delete(wantIDs, r.ID)
	}
	if len(wantIDs) != 0 {
		t.Fatalf("missing responses for IDs %v", wantIDs)
	}
}

func TestSinkOnEmptyChannelReturnsNoResponses(t *testing.T) {
	datumCh := make(chan sinker.Datum)
	close(datumCh)

	responses := sink{}.Sink(context.Background(), datumCh)

	if len(responses) != 0 {
		t.Fatalf("got %d responses, want 0", len(responses))
	}
}
