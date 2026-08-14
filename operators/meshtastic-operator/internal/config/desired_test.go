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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

func TestBuildDesiredRegionOnly(t *testing.T) {
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{Region: "US"}, "", secret.Value{})
	assert.Equal(t, map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
	}, d)
}

func TestBuildDesiredModemPresetAndRole(t *testing.T) {
	// Paths verified against meshtasticd: modemPreset -> config.lora.modemPreset,
	// role -> config.device.role.
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{
		Region: "US", ModemPreset: "MEDIUM_SLOW", Role: "ROUTER",
	}, "", secret.Value{})
	lora := d["config"].(map[string]any)["lora"].(map[string]any)
	device := d["config"].(map[string]any)["device"].(map[string]any)
	assert.Equal(t, "US", lora["region"])
	assert.Equal(t, "MEDIUM_SLOW", lora["modemPreset"])
	assert.Equal(t, true, lora["usePreset"], "a declared preset must turn preset mode on")
	assert.Equal(t, "ROUTER", device["role"])
}

func TestBuildDesiredOwner(t *testing.T) {
	// Paths verified against meshtasticd --sim: the export uses top-level
	// owner (long name) and owner_short scalars, and both round-trip through
	// --configure.
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{
		Owner: &meshv1alpha1.OwnerSpec{LongName: "NephMesh Sim 01", ShortName: "NM01"},
	}, "", secret.Value{})
	assert.Equal(t, "NephMesh Sim 01", d["owner"])
	assert.Equal(t, "NM01", d["owner_short"])
}

func TestBuildDesiredOwnerPartialAndEmpty(t *testing.T) {
	// Only the fields that are set are emitted, so an unset half cannot look
	// like drift against a device that already has its own value there.
	longOnly := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{
		Owner: &meshv1alpha1.OwnerSpec{LongName: "Base Camp"},
	}, "", secret.Value{})
	assert.Equal(t, "Base Camp", longOnly["owner"])
	_, hasShort := longOnly["owner_short"]
	assert.False(t, hasShort, "owner_short must be absent when unset")

	none := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{Owner: &meshv1alpha1.OwnerSpec{}}, "", secret.Value{})
	_, hasOwner := none["owner"]
	assert.False(t, hasOwner, "owner must be absent when both names are empty")
}

func TestBuildDesiredMQTTUsesResolvedAddress(t *testing.T) {
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: true, JSONEnabled: true},
	}
	d := BuildDesired(spec, "10.0.0.5", secret.Value{})
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
	mqtt := BuildDesired(spec, "10.0.0.5", secret.Value{})["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, true, mqtt["encryption_enabled"])
	assert.Equal(t, true, mqtt["tls_enabled"])
	assert.Equal(t, "msh/site1", mqtt["root"])
}

func TestBuildDesiredMQTTUsernameAndPassword(t *testing.T) {
	// Username is plaintext from the spec; the password is resolved from a
	// Secret and revealed only here, at the config-write point. Both must reach
	// the device config. Path verified against meshtasticd --sim (mqtt.username
	// and mqtt.password round-trip through --configure).
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: true, Username: "meshops"},
	}
	mqtt := BuildDesired(spec, "10.0.0.5", secret.New("brokerpass"))["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, "meshops", mqtt["username"])
	assert.Equal(t, "brokerpass", mqtt["password"], "the resolved password is written to the device config")
}

func TestBuildDesiredOmitsEmptyMQTTCredentials(t *testing.T) {
	// No Secret resolved and no username means neither key is emitted, so an
	// unset credential cannot look like drift against a device that has its own.
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: true},
	}
	mqtt := BuildDesired(spec, "10.0.0.5", secret.Value{})["module_config"].(map[string]any)["mqtt"].(map[string]any)
	_, hasPw := mqtt["password"]
	assert.False(t, hasPw, "no password key when no Secret is resolved")
	_, hasUser := mqtt["username"]
	assert.False(t, hasUser, "no username key when unset")
}

func TestBuildDesiredManagedMQTTDisabledReconcilesOff(t *testing.T) {
	// A non-nil but disabled MQTT is managed-off: the operator emits enabled:false
	// so it can reconcile the module off, rather than omitting it (which would let a
	// stale enabled:true on the device persist while the node reports converged).
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: false},
	}, "10.0.0.5", secret.Value{})
	mqtt := d["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, false, mqtt["enabled"], "a disabled MQTT is reconciled off, not omitted")
	_, hasAddr := mqtt["address"]
	assert.False(t, hasAddr, "no sub-fields are emitted when disabled")
}

func TestBuildDesiredNilMQTTIsUnmanaged(t *testing.T) {
	// A nil MQTT spec means the operator does not manage the module at all.
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{Region: "US"}, "10.0.0.5", secret.Value{})
	_, hasModule := d["module_config"]
	assert.False(t, hasModule, "a nil MQTT spec leaves the module untouched")
}

func TestBuildDesiredEmitsOwnedBooleansExplicitly(t *testing.T) {
	// Owned booleans are emitted even when false, so the operator can reconcile a
	// stale true (e.g. encryption left on) back to false.
	d := BuildDesired(meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: true, JSONEnabled: true, EncryptionEnabled: false},
	}, "10.0.0.5", secret.Value{})
	mqtt := d["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, true, mqtt["json_enabled"])
	assert.Equal(t, false, mqtt["encryption_enabled"], "false is emitted so it can be reconciled")
	assert.Equal(t, false, mqtt["tls_enabled"])
}

func TestBuildDesiredNeverEmitsTransmitPowerKeys(t *testing.T) {
	// Defense for the transmit interlock: the operator configures a mesh node
	// (which transmits by design on license-free ISM, its legitimate job), but
	// it must never emit a transmit-power or region-power escalation key. This
	// asserts the builder cannot be a path to raising power.
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region:      "US",
		ModemPreset: "LONG_FAST",
		Role:        "ROUTER",
		MQTT:        &meshv1alpha1.MQTTSpec{Enabled: true},
	}
	blob := fmt.Sprintf("%v", BuildDesired(spec, "10.0.0.5", secret.Value{}))
	forbidden := []string{"txPower", "tx_power", "power", "txEnabled", "tx_enabled"} // transmit-ok: asserted absent, never emitted
	for _, key := range forbidden {
		assert.NotContains(t, blob, key, "BuildDesired must never emit a transmit-power key")
	}
}

func TestBuildDesiredRoundTripsThroughConverge(t *testing.T) {
	// The whole point: BuildDesired output must read as converged against a
	// live export that contains those values (plus device defaults).
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region: "US",
		MQTT:   &meshv1alpha1.MQTTSpec{Enabled: true, JSONEnabled: true},
	}
	desired := BuildDesired(spec, "10.0.0.5", secret.Value{})
	live := map[string]any{
		"config": map[string]any{
			"lora":   map[string]any{"region": "US", "hopLimit": 3},
			"device": map[string]any{"role": "CLIENT"},
		},
		"moduleConfig": map[string]any{
			"mqtt": map[string]any{"enabled": true, "address": "10.0.0.5", "jsonEnabled": true, "encryptionEnabled": false, "tlsEnabled": false},
		},
	}
	require.True(t, IsConverged(desired, live),
		"desired built from spec should be a subset of a matching live export")
}
