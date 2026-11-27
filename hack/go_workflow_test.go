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

package hack

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// All four Go checks are required by main's branch protection. An event-level
// path filter leaves a required check pending, even on a documentation-only PR.
func TestRequiredGoChecksRunForEveryPath(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/go.yaml")
	require.NoError(t, err)
	var workflow struct {
		On   map[string]map[string]any `yaml:"on"`
		Jobs map[string]map[string]any `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	for _, event := range []string{"pull_request", "push"} {
		t.Run(event, func(t *testing.T) {
			require.Contains(t, workflow.On, event)
			trigger := workflow.On[event]
			// This covers .golangci.yml, hack/check-coverage.sh, the generation
			// header, either module, workflows, unrelated docs and future inputs.
			require.NotContains(t, trigger, "paths")
			require.NotContains(t, trigger, "paths-ignore")
			if event == "push" {
				require.Contains(t, trigger["branches"], "main")
			}
		})
	}
	for _, name := range []string{"api", "meshtastic-operator", "integration", "envtest"} {
		require.Contains(t, workflow.Jobs, name)
		require.NotContains(t, workflow.Jobs[name], "if", name)
	}
}
