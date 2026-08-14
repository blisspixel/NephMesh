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

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

// ErrUnreachable indicates the device API could not be reached, for example
// because the device is mid-reboot. It is expected and drives requeue, not a
// hard failure.
var ErrUnreachable = errors.New("device unreachable")

// ErrUnsupported indicates the node's declared transport is not implemented
// (serial and viaGateway in the in-cluster operator). It is not a transient
// connect failure: retrying every ReconnectBackoff would hammer the worker
// for a spec that cannot succeed until the transport is built.
var ErrUnsupported = errors.New("device transport not implemented")

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
	// ApplyChannels writes the given channels to the device, which reboots as a
	// result (like Apply). Channels apply through a path distinct from the scalar
	// config because the device keys them by slot; the pre-shared keys are passed
	// to the device only through a file, never a process argument or a log line,
	// so they keep the same non-exposure the broker password gets.
	ApplyChannels(ctx context.Context, channels []ChannelWrite) error
}

// ChannelWrite is one channel to write to the device. Key is the raw pre-shared
// key, wrapped so it is never rendered in a log or error; a zero Key means use
// the device's public default key. It is revealed only at the point the apply
// file is written.
type ChannelWrite struct {
	Index           int32
	Name            string
	Key             secret.Value
	UplinkEnabled   bool
	DownlinkEnabled bool
}

// Info is a small snapshot of device identity used to populate status. It
// carries only what the client can actually produce today (the node id parsed
// from the CLI); firmware version and neighbor count are added when their parse
// paths are verified against real hardware, rather than advertised as empty.
type Info struct {
	NodeID string
	// AirUtilTx and ChannelUtilization are the radio's own airtime telemetry
	// (percent), reported in --info deviceMetrics. Pointers so an absent metric
	// is distinct from a real 0.0 on an idle node. Airtime is the LoRa scaling
	// wall, so this is the ground-truth measurement of how loaded the channel is
	// (see docs/plans/airtime-budget.md).
	AirUtilTx          *float64
	ChannelUtilization *float64
}
