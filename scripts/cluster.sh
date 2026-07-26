#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
export KUBECONFIG="${repo_root}/.devcontainer/.kube/config"

profile="${MINIKUBE_PROFILE:-automotive}"
pid_file="${repo_root}/.devcontainer/.kube/port-forward.pid"

usage() {
  printf '%s\n' \
    "Usage: scripts/cluster.sh <command>" \
    "" \
    "Commands:" \
    "  start    Create/resume the Minikube cluster, install Numaflow, and start the UI port-forward" \
    "  stop     Stop the Minikube cluster and the UI port-forward" \
    "  restart  stop, then start" \
    "  status   Show Minikube and port-forward status" \
    "  delete   Delete the Minikube cluster, its kubeconfig, and stop the port-forward" >&2
}

stop_port_forward() {
  if [[ -f "${pid_file}" ]] && kill -0 "$(cat "${pid_file}")" 2>/dev/null; then
    kill "$(cat "${pid_file}")"
  fi
  rm -f "${pid_file}"
}

cmd_start() {
  exec "${repo_root}/scripts/up-cluster.sh"
}

cmd_stop() {
  stop_port_forward
  minikube stop --profile "${profile}"
}

cmd_status() {
  minikube status --profile "${profile}" || true
  if [[ -f "${pid_file}" ]] && kill -0 "$(cat "${pid_file}")" 2>/dev/null; then
    echo "port-forward: running (pid $(cat "${pid_file}")) - http://localhost:8443"
  else
    echo "port-forward: not running"
  fi
}

cmd_delete() {
  stop_port_forward
  minikube delete --profile "${profile}"
  rm -f "${KUBECONFIG}"
}

case "${1:-}" in
  start)
    cmd_start
    ;;
  stop)
    cmd_stop
    ;;
  restart)
    cmd_stop
    cmd_start
    ;;
  status)
    cmd_status
    ;;
  delete)
    cmd_delete
    ;;
  *)
    usage
    exit 1
    ;;
esac
