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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/airtime"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/reconcile"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

// DeviceFactory builds a device client for a node's connection. It is injected
// so tests can supply a fake and the binary can supply the real CLI-sidecar
// client. Returning an error means the connection could not be prepared.
type DeviceFactory func(ctx context.Context, node *meshv1alpha1.MeshtasticNode) (device.Client, error)

// MeshtasticNodeReconciler reconciles a MeshtasticNode against its device.
type MeshtasticNodeReconciler struct {
	client.Client
	// Reader is an uncached reader (the manager's APIReader) used only for
	// Secrets. Reading Secrets through the cached client would start a
	// cluster-wide Secret informer and force a cluster-wide list/watch grant;
	// an uncached get keeps the RBAC a namespaced get, per the threat model.
	Reader    client.Reader
	NewDevice DeviceFactory
}

// +kubebuilder:rbac:groups=mesh.nephmesh.io,resources=meshtasticnodes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=mesh.nephmesh.io,resources=meshtasticnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mesh.nephmesh.io,resources=meshtasticnodes/finalizers,verbs=update
//
// Secret access is NOT a cluster-wide grant. The operator reads the broker
// password (and later channel PSKs) with a namespaced get only, granted by a
// Role in the operator namespace (packages/meshtastic-operator/rbac.yaml), not
// by a kubebuilder marker (markers generate a ClusterRole, and a cluster-wide
// Secret read plus the pod token would be a cluster-wide exfiltration path on
// compromise). The read is uncached (r.Reader) so no Secret informer is opened.

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
		// The Update above changes the object, which re-triggers a reconcile
		// through the watch, so no explicit requeue is needed here.
		return ctrl.Result{}, nil
	}

	dev, err := r.NewDevice(ctx, &node)
	if err != nil {
		return ctrl.Result{}, err
	}

	mqttPassword, err := r.resolveMQTTPassword(ctx, &node)
	if err != nil {
		// The error names the secret and key, never the value.
		return ctrl.Result{}, err
	}

	desired := config.BuildDesired(node.Spec, r.resolveBroker(ctx, &node), mqttPassword)
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

// resolveMQTTPassword reads the broker password from the Secret the spec
// references, in the node's own namespace, with an uncached get. The value is
// wrapped in a redacting secret.Value immediately, so it cannot leak through a
// log line or error between here and the config-write path. Errors name the
// secret and key only, never the value.
func (r *MeshtasticNodeReconciler) resolveMQTTPassword(ctx context.Context, node *meshv1alpha1.MeshtasticNode) (secret.Value, error) {
	if node.Spec.MQTT == nil || node.Spec.MQTT.PasswordSecretRef == nil {
		return secret.Value{}, nil
	}
	ref := node.Spec.MQTT.PasswordSecretRef

	var s corev1.Secret
	key := types.NamespacedName{Namespace: node.Namespace, Name: ref.Name}
	if err := r.Reader.Get(ctx, key, &s); err != nil {
		return secret.Value{}, fmt.Errorf("reading mqtt password secret %q: %w", ref.Name, err)
	}
	data, ok := s.Data[ref.Key]
	if !ok {
		return secret.Value{}, fmt.Errorf("mqtt password secret %q has no key %q", ref.Name, ref.Key)
	}
	return secret.New(string(data)), nil
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
		applyAirtimeHealth(node, o.Info, gen)
	}
}

// applyAirtimeHealth surfaces the radio's own airtime telemetry as a condition,
// so a saturating channel (airtime is the LoRa scaling wall) is visible rather
// than silently degrading. It is set only when the device reported the metrics.
func applyAirtimeHealth(node *meshv1alpha1.MeshtasticNode, info device.Info, gen int64) {
	if info.ChannelUtilization == nil && info.AirUtilTx == nil {
		return
	}
	ch, tx := valueOr(info.ChannelUtilization), valueOr(info.AirUtilTx)
	status := metav1.ConditionTrue
	reason := meshv1alpha1.ReasonAirtimeHealthy
	if !airtime.Healthy(ch, tx) {
		status = metav1.ConditionFalse
		reason = meshv1alpha1.ReasonAirtimeHigh
	}
	meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
		Type:               meshv1alpha1.ConditionAirtimeHealthy,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: gen,
		Message:            fmt.Sprintf("channelUtilization %.1f%%, airUtilTx %.1f%%", ch, tx),
	})
}

func valueOr(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
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
