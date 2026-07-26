# Local plant development environment

This development container uses Ubuntu 24.04 and includes Go 1.23, Python
3.13, `uv`, Docker-in-Docker, `kubectl`, Helm, Minikube, CAN utilities, and
the matching VS Code extensions.

## Build caching

`devcontainer.json` still builds the Dockerfile and applies all the
features itself (git, Docker-in-Docker, Go, Python, `uv`,
kubectl/Helm/Minikube) — nothing is stripped out or replaced with a
prebuilt image. What makes that fast is `build.cacheFrom` in
`devcontainer.json`: it points at
`ghcr.io/isaiah-harville/automotive/devcontainer:latest`, so both VS
Code's "Reopen in Container" and `.github/workflows/devcontainer.yml`
pull that image's layers as a build cache instead of rebuilding
everything from scratch. A cache hit means "download," not "wait for
`apt-get`/feature installers to run again."

The workflow builds and pushes an updated cache image on every push to
`main` that touches `.devcontainer/**`, tagged `latest` and the commit
SHA, using [`devcontainers/ci`](https://github.com/devcontainers/ci)'s own
`cacheFrom`/`cacheTo` handling so CI runs get faster over time too. Pull
requests touching `.devcontainer/**` only build (using the existing cache)
to confirm the Dockerfile/features still work — they don't push, since
only `main` is trusted to publish.

**One-time setup note:** the first time this workflow runs, the resulting
GHCR package is private by default even though this repo is public — an
org owner needs to flip its visibility to public (or grant read access)
in the package settings, or the cache pull will fail with a permission
error (falling back to a full local build, not a hard failure, but worth
fixing).

The local cluster is deliberately created on demand because it needs at
least four CPUs and 8 GB of memory:

```sh
.devcontainer/scripts/cluster-up.sh
.devcontainer/scripts/deploy-local.sh
```

`cluster-up.sh` creates an isolated Minikube cluster named `automotive`,
enables Metrics Server, and installs the pinned Numaflow release and its
default JetStream inter-step buffer. `deploy-local.sh` builds all component
images directly into that cluster, so a plant does not need a central image
registry for local operation.

Both scripts scope `KUBECONFIG` to `.devcontainer/.kube/config` (gitignored)
instead of the default `~/.kube/config`, so they never touch or switch the
current-context of any kubeconfig you already have — see [Local
Deployment](../docs/local-deployment.md#kubeconfig-isolation).

Useful commands:

```sh
kubectl get pipeline,pods
kubectl logs -l numaflow.numaproj.io/pipeline-name=flash-basic --all-containers
minikube stop --profile automotive
minikube delete --profile automotive
```

The cluster bootstrap is reproducible but not yet air-gapped: it downloads
the pinned Numaflow manifests and container images. A future plant packaging
step should mirror those images and manifests locally before treating this
as an offline deployment bundle. Fleet update management is intentionally
out of scope for now.
