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
)

func TestForComparisonDropsWriteOnlyPassword(t *testing.T) {
	desired := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
		"module_config": map[string]any{"mqtt": map[string]any{
			"enabled": true, "address": "1.2.3.4", "password": "s3cret",
		}},
	}
	// The device export never echoes the MQTT password back.
	live := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
		"module_config": map[string]any{"mqtt": map[string]any{
			"enabled": true, "address": "1.2.3.4",
		}},
	}

	// The bug: comparing the full desired treats the never-echoed password as
	// permanent drift, so the node reboot-loops and never converges.
	assert.False(t, IsConverged(desired, live), "including the write-only password reports permanent drift")

	// The fix: dropping the write-only key from the comparison lets it converge.
	assert.True(t, IsConverged(ForComparison(desired), live), "excluding the write-only password converges")

	// ForComparison must not mutate the original: the full desired still carries
	// the password for the apply path.
	mqtt := desired["module_config"].(map[string]any)["mqtt"].(map[string]any)
	_, stillThere := mqtt["password"]
	assert.True(t, stillThere, "the original desired keeps the password to apply it")
}

func TestConvergedWhenDesiredSubsetPresent(t *testing.T) {
	desired := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US"}},
	}
	live := map[string]any{
		"config": map[string]any{
			"lora":   map[string]any{"region": "US", "hopLimit": 3},
			"device": map[string]any{"role": "CLIENT"},
		},
		"extra": "ignored",
	}
	assert.True(t, IsConverged(desired, live), "extra live fields must not count as drift")
	assert.Empty(t, Drift(desired, live))
}

func TestSnakeVersusCamelKeysConverge(t *testing.T) {
	// The device exports camelCase; desired is written snake_case.
	desired := map[string]any{"module_config": map[string]any{"json_enabled": true}}
	live := map[string]any{"moduleConfig": map[string]any{"jsonEnabled": true}}
	assert.True(t, IsConverged(desired, live), "key comparison must ignore case and underscores")
}

func TestDriftReportsMissingAndChangedPaths(t *testing.T) {
	desired := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "US", "modemPreset": "MEDIUM_SLOW"}},
	}
	live := map[string]any{
		"config": map[string]any{"lora": map[string]any{"region": "EU_868"}},
	}
	drift := Drift(desired, live)
	assert.Equal(t, []string{"config.lora.modemPreset", "config.lora.region"}, drift,
		"missing key and changed value both reported, sorted")
	assert.False(t, IsConverged(desired, live))
}

func TestScalarValuesCompareToleratesRepresentation(t *testing.T) {
	// A number and its string form should compare equal (YAML decoding varies).
	assert.True(t, IsConverged(
		map[string]any{"port": 4403},
		map[string]any{"port": "4403"}))
	// Whitespace differences are not drift.
	assert.True(t, IsConverged(
		map[string]any{"name": "primary"},
		map[string]any{"name": " primary "}))
}

func TestScalarComparisonIsCaseSensitive(t *testing.T) {
	// Regression: values must compare case-sensitively, or case-significant
	// drift (an MQTT root topic here) would be hidden and never corrected.
	assert.False(t, IsConverged(
		map[string]any{"root": "msh/Site1"},
		map[string]any{"root": "msh/site1"}),
		"a differently-cased value is real drift, not convergence")
}

func TestMapVersusScalarMismatchIsDrift(t *testing.T) {
	desired := map[string]any{"mqtt": map[string]any{"enabled": true}}
	live := map[string]any{"mqtt": "on"}
	assert.Equal(t, []string{"mqtt"}, Drift(desired, live),
		"a desired map against a live scalar is drift, not a panic")
}
