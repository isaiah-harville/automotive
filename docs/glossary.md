# Glossary

Background on the terminology used throughout this repo and its docs,
grouped by area. If you already know CAN/UDS or already know Numaflow,
skip to the section you don't.

## Automotive protocol stack

The three layers this repo is built on, from the wire up:

**CAN (Controller Area Network)**
A serial bus used throughout a vehicle (and in this repo's case, a
flashing station's bench setup) for electronic control units to talk to
each other. Messages ("frames") are small — classic CAN carries up to 8
bytes of data per frame, identified by an arbitration ID. `pkg/can`
defines this repo's transport abstraction (`Bus`); see
[Architecture](architecture.md).

**Arbitration ID / CAN ID**
The identifier on a CAN frame that says what it is / who it's for.
`udsflasher` uses two: `TESTER_CAN_ID` (what the diagnostic tester, i.e.
this codebase, sends on) and `ECU_CAN_ID` (what the target ECU responds
on). See [Configuration](configuration.md).

**SocketCAN**
The Linux kernel's native CAN networking stack — CAN interfaces
(`can0`, a `vcan0` virtual interface for testing, etc.) show up like
network devices and are driven through a socket API. `pkg/can/socketcan`
is meant to wrap this for real hardware; see the [Readiness
Audit](readiness-audit.md) for its current (stub) status.

**J2534 (pass-thru)**
A standardized Windows API (SAE J2534) that many vehicle manufacturers'
diagnostic tools use to talk to vendor-specific diagnostic hardware
instead of a raw CAN interface. Relevant if a plant contract specifies a
particular vendor's pass-thru device rather than a bare SocketCAN
interface — tracked as future work (issue #18).

**ISO-TP (ISO 15765-2, "ISO Transport Protocol")**
A framing protocol on top of CAN that lets payloads bigger than one
frame's 8 bytes (like a firmware image chunk) be split across multiple
CAN frames and reassembled on the other end — single-frame for short
messages, first-frame/consecutive-frame plus flow-control frames for
longer ones. `pkg/isotp` implements this. See [Protocol
Packages](protocol-packages.md).

**UDS (ISO 14229, "Unified Diagnostic Services")**
The diagnostic/flashing protocol carried over ISO-TP. Defines a
request/response vocabulary — session control, security unlock, reading
and writing data, running routines, transferring firmware, reading/
clearing fault codes — that this repo's `pkg/uds.Client` implements a
subset of.

**SID (Service Identifier)**
The first byte of a UDS request, saying which service is being invoked —
e.g. `0x10` is DiagnosticSessionControl, `0x27` is SecurityAccess. A
positive response echoes back `SID + 0x40`; a negative response is `0x7F`
followed by the original SID and an NRC.

**NRC (Negative Response Code)**
The reason byte in a UDS negative (`0x7F`) response — e.g. `0x78` means
"request received, response pending, keep waiting," `0x35` means
"invalid key." `pkg/uds/client.go` defines the ones this codebase cares
about and wraps them in `NegativeResponseError`.

**Diagnostic session**
A UDS "mode" the ECU is in — `default`, `programming`, `extended`, etc. —
selected via `DiagnosticSessionControl` (SID `0x10`). Flashing requires the
programming session. Non-default sessions time out (the "S3 timer,"
typically ~5s) if the tester goes quiet, which is why `udsflasher` sends
periodic `TesterPresent` (SID `0x3E`) during a long transfer — see
`Client.StartKeepAlive`.

**SecurityAccess / seed-key exchange**
A UDS service (SID `0x27`) that unlocks privileged operations (like
flashing) by having the tester request a random "seed" from the ECU,
compute a "key" from it using an algorithm only authorized tools know, and
send the key back. `pkg/uds.KeyGenerator` is the pluggable interface for
that algorithm; the only implementation in this repo, `XORKeyGenerator`, is
an explicit placeholder for tests, not a real vendor algorithm. See
[Protocol Packages](protocol-packages.md#securityaccess-and-keygenerator).

**RequestDownload / TransferData / RequestTransferExit**
The three UDS services (`0x34`, `0x36`, `0x37`) that move a firmware image
into an ECU: negotiate the transfer and block size, send the image in
chunks, then signal completion. `Client.TransferFirmware` composes all
three into one call.

**RoutineControl**
A UDS service (SID `0x31`) for telling the ECU to run a named "routine" —
in a flash sequence this is typically erase-before-flash and
post-flash checksum verification, identified by an ECU-specific routine
ID. `Client.StartRoutine`/`StopRoutine`/`RequestRoutineResults` wrap it.

**DTC (Diagnostic Trouble Code)**
A stored fault code an ECU records when it detects a problem — what a
mechanic's scan tool reads. `ReadDTCByStatusMask` and
`ClearDiagnosticInformation` (SIDs `0x19`/`0x14`) read and clear these.

**ECU (Electronic Control Unit)**
The embedded computer being flashed/diagnosed — an engine controller,
body control module, infotainment head unit, etc. This repo talks to one
ECU per `uds.Client`/ISO-TP connection.

## Numaflow and this repo's pipeline

**Numaflow**
A Kubernetes-native platform for running data pipelines as a graph of
small, independently deployable pieces ("vertices"), each its own
container. See [Architecture](architecture.md#why-numaflow) for why this
project uses it.

**Pipeline**
A Numaflow custom resource (CRD) — YAML wiring a set of vertices together
into a graph. `pipelines/examples/flash-basic.yaml` is this repo's example.

**Vertex**
One node in a pipeline: a source, a user-defined function, or a sink.
Numaflow runs each vertex as its own container(s) and talks to it over
gRPC on a Unix domain socket — which is why a pipeline can freely mix
languages, one per vertex (see `flash-basic`'s Go/Python mix).

**Source**
A vertex that originates messages into the pipeline. `cansource` reads
`FlashJob` records from a file and emits one message per job (see
[Message Contracts](contracts.md)).

**Mapper / UDF (User-Defined Function)**
A vertex that transforms one input message into zero or more output
messages. `udsflasher` (drives the flash sequence, emits a result) and
`report-formatter` (formats that result as text) are both mappers.

**Sink**
A vertex that consumes messages out of the pipeline with no further
output. `resultsink` logs each formatted report line.

**ISB (Inter-Step Buffer)**
The message transport Numaflow uses between vertices — this repo uses the
default JetStream-backed ISB, installed by
`.devcontainer/scripts/cluster-up.sh` alongside Numaflow itself.

**numaflow-go / pynumaflow**
The SDKs a vertex's container uses to implement the gRPC contract Numaflow
expects, in Go and Python respectively — `github.com/numaproj/numaflow-go`
(`pkg/sourcer`, `pkg/mapper`, `pkg/sinker`) and `pynumaflow`
(`from pynumaflow.mapper import ...`).

## This repo's own vocabulary

**Flashing station**
The plant-floor setup (bench or line equipment) that connects a tester —
this codebase — to a target ECU over CAN, to write new firmware to it.

**FlashJob / FlashResult**
This repo's own JSON message contract, not a UDS or Numaflow term: a
`FlashJob` describes one flash request (target ECU, memory address,
firmware bytes, security level); a `FlashResult` reports what happened.
Defined in `components/go/flowtypes`, documented in [Message
Contracts](contracts.md).

**`can/simbus`**
This repo's in-process, no-hardware substitute for a real CAN bus — a
`Bus` implementation that loops frames between two in-memory endpoints, so
`pkg/isotp` and `pkg/uds` (and `uds.FakeECU`, a scriptable fake ECU
responder built on it) can be tested with zero hardware. See the
[Readiness Audit](readiness-audit.md) for what "no real hardware" means
in practice.
