/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	gpuv1alpha1 "github.com/devplatformsolutions/leftover/api/v1alpha1"
	"github.com/devplatformsolutions/leftover/internal/awsx"
)

// LeftoverNodePoolReconciler reconciles a LeftoverNodePool object
type LeftoverNodePoolReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	AWSFactory *awsx.Factory
}

// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *LeftoverNodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	var cr gpuv1alpha1.LeftoverNodePool
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	awsCli, err := r.AWSFactory.ForRegion(ctx, cr.Spec.Region)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating AWS client for region %q: %w", cr.Spec.Region, err)
	}

	types, _, err := awsCli.ListGPUInstanceTypes(ctx, cr.Spec.Families, cr.Spec.MinGPUs)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("listing GPU instance types: %w", err)
	}

	quotes, err := awsCli.LatestSpotPrices(ctx, types, 10*time.Minute)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting latest spot prices: %w", err)
	}

	// Build scorer once
	scorer, err := awsx.NewQuoteScorer(ctx, awsCli)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("building quote scorer: %w", err)
	}

	// Threshold and batch window (optimize price, then relax in tiers)
	threshold := cr.Spec.MinSpotScore
	batchWindow := 5

	best, score, ok, err := scorer.PickCheapestInBatches(ctx, quotes, batchWindow, threshold)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("selecting cheapest in batches (score >= %d): %w", threshold, err)
	}
	if best == nil {
		fmt.Println("No spot quotes available")
		return ctrl.Result{}, nil
	}
	if ok {
		fmt.Printf("Selected: %s %s $%.4f score %d at %s\n",
			best.InstanceType, best.Zone, best.PriceUSD, score, best.Timestamp.Format(time.RFC3339))
	} else {
		fmt.Printf("No quote met score >= %d; cheapest is: %s %s $%.4f score %d at %s\n",
			threshold, best.InstanceType, best.Zone, best.PriceUSD, score, best.Timestamp.Format(time.RFC3339))
	}

	// Print the top 5 with scores for visibility
	entries := make([]awsx.SpotQuote, 0, len(quotes))
	for _, q := range quotes {
		entries = append(entries, q)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].PriceUSD < entries[j].PriceUSD })
	limit := min(5, len(entries))
	fmt.Println("Cheapest spot quotes:")
	for i := range limit {
		q := entries[i]
		s, err := scorer.ScoreFor(ctx, q.InstanceType, q.Zone)
		if err != nil {
			return ctrl.Result{}, err
		}
		fmt.Printf("%d) %s %s $%.4f %d at %s\n",
			i+1, q.InstanceType, q.Zone, q.PriceUSD, s, q.Timestamp.Format(time.RFC3339))
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LeftoverNodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gpuv1alpha1.LeftoverNodePool{}).
		Named("leftovernodepool").
		Complete(r)
}
