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

// Region and role ids are the Meshtastic protobuf enum names a device accepts
// (config.proto RegionCode and DeviceConfig.Role). RegionCode UNSET is omitted:
// its label is "please set a region", so it is not an operating region.
// Deprecated enum values stay in the sets because current firmware still
// accepts them (RegionCode UA_868, Role ROUTER_CLIENT and REPEATER).
// The CRD leaves these fields as open strings so a firmware addition does not
// reject objects until the CRD is re-released. Plan has no apiserver, so the
// compiler keeps the tables the same way it already keeps the modem-preset table.
//
// https://github.com/meshtastic/protobufs/blob/master/meshtastic/config.proto

var meshtasticRegions = map[string]struct{}{
	"US":         {},
	"EU_433":     {},
	"EU_868":     {},
	"CN":         {},
	"JP":         {},
	"ANZ":        {},
	"KR":         {},
	"TW":         {},
	"RU":         {},
	"IN":         {},
	"NZ_865":     {},
	"TH":         {},
	"LORA_24":    {},
	"UA_433":     {},
	"UA_868":     {},
	"MY_433":     {},
	"MY_919":     {},
	"SG_923":     {},
	"PH_433":     {},
	"PH_868":     {},
	"PH_915":     {},
	"ANZ_433":    {},
	"KZ_433":     {},
	"KZ_863":     {},
	"NP_865":     {},
	"BR_902":     {},
	"ITU1_2M":    {},
	"ITU2_2M":    {},
	"EU_866":     {},
	"EU_874":     {},
	"EU_917":     {},
	"EU_N_868":   {},
	"ITU3_2M":    {},
	"ITU1_70CM":  {},
	"ITU2_70CM":  {},
	"ITU3_70CM":  {},
	"ITU2_125CM": {},
}

var meshtasticRoles = map[string]struct{}{
	"CLIENT":         {},
	"CLIENT_MUTE":    {},
	"ROUTER":         {},
	"ROUTER_CLIENT":  {},
	"REPEATER":       {},
	"TRACKER":        {},
	"SENSOR":         {},
	"TAK":            {},
	"CLIENT_HIDDEN":  {},
	"LOST_AND_FOUND": {},
	"TAK_TRACKER":    {},
	"ROUTER_LATE":    {},
	"CLIENT_BASE":    {},
}

func knownRegion(id string) bool {
	_, ok := meshtasticRegions[id]
	return ok
}

func knownRole(id string) bool {
	_, ok := meshtasticRoles[id]
	return ok
}
