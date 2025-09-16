package karpenterx

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
)

var (
	ec2NodeClassGVK = schema.GroupVersionKind{
		Group:   "karpenter.k8s.aws",
		Version: "v1",
		Kind:    "EC2NodeClass",
	}
	nodePoolGVK = schema.GroupVersionKind{
		Group:   "karpenter.sh",
		Version: "v1",
		Kind:    "NodePool",
	}
)

// ResolveNodeClassName finds the EC2NodeClass by explicit name or label selector.
// Returns explicit name if both provided.
func ResolveNodeClassName(ctx context.Context, c client.Client, log logr.Logger, explicit string, selector map[string]string) (string, error) {
	if explicit != "" && len(selector) > 0 {
		log.Info("Both nodeClassName and nodeClassSelector set; using nodeClassName", "nodeClassName", explicit)
	}
	if explicit != "" {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(ec2NodeClassGVK)
		if err := c.Get(ctx, client.ObjectKey{Name: explicit}, obj); err != nil {
			return "", fmt.Errorf("ec2nodeclass %q get failed: %w", explicit, err)
		}
		return explicit, nil
	}
	if len(selector) == 0 {
		return "", fmt.Errorf("neither nodeClassName nor nodeClassSelector provided")
	}
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   ec2NodeClassGVK.Group,
		Version: ec2NodeClassGVK.Version,
		Kind:    "EC2NodeClassList",
	})
	if err := c.List(ctx, list, client.MatchingLabels(selector)); err != nil {
		return "", fmt.Errorf("listing EC2NodeClass with selector %v: %w", selector, err)
	}
	if len(list.Items) == 0 {
		return "", fmt.Errorf("no EC2NodeClass matched selector %v", selector)
	}
	if len(list.Items) > 1 {
		names := make([]string, 0, len(list.Items))
		for i := range list.Items {
			names = append(names, list.Items[i].GetName())
		}
		sort.Strings(names)
		return "", fmt.Errorf("selector %v matched multiple EC2NodeClasses: %v", selector, names)
	}
	return list.Items[0].GetName(), nil
}

// UpsertNodePool creates or updates a Karpenter NodePool with a single chosen instance type + zone.
func UpsertNodePool(ctx context.Context, c client.Client, fieldOwner, name, nodeClassName, instanceType, zone, capacityType string) error {
	if capacityType == "" {
		capacityType = "spot"
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(nodePoolGVK)
	u.Object = map[string]any{
		"apiVersion": "karpenter.sh/v1",
		"kind":       "NodePool",
		"metadata": map[string]any{
			"name": name,
			"labels": map[string]any{
				"managed-by": "leftover",
			},
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"nodeClassRef": map[string]any{
						"name":  nodeClassName,
						"group": "karpenter.k8s.aws",
						"kind":  "EC2NodeClass",
					},
					"requirements": []any{
						map[string]any{
							"key":      "kubernetes.io/arch",
							"operator": string(corev1.NodeSelectorOpIn),
							"values":   []any{"amd64"},
						},
						map[string]any{
							"key":      "karpenter.sh/capacity-type",
							"operator": string(corev1.NodeSelectorOpIn),
							"values":   []any{capacityType},
						},
						map[string]any{
							"key":      "node.kubernetes.io/instance-type",
							"operator": string(corev1.NodeSelectorOpIn),
							"values":   []any{instanceType},
						},
						map[string]any{
							"key":      "topology.kubernetes.io/zone",
							"operator": string(corev1.NodeSelectorOpIn),
							"values":   []any{zone},
						},
					},
				},
			},
		},
	}
	return c.Patch(ctx, u, client.Apply, client.FieldOwner(fieldOwner), client.ForceOwnership)
}
