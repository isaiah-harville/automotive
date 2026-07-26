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

## Kubeconfig isolation

`devcontainer.json` exports `KUBECONFIG` as `.devcontainer/.kube/config`
(gitignored) via `containerEnv`, for every shell in the container — instead
of the default `~/.kube/config`. This is deliberate: `minikube start`/
`minikube profile` write into whatever kubeconfig is active and switch its
`current-context`, which would otherwise silently repoint `kubectl` at this
throwaway cluster — or merge a new context into a kubeconfig you didn't
expect, if these scripts are ever run outside the devcontainer (`scripts/
up-cluster.sh` and `scripts/deploy-local.sh` also set it explicitly
themselves, so they're still safe run that way). Because it's scoped via
`containerEnv`, this only ever affects shells inside this disposable
container — it never touches a kubeconfig on your host, and `minikube
delete --profile automotive` cleans up completely by just deleting that
file along with the profile.

Outside the devcontainer, point your own `kubectl`/`minikube` at this
cluster directly with:

```sh
export KUBECONFIG=".devcontainer/.kube/config"
```

## Bring up the cluster

```sh
scripts/cluster.sh start
```

`scripts/cluster.sh` is a thin lifecycle wrapper (`start` / `stop` / `restart`
/ `status` / `delete`) around the scripts below. `start` creates a Minikube
profile named `automotive` (isolated from any other local cluster you
have), enables the metrics server, installs the pinned Numaflow release
plus its default JetStream inter-step buffer into the `numaflow-system`
namespace, and starts `scripts/port-forward-numaflow.sh` in the background
so the Numaflow UI stays reachable without a manual `kubectl port-forward`.
`stop` stops both the Minikube profile and that port-forward; `status`
reports on both.

Under the hood, `start` just runs `scripts/up-cluster.sh` directly, which
you can also invoke yourself.

## Deploy the example pipeline

```sh
scripts/deploy-local.sh
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

The Numaflow UI is kept available at http://localhost:8443 by the
background port-forward `up-cluster.sh` starts (see
`scripts/port-forward-numaflow.sh`; its log is
`.devcontainer/.kube/port-forward.log`). It retries automatically if the
forward drops. To run it yourself instead:

```sh
kubectl -n numaflow-system port-forward deployment/numaflow-server 8443:8443
```

## Tear down

```sh
scripts/cluster.sh stop    # or: scripts/cluster.sh delete
```

Both stop the background port-forward too. (The raw `minikube stop
--profile automotive` / `minikube delete --profile automotive` still work,
but leave the port-forward loop running - it just idles, retrying every
5s, until the cluster comes back or you kill it yourself: `kill "$(cat
.devcontainer/.kube/port-forward.pid)"`.)

## What this isn't (yet)

This bootstrap is reproducible but **not air-gapped** — `up-cluster.sh`
downloads the pinned Numaflow manifests and pulls its container images from
the internet each time. A real plant packaging step (mirroring images and
manifests locally, offline install) doesn't exist yet. Fleet management
across multiple plant clusters is also out of scope for now — this is
single-cluster, single-developer tooling.
