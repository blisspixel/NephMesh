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

// Package metrics exposes per-node mesh health as Prometheus gauges on the
// manager's existing /metrics endpoint. The project committed (docs/roadmap.md,
// "Resilience, defined") to reporting resilience as numbers rather than an
// adjective, and the research sweep named a missing observability layer as the
// single most important gap. This surfaces the KPIs the operator already reads
// (airtime utilization, readiness, apply attempts) so they can be scraped,
// alerted on, and fed to the Phase 6 closed loop.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const subsystem = "meshtasticnode"

var (
	ready = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nephmesh", Subsystem: subsystem, Name: "ready",
		Help: "1 when the node is reachable and its config is in sync, else 0.",
	}, []string{"namespace", "name"})

	configInSync = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nephmesh", Subsystem: subsystem, Name: "config_in_sync",
		Help: "1 when the device config matches the declared intent, else 0.",
	}, []string{"namespace", "name"})

	applyAttempts = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nephmesh", Subsystem: subsystem, Name: "apply_attempts",
		Help: "Consecutive non-converging apply-and-reboot attempts (0 when converged).",
	}, []string{"namespace", "name"})

	channelUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nephmesh", Subsystem: subsystem, Name: "channel_utilization_percent",
		Help: "The radio's measured channel utilization, percent. Airtime is the LoRa scaling wall.",
	}, []string{"namespace", "name"})

	airUtilTx = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "nephmesh", Subsystem: subsystem, Name: "air_util_tx_percent",
		Help: "The radio's measured transmit airtime utilization, percent.",
	}, []string{"namespace", "name"})
)

func init() {
	ctrlmetrics.Registry.MustRegister(ready, configInSync, applyAttempts, channelUtilization, airUtilTx)
}

// Sample is one node's observed health, recorded after a reconcile.
type Sample struct {
	Namespace          string
	Name               string
	Ready              bool
	ConfigInSync       bool
	ApplyAttempts      int32
	ChannelUtilization *float64 // percent; nil when the device did not report it
	AirUtilTx          *float64 // percent; nil when the device did not report it
}

// Record publishes a node's health as gauges. Airtime metrics are only set when
// the device reported them, so an unknown value is absent rather than a false 0.
func Record(s Sample) {
	l := prometheus.Labels{"namespace": s.Namespace, "name": s.Name}
	ready.With(l).Set(boolToFloat(s.Ready))
	configInSync.With(l).Set(boolToFloat(s.ConfigInSync))
	applyAttempts.With(l).Set(float64(s.ApplyAttempts))
	// When a value is unknown (the device did not report it), delete the series
	// rather than leaving a previously-set gauge lingering as stale live data.
	if s.ChannelUtilization != nil {
		channelUtilization.With(l).Set(*s.ChannelUtilization)
	} else {
		channelUtilization.Delete(l)
	}
	if s.AirUtilTx != nil {
		airUtilTx.With(l).Set(*s.AirUtilTx)
	} else {
		airUtilTx.Delete(l)
	}
}

// Forget drops a node's series, called when the node is deleted so stale gauges
// do not linger.
func Forget(namespace, name string) {
	l := prometheus.Labels{"namespace": namespace, "name": name}
	ready.Delete(l)
	configInSync.Delete(l)
	applyAttempts.Delete(l)
	channelUtilization.Delete(l)
	airUtilTx.Delete(l)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
