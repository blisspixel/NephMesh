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

package plan

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
)

const fullIntentYAML = `
apiVersion: intent.nephmesh.io/v1alpha1
kind: CommunicationIntent
metadata:
  name: regional-relief
spec:
  region: US
  role: CLIENT
  approvedModemPresets:
    - MEDIUM_SLOW
    - LONG_FAST
  expectedTraffic:
    messagesPerMinutePerNode: 4
    payloadBytes: 40
  nodes:
    - name: relief-01
      connection:
        tcp:
          host: 10.0.0.51
          port: 4403
`

func TestRunFullObjectYAML(t *testing.T) {
	out, err := Run([]byte(fullIntentYAML))
	require.NoError(t, err)
	assert.True(t, out.Feasible)
	assert.Equal(t, intentv1alpha1.ReasonRenderable, out.Reason)
	assert.Equal(t, "MEDIUM_SLOW", out.SelectedPreset)
	assert.Equal(t, 1, out.NodeCount)
	require.Len(t, out.ProposedNodes, 1)
	assert.Equal(t, "relief-01", out.ProposedNodes[0].Name)
	assert.Equal(t, "MEDIUM_SLOW", out.ProposedNodes[0].Spec.ModemPreset)
	assert.True(t, out.Airtime.Evaluated)
	assert.True(t, out.Airtime.WithinBudget)
}

func TestRunBareSpec(t *testing.T) {
	// A bare spec (no top-level spec key) is accepted too.
	bare := `
region: US
approvedModemPresets: [SHORT_FAST]
nodes:
  - name: n1
    connection:
      tcp:
        host: 10.0.0.1
`
	out, err := Run([]byte(bare))
	require.NoError(t, err)
	assert.True(t, out.Feasible)
	assert.Equal(t, "SHORT_FAST", out.SelectedPreset)
	assert.False(t, out.Airtime.Evaluated, "no expectedTraffic means airtime is not evaluated")
}

func TestRunJSONInput(t *testing.T) {
	// JSON is valid YAML, so the same path parses it.
	j := `{"spec":{"region":"US","approvedModemPresets":["LONG_FAST"],"nodes":[{"name":"n1","connection":{"tcp":{"host":"10.0.0.1"}}}]}}`
	out, err := Run([]byte(j))
	require.NoError(t, err)
	assert.True(t, out.Feasible)
	assert.Equal(t, "LONG_FAST", out.SelectedPreset)
}

func TestRunInfeasibleUnknownPreset(t *testing.T) {
	out, err := Run([]byte(`
spec:
  region: US
  approvedModemPresets: [NOPE]
  nodes:
    - name: n1
      connection:
        tcp:
          host: 10.0.0.1
`))
	require.NoError(t, err, "infeasible is a verdict, not a parse error")
	assert.False(t, out.Feasible)
	assert.Equal(t, intentv1alpha1.ReasonUnknownPreset, out.Reason)
}

func TestRunOverBudgetIsFeasibleButFlagged(t *testing.T) {
	out, err := Run([]byte(`
spec:
  region: US
  approvedModemPresets: [LONG_SLOW]
  expectedTraffic:
    messagesPerMinutePerNode: 10
    payloadBytes: 40
  nodes:
    - {name: n1, connection: {tcp: {host: 10.0.0.1}}}
    - {name: n2, connection: {tcp: {host: 10.0.0.2}}}
    - {name: n3, connection: {tcp: {host: 10.0.0.3}}}
`))
	require.NoError(t, err)
	assert.True(t, out.Feasible, "renderable")
	assert.True(t, out.Airtime.Evaluated)
	assert.False(t, out.Airtime.WithinBudget, "over budget by the floor")
	assert.Positive(t, out.Airtime.PredictedUtilizationPercent)
}

func TestRunRejectsMultipleDocuments(t *testing.T) {
	// sigs.k8s.io/yaml silently drops all but the first document; plan must reject
	// multi-document input rather than return a partial verdict.
	multi := "spec:\n  region: US\n  approvedModemPresets: [LONG_FAST]\n  nodes: [{name: n1, connection: {tcp: {host: 10.0.0.1}}}]\n" +
		"---\n" +
		"spec:\n  region: EU\n  approvedModemPresets: [LONG_FAST]\n  nodes: [{name: n2, connection: {tcp: {host: 10.0.0.2}}}]\n"
	_, err := Run([]byte(multi))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple YAML documents")
}

func TestRunLeadingSeparatorIsSingleDocument(t *testing.T) {
	// A leading "---" opens the first document; it is not a second document.
	one := "---\nspec:\n  region: US\n  approvedModemPresets: [LONG_FAST]\n  nodes: [{name: n1, connection: {tcp: {host: 10.0.0.1}}}]\n"
	out, err := Run([]byte(one))
	require.NoError(t, err)
	assert.True(t, out.Feasible)
}

func TestRunZeroTrafficReportsNotEvaluated(t *testing.T) {
	// expectedTraffic present but with a non-positive rate (reachable offline where
	// CRD admission does not apply) must read as not-evaluated, not "within budget".
	out, err := Run([]byte("spec:\n  region: US\n  approvedModemPresets: [MEDIUM_SLOW]\n  expectedTraffic: {messagesPerMinutePerNode: 0}\n  nodes: [{name: n1, connection: {tcp: {host: 10.0.0.1}}}]\n"))
	require.NoError(t, err)
	assert.True(t, out.Feasible)
	assert.False(t, out.Airtime.Evaluated, "zero traffic is not a meaningful declaration")
}

func TestRunEmptyInput(t *testing.T) {
	_, err := Run([]byte("   \n\t"))
	assert.ErrorIs(t, err, ErrEmptyInput)
}

func TestRunMalformedYAML(t *testing.T) {
	_, err := Run([]byte("region: US\n\tbad: : :"))
	assert.Error(t, err)
}

func TestOutputTextRendersVerdict(t *testing.T) {
	out, err := Run([]byte(fullIntentYAML))
	require.NoError(t, err)
	txt := out.Text()
	assert.Contains(t, txt, "FEASIBLE")
	assert.Contains(t, txt, "MEDIUM_SLOW")
	assert.Contains(t, txt, "within budget")
	assert.Contains(t, txt, "relief-01")

	infeasible, err := Run([]byte("spec:\n  region: US\n  approvedModemPresets: [NOPE]\n  nodes: [{name: n1, connection: {tcp: {host: 10.0.0.1}}}]\n"))
	require.NoError(t, err)
	assert.Contains(t, infeasible.Text(), "INFEASIBLE")
}

func TestOutputMarshalsStableShape(t *testing.T) {
	out, err := Run([]byte(fullIntentYAML))
	require.NoError(t, err)
	b, err := json.Marshal(out)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	// The wire contract an agent depends on.
	for _, k := range []string{"feasible", "reason", "message", "nodeCount", "airtime", "proposedNodes"} {
		assert.Contains(t, m, k, "output must carry the stable field %q", k)
	}
	air, ok := m["airtime"].(map[string]any)
	require.True(t, ok)
	for _, k := range []string{"evaluated", "withinBudget", "predictedUtilizationPercent"} {
		assert.Contains(t, air, k)
	}
}
