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

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func node(c ConnectionSpec) *MeshtasticNode {
	return &MeshtasticNode{Spec: MeshtasticNodeSpec{Connection: c, Region: "US"}}
}

func TestValidateConnectionExactlyOne(t *testing.T) {
	cases := []struct {
		name string
		conn ConnectionSpec
		ok   bool
	}{
		{"none", ConnectionSpec{}, false},
		{"tcp only", ConnectionSpec{TCP: &TCPConnection{Host: "h"}}, true},
		{"serial only", ConnectionSpec{Serial: &SerialConnection{Device: "/dev/ttyUSB0"}}, true},
		{"viaGateway only", ConnectionSpec{ViaGateway: &ViaGatewayConnection{
			GatewayRef: corev1.LocalObjectReference{Name: "gw"}, Dest: "!6e000001"}}, true},
		{"tcp and serial", ConnectionSpec{
			TCP: &TCPConnection{Host: "h"}, Serial: &SerialConnection{Device: "/dev/ttyUSB0"}}, false},
		{"all three", ConnectionSpec{
			TCP:        &TCPConnection{Host: "h"},
			Serial:     &SerialConnection{Device: "/dev/ttyUSB0"},
			ViaGateway: &ViaGatewayConnection{GatewayRef: corev1.LocalObjectReference{Name: "gw"}, Dest: "!x"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := node(tc.conn).Validate()
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestTCPPortDefaulting(t *testing.T) {
	assert.Equal(t, int32(4403), node(ConnectionSpec{TCP: &TCPConnection{Host: "h"}}).TCPPort(),
		"unset TCP port should default to 4403")
	assert.Equal(t, int32(5000), node(ConnectionSpec{TCP: &TCPConnection{Host: "h", Port: 5000}}).TCPPort(),
		"explicit TCP port should be returned")
	assert.Equal(t, int32(0), node(ConnectionSpec{Serial: &SerialConnection{Device: "/dev/ttyUSB0"}}).TCPPort(),
		"non-TCP connection has no TCP port")
}

func TestEffectiveDeletionPolicy(t *testing.T) {
	n := node(ConnectionSpec{TCP: &TCPConnection{Host: "h"}})
	assert.Equal(t, DeletionPolicyRetain, n.EffectiveDeletionPolicy(), "empty policy defaults to Retain")
	n.Spec.DeletionPolicy = DeletionPolicyWipe
	assert.Equal(t, DeletionPolicyWipe, n.EffectiveDeletionPolicy())
}

func TestDeepCopyIsIndependent(t *testing.T) {
	orig := node(ConnectionSpec{TCP: &TCPConnection{Host: "h", Port: 4403}})
	orig.Spec.Channels = []ChannelSpec{{Index: 0, Name: "primary"}}
	cp := orig.DeepCopy()
	cp.Spec.Region = "EU_868"
	cp.Spec.Channels[0].Name = "changed"
	assert.Equal(t, "US", orig.Spec.Region, "deep copy must not alias scalar fields")
	assert.Equal(t, "primary", orig.Spec.Channels[0].Name, "deep copy must not alias slice elements")
}
