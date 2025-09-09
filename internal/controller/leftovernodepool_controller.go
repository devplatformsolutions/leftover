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
	"github.com/go-logr/logr"
)

type LeftoverNodePoolReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	AWSFactory *awsx.Factory
}

// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools/finalizers,verbs=update

func (r *LeftoverNodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("leftovernodepool", req.NamespacedName)

	var cr gpuv1alpha1.LeftoverNodePool
	if err := r.Get(ctx, req.NamespacedName, &cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.reconcileOnce(ctx, log, &cr); err != nil {
		log.Error(err, "reconcileOnce failed")
		return ctrl.Result{RequeueAfter: time.Minute}, err
	}

	requeue := time.Duration(cr.Spec.RequeueMinutes) * time.Minute
	if requeue <= 0 {
		requeue = 7 * time.Minute
	}
	log.Info("Requeue scheduled", "after", requeue.String())
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (r *LeftoverNodePoolReconciler) reconcileOnce(ctx context.Context, log logr.Logger, cr *gpuv1alpha1.LeftoverNodePool) error {
	awsCli, err := r.AWSFactory.ForRegion(ctx, cr.Spec.Region)
	if err != nil {
		return fmt.Errorf("creating AWS client for region %q: %w", cr.Spec.Region, err)
	}

	types, _, err := awsCli.ListGPUInstanceTypes(ctx, cr.Spec.Families, cr.Spec.MinGPUs)
	if err != nil {
		return fmt.Errorf("listing GPU instance types: %w", err)
	}
	log.Info("Candidate instance types", "count", len(types))

	quotes, err := awsCli.LatestSpotPrices(ctx, types, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("getting latest spot prices: %w", err)
	}
	log.Info("Collected latest spot quotes", "count", len(quotes))

	targetCount := int32(1)
	if cr.Spec.TargetCount > 0 {
		targetCount = int32(cr.Spec.TargetCount)
	}
	scorer, err := awsx.NewQuoteScorer(ctx, awsCli, types, targetCount)
	if err != nil {
		return fmt.Errorf("building quote scorer: %w", err)
	}

	threshold := cr.Spec.MinSpotScore
	batchWindow := 5

	best, score, ok, err := scorer.PickCheapestInBatches(ctx, quotes, batchWindow, threshold)
	if err != nil {
		return fmt.Errorf("selecting cheapest in batches (score >= %d): %w", threshold, err)
	}
	if best == nil {
		log.Info("No spot quotes available")
		return nil
	}
	if ok {
		log.Info("Selected quote",
			"instanceType", best.InstanceType,
			"zone", best.Zone,
			"priceUSD", best.PriceUSD,
			"score", score,
			"timestamp", best.Timestamp.Format(time.RFC3339))
	} else {
		log.Info("No quote met score threshold; using cheapest",
			"threshold", threshold,
			"instanceType", best.InstanceType,
			"zone", best.Zone,
			"priceUSD", best.PriceUSD,
			"score", score,
			"timestamp", best.Timestamp.Format(time.RFC3339))
	}

	entries := make([]awsx.SpotQuote, 0, len(quotes))
	for _, q := range quotes {
		entries = append(entries, q)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].PriceUSD < entries[j].PriceUSD })
	limit := min(5, len(entries))
	log.Info("Cheapest spot quotes", "count", limit)
	for i := 0; i < limit; i++ {
		q := entries[i]
		s, err := scorer.ScoreFor(ctx, q.InstanceType, q.Zone)
		if err != nil {
			return err
		}
		log.Info("Quote",
			"rank", i+1,
			"instanceType", q.InstanceType,
			"zone", q.Zone,
			"priceUSD", q.PriceUSD,
			"score", s,
			"timestamp", q.Timestamp.Format(time.RFC3339))
	}
	return nil
}

func (r *LeftoverNodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gpuv1alpha1.LeftoverNodePool{}).
		Named("leftovernodepool").
		Complete(r)
}
