/*
Copyright 2026 The NephMesh Authors.

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

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/intent"
)

// CommunicationIntentReconciler compiles a CommunicationIntent into proposed
// MeshtasticNode specs and records the result on status. It is REPORT-ONLY: it
// never creates a MeshtasticNode and never writes to a device. This is the first,
// deliberately safe slice of the intent layer (ADR 0001); actuation is gated
// behind the signed-autonomy and safety work (ADR 0002).
type CommunicationIntentReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=intent.nephmesh.io,resources=communicationintents,verbs=get;list;watch
// +kubebuilder:rbac:groups=intent.nephmesh.io,resources=communicationintents/status,verbs=get;update;patch
//
// Note the absence of any create/update grant on meshtasticnodes here: report-only
// means the compiler cannot actuate, and the RBAC enforces it rather than trusting
// the code.

// Reconcile compiles the intent and writes the proposed rendering and a
// feasibility verdict to status. It never actuates.
func (r *CommunicationIntentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var ci intentv1alpha1.CommunicationIntent
	if err := r.Get(ctx, req.NamespacedName, &ci); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	before := ci.Status.DeepCopy()

	result := intent.Compile(ci.Spec)

	status := metav1.ConditionFalse
	if result.Feasible {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&ci.Status.Conditions, metav1.Condition{
		Type:               intentv1alpha1.ConditionFeasible,
		Status:             status,
		Reason:             result.Reason,
		Message:            result.Message,
		ObservedGeneration: ci.Generation,
	})

	// The fleet airtime verdict is a distinct condition: renderability and fitting
	// the airtime commons are separate questions. It is Unknown when the intent
	// declares no expectedTraffic (nothing to estimate from), rather than a
	// misleading True.
	airtimeStatus := metav1.ConditionUnknown
	airtimeReason := result.Airtime.Reason
	if airtimeReason == "" {
		airtimeReason = intentv1alpha1.ReasonAirtimeNotEvaluated
	}
	if result.Airtime.Evaluated {
		if result.Airtime.WithinBudget {
			airtimeStatus = metav1.ConditionTrue
		} else {
			airtimeStatus = metav1.ConditionFalse
		}
	}
	meta.SetStatusCondition(&ci.Status.Conditions, metav1.Condition{
		Type:               intentv1alpha1.ConditionAirtimeWithinBudget,
		Status:             airtimeStatus,
		Reason:             airtimeReason,
		Message:            result.Airtime.Message,
		ObservedGeneration: ci.Generation,
	})

	ci.Status.ObservedGeneration = ci.Generation
	ci.Status.SelectedModemPreset = result.SelectedPreset
	ci.Status.NodeCount = len(result.Proposed)
	ci.Status.PredictedChannelUtilizationPercent = result.Airtime.PredictedUtilizationPercent
	ci.Status.ProposedNodes = result.Proposed

	if apiequality.Semantic.DeepEqual(before, &ci.Status) {
		// Status is already what this compile produces. Writing it again would
		// bump resourceVersion and re-enqueue the same intent forever.
		return ctrl.Result{}, nil
	}
	if err := r.Status().Update(ctx, &ci); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler. GenerationChangedPredicate drops
// status-only events so a status write cannot hot-loop the report-only compile.
func (r *CommunicationIntentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&intentv1alpha1.CommunicationIntent{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}
