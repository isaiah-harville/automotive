# Component Library

This repo is organized as a protocol layer (`pkg/`) plus a set of thin
Numaflow vertices (`components/`) that call into it. All flashing/protocol
logic lives in `pkg/`; components should stay thin adapters between a
Numaflow message and a `pkg/` call.

## Layers

```
pkg/can      transport-agnostic CAN bus (Bus interface)
  simbus       in-process simulated bus + FakeECU, used by tests and CAN_MODE=sim
  socketcan    real Linux SocketCAN, not implemented yet (stub returns an error)
pkg/isotp    ISO-TP (ISO 15765-2) segmentation/reassembly over can.Bus
pkg/uds      UDS (ISO 14229) client built on pkg/isotp: sessions, security
             access, download/transfer/exit, reset, tester present, DTCs
```

Each layer only depends on the interfaces below it, so swapping `simbus` for
`socketcan` (once implemented) or a vendor pass-thru driver requires no
changes to `pkg/isotp` or `pkg/uds`.

## Components (Numaflow vertices)

| Component | Language | Numaflow role | Purpose |
|---|---|---|---|
| `components/go/cansource` | Go | UDSource | Reads `FlashJob` records from a newline-delimited JSON file |
| `components/go/udsflasher` | Go | Mapper | Runs the full UDS flash sequence per job, emits `FlashResult` |
| `components/python/report-formatter` | Python (pynumaflow) | Mapper | Formats `FlashResult` JSON into a readable line |
| `components/go/resultsink` | Go | UDSink | Logs each report line |

`components/go/flowtypes` holds the shared `FlashJob`/`FlashResult` JSON
contracts so every vertex agrees on field names regardless of language.

Numaflow lets every vertex be an independent container speaking a fixed gRPC
protocol over a Unix domain socket, so components can be written in whatever
language suits the task — the pipeline above deliberately mixes Go and
Python to establish that pattern early.

## Adding a new component

1. If it needs new protocol behavior, add it to `pkg/uds` (or `pkg/isotp` /
   `pkg/can` if it's a framing/transport concern), with a test against
   `uds.FakeECU` / `can/simbus`.
2. Add a thin `main.go` (or `handler.py`) under `components/go/<name>` or
   `components/python/<name>` that implements the relevant numaflow-go /
   pynumaflow interface (`sourcer.Sourcer`, `mapper.Mapper`, `sinker.Sinker`)
   and calls into `pkg/`.
3. Add a `Dockerfile` alongside it (see existing components for the
   multi-stage Go pattern or the pip-install Python pattern).
4. Wire it into a pipeline YAML under `pipelines/examples/`.

## Adding a new UDS service

Add the service ID constant and a `(*Client) MethodName(...)` method to
`pkg/uds/services.go`, following the existing services: build the request
payload, call `c.Request(sid, payload)`, parse the positive-response payload.
Extend `uds.FakeECU.handle` in `pkg/uds/fakeecu.go` with a case for the new
SID so it's testable without hardware, then add a test in
`pkg/uds/client_test.go`.

## Running the example pipeline locally (no cluster)

There's no Numaflow/k8s cluster wired up in this repo yet, so
`pipelines/examples/flash-basic.yaml` is a reviewed template, not something
you can `kubectl apply` today. To exercise the flashing logic without any of
that, run:

```sh
go test ./pkg/... ./components/go/...
```

`components/go/udsflasher/main_test.go` runs the full flash sequence against
`pipelines/examples/jobs.jsonl` using the same `CAN_MODE=sim` simulated ECU
path the pipeline uses by default.

## Real hardware

Nothing here talks to real CAN hardware yet. `pkg/can/socketcan.Open`
is a stub that returns an error — implement it (or a vendor-specific
transport under a new `pkg/can/<vendor>` package) against the `can.Bus`
interface when a plant has real hardware, and `pkg/isotp` / `pkg/uds` work
unmodified. `uds.XORKeyGenerator` in `pkg/uds/keygen.go` is a placeholder;
`SecurityAccess`'s real seed→key algorithm is always vendor/ECU-specific and
must be supplied per contract.
