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

pid_file="${repo_root}/.devcontainer/.kube/port-forward.pid"
log_file="${repo_root}/.devcontainer/.kube/port-forward.log"
if [[ -f "${pid_file}" ]] && kill -0 "$(cat "${pid_file}")" 2>/dev/null; then
  port_forward_status="already running (pid $(cat "${pid_file}"))"
else
  nohup "${repo_root}/scripts/port-forward-numaflow.sh" \
    >"${log_file}" 2>&1 &
  disown
  echo "$!" >"${pid_file}"
  port_forward_status="started (pid $!, log at ${log_file})"
fi

printf '%s\n' \
  "Local cluster '${profile}' is ready." \
  "Numaflow ${numaflow_version} is installed with the default JetStream buffer." \
  "Its kubeconfig is isolated at ${KUBECONFIG} - it will not touch ~/.kube/config." \
  "This is also exported as KUBECONFIG for every devcontainer shell (see devcontainer.json)." \
  "Deploy this project with scripts/deploy-local.sh." \
  "Numaflow UI port-forward: ${port_forward_status} - open http://localhost:8443"
