# Local Deployment

There's no shared/plant Kubernetes cluster wired up for this repo yet. The
`.devcontainer/` setup gives you an isolated local one via Minikube, so you
can actually run the `flash-basic` pipeline end-to-end rather than just
reading the YAML.

## Prerequisites

Open the repo in the devcontainer (VS Code "Reopen in Container", or any
devcontainer-compatible tool). It provisions Ubuntu 24.04 with Go, Python +
`uv`, Docker-in-Docker, `kubectl`, Helm, Minikube, and `can-utils`. On first
create, `.devcontainer/scripts/post-create.sh` runs `go mod download` and
`uv sync --locked` for the Python component automatically.

The Minikube cluster itself is **not** created automatically — it needs at
least 4 CPUs and 8 GB memory, so it's opt-in.

## Bring up the cluster

```sh
.devcontainer/scripts/cluster-up.sh
```

This creates a Minikube profile named `automotive` (isolated from any other
local cluster you have), enables the metrics server, and installs the
pinned Numaflow release plus its default JetStream inter-step buffer into
the `numaflow-system` namespace.

## Deploy the example pipeline

```sh
.devcontainer/scripts/deploy-local.sh
```

This builds all four component images (`cansource`, `udsflasher`,
`report-formatter`, `resultsink`) directly into the Minikube cluster's image
store — no central image registry needed for local operation — then applies
`pipelines/examples/jobs-configmap.yaml` and
`pipelines/examples/flash-basic.yaml`.

## Inspect it

```sh
kubectl get pipeline,pods
kubectl logs -l numaflow.numaproj.io/pipeline-name=flash-basic --all-containers
```

The Numaflow UI is available via port-forward:

```sh
kubectl -n numaflow-system port-forward deployment/numaflow-server 8443:8443
```

## Tear down

```sh
minikube stop --profile automotive
minikube delete --profile automotive
```

## What this isn't (yet)

This bootstrap is reproducible but **not air-gapped** — `cluster-up.sh`
downloads the pinned Numaflow manifests and pulls its container images from
the internet each time. A real plant packaging step (mirroring images and
manifests locally, offline install) doesn't exist yet. Fleet management
across multiple plant clusters is also out of scope for now — this is
single-cluster, single-developer tooling.
