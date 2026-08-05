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

import "fmt"

// Validate performs semantic checks that complement the CEL and OpenAPI schema
// validation done by the API server. It is safe to call in a controller as a
// defense-in-depth check; the API server enforces the same connection rule via
// CEL, so this mainly gives a clear error when types are constructed in code.
func (m *MeshtasticNode) Validate() error {
	return m.Spec.Connection.validate()
}

func (c *ConnectionSpec) validate() error {
	set := 0
	if c.TCP != nil {
		set++
	}
	if c.Serial != nil {
		set++
	}
	if c.ViaGateway != nil {
		set++
	}
	if set != 1 {
		return fmt.Errorf("connection: exactly one of tcp, serial, or viaGateway must be set, got %d", set)
	}
	return nil
}

// TCPPort returns the configured TCP port, or the 4403 default when the
// connection is TCP and the port is unset. It returns 0 for non-TCP
// connections.
func (m *MeshtasticNode) TCPPort() int32 {
	if m.Spec.Connection.TCP == nil {
		return 0
	}
	if m.Spec.Connection.TCP.Port == 0 {
		return 4403
	}
	return m.Spec.Connection.TCP.Port
}

// EffectiveDeletionPolicy returns the deletion policy, defaulting to Retain.
func (m *MeshtasticNode) EffectiveDeletionPolicy() string {
	if m.Spec.DeletionPolicy == "" {
		return DeletionPolicyRetain
	}
	return m.Spec.DeletionPolicy
}
