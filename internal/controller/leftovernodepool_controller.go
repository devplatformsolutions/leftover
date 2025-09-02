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
// TODO(user): Modify the Reconcile function to compare the state specified by
// the LeftoverNodePool object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
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

	types, meta, err := awsCli.ListGPUInstanceTypes(ctx, cr.Spec.Families, cr.Spec.MinGPUs)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("listing GPU instance types: %w", err)
	}
	fmt.Println("This is the meta", meta)
	fmt.Println("These are the types", types)

	quotes, err := awsCli.LatestSpotPrices(ctx, types, 10*time.Minute)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting latest spot prices: %w", err)
	}

	for _, quote := range quotes {
		fmt.Println("This is the quote", quote.InstanceType, quote.Zone, quote.PriceUSD)
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
