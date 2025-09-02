package v1alpha1

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	gpuv1alpha1 "github.com/devplatformsolutions/leftover/api/v1alpha1"
)

// log is for logging in this package.
var leftovernodepoollog = logf.Log.WithName("leftovernodepool-resource")

// SetupLeftoverNodePoolWebhookWithManager registers the webhook for LeftoverNodePool in the manager.
func SetupLeftoverNodePoolWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&gpuv1alpha1.LeftoverNodePool{}).
		WithValidator(&LeftoverNodePoolCustomValidator{}).
		WithDefaulter(&LeftoverNodePoolCustomDefaulter{}).
		Complete()
}

/*
Mutating (Defaulting) Webhook
*/

// +kubebuilder:webhook:path=/mutate-gpu-devplatforms-io-v1alpha1-leftovernodepool,mutating=true,failurePolicy=fail,sideEffects=None,groups=gpu.devplatforms.io,resources=leftovernodepools,verbs=create;update,versions=v1alpha1,name=mleftovernodepool-v1alpha1.kb.io,admissionReviewVersions=v1

// LeftoverNodePoolCustomDefaulter sets defaults on create/update.
// NOTE: Do not add deepcopy markers for this lightweight struct.
type LeftoverNodePoolCustomDefaulter struct{}

var _ webhook.CustomDefaulter = &LeftoverNodePoolCustomDefaulter{}

// Default applies sane defaults even when users send zero/empty values.
// Keep this aligned with your Spec markers in api/v1alpha1.
func (d *LeftoverNodePoolCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	lonp, ok := obj.(*gpuv1alpha1.LeftoverNodePool)
	if !ok {
		return fmt.Errorf("expected a LeftoverNodePool object but got %T", obj)
	}
	leftovernodepoollog.Info("Defaulting LeftoverNodePool", "name", lonp.GetName())

	// Hard defaults (mirror the +kubebuilder:default markers)
	if lonp.Spec.MinGPUs == 0 {
		lonp.Spec.MinGPUs = 1
	}
	if lonp.Spec.TargetCount == 0 {
		lonp.Spec.TargetCount = 2
	}
	if lonp.Spec.MaxInstanceTypes == 0 {
		lonp.Spec.MaxInstanceTypes = 5
	}
	if lonp.Spec.MaxZones == 0 {
		lonp.Spec.MaxZones = 2
	}
	if lonp.Spec.CapacityType == "" {
		lonp.Spec.CapacityType = "spot"
	}
	if lonp.Spec.BudgetsNodes == "" {
		lonp.Spec.BudgetsNodes = "10%"
	}
	if lonp.Spec.ConsolidateAfter == "" {
		lonp.Spec.ConsolidateAfter = "2m"
	}
	if lonp.Spec.RequeueMinutes == 0 {
		lonp.Spec.RequeueMinutes = 7
	}

	// OnDemandFallback: leave as-is to respect user input.
	// (CRD default handles the "unset" case.)

	return nil
}

/*
Validating Webhook
*/

// NOTE: change verbs to include delete if you need deletion validation.
// +kubebuilder:webhook:path=/validate-gpu-devplatforms-io-v1alpha1-leftovernodepool,mutating=false,failurePolicy=fail,sideEffects=None,groups=gpu.devplatforms.io,resources=leftovernodepools,verbs=create;update,versions=v1alpha1,name=vleftovernodepool-v1alpha1.kb.io,admissionReviewVersions=v1

// LeftoverNodePoolCustomValidator validates create/update/delete.
// NOTE: Do not add deepcopy markers for this lightweight struct.
type LeftoverNodePoolCustomValidator struct{}

var _ webhook.CustomValidator = &LeftoverNodePoolCustomValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *LeftoverNodePoolCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	lonp, ok := obj.(*gpuv1alpha1.LeftoverNodePool)
	if !ok {
		return nil, fmt.Errorf("expected a LeftoverNodePool object but got %T", obj)
	}
	leftovernodepoollog.Info("ValidateCreate", "name", lonp.GetName())
	return nil, validateSpec(&lonp.Spec)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *LeftoverNodePoolCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	lonp, ok := newObj.(*gpuv1alpha1.LeftoverNodePool)
	if !ok {
		return nil, fmt.Errorf("expected a LeftoverNodePool object for newObj but got %T", newObj)
	}
	leftovernodepoollog.Info("ValidateUpdate", "name", lonp.GetName())
	return nil, validateSpec(&lonp.Spec)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *LeftoverNodePoolCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No special rules on delete. Use controller finalizers for cleanup if needed.
	return nil, nil
}

/*
Shared spec validation
*/

func validateSpec(s *gpuv1alpha1.LeftoverNodePoolSpec) error {
	// Required
	if s.Region == "" {
		return fmt.Errorf("spec.region must be set")
	}
	// Simple numeric guards (CRD already enforces, but friendly messages help)
	if s.MinGPUs < 1 {
		return fmt.Errorf("spec.minGPUs must be >= 1")
	}
	if s.MaxInstanceTypes < 1 {
		return fmt.Errorf("spec.maxInstanceTypes must be >= 1")
	}
	if s.MaxZones < 1 {
		return fmt.Errorf("spec.maxZones must be >= 1")
	}
	// Enum guard
	switch s.CapacityType {
	case "spot", "on-demand":
	default:
		return fmt.Errorf("spec.capacityType must be one of: spot, on-demand")
	}
	// Families pattern (allow empty = any)
	if len(s.Families) > 0 {
		reFam := regexp.MustCompile(`^[a-z0-9]+[a-z0-9-]*$`)
		for _, f := range s.Families {
			if !reFam.MatchString(f) {
				return fmt.Errorf("spec.families contains invalid value %q; expected like g5, g6, p5, p4d", f)
			}
		}
	}
	// BudgetsNodes must be a percentage like "10%"
	if s.BudgetsNodes != "" {
		rePct := regexp.MustCompile(`^[0-9]{1,3}%$`)
		if !rePct.MatchString(s.BudgetsNodes) {
			return fmt.Errorf("spec.budgetsNodes must be a percentage like \"10%%\"")
		}
	}
	// ConsolidateAfter must be a valid Go duration, e.g., "2m", "30s", "1h"
	if s.ConsolidateAfter != "" {
		if _, err := time.ParseDuration(s.ConsolidateAfter); err != nil {
			return fmt.Errorf("spec.consolidateAfter must be a valid duration (e.g., \"2m\", \"30s\"): %w", err)
		}
	}
	// RequeueMinutes
	if s.RequeueMinutes < 1 {
		return fmt.Errorf("spec.requeueMinutes must be >= 1")
	}
	// Cross-field: if on-demand, fallback makes no sense
	if s.CapacityType == "on-demand" && s.OnDemandFallback {
		return fmt.Errorf("spec.onDemandFallback cannot be true when spec.capacityType is \"on-demand\"")
	}
	return nil
}
