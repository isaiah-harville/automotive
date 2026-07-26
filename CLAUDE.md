# CLAUDE.md

Guidance for Claude Code (or any agent) working in this repo.

## What this is

A library of [Numaflow](https://numaflow.numaproj.io/) pipeline components
for automotive plant ECU flashing over CAN/UDS. Layering:

```
pkg/can      transport-agnostic CAN bus (Bus interface)
  simbus       in-process simulated bus + scriptable FakeECU (no hardware needed)
  socketcan    real Linux SocketCAN — stub only, not implemented
pkg/isotp    ISO-TP (ISO 15765-2) framing over can.Bus
pkg/uds      UDS (ISO 14229) client built on pkg/isotp
components/  Numaflow vertices (Go and Python) that call into pkg/
pipelines/   example Pipeline YAML wiring components together
docs/        curated MkDocs Material site (see below)
```

Full details: [docs/architecture.md](docs/architecture.md),
[docs/glossary.md](docs/glossary.md) if you don't already know CAN/UDS or
Numaflow terminology.

## Licensing — read before touching this

This repo is **public but source-available, not open source**. No LICENSE
file is intentional — default GitHub terms apply (view/fork only, no reuse
rights). It's a commercial codebase. Do not:

- Add an OSS license file
- Write docs implying reuse/redistribution rights
- Publish this module to a public package registry (pkg.go.dev, PyPI, npm, etc.)
- Add a CONTRIBUTING.md aimed at external contributors

## Everyday commands

```sh
# Go
gofmt -l .                       # must be empty
go build ./...
go vet ./...
go test -race ./...
golangci-lint run ./...          # config: .golangci.yml

# Python (components/python/report-formatter)
cd components/python/report-formatter
uv sync --locked
uv run ruff check .
uv run ruff format --check .
uv run ty check .

# Docs (root pyproject.toml/uv.lock — separate from the component's)
uv run mkdocs serve              # local preview
uv run mkdocs build --strict     # what CI runs
```

CI (`.github/workflows/`): `go.yml` (gofmt/vet/golangci-lint/build/test
-race), `python.yml` (ruff/ty, scoped to `components/python/**`),
`docs.yml` (build + deploy to GitHub Pages on push to main), and
`devcontainer.yml` (builds and, on main, pushes a GHCR cache image used by
`devcontainer.json`'s `build.cacheFrom`).

## Testing bar

This is meant to be production-grade. No component should ship with zero
tests, and error paths (malformed input, closed connections, timeouts,
negative responses) need coverage, not just the happy path — see
`pkg/uds/client_test.go` for the pattern (raw scripted ECU responses via a
direct `isotp.Conn`, not just `FakeECU`, for wire-level failure modes).
Run `go test -race`, not just `go test` — several components use
background goroutines (e.g. `uds.Client.StartKeepAlive`) where a missed
mutex is a race, not a crash.

## Git workflow

`main` requires PRs (branch protection). For every change:

```sh
git checkout -b <type>/<short-description> main
# ... commit ...
git push -u origin <branch>
gh pr create --title "..." --body "..."   # include "Fixes #N" if closing an issue
gh pr merge --squash --delete-branch <branch>
git checkout main && git pull
```

Open a GitHub issue first for anything nontrivial, and reference it
(`Fixes #N`) in the commit/PR so it closes on merge. Labels already exist
for area (`area:uds`, `area:numaflow`, `area:ci`, ...) and priority
(`priority:high/medium/low/future`) — reuse them rather than inventing
new ones.

## Conventions worth knowing

- `pkg/` has no dependency on Numaflow or any component — protocol logic
  is fully swappable/testable independent of the pipeline runtime.
- Components are thin adapters between an SDK interface (`sourcer.Sourcer`,
  `mapper.Mapper`, `sinker.Sinker`) and `pkg/uds`. Don't put protocol logic
  in a component's `main.go`.
- `uds.Client.Request` serializes wire access via an internal mutex — safe
  to call concurrently (e.g. a keep-alive goroutine alongside a transfer),
  each call just queues behind whatever's in flight.
- No real CAN hardware exists yet anywhere in this repo or its tests.
  Everything is validated against `can/simbus` + `uds.FakeECU`. Don't
  claim something works against real hardware unless it's actually been
  run against real hardware — see
  [docs/readiness-audit.md](docs/readiness-audit.md) and keep it current
  when that changes.
