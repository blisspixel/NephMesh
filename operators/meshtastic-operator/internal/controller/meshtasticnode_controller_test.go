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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/config"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/reconcile"
)

func testScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	require.NoError(t, meshv1alpha1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s)) // Secrets, for the credential path
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
	return &MeshtasticNodeReconciler{Client: c, Reader: c, NewDevice: factory}, c
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

func TestAirtimeHealthyConditionFromTelemetry(t *testing.T) {
	ch, tx := 12.0, 3.0
	node := newNode()
	applyOutcome(node, reconcile.Outcome{Reachable: true, Info: device.Info{NodeID: "!x", ChannelUtilization: &ch, AirUtilTx: &tx}})
	c := meta.FindStatusCondition(node.Status.Conditions, meshv1alpha1.ConditionAirtimeHealthy)
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Contains(t, c.Message, "channelUtilization 12.0%")

	// Over the channel-utilization ceiling: unhealthy, surfaced not swallowed.
	high := 40.0
	applyOutcome(node, reconcile.Outcome{Reachable: true, Info: device.Info{ChannelUtilization: &high}})
	c = meta.FindStatusCondition(node.Status.Conditions, meshv1alpha1.ConditionAirtimeHealthy)
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionFalse, c.Status)

	// No telemetry reported: no condition set (the metric is best-effort).
	fresh := newNode()
	applyOutcome(fresh, reconcile.Outcome{Reachable: true, Info: device.Info{NodeID: "!y"}})
	assert.Nil(t, meta.FindStatusCondition(fresh.Status.Conditions, meshv1alpha1.ConditionAirtimeHealthy))
}

func TestChannelsInSyncCondition(t *testing.T) {
	node := newNode()
	desired := []config.ChannelState{{Index: 0, Name: "ops", PSKHash: "h1"}}

	// Device matches the declared channel: in sync.
	applyChannelsInSync(node, desired, []config.ChannelState{{Index: 0, Name: "ops", PSKHash: "h1"}}, 1)
	c := meta.FindStatusCondition(node.Status.Conditions, meshv1alpha1.ConditionChannelsInSync)
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)

	// Device key differs: drift surfaced with the field path, not the key.
	applyChannelsInSync(node, desired, []config.ChannelState{{Index: 0, Name: "ops", PSKHash: "other"}}, 1)
	c = meta.FindStatusCondition(node.Status.Conditions, meshv1alpha1.ConditionChannelsInSync)
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Contains(t, c.Message, "channel[0].psk")
	assert.NotContains(t, c.Message, "h1", "the message reports the field, never the key material")

	// No declared channels: no condition, channel management stays invisible.
	fresh := newNode()
	applyChannelsInSync(fresh, nil, nil, 1)
	assert.Nil(t, meta.FindStatusCondition(fresh.Status.Conditions, meshv1alpha1.ConditionChannelsInSync))
}

func TestReconcileResolvesChannelPSKAndReportsInSync(t *testing.T) {
	node := newNode()
	node.Finalizers = []string{meshv1alpha1.Finalizer}
	node.Spec.Channels = []meshv1alpha1.ChannelSpec{{
		Index: 0, Name: "ops",
		PSKSecretRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "ch0"}, Key: "psk",
		},
	}}
	psk := []byte{0x2a, 0x2b, 0x2c}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ch0", Namespace: "default"},
		Data:       map[string][]byte{"psk": psk},
	}
	// The device exports channel 0 with the matching name and key hash, so the
	// resolved-from-Secret desired channel is in sync.
	live := desiredUS()
	live["channels"] = []any{map[string]any{
		"index": 0, "name": "ops", "pskHash": config.PSKHash(psk),
	}}

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(node, sec).
		WithStatusSubresource(&meshv1alpha1.MeshtasticNode{}).
		Build()
	r := &MeshtasticNodeReconciler{Client: c, Reader: c, NewDevice: func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(live, 0), nil
	}}

	_, err := r.Reconcile(context.Background(), request())
	require.NoError(t, err)

	var got meshv1alpha1.MeshtasticNode
	require.NoError(t, c.Get(context.Background(), request().NamespacedName, &got))
	cond := meta.FindStatusCondition(got.Status.Conditions, meshv1alpha1.ConditionChannelsInSync)
	require.NotNil(t, cond, "a node declaring a channel gets the ChannelsInSync condition")
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
}

func TestReconcileMissingChannelSecretIsSurfaced(t *testing.T) {
	node := newNode()
	node.Finalizers = []string{meshv1alpha1.Finalizer}
	node.Spec.Channels = []meshv1alpha1.ChannelSpec{{
		Index: 1, Name: "ops",
		PSKSecretRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "absent"}, Key: "psk",
		},
	}}
	r, _ := reconcilerFor(t, node, func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(desiredUS(), 0), nil
	})
	_, err := r.Reconcile(context.Background(), request())
	require.Error(t, err, "a declared channel whose PSK Secret is missing is surfaced, not silently ignored")
	assert.Contains(t, err.Error(), "absent")
}

func TestAirtimeBudgetCondition(t *testing.T) {
	busy := 12.0 // measured channel utilization at the current (fast) preset

	// A slower, longer-range preset on an already-busy channel is predicted to
	// exceed the ceiling: condition False, with the numbers.
	node := newNode()
	node.Spec.ModemPreset = "LONG_SLOW"
	applyAirtimeBudget(node, "SHORT_FAST", device.Info{ChannelUtilization: &busy}, 1)
	c := meta.FindStatusCondition(node.Status.Conditions, meshv1alpha1.ConditionAirtimeBudget)
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, meshv1alpha1.ReasonAirtimeBudgetExceeded, c.Reason)
	assert.Contains(t, c.Message, "SHORT_FAST to LONG_SLOW")

	// A faster preset lowers utilization: within budget.
	ok := newNode()
	ok.Spec.ModemPreset = "SHORT_FAST"
	applyAirtimeBudget(ok, "LONG_SLOW", device.Info{ChannelUtilization: &busy}, 1)
	c = meta.FindStatusCondition(ok.Status.Conditions, meshv1alpha1.ConditionAirtimeBudget)
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)

	// No change declared, or no telemetry: no condition at all.
	noChange := newNode()
	noChange.Spec.ModemPreset = "LONG_FAST"
	applyAirtimeBudget(noChange, "LONG_FAST", device.Info{ChannelUtilization: &busy}, 1)
	assert.Nil(t, meta.FindStatusCondition(noChange.Status.Conditions, meshv1alpha1.ConditionAirtimeBudget))

	noTelemetry := newNode()
	noTelemetry.Spec.ModemPreset = "LONG_SLOW"
	applyAirtimeBudget(noTelemetry, "SHORT_FAST", device.Info{}, 1)
	assert.Nil(t, meta.FindStatusCondition(noTelemetry.Status.Conditions, meshv1alpha1.ConditionAirtimeBudget))
}

func TestReconcileMissingObjectIsNoOp(t *testing.T) {
	r, _ := reconcilerFor(t, newNode(), func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) {
		return device.NewFake(desiredUS(), 0), nil
	})
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "gone", Namespace: "default"}})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)
}
