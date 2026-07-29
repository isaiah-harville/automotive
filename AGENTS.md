# AGENTS.md

Guidance for Codex and other coding agents working in this repository.

## What this is

A library of [Numaflow](https://numaflow.numaproj.io/) pipeline components
for automotive plant ECU flashing over CAN/UDS. Layering:

```text
pkg/can      transport-agnostic CAN bus (Bus interface)
  simbus       in-process simulated bus + scriptable FakeECU (no hardware needed)
  socketcan    real Linux SocketCAN — stub only, not implemented
pkg/isotp    ISO-TP (ISO 15765-2) framing over can.Bus
pkg/uds      UDS (ISO 14229) client built on pkg/isotp
components/  Numaflow vertices (Go and Python) that call into pkg/
pipelines/   example Pipeline YAML wiring components together
docs/        curated MkDocs Material site (see below)
```

Full details: [docs/architecture.md](docs/architecture.md) and
[docs/glossary.md](docs/glossary.md) for CAN/UDS and Numaflow terminology.

## Licensing — read before touching this

This repository is public but source-available, not open source. The absence
of a LICENSE file is intentional: default GitHub terms apply (view/fork only,
with no reuse rights). It is a commercial codebase. Do not:

- Add an OSS license file.
- Write documentation implying reuse or redistribution rights.
- Publish this module to a public package registry (pkg.go.dev, PyPI, npm,
  and similar registries).
- Add a CONTRIBUTING.md aimed at external contributors.

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

CI (`.github/workflows/`) includes:

- `go.yml`: gofmt, vet, golangci-lint, build, and race-enabled tests.
- `python.yml`: ruff and ty, scoped to `components/python/**`.
- `docs.yml`: strict build and GitHub Pages deployment on pushes to `main`.
- `devcontainer.yml`: builds and, on `main`, pushes the GHCR cache image
  referenced by `devcontainer.json`'s `build.cacheFrom`.

## Testing bar

This is intended to be production-grade. No component should ship without
tests. Cover error paths such as malformed input, closed connections,
timeouts, and negative responses—not only the happy path. Follow
`pkg/uds/client_test.go` for wire-level failure coverage: use raw scripted ECU
responses through a direct `isotp.Conn`, rather than relying only on
`FakeECU`.

Run `go test -race`, not only `go test`. Several components use background
goroutines, such as `uds.Client.StartKeepAlive`, where a missed mutex causes
a race rather than an immediate crash.

## Git workflow

`main` requires pull requests through branch protection. For every change:

```sh
git checkout -b <type>/<short-description> main
# ... commit ...
git push -u origin <branch>
gh pr create --title "..." --body "..."   # include "Fixes #N" when closing an issue
gh pr merge --squash --delete-branch <branch>
git checkout main
git pull
```

Open a GitHub issue first for any nontrivial change and reference it with
`Fixes #N` in the commit or pull request so it closes on merge. Reuse the
existing area labels (`area:uds`, `area:numaflow`, `area:ci`, and similar)
and priority labels (`priority:high`, `priority:medium`, `priority:low`, and
`priority:future`) instead of inventing new ones.

## Conventions worth knowing

- `pkg/` has no dependency on Numaflow or any component. Protocol logic must
  remain swappable and testable independently of the pipeline runtime.
- Components are thin adapters between an SDK interface
  (`sourcer.Sourcer`, `mapper.Mapper`, or `sinker.Sinker`) and `pkg/uds`.
  Do not put protocol logic in a component's `main.go`.
- `uds.Client.Request` serializes wire access through an internal mutex. It
  is safe to call concurrently, such as from a keep-alive goroutine during a
  transfer; each call queues behind the request in flight.
- No real CAN hardware exists in this repository or its tests. Validation
  uses `can/simbus` and `uds.FakeECU`. Do not claim real-hardware support
  unless it has actually been tested. See `docs/readiness-audit.md` and keep
  it current when that changes.
