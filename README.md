# automotive-flow

A library of [Numaflow](https://numaflow.numaproj.io/) pipeline components
for automotive plant use, centered on ECU flashing stations over CAN/UDS.
Pipelines are authored directly as Numaflow `Pipeline` YAML.

There's no real hardware yet, so the protocol stack is built against a
swappable transport (`pkg/can.Bus`) and validated with a simulated ECU.

**Full documentation:** https://isaiah-harville.github.io/automotive/

## Layout

- `pkg/can`, `pkg/isotp`, `pkg/uds` — CAN transport, ISO-TP framing, and a
  UDS (ISO 14229) client, in that dependency order.
- `components/go/` and `components/python/` — Numaflow vertices (source,
  UDFs, sink) built on top of `pkg/`. The pipeline intentionally mixes Go
  and Python vertices to establish that pattern.
- `pipelines/examples/` — example `Pipeline` YAML and sample input data.
- `docs/` — source for the MkDocs documentation site (architecture,
  contracts, component library, configuration, local deployment, and a
  readiness audit).

## Getting started

```sh
go build ./...
go test ./...
```

Tests run against an in-process simulated ECU (`pkg/can/simbus` +
`uds.FakeECU`) — no hardware or cluster required.

To work on the docs site locally:

```sh
uv sync
uv run mkdocs serve
```

See the [documentation site](https://isaiah-harville.github.io/automotive/)
for the component catalog, how to add a new UDS service or component, and
the current state of real hardware support (not implemented yet —
`pkg/can/socketcan` is a stub).
