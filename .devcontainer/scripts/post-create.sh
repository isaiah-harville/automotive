#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"

cd "${repo_root}"
go mod download

cd "${repo_root}/components/python/report-formatter"
uv sync --locked

printf '%s\n' \
  "Development dependencies are ready." \
  "Run scripts/cluster.sh start to create the local plant cluster." \
  "Run scripts/deploy-local.sh to build and deploy the pipeline."

