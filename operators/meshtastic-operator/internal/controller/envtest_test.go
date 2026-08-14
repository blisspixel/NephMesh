//go:build envtest

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

// The envtest controller tier runs against a real API server and etcd, which
// the fake client cannot reproduce: server-side CEL and OpenAPI validation,
// finalizer and status-subresource semantics, and true delete-with-finalizer
// collection. It is the assume-breach admission proof (a hostile custom
// resource is refused at the door) plus the reconciler-idempotency proof.
//
// Gated by a build tag and by KUBEBUILDER_ASSETS so it runs only in the
// dedicated CI job, never in the normal unit suite. Provision the binaries with
// setup-envtest and run:
//
//	KUBEBUILDER_ASSETS=$(setup-envtest use -p path) \
//	  go test -tags envtest ./internal/controller/ -v
package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
)

var envtestClient client.Client

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// No control-plane binaries provisioned; the tagged suite is a no-op
		// rather than a hard failure where envtest cannot run.
		os.Exit(0)
	}

	scheme := runtime.NewScheme()
	if err := meshv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := intentv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	env := &envtest.Environment{
		// The generated CRD carries the CEL and OpenAPI validations under test.
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "api", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		_ = env.Stop()
		panic(err)
	}
	envtestClient = c

	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

// validNode is the smallest accepted resource; each hostile case below is a
// single mutation away from it, so a rejection pins exactly one validation.
func validNode() *meshv1alpha1.MeshtasticNode {
	return &meshv1alpha1.MeshtasticNode{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "valid-", Namespace: "default"},
		Spec: meshv1alpha1.MeshtasticNodeSpec{
			Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "device.local", Port: 4403}},
			Region:     "US",
		},
	}
}

func TestAdmissionRejectsHostileResources(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(*meshv1alpha1.MeshtasticNode)
	}{
		{"two transports set at once", func(n *meshv1alpha1.MeshtasticNode) {
			n.Spec.Connection.Serial = &meshv1alpha1.SerialConnection{Device: "/dev/ttyUSB0"}
		}},
		{"no transport set", func(n *meshv1alpha1.MeshtasticNode) {
			n.Spec.Connection = meshv1alpha1.ConnectionSpec{}
		}},
		{"region with illegal characters", func(n *meshv1alpha1.MeshtasticNode) {
			n.Spec.Region = "us-west"
		}},
		{"host with a leading dash (flag/SSRF shape)", func(n *meshv1alpha1.MeshtasticNode) {
			n.Spec.Connection.TCP.Host = "-evil.example.com"
		}},
		{"IPv4 host:port in the host field", func(n *meshv1alpha1.MeshtasticNode) {
			n.Spec.Connection.TCP.Host = "10.0.0.51:4403"
		}},
		{"port out of range", func(n *meshv1alpha1.MeshtasticNode) {
			n.Spec.Connection.TCP.Port = 70000
		}},
		{"owner short name too long", func(n *meshv1alpha1.MeshtasticNode) {
			n.Spec.Owner = &meshv1alpha1.OwnerSpec{ShortName: "TOOLONG"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := validNode()
			tc.mutate(n)
			err := envtestClient.Create(ctx, n)
			require.Error(t, err, "the API server must refuse a hostile resource at admission")
			assert.True(t, apierrors.IsInvalid(err), "rejection should be an Invalid error, got %v", err)
		})
	}
}

func TestAdmissionAcceptsValidResource(t *testing.T) {
	ctx := context.Background()
	n := validNode()
	n.Spec.ModemPreset = "LONG_FAST"
	n.Spec.Role = "CLIENT"
	n.Spec.Owner = &meshv1alpha1.OwnerSpec{ShortName: "NM01", LongName: "NephMesh Node"}
	require.NoError(t, envtestClient.Create(ctx, n), "a well-formed resource must be accepted")
	require.NoError(t, envtestClient.Delete(ctx, n))

	v6 := validNode()
	v6.Spec.Connection.TCP.Host = "2001:db8::1"
	require.NoError(t, envtestClient.Create(ctx, v6), "an IPv6 host must be accepted")
	require.NoError(t, envtestClient.Delete(ctx, v6))
}

// validIntent is the smallest accepted CommunicationIntent; each hostile case
// below is a single mutation away from it, so a rejection pins exactly one
// validation. The accept path also proves the intent CRD installs at all, which
// guards the CEL cost budget: the Connection rule inside the bounded nodes list
// must stay under the apiserver's per-schema limit.
func validIntent() *intentv1alpha1.CommunicationIntent {
	return &intentv1alpha1.CommunicationIntent{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "intent-", Namespace: "default"},
		Spec: intentv1alpha1.CommunicationIntentSpec{
			Region:               "US",
			ApprovedModemPresets: []string{"MEDIUM_SLOW"},
			Nodes: []intentv1alpha1.NodeTarget{
				{Name: "field-01", Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "device.local", Port: 4403}}},
			},
		},
	}
}

func TestIntentAdmissionRejectsHostileResources(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(*intentv1alpha1.CommunicationIntent)
	}{
		{"empty approved preset set", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.ApprovedModemPresets = []string{}
		}},
		{"no target nodes", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.Nodes = nil
		}},
		{"duplicate node name", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.Nodes = append(ci.Spec.Nodes, intentv1alpha1.NodeTarget{
				Name: "field-01", Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "other.local"}},
			})
		}},
		{"node with no transport", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.Nodes[0].Connection = meshv1alpha1.ConnectionSpec{}
		}},
		{"node host with a leading dash (flag/SSRF shape)", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.Nodes[0].Connection.TCP.Host = "-evil.example.com"
		}},
		{"region with illegal characters", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.Region = "us-west"
		}},
		{"node name not a DNS label", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.Nodes[0].Name = "Field_01"
		}},
		{"duplicate channel index", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.Channels = []meshv1alpha1.ChannelSpec{{Index: 1, Name: "a"}, {Index: 1, Name: "b"}}
		}},
		{"expectedTraffic rate below minimum", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 0}
		}},
		{"expectedTraffic payload over Meshtastic maximum", func(ci *intentv1alpha1.CommunicationIntent) {
			ci.Spec.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 5, PayloadBytes: 500}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ci := validIntent()
			tc.mutate(ci)
			err := envtestClient.Create(ctx, ci)
			require.Error(t, err, "the API server must refuse a hostile intent at admission")
			assert.True(t, apierrors.IsInvalid(err), "rejection should be an Invalid error, got %v", err)
		})
	}
}

func TestIntentAdmissionAcceptsValidResource(t *testing.T) {
	ctx := context.Background()
	ci := validIntent()
	ci.Spec.Role = "CLIENT"
	ci.Spec.Channels = []meshv1alpha1.ChannelSpec{{Index: 1, Name: "relief"}}
	ci.Spec.ExpectedTraffic = &intentv1alpha1.ExpectedTraffic{MessagesPerMinutePerNode: 4, PayloadBytes: 40}
	require.NoError(t, envtestClient.Create(ctx, ci), "a well-formed intent must be accepted")
	require.NoError(t, envtestClient.Delete(ctx, ci))
}

func TestFinalizerLifecycleAndIdempotency(t *testing.T) {
	ctx := context.Background()
	node := &meshv1alpha1.MeshtasticNode{
		ObjectMeta: metav1.ObjectMeta{Name: "lifecycle", Namespace: "default"},
		Spec: meshv1alpha1.MeshtasticNodeSpec{
			Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "device.local"}},
			Region:     "US",
		},
	}
	require.NoError(t, envtestClient.Create(ctx, node))
	t.Cleanup(func() { _ = envtestClient.Delete(ctx, node) })

	key := types.NamespacedName{Name: node.Name, Namespace: node.Namespace}
	req := ctrl.Request{NamespacedName: key}
	// One shared fake so device state persists across reconciles, seeded drifted
	// (empty) so exactly one apply converges it. rebootWindow 0 keeps it
	// reachable so the loop terminates without wall-clock waits.
	fake := device.NewFake(map[string]any{}, 0)
	r := &MeshtasticNodeReconciler{
		Client:    envtestClient,
		Reader:    envtestClient,
		NewDevice: func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) { return fake, nil },
	}

	// First reconcile installs the finalizer and does not touch the device.
	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.NoError(t, envtestClient.Get(ctx, key, node))
	assert.Contains(t, node.Finalizers, meshv1alpha1.Finalizer, "the first reconcile adds the finalizer")
	assert.Zero(t, fake.Applies, "adding the finalizer must not touch the device")

	// Drive the bounded state machine to Ready.
	ready := false
	for i := 0; i < 15 && !ready; i++ {
		_, err = r.Reconcile(ctx, req)
		require.NoError(t, err)
		require.NoError(t, envtestClient.Get(ctx, key, node))
		ready = meta.IsStatusConditionTrue(node.Status.Conditions, meshv1alpha1.ConditionReady)
	}
	require.True(t, ready, "the node should converge to Ready")
	require.GreaterOrEqual(t, fake.Applies, 1, "converging a drifted node applies at least once")
	appliesAtReady := fake.Applies

	// Idempotency: reconciling a converged node performs no further device apply.
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, appliesAtReady, fake.Applies, "a converged node must not be re-applied (idempotent)")

	// Deletion runs the finalizer path and the object is truly collected.
	require.NoError(t, envtestClient.Delete(ctx, node))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	err = envtestClient.Get(ctx, key, node)
	assert.True(t, apierrors.IsNotFound(err), "after the finalizer is removed the node is gone")
}
