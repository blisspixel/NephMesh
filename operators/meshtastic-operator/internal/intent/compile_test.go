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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

func tcp(host string) meshv1alpha1.ConnectionSpec {
	return meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: host, Port: 4403}}
}

func TestCompileRendersProposedNodes(t *testing.T) {
	r := Compile(intentv1alpha1.CommunicationIntentSpec{
		Region:               "US",
		ApprovedModemPresets: []string{"MEDIUM_SLOW", "LONG_FAST"},
		Role:                 "CLIENT",
		Channels:             []meshv1alpha1.ChannelSpec{{Index: 1, Name: "relief"}},
		Nodes: []intentv1alpha1.NodeTarget{
			{Name: "field-01", Connection: tcp("10.0.0.51")},
			{Name: "field-02", Connection: tcp("10.0.0.52")},
		},
	})
	require.True(t, r.Feasible)
	assert.Equal(t, intentv1alpha1.ReasonRenderable, r.Reason)
	assert.Equal(t, "MEDIUM_SLOW", r.SelectedPreset, "the first approved preset is selected")
	require.Len(t, r.Proposed, 2)

	first := r.Proposed[0]
	assert.Equal(t, "field-01", first.Name)
	assert.Equal(t, "US", first.Spec.Region)
	assert.Equal(t, "MEDIUM_SLOW", first.Spec.ModemPreset)
	assert.Equal(t, "CLIENT", first.Spec.Role)
	require.Len(t, first.Spec.Channels, 1)
	assert.Equal(t, "relief", first.Spec.Channels[0].Name)
	assert.Equal(t, "10.0.0.52", r.Proposed[1].Spec.Connection.TCP.Host)
}

func TestCompileInfeasibleEmptyApprovedSet(t *testing.T) {
	r := Compile(intentv1alpha1.CommunicationIntentSpec{
		Region: "US",
		Nodes:  []intentv1alpha1.NodeTarget{{Name: "n1"}},
	})
	assert.False(t, r.Feasible, "no approved preset is infeasible, not an error")
	assert.Equal(t, intentv1alpha1.ReasonNoApprovedSet, r.Reason)
	assert.Empty(t, r.Proposed)
}

func TestCompileInfeasibleUnknownPreset(t *testing.T) {
	r := Compile(intentv1alpha1.CommunicationIntentSpec{
		Region:               "US",
		ApprovedModemPresets: []string{"NOT_A_PRESET", "ALSO_BAD"},
		Nodes:                []intentv1alpha1.NodeTarget{{Name: "n1"}},
	})
	assert.False(t, r.Feasible, "an approved set with no known preset is infeasible")
	assert.Equal(t, intentv1alpha1.ReasonUnknownPreset, r.Reason)
}

func TestCompileSelectsFirstKnownPreset(t *testing.T) {
	r := Compile(intentv1alpha1.CommunicationIntentSpec{
		Region:               "US",
		ApprovedModemPresets: []string{"BOGUS", "LONG_FAST"},
		Nodes:                []intentv1alpha1.NodeTarget{{Name: "n1"}},
	})
	require.True(t, r.Feasible)
	assert.Equal(t, "LONG_FAST", r.SelectedPreset, "skips the unknown preset to find a known one")
}

func TestCompileInfeasibleNoNodes(t *testing.T) {
	r := Compile(intentv1alpha1.CommunicationIntentSpec{
		Region: "US", ApprovedModemPresets: []string{"LONG_FAST"},
	})
	assert.False(t, r.Feasible)
	assert.Equal(t, intentv1alpha1.ReasonNoTargetNodes, r.Reason)
}
