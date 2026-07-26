# Protocol Packages

This page is a map of `pkg/`, not a full API reference — for every exported
type/function/method, run `go doc -all ./pkg/<name>` or read the source; the
Go doc comments are the source of truth (see [Overview](index.md#api-reference)).

## `pkg/can`

Defines `Bus`, the minimum transport interface everything above it depends
on: `Send(Frame) error`, `Recv(deadline) (Frame, error)`, `Close() error`.
`Frame` is `{ID uint32, Data []byte}` — classic 8-byte CAN only, no CAN-FD.

- **`pkg/can/simbus`** — an in-process `Bus` pair (`NewPair()`) that models
  two ends of one CAN segment, where anything sent on one side arrives on
  the other. This is what makes the rest of the stack testable without
  hardware.
- **`pkg/can/socketcan`** — intended to wrap a real Linux SocketCAN
  interface. `Open(iface)` currently just returns an error; not implemented
  yet.

## `pkg/isotp`

Implements enough of ISO 15765-2 (ISO-TP) to carry UDS payloads over classic
CAN frames: single-frame and first-frame/consecutive-frame
segmentation/reassembly, and flow control. `Conn` is a point-to-point
connection addressed by a fixed tester/ECU CAN ID pair (physical addressing
only — no functional/broadcast addressing yet).

Limits worth knowing:

- Max payload is 4095 bytes (12-bit ISO-TP length field).
- No CAN-FD.
- Flow control is honored on send (`STmin`, block size), but the
  implementation defaults to no delay, which is fine against `simbus` and
  most bench setups.

## `pkg/uds`

A UDS (ISO 14229) client (`Client`) built on `*isotp.Conn`. `Client.Request`
is the low-level primitive — send a SID + payload, get back the response
payload, transparently retrying on NRC `0x78` (response pending) up to
`PendingTimeout`. Everything else in `pkg/uds/services.go` is a typed
wrapper around `Request` for one service:

| Method | SID | Purpose |
|---|---|---|
| `DiagnosticSessionControl` | 0x10 | Switch diagnostic session (default/programming/extended) |
| `ECUReset` | 0x11 | Reset the ECU |
| `SecurityAccess` | 0x27 | Seed/key unlock, via a pluggable `KeyGenerator` |
| `RequestDownload` | 0x34 | Begin a firmware download, get the ECU's max block length |
| `TransferData` | 0x36 | Send one block of firmware |
| `RequestTransferExit` | 0x37 | End the transfer |
| `TesterPresent` | 0x3E | Keep a non-default session alive |
| `StartRoutine` / `StopRoutine` / `RequestRoutineResults` | 0x31 | Run ECU routines (erase-before-flash, checksum verification, etc.) |
| `ReadDTCByStatusMask` | 0x19 | Read stored DTCs |
| `ClearDiagnosticInformation` | 0x14 | Clear DTCs |

`TransferFirmware` composes `RequestDownload` → `TransferData`* →
`RequestTransferExit` into one call that chunks a firmware image per the
ECU's reported max block length — this is what `udsflasher` actually calls.
It doesn't call `StartRoutine` itself; a real flash sequence built around
it needs to run the ECU's erase and checksum-verification routines
(routine IDs are ECU-specific) around that call.

### SecurityAccess and `KeyGenerator`

The seed→key algorithm in a real SecurityAccess exchange is always
vendor/ECU-specific and confidential. `pkg/uds/keygen.go` defines the
`KeyGenerator` interface and ships exactly one implementation,
`XORKeyGenerator`, which XORs the seed with a fixed mask — **not a real
security algorithm**, only good enough to exercise `SecurityAccess` against
the simulated ECU in tests and the example pipeline. A real deployment needs
a real `KeyGenerator` implementation supplied per ECU/contract.

### `FakeECU`

`pkg/uds/fakeecu.go` is a minimal scriptable UDS responder (`NewFakeECU`)
used by both the test suite and `udsflasher`'s `CAN_MODE=sim` path. It
understands exactly the services listed above and nothing else — extending
UDS service coverage means extending `FakeECU.handle` too, or the new
service won't be testable/demoable without real hardware.
