# Run the Example Flashing Mission

The `flash-basic` example is a complete simulated flashing mission. It reads
two sample jobs, flashes an in-process fake ECU, formats successful results,
and sends failures to a separate dead-letter sink:

```text
                               ┌─> report-formatter ─> resultsink
cansource ─> udsflasher ───────┤
                               └─> deadlettersink
```

No CAN hardware is required. The example uses `CAN_MODE=sim`, `can/simbus`,
and `uds.FakeECU`.

## Before you start

Open the repository in its devcontainer. In VS Code, use **Dev Containers:
Reopen in Container**. The container includes Docker, Minikube, `kubectl`,
Go, Python, and `uv`.

Allocate at least 4 CPUs, 8 GB of memory, and 30 GB of disk space to the
Docker runtime used by the devcontainer. The first run needs internet access
to download the pinned Numaflow manifests and container images.

Run every command below from the repository root inside the devcontainer.

## 1. Start the local cluster

```sh
scripts/cluster.sh start
```

This creates or resumes the isolated `automotive` Minikube profile, installs
Numaflow and its JetStream inter-step buffer, and starts a background
port-forward for the Numaflow UI.

Confirm that the cluster and UI forward are running:

```sh
scripts/cluster.sh status
kubectl get pods --namespace numaflow-system
```

Wait until the Numaflow system pods are ready before continuing. The UI is
available at <http://localhost:8443>.

## 2. Deploy the mission

```sh
scripts/deploy-local.sh
```

The script builds the four component images directly inside Minikube and
applies:

- `pipelines/examples/jobs-configmap.yaml`
- `pipelines/examples/flash-basic.yaml`

The first build can take several minutes. No image registry login is needed.

## 3. Watch it start

```sh
kubectl get pipeline flash-basic
kubectl get pods \
  --selector numaflow.numaproj.io/pipeline-name=flash-basic \
  --watch
```

Stop the watch with <kbd>Ctrl</kbd>+<kbd>C</kbd> after the pipeline pods are
running. You can also inspect the `flash-basic` pipeline in the Numaflow UI.

If a pod does not start, inspect it before redeploying:

```sh
kubectl describe pod <pod-name>
kubectl get events --sort-by=.lastTimestamp
```

## 4. Confirm the result

The checked-in source data contains `job-001` and `job-002`. View all mission
logs with:

```sh
kubectl logs \
  --selector numaflow.numaproj.io/pipeline-name=flash-basic \
  --all-containers \
  --prefix \
  --tail=200
```

A successful run produces a formatted result for each job similar to:

```text
[OK] job=job-001 ecu=ecm-01 flashed in 12ms
[OK] job=job-002 ecu=ecm-02 flashed in 8ms
```

Durations vary. Successful results follow the `flash-success` edge through
`report-formatter` to `resultsink`.

Jobs that fail validation or exhaust their retry policy follow the
`flash-dead-letter` edge to `deadlettersink`. Those log entries contain the
original `FlashJob`, the final error, and the attempt count so the job can be
inspected and replayed.

## Run the mission again

`cansource` reads the finite job file once. Recreate the pipeline to start a
fresh mission with the same jobs:

```sh
kubectl delete --filename pipelines/examples/flash-basic.yaml
kubectl apply --filename pipelines/examples/flash-basic.yaml
```

Watch the new pods and logs using the commands above.

## Use different sample jobs

Edit `pipelines/examples/jobs.jsonl`, then update the live ConfigMap:

```sh
kubectl create configmap automotive-flow-jobs \
  --from-file=jobs.jsonl=pipelines/examples/jobs.jsonl \
  --dry-run=client \
  --output=yaml \
  | kubectl apply --filename -
```

Recreate the pipeline after updating the ConfigMap so `cansource` starts
again and loads the new file.

The job schema is documented in [Message Contracts](contracts.md). Keep
`CAN_MODE=sim` while using the fake ECU; selecting `socketcan` requires a
Linux SocketCAN interface and appropriate CAN hardware.

## Stop or remove the environment

Stop the cluster while preserving it for a later run:

```sh
scripts/cluster.sh stop
```

Delete the local cluster and its isolated kubeconfig:

```sh
scripts/cluster.sh delete
```

Deleting the cluster removes its locally built component images. The next
mission will rebuild them when `scripts/deploy-local.sh` runs.

## Quick troubleshooting

| Symptom | Check |
|---|---|
| `Minikube profile 'automotive' is not running` | Run `scripts/cluster.sh start`. |
| Numaflow UI does not open | Run `scripts/cluster.sh status`, then inspect `.devcontainer/.kube/port-forward.log`. |
| Pipeline pods show `ImagePullBackOff` | Rerun `scripts/deploy-local.sh` so the images are built inside the correct Minikube profile. |
| No result logs appear | Check every pipeline pod with the all-container log command above; then inspect `cansource`, `udsflasher`, and `deadlettersink` individually in the UI. |
| A job reaches `deadlettersink` | Read its final error and attempt count. Correct the input or underlying transport problem, update the ConfigMap if needed, and recreate the pipeline. |
