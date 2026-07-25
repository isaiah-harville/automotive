# Message Contracts

Vertices in the `flash-basic` pipeline pass JSON messages between each
other. The contracts are defined once, in Go, at
`components/go/flowtypes/types.go` — the Python `report-formatter` vertex
just parses the same JSON shape without importing that package (Numaflow
vertices don't share a language, so the contract is documentation + a Go
struct, not a shared library).

## FlashJob

Emitted by `cansource`, consumed by `udsflasher`.

| Field | Type | Notes |
|---|---|---|
| `job_id` | string | Opaque identifier, echoed back in `FlashResult` |
| `ecu_id` | string | Opaque identifier, echoed back in `FlashResult` |
| `mem_addr_hex` | string | Firmware download start address, hex without `0x` (e.g. `"00100000"`) |
| `firmware_hex` | string | Hex-encoded firmware image bytes |
| `security_level` | byte | Odd SecurityAccess request-seed sub-function; defaults to `0x01` if `0` |
| `key_mask_hex` | string | Demo `XORKeyGenerator` mask, hex without `0x` (e.g. `"ff"`); defaults to `0xFF` if empty |

```json
{
  "job_id": "job-001",
  "ecu_id": "ecm-01",
  "mem_addr_hex": "00100000",
  "firmware_hex": "deadbeef...",
  "security_level": 1,
  "key_mask_hex": "ff"
}
```

See `pipelines/examples/jobs.jsonl` for a runnable sample.

## FlashResult

Emitted by `udsflasher`, consumed by `report-formatter` and (indirectly, as
a formatted line) `resultsink`.

| Field | Type | Notes |
|---|---|---|
| `job_id` | string | Echoed from the `FlashJob` |
| `ecu_id` | string | Echoed from the `FlashJob` |
| `status` | string | `"ok"` or `"error"` |
| `error` | string | Present only when `status` is `"error"` |
| `duration_ms` | int64 | Wall-clock time spent in the flash sequence |

```json
{"job_id": "job-001", "ecu_id": "ecm-01", "status": "ok", "duration_ms": 42}
{"job_id": "job-002", "ecu_id": "ecm-02", "status": "error", "error": "security access: uds: negative response to SID 0x27: NRC 0x35", "duration_ms": 8}
```

## Changing a contract

Both fields and semantics here are consumed by at least two vertices in two
languages. If you add or change a field:

1. Update the Go struct in `components/go/flowtypes/types.go` (the source
   of truth).
2. Update this page.
3. Update `report-formatter/handler.py`'s parsing if the field affects it.
4. Update `pipelines/examples/jobs.jsonl` if it's a `FlashJob` field.
