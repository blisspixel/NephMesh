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

// Command meshtastic-operator reconciles MeshtasticNode resources against their
// devices. It is a thin main: scheme registration, a manager, and the reconciler
// wired to the CLI-backed device client. All behavior lives in the internal
// packages.
package main

import (
	"context"
	"flag"
	"net"
	"os"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	intentv1alpha1 "github.com/blisspixel/nephmesh/api/intent/v1alpha1"
	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/controller"
	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/device"
)

var scheme = runtime.NewScheme()

func init() {
	utilRuntimeMust(clientgoscheme.AddToScheme(scheme))
	utilRuntimeMust(meshv1alpha1.AddToScheme(scheme))
	utilRuntimeMust(intentv1alpha1.AddToScheme(scheme))
}

func utilRuntimeMust(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	var metricsAddr, probeAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "metrics endpoint address")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "health probe address")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "enable leader election for HA")
	// Production logging by default (JSON, no stacktraces). Developers opt into
	// development-mode logging with --zap-devel; the released binary should not
	// default to dev mode.
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logf.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("setup")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "meshtastic-operator.nephmesh.io",
	})
	if err != nil {
		log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	reconciler := &controller.MeshtasticNodeReconciler{
		Client:    mgr.GetClient(),
		Reader:    mgr.GetAPIReader(), // uncached, for namespaced Secret reads
		NewDevice: newCLIDevice,
		Recorder:  mgr.GetEventRecorderFor("meshtastic-operator"),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to set up controller")
		os.Exit(1)
	}

	// The report-only intent compiler: it renders a CommunicationIntent to
	// proposed MeshtasticNode specs on status and never actuates (ADR 0001).
	intentReconciler := &controller.CommunicationIntentReconciler{Client: mgr.GetClient()}
	if err := intentReconciler.SetupWithManager(mgr); err != nil {
		log.Error(err, "unable to set up intent controller")
		os.Exit(1)
	}

	utilRuntimeMust(mgr.AddHealthzCheck("healthz", healthz.Ping))
	utilRuntimeMust(mgr.AddReadyzCheck("readyz", healthz.Ping))

	log.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited with error")
		os.Exit(1)
	}
}

// Bundled helper defaults. The image (see the Dockerfile) carries the exporter
// and channel-apply helpers here. The exporter emits the operator's managed
// fields including the channel set (which the CLI's own --export-config does not),
// and the applier writes channels through a file so keys never reach argv. Both
// are overridable by env for local runs against a checkout.
const (
	defaultExporter = "python3 /usr/local/lib/nephmesh/mesh-export.py"
	defaultApplier  = "python3 /usr/local/lib/nephmesh/mesh-apply.py"
)

func helperArgv(env, fallback string) []string {
	v := os.Getenv(env)
	if v == "" {
		v = fallback
	}
	return strings.Fields(v)
}

// newCLIDevice builds a CLI-backed device client for a node's TCP connection,
// wired to the bundled config exporter and channel-apply helpers. Serial and
// viaGateway transports are added as their reconcile paths are built; until then
// a node that selects them is reported as unsupported rather than silently
// mishandled.
func newCLIDevice(_ context.Context, node *meshv1alpha1.MeshtasticNode) (device.Client, error) {
	if tcp := node.Spec.Connection.TCP; tcp != nil {
		// Honor the declared port (the CRD defaults it to 4403); passing it as
		// host:port is understood by the CLI and split by the helpers. Without
		// this a device on a non-default port was silently contacted on 4403.
		host := tcp.Host
		if tcp.Port != 0 {
			host = net.JoinHostPort(host, strconv.Itoa(int(tcp.Port)))
		}
		return &device.CLIClient{
			Host:     host,
			Exporter: helperArgv("NEPHMESH_EXPORTER", defaultExporter),
			Applier:  helperArgv("NEPHMESH_APPLIER", defaultApplier),
		}, nil
	}
	return &device.Unsupported{Transport: transportName(node)}, nil
}

func transportName(node *meshv1alpha1.MeshtasticNode) string {
	switch {
	case node.Spec.Connection.Serial != nil:
		return "serial"
	case node.Spec.Connection.ViaGateway != nil:
		return "viaGateway"
	default:
		return "none"
	}
}
