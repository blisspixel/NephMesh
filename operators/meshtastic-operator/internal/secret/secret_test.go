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

package secret

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sentinel = "s3nt1nel-p@ssw0rd-never-log-me"

func TestRevealReturnsTheSecret(t *testing.T) {
	assert.Equal(t, sentinel, New(sentinel).Reveal())
}

func TestEveryRenderPathRedacts(t *testing.T) {
	v := New(sentinel)

	// Every fmt verb a careless log line might use.
	for _, s := range []string{
		fmt.Sprintf("%v", v),
		fmt.Sprintf("%+v", v),
		fmt.Sprintf("%#v", v),
		fmt.Sprintf("pw=%s.", v), // %s verb, wrapped so it is not a bare Sprintf
		fmt.Sprint(v),
		v.String(),
		v.GoString(),
	} {
		assert.NotContains(t, s, sentinel, "a format verb leaked the secret: %q", s)
	}

	// JSON, including structured loggers that serialize fields as JSON.
	b, err := json.Marshal(v)
	require.NoError(t, err)
	assert.NotContains(t, string(b), sentinel)

	// Wrapped inside a struct and an error, the two most common leak paths.
	type holder struct{ Password Value }
	assert.NotContains(t, fmt.Sprintf("%+v", holder{Password: v}), sentinel)
	assert.NotContains(t, fmt.Errorf("apply failed with %v", v).Error(), sentinel)

	// The logr structured-logging hook.
	assert.NotContains(t, fmt.Sprintf("%v", v.MarshalLog()), sentinel)
}

func TestZeroValueFormatsCleanlyAndIsZero(t *testing.T) {
	var v Value
	assert.True(t, v.IsZero())
	assert.Equal(t, "", v.String())
	assert.False(t, New("x").IsZero())

	b, err := json.Marshal(v)
	require.NoError(t, err)
	assert.Equal(t, `""`, string(b))
}
