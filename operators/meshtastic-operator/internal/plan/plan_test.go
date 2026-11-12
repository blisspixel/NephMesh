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
	"strings"
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
	require.NotNil(t, out.Airtime.WithinBudget)
	assert.True(t, *out.Airtime.WithinBudget)
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
	assert.Nil(t, out.Airtime.WithinBudget, "an unevaluated budget is null, not false")
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	air := m["airtime"].(map[string]any)
	assert.Equal(t, false, air["evaluated"])
	assert.Nil(t, air["withinBudget"])
	assert.Equal(t, intentv1alpha1.ReasonAirtimeNotEvaluated, air["reason"])
	assert.NotContains(t, out.Text(), "over budget")
	assert.Contains(t, out.Text(), "not evaluated")
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
	require.NotNil(t, out.Airtime.WithinBudget)
	assert.False(t, *out.Airtime.WithinBudget, "over budget by the floor")
	assert.Positive(t, out.Airtime.PredictedUtilizationPercent)
	raw, err := json.Marshal(out.Airtime)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"withinBudget":false`)
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

func TestRunNonPositiveTrafficIsInfeasible(t *testing.T) {
	// The CRD minimum is 1. Zero and negative rates are refusals, and the
	// unevaluated airtime verdict must not read as over budget.
	for _, rate := range []string{"0", "-5"} {
		t.Run(rate, func(t *testing.T) {
			doc := "spec:\n  region: US\n  approvedModemPresets: [MEDIUM_SLOW]\n  expectedTraffic: {messagesPerMinutePerNode: " + rate + "}\n  nodes: [{name: n1, connection: {tcp: {host: 10.0.0.1}}}]\n"
			out, err := Run([]byte(doc))
			require.NoError(t, err)
			assert.False(t, out.Feasible)
			assert.Equal(t, intentv1alpha1.ReasonMessagesPerMinuteOutOfRange, out.Reason)
			assert.False(t, out.Airtime.Evaluated)
			assert.Nil(t, out.Airtime.WithinBudget)
			assert.NotContains(t, out.Text(), "over budget")
			assert.Contains(t, out.Text(), "not evaluated")
		})
	}
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
	assert.Contains(t, txt, "feasible: 1 node at MEDIUM_SLOW in US")
	assert.Contains(t, txt, "within budget")
	assert.Contains(t, txt, "relief-01")
	assert.NotContains(t, txt, "Renderable")
	assert.NotContains(t, txt, "WithinBudget")
	assert.NotContains(t, txt, "node(s)")

	infeasible, err := Run([]byte("spec:\n  region: US\n  approvedModemPresets: [NOPE]\n  nodes: [{name: n1, connection: {tcp: {host: 10.0.0.1}}}]\n"))
	require.NoError(t, err)
	assert.Contains(t, infeasible.Text(), "infeasible:")
	assert.NotContains(t, infeasible.Text(), "UnknownPreset")
}

func TestRunRejectsWrongKind(t *testing.T) {
	_, err := Run([]byte("apiVersion: mesh.nephmesh.io/v1alpha1\nkind: MeshtasticNode\nmetadata:\n  name: field-01\nspec:\n  region: US\n"))
	require.Error(t, err)
	assert.EqualError(t, err, "expected kind CommunicationIntent, got MeshtasticNode")
}

func TestRunBlockScalarSeparatorIsOneDocument(t *testing.T) {
	doc := strings.Replace(fullIntentYAML, "metadata:\n  name: regional-relief\n",
		"metadata:\n  name: regional-relief\n  annotations:\n    note: |\n      ---\n", 1)
	out, err := Run([]byte(doc))
	require.NoError(t, err)
	assert.True(t, out.Feasible)
	assert.Equal(t, 1, out.NodeCount)
}

func TestRunFoldedAndQuotedSeparatorAreOneDocument(t *testing.T) {
	folded := strings.Replace(fullIntentYAML, "metadata:\n  name: regional-relief\n",
		"metadata:\n  name: regional-relief\n  annotations:\n    note: >\n      ---\n", 1)
	out, err := Run([]byte(folded))
	require.NoError(t, err)
	assert.True(t, out.Feasible)

	quoted := strings.Replace(fullIntentYAML, "metadata:\n  name: regional-relief\n",
		"metadata:\n  name: regional-relief\n  annotations:\n    note: \"---\"\n", 1)
	out, err = Run([]byte(quoted))
	require.NoError(t, err)
	assert.True(t, out.Feasible)
}

func TestRunRejectsSeparatorWithTrailingComment(t *testing.T) {
	multi := "spec:\n  region: US\n  approvedModemPresets: [LONG_FAST]\n  nodes: [{name: n1, connection: {tcp: {host: 10.0.0.1}}}]\n" +
		"--- # second intent\n" +
		"spec:\n  region: EU\n  approvedModemPresets: [LONG_FAST]\n  nodes: [{name: n2, connection: {tcp: {host: 10.0.0.2}}}]\n"
	_, err := Run([]byte(multi))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple YAML documents")
}

func TestRunRefusesIssueRepros(t *testing.T) {
	base := "spec:\n  region: US\n  role: CLIENT\n  approvedModemPresets: [MEDIUM_SLOW]\n  nodes:\n    - name: a\n      connection: {tcp: {host: h}}\n      owner: {shortName: AB}\n"
	cases := []struct {
		name   string
		doc    string
		reason string
	}{
		{"region MARS", strings.Replace(base, "region: US", "region: MARS", 1), intentv1alpha1.ReasonInvalidRegion},
		{"role OVERLORD", strings.Replace(base, "role: CLIENT", "role: OVERLORD", 1), intentv1alpha1.ReasonInvalidRole},
		{"short name", strings.Replace(base, "shortName: AB", "shortName: TOOLONGNAME", 1), intentv1alpha1.ReasonShortNameTooLong},
		{"channel 99", strings.Replace(base, "approvedModemPresets: [MEDIUM_SLOW]\n", "approvedModemPresets: [MEDIUM_SLOW]\n  channels:\n    - {index: 99, name: x}\n", 1), intentv1alpha1.ReasonChannelIndexOutOfRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Run([]byte(tc.doc))
			require.NoError(t, err)
			assert.False(t, out.Feasible)
			assert.Equal(t, tc.reason, out.Reason)
			assert.Contains(t, out.Text(), "infeasible:")
			assert.NotContains(t, out.Text(), tc.reason)
		})
	}
}

func TestRunWarnsOnUnknownField(t *testing.T) {
	doc := "apiVersion: intent.nephmesh.io/v1alpha1\nkind: CommunicationIntent\nmetadata:\n  name: regional\n  annotations:\n    note: kept\nspec:\n  region: US\n  frobnicate: 1\n  approvedModemPresets: [LONG_FAST]\n  nodes:\n    - name: n1\n      connection:\n        tcp:\n          host: 10.0.0.1\n          bogus: 1\n"
	out, err := Run([]byte(doc))
	require.NoError(t, err)
	assert.True(t, out.Feasible, "an unknown field is pruned, not a refusal")
	assert.Contains(t, out.Warnings, "unknown field spec.frobnicate")
	assert.Contains(t, out.Warnings, "unknown field spec.nodes[0].connection.tcp.bogus")
	assert.Contains(t, out.Text(), "warning: unknown field spec.frobnicate")
	assert.NotContains(t, out.Text(), "annotations")
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
