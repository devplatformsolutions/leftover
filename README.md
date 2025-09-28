# Leftover

> **Eat the cloud’s leftovers — autoscale on spare GPUs.**
> Leftover is a Kubernetes operator that discovers the **cheapest available GPU Spot capacity** and updates a **single Karpenter NodePool** pointing at the currently preferred (instance type, AZ) pair.

[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](#license)
![Status: Alpha](https://img.shields.io/badge/status-alpha-orange)
![Kubernetes](https://img.shields.io/badge/kubernetes-1.27%2B-informational)
![Go](https://img.shields.io/badge/Go-1.21%2B-informational)

---

## Current MVP Behavior

The present code (alpha) picks exactly ONE best (instanceType, availabilityZone) for a `LeftoverNodePool` at each reconciliation based on:
1. GPU instance type discovery (families + min GPU filter)
2. Recent Spot price quotes (last ~10 minutes)
3. Spot placement scores (AZ “score” for capacity)
4. Price (ascending) scanned in small batches until a score threshold is met (`minSpotScore`), else the absolute cheapest

It then Server‑Side Applies a single `NodePool` (`leftover-<crName>`) with strict `requirements` limiting scheduling to that instance type & zone.

Planned (not yet implemented in code despite spec fields existing):
* Multiple fallback instance types / zones (`maxInstanceTypes`, `maxZones`)
* On‑demand fallback (`onDemandFallback`)
* Passing labels/taints/budgets into the rendered NodePool
* Subnet / security group selectors auto-wiring the EC2NodeClass
* Hysteresis / flapping avoidance

---

## Why Leftover?

* 💸 Reduce GPU cost by always chasing the currently cheapest viable Spot option meeting a score threshold.
* 🧠 Incorporates AWS Spot placement scoring (capacity risk signal).
* 🧱 Declarative intent via a CRD (`LeftoverNodePool`).
* 🔄 Periodic re-evaluation (`requeueMinutes`, default 7).

---

## High‑level Architecture (MVP)

```
+-------------------------------+
| LeftoverNodePool (CR)         |
| gpu.devplatforms.io/v1alpha1  |
+---------------+---------------+
                |
                v
        Leftover Controller
        - DescribeInstanceTypes
        - DescribeSpotPriceHistory
        - GetSpotPlacementScores
        - Rank & Pick ONE (type, AZ)
        - Patch Karpenter NodePool
                |
                v
+-------------------------------+
| Karpenter NodePool (v1)       |
| (references existing          |
|  EC2NodeClass you provide)    |
+-------------------------------+
```

NOTE: The operator currently expects an existing `EC2NodeClass` (you pass its name via `spec.nodeClassName`). It does not create or mutate the NodeClass yet.

---

## Prerequisites

* Kubernetes 1.27+
* Karpenter (v1 API) installed
* An `EC2NodeClass` in the cluster (you manage it)
* AWS credentials (IRSA recommended) with:
  * `ec2:Describe*`
  * `ec2:GetSpotPlacementScores`
* (Optional roadmap) `pricing:GetProducts`
* For local dev: environment AWS creds (no IMDS)

---

## Install with Helm

This repo includes a starter Helm chart under `charts/leftover`.

Quick start (local chart):

```bash
# 1) Vendor optional deps (cert-manager) if you plan to enable webhooks
helm dependency update charts/leftover

# 2) Install (Secret-based AWS creds, webhooks disabled)
helm upgrade --install leftover charts/leftover \
  --namespace leftover-system --create-namespace \
  --set image.repository=ghcr.io/devplatformsolutions/leftover \
  --set image.tag=latest \
  --set aws.secretName=aws-credentials \
  --set webhooks.enabled=false

# If using IRSA instead of a Secret
#   --set aws.irsaRoleArn=arn:aws:iam::<ACCOUNT_ID>:role/<ROLE>
```

Enable admission webhooks (requires cert-manager):

```bash
helm dependency update charts/leftover
helm upgrade --install leftover charts/leftover \
  -n leftover-system --create-namespace \
  --set webhooks.enabled=true \
  --set certManager.enabled=true
```

Notes:
- CRDs: the chart bundles the `LeftoverNodePool` CRD and installs it by default (`crds.install=true`).
  - To manage CRDs outside the chart, set `--set crds.install=false` and run `make install` (or ship CRDs separately).
- cert-manager: included as an optional chart dependency, gated by `certManager.enabled`.
  - Keep `webhooks.enabled=false` unless cert-manager is present, since the webhook server needs TLS.
- Metrics: by default, metrics are enabled and served on 8443 (HTTPS). Disable with `--set metrics.enabled=false`.

Key values (abridged):
- `image.repository`, `image.tag`, `image.pullPolicy`
- `aws.secretName` (envFrom), `aws.irsaRoleArn` (SA annotation), `aws.disableIMDS`
- `webhooks.enabled`, `certManager.enabled`
- `serviceAccount.create`, `serviceAccount.name`, `serviceAccount.annotations`
- `resources`, `pod.annotations|labels|nodeSelector|tolerations|affinity`
- `crds.install`

---

## Install CRDs (Dev)

```bash
make install
```

Run locally without webhooks:

```bash
ENABLE_WEBHOOKS=false make run
```

---

## Minimal Example

`example/class.yaml` (you provide this manually – adjust role, tags):

```yaml
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: karpenter-quick-test
spec:
  amiFamily: AL2
  role: KarpenterNodeRole-CLUSTER_NAME
  amiSelectorTerms:
    - tags:
        Name: KarpenterNode-CLUSTER_NAME
  subnetSelectorTerms:
    - tags:
        kubernetes.io/cluster/CLUSTER_NAME: owned
  securityGroupSelectorTerms:
    - tags:
        kubernetes.io/cluster/CLUSTER_NAME: owned
```

`example/test.yaml`:

```yaml
apiVersion: gpu.devplatforms.io/v1alpha1
kind: LeftoverNodePool
metadata:
  name: quick-test
spec:
  region: us-east-1
  nodeClassName: karpenter-quick-test
  families: ["g4dn","g4ad","g5"]   # optional; empty = any GPU family discovered
  minGPUs: 4
  minSpotScore: 6                  # score threshold (0-10)
  targetCount: 2                   # used when requesting placement scores
  requeueMinutes: 7                # periodic refresh
  capacityType: spot               # default
```

Apply:

```bash
kubectl apply -f example/class.yaml
kubectl apply -f example/test.yaml
```

After the first reconcile:

```bash
kubectl describe nodepool leftover-quick-test
kubectl get leftovernodepool quick-test -o yaml
```

---

## Generated NodePool (Shape)

The operator patches a NodePool similar to:

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: leftover-quick-test
  labels:
    managed-by: leftover
spec:
  template:
    spec:
      nodeClassRef:
        name: karpenter-quick-test
        group: karpenter.k8s.aws
        kind: EC2NodeClass
      requirements:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["spot"]
        - key: node.kubernetes.io/instance-type
          operator: In
          values: ["g4dn.12xlarge"]
        - key: topology.kubernetes.io/zone
          operator: In
          values: ["us-east-1a"]
```

Karpenter later injects defaults (e.g. disruption / expireAfter).

---

## CRD Spec (Selected Fields)

Implemented (used now):
* `region`
* `families`
* `nodeClassName` (or `nodeClassSelector`)
* `minGPUs`
* `targetCount`
* `minSpotScore`
* `capacityType`
* `requeueMinutes`

Defined but NOT yet acted on (roadmap):
* `maxInstanceTypes`, `maxZones`
* `labels`, `taints`
* `budgetsNodes`, `consolidateAfter`
* `subnetSelectorTags`, `securityGroupSelectorTags`
* `onDemandFallback`

---

## Status Fields

```yaml
status:
  selectedInstanceTypes: ["g4dn.12xlarge"]
  selectedZones: ["us-east-1a"]
  lastPriceUSD: "1.2746"
  lastScore: 9
  lastSyncTime: 2025-09-16T19:04:07Z
  conditions:
    - type: Ready
      status: "True"
      reason: Reconciled
      message: NodePool updated
```

---

## How Selection Works (Detailed)

1. Discover GPU instance types (filter families + minGPUs)
2. Fetch recent Spot price history (window ~10m; latest per (type, AZ))
3. Fetch Spot placement scores (AZ-level; reused for all instance types)
4. Sort quotes by price ascending
5. Scan in windows (batch size 5) until a quote meets `minSpotScore`
6. If none meet score threshold, use absolute cheapest
7. Apply NodePool requirements for that single winning (type, AZ)

---

## Development

Regenerate types / manifests after API edits:

```bash
make generate
make manifests
```

Run tests:

```bash
make test
```

---

## IAM (IRSA) Policy Sketch

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": [ "ec2:Describe*", "ec2:GetSpotPlacementScores" ], "Resource": "*" }
  ]
}
```

Add `pricing:GetProducts` later if OD pricing comparisons are introduced.

## Compatibility

* **Karpenter**: v1 API (`NodePool`) and AWS provider `EC2NodeClass` v1beta1
* **AWS Regions**: any where Spot + desired GPU families are available

---

## Roadmap

* ✅ CRD, defaulting/validation webhooks (cluster‑scoped)
* ✅ MVP reconcile: rank & render Karpenter manifests
* ⏭️ Caching of AWS calls (5–10 min)
* ⏭️ Hysteresis (price/score thresholds)
* ⏭️ Optional On‑Demand fallback NodePool
* ⏭️ Prometheus metrics & dashboards
* ✅ Helm chart
* ⏭️ Multi‑cluster/global optimization

---

## Contributing

PRs welcome! Please open an issue to discuss major changes.
Run `make test` before submitting (controller + webhook unit tests, envtest coming soon).

---

## License

Licensed under the **Apache License, Version 2.0**.
See [LICENSE](./LICENSE) for details.
