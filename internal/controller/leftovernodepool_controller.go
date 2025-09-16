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
	"reflect"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gpuv1alpha1 "github.com/devplatformsolutions/leftover/api/v1alpha1"
	"github.com/devplatformsolutions/leftover/internal/awsx"
	"github.com/devplatformsolutions/leftover/internal/karpenterx"
)

// LeftoverNodePoolReconciler reconciles LeftoverNodePool resources.
type LeftoverNodePoolReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	AWSFactory *awsx.Factory
}

// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gpu.devplatforms.io,resources=leftovernodepools/finalizers,verbs=update
// +kubebuilder:rbac:groups=karpenter.k8s.aws,resources=ec2nodeclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=karpenter.sh,resources=nodepools,verbs=get;list;watch;create;update;patch;delete

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
	origStatus := cr.Status.DeepCopy()

	// Resolve EC2NodeClass
	nodeClassName, err := karpenterx.ResolveNodeClassName(ctx, r.Client, log, cr.Spec.NodeClassName, cr.Spec.NodeClassSelector)
	if err != nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "InvalidSpec",
			Message:            err.Error(),
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}

	awsCli, err := r.AWSFactory.ForRegion(ctx, cr.Spec.Region)
	if err != nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "AWSClientError",
			Message:            err.Error(),
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}

	types, _, err := awsCli.ListGPUInstanceTypes(ctx, cr.Spec.Families, cr.Spec.MinGPUs)
	if err != nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "ListTypesError",
			Message:            err.Error(),
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}
	log.Info("Candidate instance types", "count", len(types))

	quotes, err := awsCli.LatestSpotPrices(ctx, types, 10*time.Minute)
	if err != nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "SpotPriceError",
			Message:            err.Error(),
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}
	log.Info("Collected latest spot quotes", "count", len(quotes))

	targetCount := int32(1)
	if cr.Spec.TargetCount > 0 {
		targetCount = cr.Spec.TargetCount
	}
	scorer, err := awsx.NewQuoteScorer(ctx, awsCli, types, targetCount)
	if err != nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "ScorerError",
			Message:            err.Error(),
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}

	threshold := cr.Spec.MinSpotScore
	best, score, ok, err := scorer.PickCheapestInBatches(ctx, quotes, 5, threshold)
	if err != nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "SelectionError",
			Message:            err.Error(),
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}
	if best == nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "NoQuotes",
			Message:            "no spot quotes available",
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}

	if ok {
		log.Info("Selected quote", "instanceType", best.InstanceType, "zone", best.Zone, "priceUSD", best.PriceUSD, "score", score, "timestamp", best.Timestamp.Format(time.RFC3339))
	} else {
		log.Info("No quote met score threshold; using cheapest", "threshold", threshold, "instanceType", best.InstanceType, "zone", best.Zone, "priceUSD", best.PriceUSD, "score", score)
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
		s, _ := scorer.ScoreFor(ctx, q.InstanceType, q.Zone)
		log.Info("Quote", "rank", i+1, "instanceType", q.InstanceType, "zone", q.Zone, "priceUSD", q.PriceUSD, "score", s, "timestamp", q.Timestamp.Format(time.RFC3339))
	}

	capacityType := cr.Spec.CapacityType
	if capacityType == "" {
		capacityType = "spot"
	}
	poolName := fmt.Sprintf("leftover-%s", cr.Name)
	if err := karpenterx.UpsertNodePool(ctx, r.Client, "leftover", poolName, nodeClassName, best.InstanceType, best.Zone, capacityType); err != nil {
		r.setConditionNoWrite(cr, metav1.Condition{
			Type:               gpuv1alpha1.ConditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "ApplyNodePoolError",
			Message:            err.Error(),
			ObservedGeneration: cr.GetGeneration(),
		})
		return r.updateStatusIfChanged(ctx, log, cr, origStatus)
	}
	log.Info("Upserted NodePool", "name", poolName, "nodeClass", nodeClassName)

	newInstanceTypes := []string{best.InstanceType}
	newZones := []string{best.Zone}
	priceStr := fmt.Sprintf("%.4f", best.PriceUSD)

	selectionChanged := !reflect.DeepEqual(cr.Status.SelectedInstanceTypes, newInstanceTypes) ||
		!reflect.DeepEqual(cr.Status.SelectedZones, newZones) ||
		cr.Status.LastPriceUSD != priceStr ||
		cr.Status.LastScore != int(score)

	cr.Status.SelectedInstanceTypes = newInstanceTypes
	cr.Status.SelectedZones = newZones
	cr.Status.LastPriceUSD = priceStr
	cr.Status.LastScore = int(score)
	if selectionChanged {
		cr.Status.LastSyncTime = metav1.Now()
	}

	r.setConditionNoWrite(cr, metav1.Condition{
		Type:               gpuv1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "NodePool updated",
		ObservedGeneration: cr.GetGeneration(),
	})

	return r.updateStatusIfChanged(ctx, log, cr, origStatus)
}

func (r *LeftoverNodePoolReconciler) setConditionNoWrite(cr *gpuv1alpha1.LeftoverNodePool, cond metav1.Condition) {
	meta.SetStatusCondition(&cr.Status.Conditions, cond)
}

func (r *LeftoverNodePoolReconciler) updateStatusIfChanged(ctx context.Context, log logr.Logger, cr *gpuv1alpha1.LeftoverNodePool, orig *gpuv1alpha1.LeftoverNodePoolStatus) error {
	if orig == nil {
		return nil
	}
	if reflect.DeepEqual(orig, &cr.Status) {
		return nil
	}
	if err := r.Status().Update(ctx, cr); err != nil {
		log.Error(err, "status update failed")
		return err
	}
	return nil
}

// SetupWithManager wires the controller into the manager.
func (r *LeftoverNodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gpuv1alpha1.LeftoverNodePool{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
