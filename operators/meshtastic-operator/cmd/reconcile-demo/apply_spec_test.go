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

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplySpecDefaultDemoIntent(t *testing.T) {
	spec := applySpec("", "", "", "", "")
	assert.Equal(t, "US", spec.Region)
	assert.Equal(t, "MEDIUM_SLOW", spec.ModemPreset)
	require.NotNil(t, spec.Owner)
	assert.Equal(t, "NephMesh Field 01", spec.Owner.LongName)
	assert.Empty(t, spec.Role)
}

func TestApplySpecRoleAloneDoesNotRewriteOwnerOrPreset(t *testing.T) {
	spec := applySpec("US", "", "ROUTER", "", "")
	assert.Equal(t, "US", spec.Region)
	assert.Equal(t, "ROUTER", spec.Role)
	assert.Empty(t, spec.ModemPreset)
	assert.Nil(t, spec.Owner)
}

func TestApplySpecOwnerWithoutRole(t *testing.T) {
	spec := applySpec("", "", "", "Field", "F1")
	assert.Empty(t, spec.Role)
	require.NotNil(t, spec.Owner)
	assert.Equal(t, "Field", spec.Owner.LongName)
	assert.Equal(t, "F1", spec.Owner.ShortName)
}
