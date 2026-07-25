// Command resultsink is a Numaflow user-defined sink that logs each
// flash-report message it receives to stdout. It exists as the terminal
// vertex of the example pipeline; swap it for a real sink (database, MES
// system, ticketing webhook) once a plant needs to persist results.
package main

import (
	"context"
	"log"

	"github.com/numaproj/numaflow-go/pkg/sinker"
)

type sink struct{}

func (sink) Sink(ctx context.Context, datumStreamCh <-chan sinker.Datum) sinker.Responses {
	responses := sinker.ResponsesBuilder()
	for d := range datumStreamCh {
		log.Printf("resultsink: %s", d.Value())
		responses = responses.Append(sinker.ResponseOK(d.ID()))
	}
	return responses
}

func main() {
	if err := sinker.NewServer(sink{}).Start(context.Background()); err != nil {
		log.Fatalf("resultsink: server error: %v", err)
	}
}
