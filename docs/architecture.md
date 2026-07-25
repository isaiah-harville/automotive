# Architecture

## Layering

```
pkg/can      transport-agnostic CAN bus (the Bus interface)
  simbus       in-process simulated bus + a scriptable FakeECU
  socketcan    real Linux SocketCAN — not implemented yet, stub only
pkg/isotp    ISO-TP (ISO 15765-2) segmentation/reassembly over can.Bus
pkg/uds      UDS (ISO 14229) client built on pkg/isotp
components/  Numaflow vertices (source/UDF/sink) that call into pkg/
```

Each layer depends only on the interface below it:

- `pkg/isotp` depends on `can.Bus`, never on a concrete transport.
- `pkg/uds` depends on `*isotp.Conn`, never on `can.Bus` directly.
- Components depend on `pkg/uds` (and occasionally `pkg/can` directly, to
  choose a transport at startup) and the numaflow-go / pynumaflow SDKs.

This means swapping `can/simbus` for `can/socketcan` (once implemented), or
for a vendor pass-thru driver under a new `pkg/can/<vendor>` package,
requires no changes to `pkg/isotp` or `pkg/uds` — only to whatever chooses
the transport at the component's startup (see `udsflasher`'s `CAN_MODE`
handling in [Configuration](configuration.md)).

## Why Numaflow

[Numaflow](https://numaflow.numaproj.io/) runs a pipeline as a graph of
vertices (sources, user-defined functions, sinks), where **each vertex is
its own container** speaking a fixed gRPC protocol to the Numaflow runtime
over a Unix domain socket. That has two consequences that shape this repo:

1. **Language is a per-vertex choice, not a pipeline-wide one.** The
   `flash-basic` example pipeline deliberately mixes a Go source
   (`cansource`), a Go mapper (`udsflasher`), a Python mapper
   (`report-formatter`), and a Go sink (`resultsink`) to establish that
   pattern early — see [Component Library](components.md).
2. **A vertex only needs to implement one of a small set of SDK
   interfaces** (`Sourcer`, `Mapper`/`Mapper`, `Sinker`). Components in this
   repo are kept as thin adapters between that interface and `pkg/uds` —
   all protocol logic lives in `pkg/`, not in a component's `main.go`.

Pipelines are authored as hand-written `Pipeline` YAML today (see
`pipelines/examples/`); there's no visual builder, since Numaflow doesn't
ship one.

## Message flow (flash-basic)

```
cansource ──▶ udsflasher ──▶ report-formatter ──▶ resultsink
 (Go)          (Go)            (Python)             (Go)
```

- **cansource** reads `FlashJob` records from a newline-delimited JSON file
  and emits one message per job.
- **udsflasher** drives the full UDS flash sequence (session control →
  security access → download/transfer/exit → reset) against an ECU —
  simulated by default — and emits a `FlashResult`.
- **report-formatter** turns the `FlashResult` JSON into a human-readable
  line. It has no protocol logic of its own; it exists to prove the
  mixed-language pattern.
- **resultsink** logs each report line.

See [Message Contracts](contracts.md) for the exact JSON shapes passed
between vertices.

## Concurrency model

A single physical CAN bus supports one flashing conversation at a time.
`udsflasher` reflects that: it holds one `uds.Client` over one CAN
connection and serializes all `Map` calls behind a mutex, regardless of how
much parallelism Numaflow itself is configured for. Scaling to multiple
physical flashing stations running in parallel means running multiple
`udsflasher` replicas, each owning its own CAN connection — that partitioning
strategy isn't built yet (tracked as a known gap; see [Readiness
Audit](readiness-audit.md)).
