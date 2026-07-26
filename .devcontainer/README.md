# Local plant development environment

This development container uses Ubuntu 24.04 and includes Go 1.23, Python
3.13, `uv`, Docker-in-Docker, `kubectl`, Helm, Minikube, CAN utilities, and
the matching VS Code extensions.

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

