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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/reconcile"
)

func testScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	require.NoError(t, meshv1alpha1.AddToScheme(s))
	return s
}

func newNode() *meshv1alpha1.MeshtasticNode {
	return &meshv1alpha1.MeshtasticNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Namespace: "default"},
		Spec: meshv1alpha1.MeshtasticNodeSpec{
			Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "h"}},
			Region:     "US",
		},
	}
}

func desiredUS() map[string]any {
	return map[string]any{"config": map[string]any{"lora": map[string]any{"region": "US"}}}
}

func reconcilerFor(t *testing.T, node *meshv1alpha1.MeshtasticNode, factory DeviceFactory) (*MeshtasticNodeReconciler, client.Client) {
	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(node).
		WithStatusSubresource(&meshv1alpha1.MeshtasticNode{}).
		Build()
	return &MeshtasticNodeReconciler{Client: c, NewDevice: factory}, c
}

func request() ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: "node1", Namespace: "default"}}
}

func TestReconcileAddsFinalizerFirst(t *testing.T) {
	node := newNode()
	r, c := reconcilerFor(t, node, func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(desiredUS(), 0), nil
	})
	_, err := r.Reconcile(context.Background(), request())
	require.NoError(t, err)

	var got meshv1alpha1.MeshtasticNode
	require.NoError(t, c.Get(context.Background(), request().NamespacedName, &got))
	assert.Contains(t, got.Finalizers, meshv1alpha1.Finalizer,
		"the first pass adds the finalizer; the Update re-triggers reconcile via the watch")
}

func TestReconcileConvergedSetsReady(t *testing.T) {
	node := newNode()
	node.Finalizers = []string{meshv1alpha1.Finalizer}
	r, c := reconcilerFor(t, node, func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(desiredUS(), 0), nil // already matches desired
	})
	res, err := r.Reconcile(context.Background(), request())
	require.NoError(t, err)
	assert.Equal(t, reconcile.DriftCheckInterval, res.RequeueAfter)

	var got meshv1alpha1.MeshtasticNode
	require.NoError(t, c.Get(context.Background(), request().NamespacedName, &got))
	assert.True(t, meta.IsStatusConditionTrue(got.Status.Conditions, meshv1alpha1.ConditionReady))
	assert.True(t, meta.IsStatusConditionTrue(got.Status.Conditions, meshv1alpha1.ConditionConfigInSync))
	assert.Equal(t, "!6e000001", got.Status.NodeID)
}

func TestReconcileDriftedSetsRebootPending(t *testing.T) {
	node := newNode()
	node.Finalizers = []string{meshv1alpha1.Finalizer}
	r, c := reconcilerFor(t, node, func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(map[string]any{}, 2), nil // empty: drifted from US
	})
	res, err := r.Reconcile(context.Background(), request())
	require.NoError(t, err)
	assert.Equal(t, reconcile.RebootWait, res.RequeueAfter)

	var got meshv1alpha1.MeshtasticNode
	require.NoError(t, c.Get(context.Background(), request().NamespacedName, &got))
	assert.True(t, meta.IsStatusConditionTrue(got.Status.Conditions, meshv1alpha1.ConditionRebootPending))
	assert.False(t, meta.IsStatusConditionTrue(got.Status.Conditions, meshv1alpha1.ConditionReady))
}

func TestReconcileDeletionRemovesFinalizer(t *testing.T) {
	node := newNode()
	node.Finalizers = []string{meshv1alpha1.Finalizer}
	r, c := reconcilerFor(t, node, func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(desiredUS(), 0), nil
	})
	// Deleting an object with a finalizer sets DeletionTimestamp rather than
	// removing it, so the next reconcile must clear the finalizer.
	require.NoError(t, c.Delete(context.Background(), node))
	_, err := r.Reconcile(context.Background(), request())
	require.NoError(t, err)

	var got meshv1alpha1.MeshtasticNode
	err = c.Get(context.Background(), request().NamespacedName, &got)
	assert.True(t, err != nil, "with its finalizer removed, the object is gone")
}

func TestReconcileMissingObjectIsNoOp(t *testing.T) {
	r, _ := reconcilerFor(t, newNode(), func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(desiredUS(), 0), nil
	})
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)
}