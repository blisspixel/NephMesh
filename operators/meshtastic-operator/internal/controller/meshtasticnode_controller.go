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

// Package controller wires the pure convergence state machine (internal/reconcile)
// to Kubernetes. It keeps almost no logic of its own: it loads the resource,
// resolves the device connection and broker address, runs one Converge step,
// and records the outcome as status conditions. All the interesting behavior,
// and all the tests, live in the internal packages behind a device interface,
// so this file is deliberately thin.
package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/reconcile"
)

// DeviceFactory builds a device client for a node's connection. It is injected
// so tests can supply a fake and the binary can supply the real CLI-sidecar
// client. Returning an error means the connection could not be prepared.
type DeviceFactory func(ctx context.Context, node *meshv1alpha1.MeshtasticNode) (device.Client, error)

// MeshtasticNodeReconciler reconciles a MeshtasticNode against its device.
type MeshtasticNodeReconciler struct {
	client.Client
	NewDevice DeviceFactory
}

// +kubebuilder:rbac:groups=mesh.nephmesh.io,resources=meshtasticnodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=mesh.nephmesh.io,resources=meshtasticnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mesh.nephmesh.io,resources=meshtasticnodes/finalizers,verbs=update
//
// Note: no Secret access is granted. Channel PSK and MQTT password secrets are
// not read yet; when that path ships it will use a namespaced Role scoped to
// the operator namespace, not a cluster-wide Secret grant. A broad unused grant
// plus the pod's service-account token would be a cluster-wide exfiltration
// path on compromise, so it is deliberately absent (least privilege).

// Reconcile runs one bounded convergence step and records conditions.
func (r *MeshtasticNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var node meshv1alpha1.MeshtasticNode
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deletion: with Retain (the default) we simply stop managing the device,
	// so there is nothing to clean up and the finalizer, if present, is removed.
	// Wipe is a later-phase feature and is intentionally not performed here yet.
	if !node.DeletionTimestamp.IsZero() {
		if hasFinalizer(&node) {
			removeFinalizer(&node)
			if err := r.Update(ctx, &node); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if !hasFinalizer(&node) {
		addFinalizer(&node)
		if err := r.Update(ctx, &node); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	dev, err := r.NewDevice(ctx, &node)
	if err != nil {
		return ctrl.Result{}, err
	}

	desired := config.BuildDesired(node.Spec, r.resolveBroker(ctx, &node))
	prior := reconcile.State{
		RebootPending: meta.IsStatusConditionTrue(node.Status.Conditions, meshv1alpha1.ConditionRebootPending),
		ApplyAttempts: node.Status.ApplyAttempts,
	}

	outcome, err := reconcile.Converge(ctx, dev, desired, prior)
	if err != nil {
		log.Error(err, "convergence step failed")
		return ctrl.Result{}, err // rate-limited retry for unexpected failures
	}

	applyOutcome(&node, outcome)
	if statusErr := r.Status().Update(ctx, &node); statusErr != nil {
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{RequeueAfter: outcome.Requeue}, nil
}

// resolveBroker returns the MQTT address to write to the device. When the spec
// gives a broker name, the device needs an address it can actually reach; a
// future revision resolves a Service name to a ClusterIP here (the Phase 1 demo
// showed the device connects to an IP but not a hostname). For now it passes
// the configured address through unchanged.
func (r *MeshtasticNodeReconciler) resolveBroker(_ context.Context, node *meshv1alpha1.MeshtasticNode) string {
	if node.Spec.MQTT == nil {
		return ""
	}
	return node.Spec.MQTT.Address
}

func applyOutcome(node *meshv1alpha1.MeshtasticNode, o reconcile.Outcome) {
	gen := node.Generation
	set := func(condType string, ok bool, reason string) {
		status := metav1.ConditionFalse
		if ok {
			status = metav1.ConditionTrue
		}
		meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
			Type:               condType,
			Status:             status,
			Reason:             reason,
			ObservedGeneration: gen,
		})
	}
	set(meshv1alpha1.ConditionReachable, o.Reachable, o.Reason)
	set(meshv1alpha1.ConditionConfigInSync, o.ConfigInSync, o.Reason)
	set(meshv1alpha1.ConditionRebootPending, o.RebootPending, o.Reason)
	set(meshv1alpha1.ConditionDegraded, o.Degraded, o.Reason)
	readyReason := meshv1alpha1.ReasonNotReady
	if o.Ready {
		readyReason = meshv1alpha1.ReasonReady
	}
	set(meshv1alpha1.ConditionReady, o.Ready, readyReason)

	node.Status.ObservedGeneration = gen
	node.Status.ApplyAttempts = o.ApplyAttempts
	if o.Reachable {
		node.Status.NodeID = o.Info.NodeID
		now := metav1.NewTime(time.Now())
		node.Status.LastHeard = &now
	}
}

func hasFinalizer(node *meshv1alpha1.MeshtasticNode) bool {
	for _, f := range node.Finalizers {
		if f == meshv1alpha1.Finalizer {
			return true
		}
	}
	return false
}

func addFinalizer(node *meshv1alpha1.MeshtasticNode) {
	node.Finalizers = append(node.Finalizers, meshv1alpha1.Finalizer)
}

func removeFinalizer(node *meshv1alpha1.MeshtasticNode) {
	kept := node.Finalizers[:0]
	for _, f := range node.Finalizers {
		if f != meshv1alpha1.Finalizer {
			kept = append(kept, f)
		}
	}
	node.Finalizers = kept
}

// SetupWithManager registers the reconciler. One reconcile per node key is
// guaranteed by the workqueue, which is exactly the single-client serialization
// the device needs; MaxConcurrentReconciles only lets different nodes reconcile
// in parallel, never the same node concurrently.
func (r *MeshtasticNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&meshv1alpha1.MeshtasticNode{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 4}).
		Complete(r)
}
