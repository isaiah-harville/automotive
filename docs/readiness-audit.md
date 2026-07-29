# Readiness Audit

A candid list of what's real, what's simulated, and what's just a stub —
last reviewed alongside the initial scaffold. Update this page when any of
these items change status; treat it as load-bearing, not aspirational.

## Real and tested

- **ISO-TP framing** (`pkg/isotp`) — single-frame and multi-frame
  segmentation/reassembly with flow control, tested against `can/simbus`.
  Physical addressing only.
- **UDS client** (`pkg/uds`) — session control, security access,
  download/transfer/exit, reset, tester present, DTC read/clear, all tested
  end-to-end against `uds.FakeECU` including a full flash sequence
  (`TestFullFlashSequence`).
- **The `flash-basic` pipeline's logic** — `components/go/udsflasher`'s
  `flash()` function is integration-tested against the actual sample
  fixture (`pipelines/examples/jobs.jsonl`), not just unit-tested in
  isolation.
- **Linux SocketCAN transport** (`pkg/can/socketcan`) — binds an `AF_CAN`
  raw socket to a named interface and sends/receives classic CAN frames.
  Frame encoding and error paths are unit-tested; live hardware validation
  is still outstanding.

## Simulated, not real

- **No real CAN hardware anywhere.** Every test and the default pipeline
  configuration (`CAN_MODE=sim`) run against `can/simbus`, an in-process
  loopback — not a real bus, not real electrical/timing behavior.
- **SocketCAN has not been exercised against real CAN hardware.** The
  transport is implemented and can target Linux `can0`/`vcan0` interfaces,
  but current automated tests do not validate electrical or ECU behavior.
- **SecurityAccess key exchange is not a real security algorithm.**
  `uds.XORKeyGenerator` XORs the seed with a fixed mask, and in
  `CAN_MODE=sim`, `udsflasher`'s `FakeECU` is configured with
  `ExpectedKey = nil` — it accepts *any* key. Nothing here validates that a
  real ECU's seed/key exchange would actually succeed. A real deployment
  needs a real, vendor-supplied `KeyGenerator`.

## Not implemented

- **Functional (broadcast) CAN addressing** — `pkg/isotp` only supports
  physical 1:1 addressing.
- **CAN-FD** — classic 8-byte frames only.
- **`TransferFirmware` doesn't call `StartRoutine`.** `RoutineControl`
  (0x31) exists on `Client` now, but the actual erase-before-flash /
  checksum-verification routine IDs and calling convention are ECU-specific
  and aren't wired into the default flash sequence yet.
- Most other UDS services beyond the ones listed in [Protocol
  Packages](protocol-packages.md) are still missing.
- **Multi-station concurrency** — `udsflasher` serializes everything behind
  one mutex around one CAN connection. Running multiple physical flashing
  stations in parallel needs a partitioning strategy that doesn't exist yet.
- **Retry persistence** — `udsflasher` performs bounded in-process retries
  and routes exhausted jobs to a separate sink, but the example
  `deadlettersink` only logs them. A production deployment still needs
  durable dead-letter storage and an authenticated replay workflow.
- **Vendor pass-thru integration (J2534 or similar)** — no design yet;
  depends on which vendor tooling a plant contract specifies.
- **CI for Docker image builds** — CI runs `go test`/`go vet`/`gofmt` and
  Python `ruff`/`ty`, but doesn't build or smoke-test the container images
  themselves.
- **Air-gapped / offline plant deployment** — see [Local
  Deployment](local-deployment.md#what-this-isnt-yet).

## Open questions

- What's the actual target ECU/vendor for the first real deployment? That
  determines the SecurityAccess algorithm, whether SocketCAN or a vendor
  pass-thru device is the right transport, and which UDS services beyond
  the current set are actually needed.
- Is a visual pipeline builder actually needed, or is hand-authored YAML
  sufficient long-term? (Currently: hand-authored YAML, by explicit choice.)
