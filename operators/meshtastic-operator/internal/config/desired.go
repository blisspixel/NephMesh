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
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/secret"
)

// BuildDesired translates a MeshtasticNode spec into the subset of the device's
// exported configuration that the spec declares, so IsConverged can compare it
// against a live export. Each field is emitted only after its export path has
// been confirmed to round-trip against a real device (validated against
// meshtasticd, region and MQTT in the Phase 1 demo, role and modemPreset in the
// operator integration test), because emitting a path the device does not echo
// back would look like permanent drift and re-apply forever. Owner is emitted
// as the top-level owner/owner_short scalars the export uses, confirmed to
// round-trip through --configure against meshtasticd --sim. Channels are not
// emitted here: the export encodes the whole channel set as a single
// channel_url (a base64 protobuf), not discrete per-channel fields, so they
// need a different apply path and are handled separately. The mqttAddress
// argument is the resolved broker address to write, since the device needs a
// reachable address, not a Service name it cannot use. mqttPassword is the
// broker password resolved from its Secret; it is revealed here, at the single
// point that builds the config the device applies, and nowhere else.
//
// This function is pure and fully unit tested; the device round-trip itself is
// covered by the sim integration test.
func BuildDesired(spec meshv1alpha1.MeshtasticNodeSpec, mqttAddress string, mqttPassword secret.Value) map[string]any {
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

	if spec.Owner != nil {
		if spec.Owner.LongName != "" {
			desired["owner"] = spec.Owner.LongName
		}
		if spec.Owner.ShortName != "" {
			desired["owner_short"] = spec.Owner.ShortName
		}
	}

	// A non-nil MQTT spec means the operator manages the MQTT module (a nil spec
	// leaves it untouched). Emit `enabled` always, including false, so the operator
	// can also turn MQTT off; and when enabled, emit the owned booleans explicitly
	// rather than only when true, so a stale `encryption_enabled: true` on the
	// device is detected and reconciled back to false. The device echoes each of
	// these, so an unemitted key would let an old value persist unseen while the
	// node still reports converged.
	if spec.MQTT != nil {
		mqtt := map[string]any{"enabled": spec.MQTT.Enabled}
		if spec.MQTT.Enabled {
			if mqttAddress != "" {
				mqtt["address"] = mqttAddress
			}
			mqtt["json_enabled"] = spec.MQTT.JSONEnabled
			mqtt["encryption_enabled"] = spec.MQTT.EncryptionEnabled
			mqtt["tls_enabled"] = spec.MQTT.TLSEnabled
			if spec.MQTT.Root != "" {
				mqtt["root"] = spec.MQTT.Root
			}
			if spec.MQTT.Username != "" {
				mqtt["username"] = spec.MQTT.Username
			}
			if !mqttPassword.IsZero() {
				mqtt["password"] = mqttPassword.Reveal()
			}
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
