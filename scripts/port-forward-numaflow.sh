#!/usr/bin/env bash
set -uo pipefail

repo_root="$(git rev-parse --show-toplevel)"
export KUBECONFIG="${repo_root}/.devcontainer/.kube/config"

profile="${MINIKUBE_PROFILE:-automotive}"

# Long-running: keeps the Numaflow UI reachable at localhost:8443 for the
# life of the devcontainer, restarting the forward whenever it drops (pod
# restart, minikube pause, etc). Launched in the background by
# up-cluster.sh (and `scripts/cluster.sh start`); not meant to be run in the
# foreground. `scripts/cluster.sh stop` sends SIGTERM here, so the trap below
# has to kill the in-flight kubectl child too - otherwise it leaks past
# this script exiting.
child_pid=""
cleanup() {
  [[ -n "${child_pid}" ]] && kill "${child_pid}" 2>/dev/null
  exit 0
}
trap cleanup TERM INT

while true; do
  if ! minikube status --profile "${profile}" >/dev/null 2>&1; then
    sleep 5
    continue
  fi

  if ! kubectl wait \
    --namespace numaflow-system \
    --for condition=Available \
    --timeout 30s \
    deployment/numaflow-server >/dev/null 2>&1; then
    sleep 5
    continue
  fi

  kubectl --namespace numaflow-system port-forward \
    --address 0.0.0.0 \
    deployment/numaflow-server 8443:8443 &
  child_pid=$!
  wait "${child_pid}"
  child_pid=""
  sleep 2
done
