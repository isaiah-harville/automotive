#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

# Must match the KUBECONFIG cluster-up.sh created - keeps this script off the
# caller's real ~/.kube/config too.
export KUBECONFIG="${repo_root}/.devcontainer/.kube/config"

profile="${MINIKUBE_PROFILE:-automotive}"

if ! minikube status --profile "${profile}" >/dev/null 2>&1; then
  printf '%s\n' \
    "Minikube profile '${profile}' is not running." \
    "Start it with .devcontainer/scripts/cluster-up.sh." >&2
  exit 1
fi

cd "${repo_root}"
minikube profile "${profile}"

for component in cansource udsflasher resultsink; do
  minikube image build \
    --profile "${profile}" \
    --tag "automotive-flow/${component}:latest" \
    --file "components/go/${component}/Dockerfile" \
    .
done

minikube image build \
  --profile "${profile}" \
  --tag automotive-flow/report-formatter:latest \
  --file components/python/report-formatter/Dockerfile \
  components/python/report-formatter

kubectl apply --filename pipelines/examples/jobs-configmap.yaml
kubectl apply --filename pipelines/examples/flash-basic.yaml

printf '%s\n' \
  "The flash-basic pipeline has been deployed to '${profile}'." \
  "Inspect it with: kubectl get pipeline,pods" \
  "Open the Numaflow UI with: kubectl -n numaflow-system port-forward deployment/numaflow-server 8443:8443"

