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

package v1alpha1

// Condition types. All are positive-polarity per the Kubernetes convention
// (never a "NotReady" type; use Ready=False instead).
const (
	// ConditionReachable is True when a device API session is established.
	ConditionReachable = "Reachable"
	// ConditionConfigInSync is True when the exported config matches the spec.
	ConditionConfigInSync = "ConfigInSync"
	// ConditionRebootPending is True after a config write, while the device
	// reboots and before the applied config is re-verified.
	ConditionRebootPending = "RebootPending"
	// ConditionReady is the composite: Reachable and ConfigInSync and not
	// RebootPending.
	ConditionReady = "Ready"
	// ConditionDegraded is True when an apply verified-failed after retries.
	ConditionDegraded = "Degraded"
	// ConditionAirtimeHealthy is True when the device's measured airtime
	// utilization is within the recommended ceilings, False when the channel is
	// saturating. It is observability, not a gate: airtime is the LoRa scaling
	// wall, so a saturating channel is surfaced rather than silently degrading.
	ConditionAirtimeHealthy = "AirtimeHealthy"
	// ConditionChannelsInSync is True when every channel the spec declares
	// matches the device (name, uplink, downlink, and key compared by hash),
	// False when any declared channel drifts. It is observability today: the
	// operator reports channel drift but does not yet apply it, because the
	// channel apply is a distinct path (the export encodes channels as a single
	// channel_url), so detecting drift is deliberately decoupled from acting on
	// it until the apply is validated against a device.
	ConditionChannelsInSync = "ChannelsInSync"
)

// Condition reasons. CamelCase machine tokens, required by the conditions API.
const (
	ReasonConnected          = "Connected"
	ReasonConnectFailed      = "ConnectFailed"
	ReasonGatewayUnreachable = "GatewayUnreachable"

	ReasonInSync        = "InSync"
	ReasonDriftDetected = "DriftDetected"
	ReasonApplyFailed   = "ApplyFailed"
	ReasonConfigApplied = "ConfigApplied"
	ReasonSecretMissing = "SecretMissing"

	ReasonReady    = "Ready"
	ReasonNotReady = "NotReady"

	ReasonAirtimeHealthy = "AirtimeHealthy"
	ReasonAirtimeHigh    = "AirtimeHigh"

	ReasonChannelsInSync  = "ChannelsInSync"
	ReasonChannelsDrifted = "ChannelsDrifted"
)

// DeletionPolicy values.
const (
	// DeletionPolicyRetain stops managing the device on delete and leaves it
	// running with its last-applied config. This is the default.
	DeletionPolicyRetain = "Retain"
	// DeletionPolicyWipe factory-resets the device during finalization.
	DeletionPolicyWipe = "Wipe"
)

// Finalizer is added to a MeshtasticNode so the controller can quiesce the
// device link before the object is removed.
const Finalizer = "mesh.nephmesh.io/finalizer"
