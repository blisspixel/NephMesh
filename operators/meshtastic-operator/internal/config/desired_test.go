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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

func TestBuildDesiredRegionOnly(t *testing.T) {
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{Region: "US"}, "")
	assert.Equal(t, map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
	}, d)
}

func TestBuildDesiredMQTTUsesResolvedAddress(t *testing.T) {
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: true, JSONEnabled: true},
	}
	d := BuildDesired(spec, "10.0.0.5")
	mqtt := d["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, true, mqtt["enabled"])
	assert.Equal(t, "10.0.0.5", mqtt["address"], "the resolved broker address is written, not a Service name")
	assert.Equal(t, true, mqtt["json_enabled"])
}

func TestBuildDesiredFullMQTT(t *testing.T) {
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT: &meshv1alpha1.MQTTSpec{
			Enabled: true, EncryptionEnabled: true, TLSEnabled: true, Root: "msh/site1",
		},
	}
	mqtt := BuildDesired(spec, "10.0.0.5")["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, true, mqtt["encryption_enabled"])
	assert.Equal(t, true, mqtt["tls_enabled"])
	assert.Equal(t, "msh/site1", mqtt["root"])
}

func TestBuildDesiredOmitsDisabledMQTT(t *testing.T) {
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: false},
	}, "10.0.0.5")
	_, hasModule := d["module_config"]
	assert.False(t, hasModule, "a disabled MQTT module contributes no desired config")
}

func TestBuildDesiredRoundTripsThroughConverge(t *testing.T) {
	// The whole point: BuildDesired output must read as converged against a
	// live export that contains those values (plus device defaults).
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: true, JSONEnabled: true},
	}
	desired := BuildDesired(spec, "10.0.0.5")
	live := map[string]any{
		"config": map[string]any{
			"lora":   map[string]any{"region": "US", "hopLimit": 3},
			"device": map[string]any{"role": "CLIENT"},
		},
		"moduleConfig": map[string]any{
			"mqtt": map[string]any{"enabled": true, "address": "10.0.0.5", "jsonEnabled": true, "tlsEnabled": false},
		},
	}
	require.True(t, IsConverged(desired, live),
		"desired built from spec should be a subset of a matching live export")
}
