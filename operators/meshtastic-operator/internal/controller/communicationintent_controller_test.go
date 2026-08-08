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

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

func intentScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	require.NoError(t, intentv1alpha1.AddToScheme(s))
	require.NoError(t, meshv1alpha1.AddToScheme(s)) // to prove no MeshtasticNode is created
	return s
}

func TestCommunicationIntentReportsProposedNodes(t *testing.T) {
	ci := &intentv1alpha1.CommunicationIntent{
		ObjectMeta: metav1.ObjectMeta{Name: "regional", Namespace: "default"},
		Spec: intentv1alpha1.CommunicationIntentSpec{
			Region:               "US",
			ApprovedModemPresets: []string{"MEDIUM_SLOW"},
			Role:                 "CLIENT",
			Nodes: []intentv1alpha1.NodeTarget{
				{Name: "field-01", Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "10.0.0.51", Port: 4403}}},
			},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(intentScheme(t)).
		WithObjects(ci).
		WithStatusSubresource(&intentv1alpha1.CommunicationIntent{}).
		Build()
	r := &CommunicationIntentReconciler{Client: c}

	key := types.NamespacedName{Name: "regional", Namespace: "default"}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)

	var got intentv1alpha1.CommunicationIntent
	require.NoError(t, c.Get(context.Background(), key, &got))
	assert.True(t, meta.IsStatusConditionTrue(got.Status.Conditions, intentv1alpha1.ConditionFeasible))
	assert.Equal(t, "MEDIUM_SLOW", got.Status.SelectedModemPreset)
	assert.Equal(t, 1, got.Status.NodeCount)
	require.Len(t, got.Status.ProposedNodes, 1)
	assert.Equal(t, "field-01", got.Status.ProposedNodes[0].Name)
	assert.Equal(t, "MEDIUM_SLOW", got.Status.ProposedNodes[0].Spec.ModemPreset)

	// Report-only: the compiler must NOT have created a MeshtasticNode.
	var nodes meshv1alpha1.MeshtasticNodeList
	require.NoError(t, c.List(context.Background(), &nodes))
	assert.Empty(t, nodes.Items, "report-only: no MeshtasticNode is created")
}

func TestCommunicationIntentReportsAirtimeOverBudget(t *testing.T) {
	ci := &intentv1alpha1.CommunicationIntent{
		ObjectMeta: metav1.ObjectMeta{Name: "dense", Namespace: "default"},
		Spec: intentv1alpha1.CommunicationIntentSpec{
			Region:               "US",
			ApprovedModemPresets: []string{"LONG_SLOW"},
			ExpectedTraffic:      &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 10, PayloadBytes: 40},
			Nodes: []intentv1alpha1.NodeTarget{
				{Name: "n1", Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "10.0.0.1"}}},
				{Name: "n2", Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "10.0.0.2"}}},
				{Name: "n3", Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "10.0.0.3"}}},
			},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(intentScheme(t)).
		WithObjects(ci).
		WithStatusSubresource(&intentv1alpha1.CommunicationIntent{}).
		Build()
	r := &CommunicationIntentReconciler{Client: c}

	key := types.NamespacedName{Name: "dense", Namespace: "default"}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)

	var got intentv1alpha1.CommunicationIntent
	require.NoError(t, c.Get(context.Background(), key, &got))
	// Renderable but over the airtime budget: two distinct verdicts.
	assert.True(t, meta.IsStatusConditionTrue(got.Status.Conditions, intentv1alpha1.ConditionFeasible))
	cond := meta.FindStatusCondition(got.Status.Conditions, intentv1alpha1.ConditionAirtimeWithinBudget)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, intentv1alpha1.ReasonOverBudget, cond.Reason)
	assert.Positive(t, got.Status.PredictedChannelUtilizationPercent)
}

func TestCommunicationIntentAirtimeUnknownWithoutTraffic(t *testing.T) {
	ci := &intentv1alpha1.CommunicationIntent{
		ObjectMeta: metav1.ObjectMeta{Name: "notraffic", Namespace: "default"},
		Spec: intentv1alpha1.CommunicationIntentSpec{
			Region:               "US",
			ApprovedModemPresets: []string{"MEDIUM_SLOW"},
			Nodes: []intentv1alpha1.NodeTarget{
				{Name: "n1", Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "10.0.0.1"}}},
			},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(intentScheme(t)).
		WithObjects(ci).
		WithStatusSubresource(&intentv1alpha1.CommunicationIntent{}).
		Build()
	r := &CommunicationIntentReconciler{Client: c}

	key := types.NamespacedName{Name: "notraffic", Namespace: "default"}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)

	var got intentv1alpha1.CommunicationIntent
	require.NoError(t, c.Get(context.Background(), key, &got))
	cond := meta.FindStatusCondition(got.Status.Conditions, intentv1alpha1.ConditionAirtimeWithinBudget)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionUnknown, cond.Status, "no declared traffic means no airtime verdict")
	assert.Equal(t, intentv1alpha1.ReasonAirtimeNotEvaluated, cond.Reason)
	assert.Zero(t, got.Status.PredictedChannelUtilizationPercent)
}

func TestCommunicationIntentReportsInfeasible(t *testing.T) {
	ci := &intentv1alpha1.CommunicationIntent{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
		Spec: intentv1alpha1.CommunicationIntentSpec{
			Region:               "US",
			ApprovedModemPresets: []string{"NOT_A_PRESET"},
			Nodes:                []intentv1alpha1.NodeTarget{{Name: "n1"}},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(intentScheme(t)).
		WithObjects(ci).
		WithStatusSubresource(&intentv1alpha1.CommunicationIntent{}).
		Build()
	r := &CommunicationIntentReconciler{Client: c}

	key := types.NamespacedName{Name: "bad", Namespace: "default"}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err, "infeasible is a reported verdict, not a reconcile error")

	var got intentv1alpha1.CommunicationIntent
	require.NoError(t, c.Get(context.Background(), key, &got))
	cond := meta.FindStatusCondition(got.Status.Conditions, intentv1alpha1.ConditionFeasible)
	require.NotNil(t, cond)
	assert.Equal(t, metav1.ConditionFalse, cond.Status)
	assert.Equal(t, intentv1alpha1.ReasonUnknownPreset, cond.Reason)
	assert.Empty(t, got.Status.ProposedNodes)
}
