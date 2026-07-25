# Component Library

This page catalogs what's built and how to extend it. For how the layers
relate to each other, see [Architecture](architecture.md); for the JSON
message shapes, see [Message Contracts](contracts.md).

## Catalog

| Component | Language | Numaflow role | Purpose |
|---|---|---|---|
| `components/go/cansource` | Go | Source | Reads `FlashJob` records from a newline-delimited JSON file |
| `components/go/udsflasher` | Go | Mapper | Runs the full UDS flash sequence per job, emits `FlashResult` |
| `components/python/report-formatter` | Python (pynumaflow) | Mapper | Formats `FlashResult` JSON into a readable line |
| `components/go/resultsink` | Go | Sink | Logs each report line |

`components/go/flowtypes` isn't a vertex — it's the shared Go struct
definitions for `FlashJob`/`FlashResult` that `cansource`, `udsflasher`, and
`resultsink` all import.

## Adding a new component

1. If it needs new protocol behavior, add it to `pkg/uds` (or `pkg/isotp` /
   `pkg/can` for a framing/transport concern), with a test against
   `uds.FakeECU` / `can/simbus`.
2. Add a thin `main.go` (or `handler.py`) under `components/go/<name>` or
   `components/python/<name>` that implements the relevant numaflow-go /
   pynumaflow interface (`sourcer.Sourcer`, `mapper.Mapper`,
   `sinker.Sinker`) and calls into `pkg/`. Keep protocol logic out of the
   component — it belongs in `pkg/`.
3. Add a `Dockerfile` alongside it (see existing components for the
   multi-stage Go pattern or the `uv`-based Python pattern).
4. Wire it into a pipeline YAML under `pipelines/examples/`.
5. If it takes new configuration, document it in
   [Configuration](configuration.md).

## Adding a new UDS service

Add the service ID constant and a `(*Client) MethodName(...)` method to
`pkg/uds/services.go`, following the existing services: build the request
payload, call `c.Request(sid, payload)`, parse the positive-response
payload. Extend `uds.FakeECU.handle` in `pkg/uds/fakeecu.go` with a case for
the new SID so it's testable without hardware, then add a test in
`pkg/uds/client_test.go`.

## Running the example pipeline locally

There's a devcontainer with a scripted Minikube bootstrap for this — see
[Local Deployment](local-deployment.md). To exercise just the flashing
logic without a cluster:

```sh
go test ./pkg/... ./components/go/...
```

`components/go/udsflasher/main_test.go` runs the full flash sequence against
`pipelines/examples/jobs.jsonl` using the same `CAN_MODE=sim` simulated ECU
path the pipeline uses by default.
