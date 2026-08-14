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

// Package reconcile holds the device convergence state machine, kept separate
// from the controller-runtime wiring so it can be unit-tested against a fake
// device with no cluster and no hardware. It never blocks: each call does one
// bounded unit of work and reports how long to wait before the next call, so
// the controller worker is never parked waiting on a rebooting device.
package reconcile

import (
	"context"
	"errors"
	"time"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
)

// Timing for the non-blocking loop. Reconnect is paced (not tight) because the
// device API is single-client and dislikes rapid reconnection; the reboot wait
// approximates how long a config-triggered reboot takes; the drift check is a
// slow steady-state poll.
const (
	ReconnectBackoff   = 10 * time.Second
	RebootWait         = 25 * time.Second
	DriftCheckInterval = 5 * time.Minute
	// MaxApplyAttempts bounds the apply-and-reboot loop. If the device has not
	// converged after this many consecutive applies, the desired state is very
	// likely unreachable (for example a field whose value the device never
	// echoes back in its export), so the node is marked Degraded and left
	// alone rather than rebooting every RebootWait forever.
	MaxApplyAttempts = 5
)

// Outcome is the result of one convergence step: the condition state to record
// and how long to wait before the next step. It is deliberately plain data so
// the controller can translate it into metav1.Condition writes and a
// ctrl.Result without the state machine knowing about Kubernetes.
type Outcome struct {
	Reachable     bool
	ConfigInSync  bool
	RebootPending bool
	Ready         bool
	Degraded      bool
	Reason        string
	Requeue       time.Duration
	// ApplyAttempts is the running count of consecutive non-converging applies,
	// to be persisted in status and fed back on the next step.
	ApplyAttempts int32
	// Info is populated when the device was reachable this step.
	Info device.Info
	// LiveChannels is the device's channel set as observed in this step's export
	// (empty when the device was unreachable). It is surfaced so the controller,
	// which owns Secret resolution, can compare it against the declared channels
	// and report drift, without a second export. Channels are not part of the
	// converging config diff (they apply through a distinct path), so this is
	// observability, not a gate.
	LiveChannels []config.ChannelState
	// CurrentModemPreset is the modem preset the device reports in this step's
	// export (empty when unreachable or not preset-based). It is surfaced so the
	// controller can predict the airtime effect of a declared preset change
	// against the radio's own measured utilization.
	CurrentModemPreset string
	// Drift is the list of declared fields still not matching the device this
	// step (dotted config paths and channel[i].field entries), so the controller
	// can name what has not converged in a condition message and a log line rather
	// than reporting a bare "not in sync".
	Drift []string
	// MQTTPasswordHash is the SHA-256 of the desired MQTT password to persist
	// as last-applied (empty when no password is declared). The controller
	// writes it to status so a Secret-only rotation is visible next step.
	MQTTPasswordHash string
	// ChannelsUnobserved is true when the node declares channels but the export
	// did not include a channel set (stock --export-config). Converge does not
	// treat that as missing-channel drift, which would apply-and-reboot forever.
	ChannelsUnobserved bool
}

// State is the reconcile memory Converge needs from the previous step: whether a
// reboot was pending, how many consecutive applies have not yet converged, and
// the last-applied MQTT password hash (write-only on the device).
type State struct {
	RebootPending    bool
	ApplyAttempts    int32
	MQTTPasswordHash string
}

// NextState is the State to feed the next Converge call.
func (o Outcome) NextState() State {
	return State{
		RebootPending:    o.RebootPending,
		ApplyAttempts:    o.ApplyAttempts,
		MQTTPasswordHash: o.MQTTPasswordHash,
	}
}

// DesiredChannels is what Converge needs to reconcile channels: the comparable
// state to detect drift (Compare) and the write payload to apply it (Write). The
// controller builds both together from the spec and the resolved Secrets, so
// Converge stays free of Kubernetes and Secret handling. An empty value means the
// node declares no channels, and channel reconciliation is a no-op.
type DesiredChannels struct {
	Compare []config.ChannelState
	Write   []device.ChannelWrite
}

// Converge performs one non-blocking step toward making the device match
// desired. The prior State carries the RebootPending condition (so a device
// unreachable right after an apply is reported as still rebooting, not as a
// fresh connection failure) and the apply-attempt count (so the reboot loop is
// bounded). It returns an error only for genuinely unexpected failures (which
// the controller should retry with backoff); an unreachable device is a normal
// requeue, not an error.
func Converge(ctx context.Context, dev device.Client, desired map[string]any, chans DesiredChannels, prior State) (Outcome, error) {
	live, err := dev.ExportConfig(ctx)
	if errors.Is(err, device.ErrUnreachable) {
		reason := meshv1alpha1.ReasonConnectFailed
		if prior.RebootPending {
			reason = meshv1alpha1.ReasonConfigApplied
		}
		return unreachableOutcome(prior, reason), nil
	}
	if err != nil {
		return Outcome{ApplyAttempts: prior.ApplyAttempts, MQTTPasswordHash: prior.MQTTPasswordHash}, err
	}

	// Read telemetry once, up front. It carries the node id for status and the
	// radio's measured channel utilization, which the airtime prediction needs
	// even when the config is drifted (a pending preset change is exactly when the
	// prediction matters). If the single-client device dropped between the export
	// and this call (it is mid-reboot), treat the step as unreachable rather than
	// reporting a converged, Ready state with no identity.
	// Prefer the local node's identity and airtime telemetry from the export
	// itself: the exporter reads them from the library's own local-node view,
	// which is reliable on a multi-node mesh, unlike scanning --info for the first
	// "!id" and the first metric (which may belong to a neighbor). Fall back to
	// --info only when the export did not carry them (the CLI --export-config path
	// with no exporter configured).
	info := infoFromExport(live)
	if info.NodeID == "" {
		var infoErr error
		info, infoErr = dev.Info(ctx)
		if errors.Is(infoErr, device.ErrUnreachable) {
			reason := meshv1alpha1.ReasonConnectFailed
			if prior.RebootPending {
				reason = meshv1alpha1.ReasonConfigApplied
			}
			return unreachableOutcome(prior, reason), nil
		}
		if infoErr != nil {
			// An unexpected --info failure (not a mid-reboot drop): surface it for
			// a rate-limited retry rather than proceeding with empty identity and
			// possibly reporting a converged, Ready state built on nothing.
			return Outcome{ApplyAttempts: prior.ApplyAttempts, MQTTPasswordHash: prior.MQTTPasswordHash}, infoErr
		}
	}
	liveChannels := config.LiveChannels(live)
	currentPreset := liveModemPreset(live)
	passwordHash := config.WriteOnlyPasswordHash(desired)
	scalarDrift := config.Drift(config.ForComparison(desired), live)
	// A Secret-only MQTT password rotation is invisible in the live export
	// (write-only). Compare the desired hash to the last-applied hash.
	writeOnlyDrift := passwordHash != "" && passwordHash != prior.MQTTPasswordHash
	if writeOnlyDrift {
		scalarDrift = append(scalarDrift, "module_config.mqtt.password")
	}
	channelsUnknown := len(chans.Compare) > 0 && !config.ChannelSetPresent(live)
	var channelDrift []string
	if !channelsUnknown {
		channelDrift = config.ChannelDrift(chans.Compare, liveChannels)
	}
	scalarConverged := len(scalarDrift) == 0
	channelsConverged := !channelsUnknown && len(channelDrift) == 0
	allDrift := append(append([]string{}, scalarDrift...), channelDrift...)

	if scalarConverged && channelsConverged {
		if info.NodeID == "" {
			// Config is converged but neither the export nor the --info fallback
			// carried a node id (parseInfo never errors, so an empty read reaches
			// here). Do not report a Ready state on no identity, and do not let the
			// controller blank a previously-good status node id: requeue for a fresh
			// read as if mid-reboot. This only guards the Ready path; the apply and
			// reboot flow below runs regardless of identity.
			reason := meshv1alpha1.ReasonConnectFailed
			if prior.RebootPending {
				reason = meshv1alpha1.ReasonConfigApplied
			}
			return unreachableOutcome(prior, reason), nil
		}
		return Outcome{
			Reachable: true, ConfigInSync: true, Ready: true,
			Reason: meshv1alpha1.ReasonInSync, Info: info, Requeue: DriftCheckInterval,
			ApplyAttempts:      0, // converged: reset the counter
			LiveChannels:       liveChannels,
			CurrentModemPreset: currentPreset,
			MQTTPasswordHash:   passwordHash,
		}, nil
	}

	// Drift remains (scalar config, channels, or both). If we have already applied
	// MaxApplyAttempts times without converging, stop: the desired state is very
	// likely something the device will not echo back, so rebooting again would
	// loop forever. Surface it.
	if prior.ApplyAttempts >= MaxApplyAttempts {
		return Outcome{
			Reachable: true, ConfigInSync: scalarConverged, Degraded: true,
			Reason: meshv1alpha1.ReasonApplyFailed, ApplyAttempts: prior.ApplyAttempts,
			Requeue:            DriftCheckInterval,
			Info:               info,
			LiveChannels:       liveChannels,
			CurrentModemPreset: currentPreset,
			Drift:              allDrift,
			MQTTPasswordHash:   prior.MQTTPasswordHash,
			ChannelsUnobserved: channelsUnknown,
		}, nil
	}

	// Declared channels but the export had no channel set: do not apply
	// channels (that would reboot-loop against stock --export-config) and do
	// not report Ready. If scalar config is also drifted, apply that only.
	if channelsUnknown && scalarConverged {
		return Outcome{
			Reachable: true, ConfigInSync: true, Ready: false,
			Reason:             meshv1alpha1.ReasonChannelsUnobserved,
			Requeue:            ReconnectBackoff,
			ApplyAttempts:      prior.ApplyAttempts,
			Info:               info,
			LiveChannels:       liveChannels,
			CurrentModemPreset: currentPreset,
			MQTTPasswordHash:   prior.MQTTPasswordHash,
			ChannelsUnobserved: true,
		}, nil
	}

	// Apply the surface that drifted. Scalar config (including a write-only
	// password rotation) goes first: it carries the module config whose reboot
	// also settles the device, and only when the scalar config already matches
	// do we apply channels (their own distinct, key-bearing path). Either apply
	// reboots the device.
	var applyErr error
	if !scalarConverged {
		applyErr = dev.Apply(ctx, desired)
	} else {
		applyErr = dev.ApplyChannels(ctx, chans.Write)
	}
	if applyErr != nil {
		if errors.Is(applyErr, device.ErrUnreachable) {
			// The write likely landed and the device is already rebooting
			// (session dropped). Count it toward the apply bound and wait for
			// the reboot, the same as a clean apply, or a field the device
			// never echoes would reboot forever.
			appliedHash := prior.MQTTPasswordHash
			if !scalarConverged {
				appliedHash = passwordHash
			}
			return Outcome{
				Reachable: false, RebootPending: true,
				ApplyAttempts:    prior.ApplyAttempts + 1,
				Reason:           meshv1alpha1.ReasonConfigApplied,
				Requeue:          RebootWait,
				MQTTPasswordHash: appliedHash,
			}, nil
		}
		return Outcome{ApplyAttempts: prior.ApplyAttempts, MQTTPasswordHash: prior.MQTTPasswordHash}, applyErr
	}
	// The MQTT module thread starts only at boot, and a channel write reboots the
	// device on its own, so reboot explicitly to make activation deterministic.
	// The device may already be rebooting from the apply (unreachable, or any
	// other reboot error after a successful write). Record reboot-pending so
	// the attempt count is persisted; the controller used to discard Outcome
	// on a reboot error and the apply bound never advanced.
	_ = dev.Reboot(ctx)

	appliedHash := prior.MQTTPasswordHash
	if !scalarConverged {
		appliedHash = passwordHash
	}
	return Outcome{
		Reachable: true, ConfigInSync: scalarConverged && !writeOnlyDrift, RebootPending: true,
		Reason: meshv1alpha1.ReasonConfigApplied, Requeue: RebootWait,
		ApplyAttempts:      prior.ApplyAttempts + 1,
		Info:               info,
		LiveChannels:       liveChannels,
		CurrentModemPreset: currentPreset,
		Drift:              allDrift,
		MQTTPasswordHash:   appliedHash,
		ChannelsUnobserved: channelsUnknown,
	}, nil
}

// infoFromExport reads the local node id and airtime telemetry the exporter
// emits, so the reliable local-node values are used without a separate --info
// call. NodeID is empty when the export did not carry it (the CLI export path).
func infoFromExport(live map[string]any) device.Info {
	var info device.Info
	if id, ok := live["nodeId"].(string); ok {
		info.NodeID = id
	}
	if dm, ok := live["deviceMetrics"].(map[string]any); ok {
		info.AirUtilTx = floatFromAny(dm["airUtilTx"])
		info.ChannelUtilization = floatFromAny(dm["channelUtilization"])
	}
	return info
}

func floatFromAny(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		f := float64(n)
		return &f
	case int:
		f := float64(n)
		return &f
	case int64:
		f := float64(n)
		return &f
	default:
		return nil
	}
}

// unreachableOutcome is a device that did not answer this step. A node that
// already hit the apply bound stays Degraded rather than flickering recovered
// while it is offline.
func unreachableOutcome(prior State, reason string) Outcome {
	return Outcome{
		Reachable:        false,
		RebootPending:    prior.RebootPending,
		ApplyAttempts:    prior.ApplyAttempts,
		Degraded:         prior.ApplyAttempts >= MaxApplyAttempts,
		Reason:           reason,
		Requeue:          ReconnectBackoff,
		MQTTPasswordHash: prior.MQTTPasswordHash,
	}
}

// liveModemPreset reads the modem preset from a device export, or "" when absent.
func liveModemPreset(live map[string]any) string {
	cfg, ok := live["config"].(map[string]any)
	if !ok {
		return ""
	}
	lora, ok := cfg["lora"].(map[string]any)
	if !ok {
		return ""
	}
	preset, _ := lora["modemPreset"].(string)
	return preset
}
