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

// Package device abstracts the Meshtastic device control surface so the
// reconciler can be developed and tested without hardware. The real
// implementation drives the CLI sidecar over the TCP 4403 API; tests use the
// in-memory Fake, which reproduces the two behaviors that shape the reconcile
// loop: the device is single-client and it reboots on config apply, becoming
// briefly unreachable.
package device

import (
	"context"
	"errors"
)

// ErrUnreachable indicates the device API could not be reached, for example
// because the device is mid-reboot. It is expected and drives requeue, not a
// hard failure.
var ErrUnreachable = errors.New("device unreachable")

// Client is the minimal control surface the reconciler needs. Every method
// returns ErrUnreachable when the device cannot be reached so the caller can
// distinguish "try again shortly" from a genuine error.
type Client interface {
	// ExportConfig returns the device's live configuration as decoded YAML.
	ExportConfig(ctx context.Context) (map[string]any, error)
	// Apply writes the desired configuration. The device reboots as a result,
	// so it is expected to be unreachable shortly after this returns.
	Apply(ctx context.Context, desired map[string]any) error
	// Reboot restarts the device. Config module threads (for example MQTT)
	// start only at boot, so an explicit reboot makes their activation
	// deterministic after an Apply.
	Reboot(ctx context.Context) error
	// Info returns lightweight device identity and liveness for status.
	Info(ctx context.Context) (Info, error)
}

// Info is a small snapshot of device identity used to populate status. It
// carries only what the client can actually produce today (the node id parsed
// from the CLI); firmware version and neighbor count are added when their parse
// paths are verified against real hardware, rather than advertised as empty.
type Info struct {
	NodeID string
}
