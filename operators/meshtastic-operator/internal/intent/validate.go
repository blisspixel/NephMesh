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

package intent

import (
	"fmt"
	"regexp"
	"unicode/utf8"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

// Bounds and patterns below match the kubebuilder markers on
// CommunicationIntentSpec and the embedded MeshtasticNode fields (region,
// role, channels, connection, owner, expectedTraffic). OpenAPI maxLength counts
// Unicode code points, so lengths use rune counts. A payloadBytes of 0 is
// treated as unset: the Go field cannot tell an omitted value from an explicit
// 0, and an omitted payload means "use the representative size".

const (
	maxRegionLen         = 32
	maxRoleLen           = 32
	maxPresets           = 16
	maxChannels          = 8
	maxChannelNameLen    = 12
	minChannelIndex      = 0
	maxChannelIndex      = 7
	minMessagesPerMinute = 1
	maxMessagesPerMinute = 600
	maxPayloadBytes      = 237
	maxNodes             = 64
	maxNodeNameLen       = 253
	maxHostLen           = 253
	maxSerialLen         = 255
	minPort              = 1
	maxPort              = 65535
	maxShortNameLen      = 4
	maxLongNameLen       = 40
)

var (
	regionPattern   = regexp.MustCompile(`^[A-Z0-9_]+$`)
	nodeNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	hostPattern     = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$|^[0-9a-fA-F:]*:[0-9a-fA-F:]+$`)
	serialPattern   = regexp.MustCompile(`^[a-zA-Z0-9/][a-zA-Z0-9/._-]*$`)
	destPattern     = regexp.MustCompile(`^![0-9a-fA-F]{8}$`)
)

// validate reports the first constraint the CRD or a Meshtastic device would
// refuse. Field order follows the spec: region, approved presets (count only),
// role, channels, expectedTraffic, then each node. Known-preset selection stays
// in Compile, because an approved set may list an unknown preset behind a known one.
func validate(spec intentv1alpha1.CommunicationIntentSpec) (string, string) {
	if reason, message := validateRegion(spec.Region); reason != "" {
		return reason, message
	}
	if reason, message := validatePresets(spec.ApprovedModemPresets); reason != "" {
		return reason, message
	}
	if reason, message := validateRole(spec.Role); reason != "" {
		return reason, message
	}
	if reason, message := validateChannels(spec.Channels); reason != "" {
		return reason, message
	}
	if reason, message := validateTraffic(spec.ExpectedTraffic); reason != "" {
		return reason, message
	}
	return validateNodes(spec.Nodes)
}

func validateRegion(region string) (string, string) {
	if region == "" {
		return intentv1alpha1.ReasonInvalidRegion, "region is required"
	}
	if utf8.RuneCountInString(region) > maxRegionLen {
		return intentv1alpha1.ReasonInvalidRegion, fmt.Sprintf("region %q is longer than %d characters", region, maxRegionLen)
	}
	if !regionPattern.MatchString(region) {
		return intentv1alpha1.ReasonInvalidRegion, fmt.Sprintf("region %q must contain only A-Z, digits, and underscores", region)
	}
	if !knownRegion(region) {
		return intentv1alpha1.ReasonInvalidRegion, fmt.Sprintf("region %q is not a Meshtastic region", region)
	}
	return "", ""
}

func validateRole(role string) (string, string) {
	if role == "" {
		return "", ""
	}
	if utf8.RuneCountInString(role) > maxRoleLen {
		return intentv1alpha1.ReasonInvalidRole, fmt.Sprintf("role %q is longer than %d characters", role, maxRoleLen)
	}
	// Region and role share the CRD pattern ^[A-Z0-9_]+$.
	if !regionPattern.MatchString(role) {
		return intentv1alpha1.ReasonInvalidRole, fmt.Sprintf("role %q must contain only A-Z, digits, and underscores", role)
	}
	if !knownRole(role) {
		return intentv1alpha1.ReasonInvalidRole, fmt.Sprintf("role %q is not a Meshtastic device role", role)
	}
	return "", ""
}

func validatePresets(presets []string) (string, string) {
	if len(presets) == 0 {
		return intentv1alpha1.ReasonNoApprovedSet, "approvedModemPresets is empty; no preset to render"
	}
	if len(presets) > maxPresets {
		return intentv1alpha1.ReasonTooManyPresets, fmt.Sprintf("approvedModemPresets has %d entries; the maximum is %d", len(presets), maxPresets)
	}
	return "", ""
}

func validateChannels(channels []meshv1alpha1.ChannelSpec) (string, string) {
	if len(channels) > maxChannels {
		return intentv1alpha1.ReasonTooManyChannels, fmt.Sprintf("channels has %d entries; the maximum is %d", len(channels), maxChannels)
	}
	seen := make(map[int32]struct{}, len(channels))
	for _, ch := range channels {
		if ch.Index < minChannelIndex || ch.Index > maxChannelIndex {
			return intentv1alpha1.ReasonChannelIndexOutOfRange, fmt.Sprintf("channel index %d is outside %d..%d", ch.Index, minChannelIndex, maxChannelIndex)
		}
		if _, dup := seen[ch.Index]; dup {
			return intentv1alpha1.ReasonDuplicateChannel, fmt.Sprintf("duplicate channel index %d", ch.Index)
		}
		seen[ch.Index] = struct{}{}
		if utf8.RuneCountInString(ch.Name) > maxChannelNameLen {
			return intentv1alpha1.ReasonChannelNameTooLong, fmt.Sprintf("channel index %d name %q is longer than %d characters", ch.Index, ch.Name, maxChannelNameLen)
		}
		if ch.PSKSecretRef != nil && ch.PSKSecretRef.Key == "" {
			return intentv1alpha1.ReasonPSKSecretKeyRequired, fmt.Sprintf("channel index %d pskSecretRef.key is required", ch.Index)
		}
	}
	return "", ""
}

func validateTraffic(traffic *intentv1alpha1.ExpectedTraffic) (string, string) {
	if traffic == nil {
		return "", ""
	}
	rate := traffic.MessagesPerMinutePerNode
	if rate < minMessagesPerMinute || rate > maxMessagesPerMinute {
		return intentv1alpha1.ReasonMessagesPerMinuteOutOfRange, fmt.Sprintf("expectedTraffic.messagesPerMinutePerNode %d is outside %d..%d", rate, minMessagesPerMinute, maxMessagesPerMinute)
	}
	// 0 means the field was omitted (see the file comment). A negative value or
	// a value past the Meshtastic maximum payload is a CRD refusal.
	if traffic.PayloadBytes != 0 && (traffic.PayloadBytes < 1 || traffic.PayloadBytes > maxPayloadBytes) {
		return intentv1alpha1.ReasonPayloadBytesOutOfRange, fmt.Sprintf("expectedTraffic.payloadBytes %d is outside 1..%d", traffic.PayloadBytes, maxPayloadBytes)
	}
	return "", ""
}

func validateNodes(nodes []intentv1alpha1.NodeTarget) (string, string) {
	if len(nodes) == 0 {
		return intentv1alpha1.ReasonNoTargetNodes, "no target nodes to render"
	}
	if len(nodes) > maxNodes {
		return intentv1alpha1.ReasonTooManyNodes, fmt.Sprintf("nodes has %d entries; the maximum is %d", len(nodes), maxNodes)
	}
	seen := make(map[string]struct{}, len(nodes))
	for _, n := range nodes {
		if reason, message := validateNodeName(n.Name); reason != "" {
			return reason, message
		}
		if _, dup := seen[n.Name]; dup {
			return intentv1alpha1.ReasonDuplicateNode, fmt.Sprintf("duplicate node name %q: two targets cannot render to the same MeshtasticNode", n.Name)
		}
		seen[n.Name] = struct{}{}
		if reason, message := validateConnection(n); reason != "" {
			return reason, message
		}
		if reason, message := validateOwner(n); reason != "" {
			return reason, message
		}
	}
	return "", ""
}

func validateNodeName(name string) (string, string) {
	if utf8.RuneCountInString(name) > maxNodeNameLen {
		return intentv1alpha1.ReasonNodeNameInvalid, fmt.Sprintf("node name %q is longer than %d characters", name, maxNodeNameLen)
	}
	if !nodeNamePattern.MatchString(name) {
		return intentv1alpha1.ReasonNodeNameInvalid, fmt.Sprintf("node name %q must be a lowercase DNS label", name)
	}
	return "", ""
}

func validateConnection(n intentv1alpha1.NodeTarget) (string, string) {
	set := 0
	if n.Connection.TCP != nil {
		set++
	}
	if n.Connection.Serial != nil {
		set++
	}
	if n.Connection.ViaGateway != nil {
		set++
	}
	if set != 1 {
		return intentv1alpha1.ReasonConnectionInvalid, fmt.Sprintf("node %q connection must set exactly one of tcp, serial, or viaGateway", n.Name)
	}
	if tcp := n.Connection.TCP; tcp != nil {
		if utf8.RuneCountInString(tcp.Host) > maxHostLen {
			return intentv1alpha1.ReasonHostInvalid, fmt.Sprintf("node %q tcp.host is longer than %d characters", n.Name, maxHostLen)
		}
		if !hostPattern.MatchString(tcp.Host) {
			return intentv1alpha1.ReasonHostInvalid, fmt.Sprintf("node %q tcp.host %q is not a hostname, IPv4 address, or IPv6 address", n.Name, tcp.Host)
		}
		if tcp.Port != 0 && (tcp.Port < minPort || tcp.Port > maxPort) {
			return intentv1alpha1.ReasonPortOutOfRange, fmt.Sprintf("node %q tcp.port %d is outside %d..%d", n.Name, tcp.Port, minPort, maxPort)
		}
	}
	if serial := n.Connection.Serial; serial != nil {
		if utf8.RuneCountInString(serial.Device) > maxSerialLen {
			return intentv1alpha1.ReasonSerialDeviceInvalid, fmt.Sprintf("node %q serial.device is longer than %d characters", n.Name, maxSerialLen)
		}
		if !serialPattern.MatchString(serial.Device) {
			return intentv1alpha1.ReasonSerialDeviceInvalid, fmt.Sprintf("node %q serial.device %q is not a device path", n.Name, serial.Device)
		}
	}
	if gw := n.Connection.ViaGateway; gw != nil {
		if !destPattern.MatchString(gw.Dest) {
			return intentv1alpha1.ReasonGatewayDestInvalid, fmt.Sprintf("node %q viaGateway.dest %q must be ! plus 8 hex digits", n.Name, gw.Dest)
		}
	}
	return "", ""
}

func validateOwner(n intentv1alpha1.NodeTarget) (string, string) {
	if n.Owner == nil {
		return "", ""
	}
	if utf8.RuneCountInString(n.Owner.ShortName) > maxShortNameLen {
		return intentv1alpha1.ReasonShortNameTooLong, fmt.Sprintf("node %q owner.shortName %q is longer than %d characters", n.Name, n.Owner.ShortName, maxShortNameLen)
	}
	if utf8.RuneCountInString(n.Owner.LongName) > maxLongNameLen {
		return intentv1alpha1.ReasonLongNameTooLong, fmt.Sprintf("node %q owner.longName %q is longer than %d characters", n.Name, n.Owner.LongName, maxLongNameLen)
	}
	return "", ""
}
