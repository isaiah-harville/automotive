# Configuration

All configuration is environment variables, set on the vertex's container in
pipeline YAML (see `pipelines/examples/flash-basic.yaml`).

## `cansource`

| Variable | Default | Notes |
|---|---|---|
| `JOBS_FILE` | `/etc/automotive-flow/jobs.jsonl` | Path to a newline-delimited JSON file of `FlashJob` records, loaded once at startup |

## `udsflasher`

| Variable | Default | Notes |
|---|---|---|
| `CAN_MODE` | `sim` | `sim` runs against an in-process simulated ECU (`can/simbus` + `uds.FakeECU`); `socketcan` uses a real interface — **not implemented yet**, will error |
| `CAN_IFACE` | `can0` | Interface name, only used when `CAN_MODE=socketcan` |
| `TESTER_CAN_ID` | `7E0` | Hex CAN arbitration ID the tester sends on |
| `ECU_CAN_ID` | `7E8` | Hex CAN arbitration ID the ECU responds on |
| `FLASH_MAX_ATTEMPTS` | `3` | Maximum flash attempts, including the initial attempt; valid range is 1–10 |
| `FLASH_RETRY_BACKOFF` | `1s` | Initial delay before retrying a transient failure; doubles after each failed attempt |

In `CAN_MODE=sim`, `udsflasher` starts a `uds.FakeECU` with
`ExpectedKey = nil`, meaning **it accepts any SecurityAccess key** — that's
correct for exercising the pipeline end-to-end without hardware, but means
`sim` mode never actually validates a `key_mask_hex` value from a
`FlashJob`. See [Readiness Audit](readiness-audit.md).

Input-validation failures, ECU negative responses, and a closed CAN bus are
not retried. Other transport/protocol failures use the bounded retry policy.
Jobs that still fail are routed to the `deadlettersink` vertex with the
original job attached.

## `report-formatter`

No configuration; it only transforms JSON already produced by `udsflasher`.

## `resultsink`

No configuration; logs to stdout.

## Devcontainer / local cluster scripts

| Variable | Default | Used by |
|---|---|---|
| `MINIKUBE_PROFILE` | `automotive` | `up-cluster.sh`, `deploy-local.sh` |
| `MINIKUBE_CPUS` | `4` | `up-cluster.sh` |
| `MINIKUBE_MEMORY` | `8192` (MB) | `up-cluster.sh` |
| `NUMAFLOW_VERSION` | `v1.7.5` | `up-cluster.sh`, pins the Numaflow manifests/images installed |

See [Local Deployment](local-deployment.md) for how these scripts fit
together.
