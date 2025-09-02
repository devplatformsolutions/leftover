# Leftover

> **Eat the cloud’s leftovers — autoscale on spare GPUs.**
> Leftover is a Kubernetes operator that discovers the **cheapest available GPU Spot capacity** and generates/updates **Karpenter** NodePools + EC2NodeClass for cost‑efficient autoscaling.

[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](#license)
![Status: Alpha](https://img.shields.io/badge/status-alpha-orange)
![Kubernetes](https://img.shields.io/badge/kubernetes-1.27%2B-informational)
![Go](https://img.shields.io/badge/Go-1.21%2B-informational)

---

## Why Leftover?

Clouds constantly have **spare, volatile GPU capacity**. Most teams ignore it because it’s unpredictable. **Leftover** hunts for that capacity, ranks it by **price per GPU** and **likelihood of fulfillment**, and feeds it to **Karpenter** as NodePools that your workloads can use — automatically and declaratively.

**Key benefits**

* 💸 **Save money**: Prefer cheapest GPU Spot across instance families & AZs.
* ⚡ **Autoscale fast**: Emits Karpenter NodePools configured for Spot.
* 🧠 **Smart selection**: Ranks by **price/GPU** and **spot placement scores**.
* 🧱 **Declarative**: A single CRD (`LeftoverNodePool`) is your intent; the operator reconciles the rest.
* 🔒 **Enterprise‑friendly**: Apache 2.0, IRSA support, clean RBAC.

---

## High‑level Architecture

```
+-------------------+          +-------------------------------+
| Leftover CR (CRD) |  --->    | Leftover Operator (Reconcile) |
| LeftoverNodePool  |          | - DescribeInstanceTypes       |
| gpu.devplatforms.io/v1alpha1 | - SpotPriceHistory            |
+-------------------+          | - GetSpotPlacementScores      |
                               | - Rank & Select               |
                               | - Render Karpenter objects    |
                               +-------------------------------+
                                             |
                                             v
                                +----------------------------+
                                |  Karpenter NodePool        |
                                |  karpenter.sh/v1           |
                                |  + EC2NodeClass (AWS)      |
                                |  karpenter.k8s.aws/v1beta1 |
                                +----------------------------+
```

---

## Prerequisites

* **Kubernetes** 1.27+
* **Karpenter** installed and configured in your cluster
* **AWS EKS** (recommended) or self‑managed cluster with AWS credentials
* **IAM (IRSA)** for the operator ServiceAccount with:

  * `ec2:Describe*`
  * `ec2:GetSpotPlacementScores`
  * *(optional)* `pricing:GetProducts` (if you want OD price comparisons)
* (For webhooks) **cert‑manager** in‑cluster

---

## Quickstart

### 1) Install CRDs locally (dev loop)

```bash
make install
```

### 2) Run the operator locally (without webhooks)

```bash
ENABLE_WEBHOOKS=false make run
```

> Why? Local runs don’t provision TLS certs for the webhook server. Disable during development.
> For full webhook testing, deploy in‑cluster (below).

### 3) Apply a minimal `LeftoverNodePool`

```yaml
# save as quick-test.yaml
apiVersion: gpu.devplatforms.io/v1alpha1
kind: LeftoverNodePool
metadata:
  name: gpu-spot-euc1
spec:
  region: eu-central-1
  families: ["g5","g6","p5"]
  capacityType: spot
  targetCount: 2
  maxInstanceTypes: 5
  maxZones: 2
  budgetsNodes: "10%"
  consolidateAfter: "2m"
  labels:
    workload: gpu
    nvidia.com/gpu.present: "true"
  taints:
    - "nvidia.com/gpu=true:NoSchedule"
  subnetSelectorTags:
    kubernetes.io/role/internal-elb: "1"
    kubernetes.io/cluster/your-cluster: owned
  securityGroupSelectorTags:
    kubernetes.io/cluster/your-cluster: owned
```

```bash
kubectl apply -f quick-test.yaml
kubectl describe leftovernodepool gpu-spot-euc1
```

Leftover will reconcile and create/update:

* `NodePool` (api: `karpenter.sh/v1`)
* `EC2NodeClass` (api: `karpenter.k8s.aws/v1beta1`)

named after your resource (e.g., `leftover-gpu-spot-euc1`, `leftover-gpu-spot-euc1-class`).

---

## In‑Cluster Deployment (with webhooks)

1. Install **cert‑manager** (once per cluster):

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl -n cert-manager wait deploy/cert-manager-webhook --for=condition=Available --timeout=120s
```

2. Build & deploy the operator:

```bash
make docker-build IMG=ghcr.io/devplatformsolutions/leftover:dev
make docker-push  IMG=ghcr.io/devplatformsolutions/leftover:dev
make deploy       IMG=ghcr.io/devplatformsolutions/leftover:dev
```

3. Apply your `LeftoverNodePool` (cluster‑scoped; no namespace):

```bash
kubectl apply -f config/samples/gpu_v1alpha1_leftovernodepool.yaml
kubectl get leftovernodepools.gpu.devplatforms.io
```

---

## CRD: `LeftoverNodePool` (Spec)

**Group/Version**: `gpu.devplatforms.io/v1alpha1`
**Scope**: Cluster

| Field                       | Type               | Default | Description                                                                  |
| --------------------------- | ------------------ | ------- | ---------------------------------------------------------------------------- |
| `region`                    | string             | —       | AWS region (e.g., `eu-central-1`). **Required**                              |
| `families`                  | \[]string          | `[]`    | Allowed instance families (e.g., `['g5','g6','p5']`). Empty = any GPU family |
| `minGPUs`                   | int                | `1`     | Minimum GPU count per instance                                               |
| `targetCount`               | int32              | `2`     | Target node count for spot placement scoring                                 |
| `maxInstanceTypes`          | int                | `5`     | Max instance types to include in NodePool                                    |
| `maxZones`                  | int                | `2`     | Max AZs to allow                                                             |
| `capacityType`              | string             | `spot`  | `spot` or `on-demand`                                                        |
| `subnetSelectorTags`        | map\[string]string | —       | Selector tags for EC2 subnets                                                |
| `securityGroupSelectorTags` | map\[string]string | —       | Selector tags for EC2 security groups                                        |
| `budgetsNodes`              | string             | `"10%"` | Karpenter disruption budget (percentage)                                     |
| `consolidateAfter`          | string (duration)  | `"2m"`  | Consolidation delay for Karpenter                                            |
| `labels`                    | map\[string]string | `{}`    | Node labels                                                                  |
| `taints`                    | \[]string          | `[]`    | Node taints (e.g., `nvidia.com/gpu=true:NoSchedule`)                         |
| `requeueMinutes`            | int                | `7`     | Periodic reconcile interval                                                  |
| `onDemandFallback`          | bool               | `true`  | If no viable Spot, optionally create OD NodePool                             |

**Status**

* `selectedInstanceTypes`: \[]string
* `selectedZones`: \[]string
* `lastSyncTime`: time
* `conditions`: \[]Condition (`Ready`, `Degraded`, …)

---

## How Selection Works (MVP)

1. **Discover GPU instance types** (EC2 `DescribeInstanceTypes`) filtered by families & GPU count.
2. **Fetch recent Spot quotes** (EC2 `DescribeSpotPriceHistory`, last \~10m).
3. **Get placement scores** (`GetSpotPlacementScores`) for `targetCount`.
4. **Rank** by **price per GPU**, then **score**, then absolute price.
5. **Select** up to `maxInstanceTypes` & `maxZones`.
6. **Render** Karpenter `NodePool` + `EC2NodeClass` and apply with Server‑Side Apply (SSA).

> Roadmap includes hysteresis (avoid flapping), optional on‑demand fallback, caching, and metrics.

---

## Development

Local dev without webhooks:

```bash
ENABLE_WEBHOOKS=false make run
```

With webhooks (in‑cluster):

```bash
# ensure cert-manager is installed, then
make deploy IMG=ghcr.io/devplatformsolutions/leftover:dev
```

Regenerate code & manifests after editing API types or webhooks:

```bash
make generate
make manifests
```

---

## IAM (IRSA) Template (Sketch)

Attach to the operator’s ServiceAccount:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": [ "ec2:Describe*", "ec2:GetSpotPlacementScores" ], "Resource": "*" },
    { "Effect": "Allow", "Action": [ "pricing:GetProducts" ], "Resource": "*" }
  ]
}
```

---

## Compatibility

* **Karpenter**: v1 API (`NodePool`) and AWS provider `EC2NodeClass` v1beta1
* **AWS Regions**: any where Spot + desired GPU families are available

---

## Roadmap

* ✅ CRD, defaulting/validation webhooks (cluster‑scoped)
* ⏭️ MVP reconcile: rank & render Karpenter manifests
* ⏭️ Caching of AWS calls (5–10 min)
* ⏭️ Hysteresis (price/score thresholds)
* ⏭️ Optional On‑Demand fallback NodePool
* ⏭️ Prometheus metrics & dashboards
* ⏭️ Helm chart
* ⏭️ Multi‑cluster/global optimization

---

## Contributing

PRs welcome! Please open an issue to discuss major changes.
Run `make test` before submitting (controller + webhook unit tests, envtest coming soon).

---

## License

Licensed under the **Apache License, Version 2.0**.
See [LICENSE](./LICENSE) for details.
