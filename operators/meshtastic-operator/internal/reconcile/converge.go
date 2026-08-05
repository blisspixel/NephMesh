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
	ReconnectBackoff  = 10 * time.Second
	RebootWait        = 25 * time.Second
	DriftCheckInterval = 5 * time.Minute
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
	Reason        string
	Requeue       time.Duration
	// Info is populated when the device was reachable this step.
	Info device.Info
}

// Converge performs one non-blocking step toward making the device match
// desired. wasRebootPending carries the prior RebootPending condition so a
// device that is unreachable right after an apply is reported as still
// rebooting rather than as a fresh connection failure. It returns an error only
// for genuinely unexpected failures (which the controller should retry with
// backoff); an unreachable device is a normal requeue, not an error.
func Converge(ctx context.Context, dev device.Client, desired map[string]any, wasRebootPending bool) (Outcome, error) {
	live, err := dev.ExportConfig(ctx)
	if errors.Is(err, device.ErrUnreachable) {
		reason := meshv1alpha1.ReasonConnectFailed
		if wasRebootPending {
			reason = meshv1alpha1.ReasonConfigApplied
		}
		return Outcome{Reachable: false, RebootPending: wasRebootPending, Reason: reason, Requeue: ReconnectBackoff}, nil
	}
	if err != nil {
		return Outcome{}, err
	}

	if config.IsConverged(desired, live) {
		info, _ := dev.Info(ctx) // best effort; identity is not load-bearing here
		return Outcome{
			Reachable: true, ConfigInSync: true, Ready: true,
			Reason: meshv1alpha1.ReasonInSync, Info: info, Requeue: DriftCheckInterval,
		}, nil
	}

	if err := dev.Apply(ctx, desired); err != nil {
		if errors.Is(err, device.ErrUnreachable) {
			return Outcome{Reachable: false, RebootPending: wasRebootPending, Reason: meshv1alpha1.ReasonConfigApplied, Requeue: ReconnectBackoff}, nil
		}
		return Outcome{}, err
	}
	// The MQTT module thread starts only at boot, so reboot explicitly after an
	// apply to make its activation deterministic. Best effort: the device may
	// already be rebooting from the apply itself.
	_ = dev.Reboot(ctx)

	return Outcome{
		Reachable: true, ConfigInSync: false, RebootPending: true,
		Reason: meshv1alpha1.ReasonConfigApplied, Requeue: RebootWait,
	}, nil
}
