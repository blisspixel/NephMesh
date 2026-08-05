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

package device

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnsupportedIsAlwaysUnreachable(t *testing.T) {
	u := &Unsupported{Transport: "serial"}
	ctx := context.Background()

	_, err := u.ExportConfig(ctx)
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.ErrorIs(t, u.Apply(ctx, nil), ErrUnreachable)
	assert.ErrorIs(t, u.Reboot(ctx), ErrUnreachable)
	_, err = u.Info(ctx)
	assert.ErrorIs(t, err, ErrUnreachable)
	assert.Contains(t, err.Error(), "serial", "the error names the unimplemented transport")
}
