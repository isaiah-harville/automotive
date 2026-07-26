#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

# Isolate this cluster's kubeconfig from the caller's real ~/.kube/config so
# running this script (in the devcontainer or, accidentally, on the host)
# never merges in a new context or flips someone's current-context out from
# under them. See docs/local-deployment.md.
export KUBECONFIG="${repo_root}/.devcontainer/.kube/config"
mkdir -p "$(dirname "${KUBECONFIG}")"

profile="${MINIKUBE_PROFILE:-automotive}"
cpus="${MINIKUBE_CPUS:-4}"
memory="${MINIKUBE_MEMORY:-8192}"
numaflow_version="${NUMAFLOW_VERSION:-v1.7.5}"
numaflow_base_url="https://raw.githubusercontent.com/numaproj/numaflow/${numaflow_version}"

if ! minikube status --profile "${profile}" >/dev/null 2>&1; then
  minikube start \
    --profile "${profile}" \
    --driver docker \
    --container-runtime containerd \
    --cpus "${cpus}" \
    --memory "${memory}" \
    --disk-size 30g
fi

minikube profile "${profile}"
minikube addons enable metrics-server --profile "${profile}"

kubectl create namespace numaflow-system \
  --dry-run=client \
  --output yaml \
  | kubectl apply -f -

kubectl apply \
  --namespace numaflow-system \
  --filename "${numaflow_base_url}/config/install.yaml"

kubectl wait \
  --for condition=Established \
  --timeout 3m \
  customresourcedefinition/pipelines.numaflow.numaproj.io

kubectl wait \
  --namespace numaflow-system \
  --for condition=Available \
  --timeout 10m \
  deployment \
  --all

kubectl apply \
  --filename "${numaflow_base_url}/examples/0-isbsvc-jetstream.yaml"

printf '%s\n' \
  "Local cluster '${profile}' is ready." \
  "Numaflow ${numaflow_version} is installed with the default JetStream buffer." \
  "Its kubeconfig is isolated at ${KUBECONFIG} - it will not touch ~/.kube/config." \
  "Deploy this project with .devcontainer/scripts/deploy-local.sh." \
  "To use kubectl/minikube against it directly: export KUBECONFIG=\"${KUBECONFIG}\""

