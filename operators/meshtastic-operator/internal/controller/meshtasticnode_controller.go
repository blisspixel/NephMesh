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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/airtime"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/metrics"
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
	// Recorder emits Kubernetes Events so operational transitions (config applied,
	// device degraded, a missing Secret) show up in `kubectl describe`, not just in
	// conditions. It may be nil in unit tests that do not assert on events.
	Recorder record.EventRecorder
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
		metrics.Forget(node.Namespace, node.Name)
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
		return r.secretFailure(ctx, &node, err)
	}

	desiredChannels, err := r.buildDesiredChannels(ctx, &node)
	if err != nil {
		// The error names the secret and key, never the key material.
		return r.secretFailure(ctx, &node, err)
	}

	desired := config.BuildDesired(node.Spec, r.resolveBroker(ctx, &node), mqttPassword)
	prior := reconcile.State{
		RebootPending:    meta.IsStatusConditionTrue(node.Status.Conditions, meshv1alpha1.ConditionRebootPending),
		ApplyAttempts:    node.Status.ApplyAttempts,
		MQTTPasswordHash: node.Status.LastAppliedMQTTPasswordHash,
	}
	// A new spec generation is a new desired state: reset the apply bound so a
	// Degraded node is retried after the user fixes the field that would not
	// converge, instead of staying Degraded forever.
	if node.Generation != node.Status.ObservedGeneration {
		prior.ApplyAttempts = 0
	}
	priorDegraded := meta.IsStatusConditionTrue(node.Status.Conditions, meshv1alpha1.ConditionDegraded)
	priorReachable := meta.IsStatusConditionTrue(node.Status.Conditions, meshv1alpha1.ConditionReachable)

	outcome, err := reconcile.Converge(ctx, dev, desired, desiredChannels, prior)
	if err != nil {
		log.Error(err, "convergence step failed")
		return ctrl.Result{}, err // rate-limited retry for unexpected failures
	}

	applyOutcome(&node, outcome)
	if outcome.ChannelsUnobserved {
		meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
			Type:               meshv1alpha1.ConditionChannelsInSync,
			Status:             metav1.ConditionUnknown,
			Reason:             meshv1alpha1.ReasonChannelsUnobserved,
			ObservedGeneration: node.Generation,
			Message:            "device export did not include a channel set; not treating declared channels as missing",
		})
	} else if outcome.Reachable {
		applyChannelsInSync(&node, desiredChannels.Compare, outcome.LiveChannels, node.Generation)
	}
	if outcome.Reachable {
		applyAirtimeBudget(&node, outcome.CurrentModemPreset, outcome.Info, node.Generation)
	} else {
		// No current view of the device this step: drop the channel- and
		// telemetry-derived conditions rather than leaving a stale ChannelsInSync or
		// AirtimeBudget verdict from a prior reachable step for the whole outage.
		// (AirtimeHealthy already self-clears via applyOutcome, and the Prometheus
		// gauges are cleared on nil.)
		meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionChannelsInSync)
		meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionAirtimeBudget)
	}

	// Log and emit Events on the meaningful transitions this step, so a rollout or
	// a stuck node leaves a trail in the logs and in `kubectl describe`. These fire
	// on transitions, not every reconcile, to avoid noise; no secret material or
	// the desired config (which carries the password) is logged.
	switch {
	case outcome.Degraded && !priorDegraded:
		log.Info("node degraded, did not converge", "applyAttempts", outcome.ApplyAttempts, "drift", outcome.Drift)
		r.event(&node, corev1.EventTypeWarning, meshv1alpha1.ReasonApplyFailed,
			fmt.Sprintf("did not converge after %d applies; still drifted: %s", outcome.ApplyAttempts, strings.Join(outcome.Drift, ", ")))
	case outcome.RebootPending && !prior.RebootPending:
		log.Info("applied config, device rebooting", "applyAttempts", outcome.ApplyAttempts, "drift", outcome.Drift, "requeueAfter", outcome.Requeue.String())
		r.event(&node, corev1.EventTypeNormal, meshv1alpha1.ReasonConfigApplied, "applied config changes; device rebooting")
	}
	if !outcome.Reachable && priorReachable {
		log.Info("device became unreachable", "reason", outcome.Reason)
		r.event(&node, corev1.EventTypeWarning, meshv1alpha1.ReasonConnectFailed, "device became unreachable")
	}

	metrics.Record(metrics.Sample{
		Namespace: node.Namespace, Name: node.Name,
		Ready: outcome.Ready, ConfigInSync: outcome.ConfigInSync, ApplyAttempts: outcome.ApplyAttempts,
		Reachable: outcome.Reachable, Degraded: outcome.Degraded,
		ChannelUtilization: outcome.Info.ChannelUtilization, AirUtilTx: outcome.Info.AirUtilTx,
	})
	if statusErr := r.Status().Update(ctx, &node); statusErr != nil {
		// An apply already wrote the radio. A conflict retry would run
		// Converge again immediately and apply a second time against a
		// device that is mid-reboot. Wait the reboot interval instead.
		if apierrors.IsConflict(statusErr) && outcome.RebootPending {
			log.Error(statusErr, "status conflict after apply; waiting to re-export rather than applying again")
			return ctrl.Result{RequeueAfter: outcome.Requeue}, nil
		}
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
	if len(data) == 0 {
		// An empty value would be dropped by BuildDesired and silently configure the
		// broker connection with no password, the same silent downgrade the channel
		// path refuses. Fail loudly instead of degrading auth without notice.
		return secret.Value{}, fmt.Errorf("mqtt password secret %q key %q is empty; refusing to configure MQTT with no password", ref.Name, ref.Key)
	}
	return secret.New(string(data)), nil
}

// buildDesiredChannels reads each declared channel's pre-shared key from the
// Secret it references, in the node's own namespace with an uncached get (the
// same namespaced, get-only path the broker password uses), and returns both what
// the reconciler needs: the comparable state (the key as a hash, for drift
// detection and the status condition) and the write payload (the key wrapped in
// the redacting type, revealed only when the apply file is written). A channel
// without a pskSecretRef uses the device's public default key, whose hash is the
// SHA-256 of the single 0x01 byte the device stores. The Secret value is the raw
// key bytes (Kubernetes already decodes Secret data), a 16- or 32-byte Meshtastic
// key, not a base64 string. Errors name the secret and key, never the material.
func (r *MeshtasticNodeReconciler) buildDesiredChannels(ctx context.Context, node *meshv1alpha1.MeshtasticNode) (reconcile.DesiredChannels, error) {
	var out reconcile.DesiredChannels
	for _, ch := range node.Spec.Channels {
		var key secret.Value
		if ch.PSKSecretRef != nil {
			var s corev1.Secret
			name := types.NamespacedName{Namespace: node.Namespace, Name: ch.PSKSecretRef.Name}
			if err := r.Reader.Get(ctx, name, &s); err != nil {
				return reconcile.DesiredChannels{}, fmt.Errorf("reading channel %d psk secret %q: %w", ch.Index, ch.PSKSecretRef.Name, err)
			}
			data, ok := s.Data[ch.PSKSecretRef.Key]
			if !ok {
				return reconcile.DesiredChannels{}, fmt.Errorf("channel %d psk secret %q has no key %q", ch.Index, ch.PSKSecretRef.Name, ch.PSKSecretRef.Key)
			}
			if len(data) == 0 {
				// An explicit pskSecretRef with an empty value would fall through to
				// the device's public default key, making a "private" channel
				// world-readable. Surface it instead of silently downgrading.
				return reconcile.DesiredChannels{}, fmt.Errorf("channel %d psk secret %q key %q is empty; refusing to fall back to the public default key", ch.Index, ch.PSKSecretRef.Name, ch.PSKSecretRef.Key)
			}
			key = secret.New(string(data))
		}
		// The compare hash: an explicit key hashes its raw bytes, a default (no
		// ref) hashes the 0x01 shorthand the device stores, so a default channel
		// does not read as permanent drift. The well-known 16-byte expanded
		// default is treated as the shorthand so a Secret holding it does not
		// never-converge against a device that stored 0x01.
		raw := config.DefaultPSKShorthand
		if !key.IsZero() {
			raw = []byte(key.Reveal())
			if config.IsDefaultPSK(raw) {
				key = secret.Value{}
				raw = config.DefaultPSKShorthand
			}
		}
		out.Compare = append(out.Compare, config.ChannelState{
			Index: ch.Index, Name: ch.Name, PSKHash: config.PSKHash(raw),
			UplinkEnabled: ch.UplinkEnabled, DownlinkEnabled: ch.DownlinkEnabled,
		})
		out.Write = append(out.Write, device.ChannelWrite{
			Index: ch.Index, Name: ch.Name, Key: key,
			UplinkEnabled: ch.UplinkEnabled, DownlinkEnabled: ch.DownlinkEnabled,
		})
	}
	return out, nil
}

// applyChannelsInSync records whether the declared channels match the device.
// Ready is gated on channel convergence in Converge; this writes the condition
// the operator surfaces. A node that declares no channels gets no condition.
func applyChannelsInSync(node *meshv1alpha1.MeshtasticNode, desired, live []config.ChannelState, gen int64) {
	if len(desired) == 0 {
		// The node declares no channels; drop any prior condition so it does not
		// linger after channels are removed from the spec.
		meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionChannelsInSync)
		return
	}
	drift := config.ChannelDrift(desired, live)
	status := metav1.ConditionTrue
	reason := meshv1alpha1.ReasonChannelsInSync
	message := "declared channels match the device"
	if len(drift) > 0 {
		status = metav1.ConditionFalse
		reason = meshv1alpha1.ReasonChannelsDrifted
		message = "channel drift: " + strings.Join(drift, ", ")
	}
	meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
		Type:               meshv1alpha1.ConditionChannelsInSync,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: gen,
		Message:            message,
	})
}

// event emits a Kubernetes Event when a recorder is configured (it is nil in some
// unit tests). Messages must never contain secret material.
func (r *MeshtasticNodeReconciler) event(node *meshv1alpha1.MeshtasticNode, eventtype, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(node, eventtype, reason, message)
	}
}

// secretFailure surfaces a Secret-resolution error instead of leaving the object's
// conditions and metrics frozen at their last-good values, which would show the
// node Ready while it is stuck failing (a deleted or rotated Secret). It marks the
// node not-Ready with the error (which names the secret and key, never the value),
// records a Warning Event, drops the ready metric, writes status, and returns the
// error for a rate-limited retry.
func (r *MeshtasticNodeReconciler) secretFailure(ctx context.Context, node *meshv1alpha1.MeshtasticNode, cause error) (ctrl.Result, error) {
	// We did not talk to the device this step. Drop device-derived conditions
	// rather than leaving Ready=False next to a stale Reachable=True from the
	// last good reconcile.
	meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionReachable)
	meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionConfigInSync)
	meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionChannelsInSync)
	meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionAirtimeHealthy)
	meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionAirtimeBudget)
	meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionRebootPending)
	meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionDegraded)
	meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
		Type: meshv1alpha1.ConditionReady, Status: metav1.ConditionFalse,
		Reason: meshv1alpha1.ReasonSecretMissing, ObservedGeneration: node.Generation,
		Message: cause.Error(),
	})
	r.event(node, corev1.EventTypeWarning, meshv1alpha1.ReasonSecretMissing, cause.Error())
	metrics.Record(metrics.Sample{
		Namespace: node.Namespace, Name: node.Name,
		Ready: false, Reachable: false, ApplyAttempts: node.Status.ApplyAttempts,
	})
	if statusErr := r.Status().Update(ctx, node); statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, cause
}

func applyOutcome(node *meshv1alpha1.MeshtasticNode, o reconcile.Outcome) {
	gen := node.Generation
	set := func(condType string, ok bool, reason, message string) {
		status := metav1.ConditionFalse
		if ok {
			status = metav1.ConditionTrue
		}
		meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
			Type:               condType,
			Status:             status,
			Reason:             reason,
			ObservedGeneration: gen,
			Message:            message,
		})
	}
	driftMsg := ""
	if len(o.Drift) > 0 {
		driftMsg = "still drifted: " + strings.Join(o.Drift, ", ")
	}
	degradedMsg := ""
	if o.Degraded {
		degradedMsg = fmt.Sprintf("did not converge after %d applies; %s", o.ApplyAttempts, driftMsg)
	}
	set(meshv1alpha1.ConditionReachable, o.Reachable, o.Reason, "")
	set(meshv1alpha1.ConditionConfigInSync, o.ConfigInSync, o.Reason, driftMsg)
	set(meshv1alpha1.ConditionRebootPending, o.RebootPending, o.Reason, "")
	set(meshv1alpha1.ConditionDegraded, o.Degraded, o.Reason, degradedMsg)
	readyReason := meshv1alpha1.ReasonNotReady
	if o.Ready {
		readyReason = meshv1alpha1.ReasonReady
	}
	set(meshv1alpha1.ConditionReady, o.Ready, readyReason, "")

	node.Status.ObservedGeneration = gen
	node.Status.ApplyAttempts = o.ApplyAttempts
	node.Status.LastAppliedMQTTPasswordHash = o.MQTTPasswordHash
	if o.Reachable {
		if o.Info.NodeID != "" {
			node.Status.NodeID = o.Info.NodeID
		}
		// Do not rewrite LastHeard on every poll. A status write retriggers the
		// watch; without this throttle (and ignoreStatusOnly) a Ready node would
		// hammer the single-client device API instead of waiting DriftCheckInterval.
		if node.Status.LastHeard == nil || time.Since(node.Status.LastHeard.Time) >= lastHeardMinInterval {
			now := metav1.NewTime(time.Now())
			node.Status.LastHeard = &now
		}
		applyAirtimeHealth(node, o.Info, gen)
	} else {
		meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionAirtimeHealthy)
	}
}

// applyAirtimeHealth surfaces the radio's own airtime telemetry as a condition,
// so a saturating channel (airtime is the LoRa scaling wall) is visible rather
// than silently degrading. It is set only when the device reported the metrics.
func applyAirtimeHealth(node *meshv1alpha1.MeshtasticNode, info device.Info, gen int64) {
	if info.ChannelUtilization == nil && info.AirUtilTx == nil {
		// No telemetry this step: drop any prior condition rather than leaving a
		// stale health verdict the device is no longer reporting.
		meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionAirtimeHealthy)
		return
	}
	// Evaluate only the metrics the device actually reported. An absent metric is
	// neither healthy nor unhealthy; substituting 0.0 (as a plain valueOr would)
	// falsely reads as an idle transmitter and could mask a saturated node.
	healthy := true
	if v := info.ChannelUtilization; v != nil && *v > airtime.RecommendedChannelUtilizationPercent {
		healthy = false
	}
	if v := info.AirUtilTx; v != nil && *v > airtime.RecommendedAirUtilTxPercent {
		healthy = false
	}
	status := metav1.ConditionTrue
	reason := meshv1alpha1.ReasonAirtimeHealthy
	if !healthy {
		status = metav1.ConditionFalse
		reason = meshv1alpha1.ReasonAirtimeHigh
	}
	meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
		Type:               meshv1alpha1.ConditionAirtimeHealthy,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: gen,
		Message:            fmt.Sprintf("channelUtilization %s, airUtilTx %s", pctOrNA(info.ChannelUtilization), pctOrNA(info.AirUtilTx)),
	})
}

// pctOrNA formats an optional telemetry percent, or "not reported" when the device
// did not include it, so an absent metric is never displayed as a measured 0.
func pctOrNA(v *float64) string {
	if v == nil {
		return "not reported"
	}
	return fmt.Sprintf("%.1f%%", *v)
}

// applyAirtimeBudget predicts the airtime effect of a declared modem-preset
// change and surfaces it as the AirtimeBudget condition. It sets the condition
// only when a change is actually declared (the desired preset differs from the
// device's current preset) and the radio reported its channel utilization, so the
// prediction has real inputs; a no-change or no-telemetry reconcile carries no
// condition. It is advisory: the operator still applies the declared preset, but
// reports whether the change is predicted to push the channel past the
// recommended utilization ceiling (hard refusal of an over-budget fleet change is
// the Porch validator's job, see docs/plans/airtime-budget.md).
func applyAirtimeBudget(node *meshv1alpha1.MeshtasticNode, currentPreset string, info device.Info, gen int64) {
	desiredPreset := node.Spec.ModemPreset
	var predicted float64
	var ok bool
	if desiredPreset != "" && currentPreset != "" && desiredPreset != currentPreset && info.ChannelUtilization != nil {
		predicted, ok = airtime.PredictedChannelUtilizationPercent(currentPreset, desiredPreset, *info.ChannelUtilization)
	}
	if !ok {
		// No predictable pending preset change (none declared, already applied, or
		// no telemetry): any prior AirtimeBudget condition is stale, so drop it
		// rather than leave a misleading False after the change has converged.
		meta.RemoveStatusCondition(&node.Status.Conditions, meshv1alpha1.ConditionAirtimeBudget)
		return
	}
	status := metav1.ConditionTrue
	reason := meshv1alpha1.ReasonAirtimeBudgetOK
	if !airtime.WithinChannelBudget(predicted) {
		status = metav1.ConditionFalse
		reason = meshv1alpha1.ReasonAirtimeBudgetExceeded
	}
	meta.SetStatusCondition(&node.Status.Conditions, metav1.Condition{
		Type:               meshv1alpha1.ConditionAirtimeBudget,
		Status:             status,
		Reason:             reason,
		ObservedGeneration: gen,
		Message: fmt.Sprintf("changing modem preset %s to %s is predicted to move channel utilization from %.1f%% to ~%.1f%% (ceiling %.0f%%)",
			currentPreset, desiredPreset, *info.ChannelUtilization, predicted, airtime.RecommendedChannelUtilizationPercent),
	})
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
		For(&meshv1alpha1.MeshtasticNode{}, builder.WithPredicates(ignoreStatusOnly())).
		WithOptions(controller.Options{MaxConcurrentReconciles: 4}).
		Complete(r)
}

// lastHeardMinInterval is how old LastHeard must be before a reachable step
// rewrites it. Shorter than DriftCheckInterval so a 5-minute poll still
// refreshes the column, long enough that a burst of status writes cannot
// form a tight loop by themselves.
const lastHeardMinInterval = 30 * time.Second

// ignoreStatusOnly drops watch events that are only a status write. Spec
// generation changes, finalizer edits, and deletion still reconcile. Combined
// with RequeueAfter this is what makes DriftCheckInterval real: without it
// every LastHeard bump re-enqueues the same node immediately.
func ignoreStatusOnly() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return true
			}
			if !e.ObjectNew.GetDeletionTimestamp().IsZero() {
				return true
			}
			if e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() {
				return true
			}
			return !sameStrings(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers())
		},
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
