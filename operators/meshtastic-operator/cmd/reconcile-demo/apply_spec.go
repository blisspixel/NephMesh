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

package main

import meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"

// applySpec builds the MeshtasticNode spec reconcile-demo will drive. An
// all-empty flag set is the canned demo intent (a non-default preset, because
// the export omits fields left at the device default). Any explicit flag
// switches to a sparse spec so a caller can change one field (region, role,
// owner, preset) without also rewriting the others.
func applySpec(region, preset, role, owner, ownerShort string) meshv1alpha1.MeshtasticNodeSpec {
	if region == "" && preset == "" && role == "" && owner == "" && ownerShort == "" {
		return meshv1alpha1.MeshtasticNodeSpec{
			Region:      "US",
			ModemPreset: "MEDIUM_SLOW",
			Owner:       &meshv1alpha1.OwnerSpec{LongName: "NephMesh Field 01", ShortName: "NF01"},
		}
	}
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Region:      region,
		ModemPreset: preset,
		Role:        role,
	}
	if owner != "" || ownerShort != "" {
		spec.Owner = &meshv1alpha1.OwnerSpec{LongName: owner, ShortName: ownerShort}
	}
	return spec
}
