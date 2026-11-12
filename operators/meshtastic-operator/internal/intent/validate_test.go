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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

func baseSpec() intentv1alpha1.CommunicationIntentSpec {
	return intentv1alpha1.CommunicationIntentSpec{
		Region:               "US",
		ApprovedModemPresets: []string{"LONG_FAST"},
		Nodes: []intentv1alpha1.NodeTarget{{
			Name:       "n1",
			Connection: tcp("10.0.0.1"),
		}},
	}
}

func TestCompileRefusesCRDAndDeviceViolations(t *testing.T) {
	longRegion := strings.Repeat("A", 33)
	longRole := strings.Repeat("B", 33)
	longNode := strings.Repeat("a", 254)
	longHost := strings.Repeat("h", 254)
	longSerial := strings.Repeat("s", 256)
	longName := strings.Repeat("N", 41)
	cases := []struct {
		name   string
		reason string
		mutate func(*intentv1alpha1.CommunicationIntentSpec)
	}{
		{"unknown region", intentv1alpha1.ReasonInvalidRegion, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Region = "MARS"
		}},
		{"unset region", intentv1alpha1.ReasonInvalidRegion, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Region = "UNSET"
		}},
		{"empty region", intentv1alpha1.ReasonInvalidRegion, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Region = ""
		}},
		{"region pattern", intentv1alpha1.ReasonInvalidRegion, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Region = "us-west"
		}},
		{"region length", intentv1alpha1.ReasonInvalidRegion, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Region = longRegion
		}},
		{"unknown role", intentv1alpha1.ReasonInvalidRole, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Role = "OVERLORD"
		}},
		{"role pattern", intentv1alpha1.ReasonInvalidRole, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Role = "client"
		}},
		{"role length", intentv1alpha1.ReasonInvalidRole, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Role = longRole
		}},
		{"too many presets", intentv1alpha1.ReasonTooManyPresets, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.ApprovedModemPresets = make([]string, 17)
			for i := range s.ApprovedModemPresets {
				s.ApprovedModemPresets[i] = "LONG_FAST"
			}
		}},
		{"channel index high", intentv1alpha1.ReasonChannelIndexOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Channels = []meshv1alpha1.ChannelSpec{{Index: 99, Name: "x"}}
		}},
		{"channel index negative", intentv1alpha1.ReasonChannelIndexOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Channels = []meshv1alpha1.ChannelSpec{{Index: -1}}
		}},
		{"duplicate channel", intentv1alpha1.ReasonDuplicateChannel, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Channels = []meshv1alpha1.ChannelSpec{{Index: 1, Name: "a"}, {Index: 1, Name: "b"}}
		}},
		{"channel name", intentv1alpha1.ReasonChannelNameTooLong, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Channels = []meshv1alpha1.ChannelSpec{{Index: 1, Name: strings.Repeat("c", 13)}}
		}},
		{"too many channels", intentv1alpha1.ReasonTooManyChannels, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Channels = make([]meshv1alpha1.ChannelSpec, 9)
		}},
		{"psk key required", intentv1alpha1.ReasonPSKSecretKeyRequired, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Channels = []meshv1alpha1.ChannelSpec{{
				Index:        1,
				PSKSecretRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "psk"}},
			}}
		}},
		{"negative traffic", intentv1alpha1.ReasonMessagesPerMinuteOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: -5}
		}},
		{"zero traffic", intentv1alpha1.ReasonMessagesPerMinuteOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 0}
		}},
		{"traffic above max", intentv1alpha1.ReasonMessagesPerMinuteOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 601}
		}},
		{"payload above max", intentv1alpha1.ReasonPayloadBytesOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 5, PayloadBytes: 500}
		}},
		{"negative payload", intentv1alpha1.ReasonPayloadBytesOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 5, PayloadBytes: -1}
		}},
		{"too many nodes", intentv1alpha1.ReasonTooManyNodes, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes = make([]intentv1alpha1.NodeTarget, 65)
			for i := range s.Nodes {
				s.Nodes[i] = intentv1alpha1.NodeTarget{Name: "n", Connection: tcp("h")}
			}
		}},
		{"node name", intentv1alpha1.ReasonNodeNameInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Name = "Field_01"
		}},
		{"node name length", intentv1alpha1.ReasonNodeNameInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Name = longNode
		}},
		{"no transport", intentv1alpha1.ReasonConnectionInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection = meshv1alpha1.ConnectionSpec{}
		}},
		{"two transports", intentv1alpha1.ReasonConnectionInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection.Serial = &meshv1alpha1.SerialConnection{Device: "/dev/ttyUSB0"}
		}},
		{"host flag shape", intentv1alpha1.ReasonHostInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection.TCP.Host = "-evil.example.com"
		}},
		{"host port", intentv1alpha1.ReasonHostInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection.TCP.Host = "10.0.0.51:4403"
		}},
		{"host length", intentv1alpha1.ReasonHostInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection.TCP.Host = longHost
		}},
		{"port high", intentv1alpha1.ReasonPortOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection.TCP.Port = 70000
		}},
		{"port negative", intentv1alpha1.ReasonPortOutOfRange, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection.TCP.Port = -1
		}},
		{"serial path", intentv1alpha1.ReasonSerialDeviceInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection = meshv1alpha1.ConnectionSpec{Serial: &meshv1alpha1.SerialConnection{Device: "-/dev/ttyUSB0"}}
		}},
		{"serial length", intentv1alpha1.ReasonSerialDeviceInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection = meshv1alpha1.ConnectionSpec{Serial: &meshv1alpha1.SerialConnection{Device: longSerial}}
		}},
		{"gateway dest", intentv1alpha1.ReasonGatewayDestInvalid, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Connection = meshv1alpha1.ConnectionSpec{ViaGateway: &meshv1alpha1.ViaGatewayConnection{
				GatewayRef: corev1.LocalObjectReference{Name: "gw"},
				Dest:       "!nothex",
			}}
		}},
		{"short name", intentv1alpha1.ReasonShortNameTooLong, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Owner = &meshv1alpha1.OwnerSpec{ShortName: "TOOLONGNAME"}
		}},
		{"long name", intentv1alpha1.ReasonLongNameTooLong, func(s *intentv1alpha1.CommunicationIntentSpec) {
			s.Nodes[0].Owner = &meshv1alpha1.OwnerSpec{LongName: longName}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := baseSpec()
			tc.mutate(&spec)
			r := Compile(spec)
			assert.False(t, r.Feasible)
			assert.Equal(t, tc.reason, r.Reason)
			assert.NotEmpty(t, r.Message)
			assert.Empty(t, r.Proposed)
		})
	}
}

func TestCompileReportsFirstViolation(t *testing.T) {
	spec := baseSpec()
	spec.Region = "MARS"
	spec.Role = "OVERLORD"
	spec.Channels = []meshv1alpha1.ChannelSpec{{Index: 99, Name: "x"}}
	spec.Nodes[0].Owner = &meshv1alpha1.OwnerSpec{ShortName: "TOOLONG"}
	r := Compile(spec)
	assert.False(t, r.Feasible)
	assert.Equal(t, intentv1alpha1.ReasonInvalidRegion, r.Reason)
	assert.Contains(t, r.Message, "MARS")
}

func TestCompileAcceptsKnownRegionRoleAndBounds(t *testing.T) {
	spec := baseSpec()
	spec.Region = "EU_868"
	spec.Role = "ROUTER_LATE"
	spec.Channels = []meshv1alpha1.ChannelSpec{{Index: 0, Name: strings.Repeat("n", 12)}, {Index: 7, Name: "edge"}}
	spec.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 600, PayloadBytes: 237}
	spec.Nodes[0].Owner = &meshv1alpha1.OwnerSpec{ShortName: "RN01", LongName: strings.Repeat("L", 40)}
	spec.Nodes[0].Connection.TCP.Port = 0
	spec.Nodes[0].Connection.TCP.Host = "2001:db8::1"
	r := Compile(spec)
	require.True(t, r.Feasible, r.Message)
	assert.Equal(t, "EU_868", r.Proposed[0].Spec.Region)
	assert.Equal(t, "ROUTER_LATE", r.Proposed[0].Spec.Role)

	// Deprecated protobuf values are still device-accepted.
	spec.Region = "UA_868"
	spec.Role = "REPEATER"
	r = Compile(spec)
	require.True(t, r.Feasible, r.Message)

	serial := baseSpec()
	serial.Nodes[0].Connection = meshv1alpha1.ConnectionSpec{Serial: &meshv1alpha1.SerialConnection{Device: "/dev/ttyUSB0"}}
	require.True(t, Compile(serial).Feasible)

	gw := baseSpec()
	gw.Nodes[0].Connection = meshv1alpha1.ConnectionSpec{ViaGateway: &meshv1alpha1.ViaGatewayConnection{
		GatewayRef: corev1.LocalObjectReference{Name: "gw"},
		Dest:       "!6e000001",
	}}
	require.True(t, Compile(gw).Feasible)
}
