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
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
)

// TestReconcileNeverLogsOrStoresSecretMaterial is the no-secret-in-logs
// assume-breach proof: a sentinel broker password flows through a full
// reconcile (Secret read, config build, device apply) and must reach the
// device, yet must never appear in a log line or in the stored status.
func TestReconcileNeverLogsOrStoresSecretMaterial(t *testing.T) {
	const sentinel = "s3nt1nel-p@ssw0rd-never-log-me"

	node := &meshv1alpha1.MeshtasticNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1", Namespace: "default"},
		Spec: meshv1alpha1.MeshtasticNodeSpec{
			Connection: meshv1alpha1.ConnectionSpec{TCP: &meshv1alpha1.TCPConnection{Host: "h"}},
			Region:     "US",
			MQTT: &meshv1alpha1.MQTTSpec{
				Enabled:  true,
				Address:  "10.0.0.5",
				Username: "meshops",
				PasswordSecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "broker-creds"},
					Key:                  "password",
				},
			},
		},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "broker-creds", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte(sentinel)},
	}

	c := fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(node, sec).
		WithStatusSubresource(&meshv1alpha1.MeshtasticNode{}).
		Build()

	// Seed the device drifted so the reconcile actually applies the config that
	// carries the password, exercising the full write path.
	fakeDev := device.NewFake(map[string]any{}, 0)
	r := &MeshtasticNodeReconciler{
		Client:    c,
		Reader:    c,
		NewDevice: func(context.Context, *meshv1alpha1.MeshtasticNode) (device.Client, error) { return fakeDev, nil },
	}

	// Capture everything the reconciler logs, at maximum verbosity.
	var buf bytes.Buffer
	logger := funcr.New(
		func(prefix, args string) { buf.WriteString(prefix + " " + args + "\n") },
		funcr.Options{Verbosity: 10},
	)
	ctx := logf.IntoContext(context.Background(), logger)

	for i := 0; i < 8; i++ {
		_, err := r.Reconcile(ctx, request())
		require.NoError(t, err)
	}

	// The password did reach the device (redaction must not break function). Read
	// the raw applied config, not ExportConfig: the fake models the real device,
	// which never echoes the password back.
	applied := fakeDev.Applied()
	mqtt := applied["module_config"].(map[string]any)["mqtt"].(map[string]any)
	assert.Equal(t, sentinel, mqtt["password"], "the password must reach the device config")

	// But it must never appear in a log line or anywhere in the stored object.
	assert.NotContains(t, buf.String(), sentinel, "the secret must never be logged")

	got := &meshv1alpha1.MeshtasticNode{}
	require.NoError(t, c.Get(ctx, types.NamespacedName{Name: "node1", Namespace: "default"}, got))
	assert.NotContains(t, fmt.Sprintf("%+v", got), sentinel, "the secret must never be stored on the resource")
}
