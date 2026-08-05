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

import meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"

// BuildDesired translates a MeshtasticNode spec into the subset of the device's
// exported configuration that the spec declares, so IsConverged can compare it
// against a live export. Each field is emitted only after its export path has
// been confirmed to round-trip against a real device (validated against
// meshtasticd, region and MQTT in the Phase 1 demo, role and modemPreset in the
// operator integration test), because emitting a path the device does not echo
// back would look like permanent drift and re-apply forever. Owner and channels
// are added the same way when their paths are confirmed. The mqttAddress
// argument is the resolved broker address to write, since the device needs a
// reachable address, not a Service name it cannot use.
//
// This function is pure and fully unit tested; the device round-trip itself is
// covered by the sim integration test.
func BuildDesired(spec meshv1alpha1.MeshtasticNodeSpec, mqttAddress string) map[string]any {
	desired := map[string]any{}

	if spec.Region != "" {
		setPath(desired, []string{"config", "lora"}, "region", spec.Region)
	}
	if spec.ModemPreset != "" {
		setPath(desired, []string{"config", "lora"}, "modemPreset", spec.ModemPreset)
	}
	if spec.Role != "" {
		setPath(desired, []string{"config", "device"}, "role", spec.Role)
	}

	if spec.MQTT != nil && spec.MQTT.Enabled {
		mqtt := map[string]any{"enabled": true}
		if mqttAddress != "" {
			mqtt["address"] = mqttAddress
		}
		if spec.MQTT.JSONEnabled {
			mqtt["json_enabled"] = true
		}
		if spec.MQTT.EncryptionEnabled {
			mqtt["encryption_enabled"] = true
		}
		if spec.MQTT.TLSEnabled {
			mqtt["tls_enabled"] = true
		}
		if spec.MQTT.Root != "" {
			mqtt["root"] = spec.MQTT.Root
		}
		desired["module_config"] = map[string]any{"mqtt": mqtt}
	}

	return desired
}

func setPath(root map[string]any, path []string, key string, value any) {
	m := root
	for _, p := range path {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[key] = value
}
