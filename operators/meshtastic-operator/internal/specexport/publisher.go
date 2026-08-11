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

// Package specexport publishes the per-band spectrum reduction as Prometheus
// gauges. It is the exporter half of the SDR pillar: the research sweep found no
// existing exporter that turns raw sweep power into per-band aggregates, so this
// is the novel glue that puts sensed spectrum on the same observability plane as
// the operator's node health, ready to feed the Phase 6 closed loop.
//
// It publishes aggregates, never a per-bin series, so metric cardinality stays
// bounded no matter how wide the sweep. A band the sweep did not cover has its
// value gauges deleted rather than left reading a false zero, the same
// absent-not-false-zero discipline the node metrics use.
package specexport

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/blisspixel/nephmesh/operators/meshtastic-operator/internal/spectrum"
)

// Publisher owns the spectrum gauges and updates them from each sweep.
type Publisher struct {
	occupancy  *prometheus.GaugeVec
	peakDBM    *prometheus.GaugeVec
	peakFreqHz *prometheus.GaugeVec
	noiseDBM   *prometheus.GaugeVec
	bins       *prometheus.GaugeVec
	sweeps     prometheus.Counter
	sweepErr   prometheus.Counter
	lastSweep  prometheus.Gauge
}

// New builds a Publisher and registers its metrics with reg. Using an injected
// registerer keeps it usable both as a standalone exporter (a fresh registry)
// and, later, alongside the operator's own registry.
func New(reg prometheus.Registerer) *Publisher {
	const ns, sub = "nephmesh", "spectrum"
	p := &Publisher{
		occupancy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "occupancy_percent",
			Help: "Fraction of a band's bins above its noise-floor-relative threshold, percent. How busy the whole band is.",
		}, []string{"band"}),
		peakDBM: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "peak_db",
			Help: "Strongest bin power in the band, relative dB (uncalibrated, not dBm). The sensitive signal for a single active transmitter.",
		}, []string{"band"}),
		peakFreqHz: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "peak_frequency_hz",
			Help: "Frequency of the strongest bin in the band, Hz.",
		}, []string{"band"}),
		noiseDBM: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "noise_floor_db",
			Help: "Estimated per-band noise floor, relative dB (uncalibrated, not dBm), the percentile the occupancy threshold is measured against.",
		}, []string{"band"}),
		bins: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "bins",
			Help: "Number of sweep bins observed in the band. 0 means the last sweep did not cover the band.",
		}, []string{"band"}),
		sweeps: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "sweeps_total",
			Help: "Total sweeps successfully parsed and published.",
		}),
		sweepErr: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "sweep_errors_total",
			Help: "Total sweeps that failed to capture or parse.",
		}),
		lastSweep: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "last_sweep_timestamp_seconds",
			Help: "Unix time of the last successfully published sweep.",
		}),
	}
	reg.MustRegister(p.occupancy, p.peakDBM, p.peakFreqHz, p.noiseDBM, p.bins, p.sweeps, p.sweepErr, p.lastSweep)
	return p
}

// Publish records one sweep's per-band stats and stamps it with the given Unix
// time. A band with no bins in this sweep keeps a bins=0 marker but has its value
// gauges deleted, so a gap reads as "not covered" rather than a false 0 percent.
func (p *Publisher) Publish(stats []spectrum.BandStats, unixTime int64) {
	for _, s := range stats {
		l := prometheus.Labels{"band": s.Band.Name}
		p.bins.With(l).Set(float64(s.BinCount))
		if s.BinCount == 0 {
			p.occupancy.Delete(l)
			p.peakDBM.Delete(l)
			p.peakFreqHz.Delete(l)
			p.noiseDBM.Delete(l)
			continue
		}
		p.occupancy.With(l).Set(s.OccupancyPercent)
		p.peakDBM.With(l).Set(s.MaxDB)
		p.peakFreqHz.With(l).Set(s.PeakFreqHz)
		p.noiseDBM.With(l).Set(s.NoiseFloorDB)
	}
	p.sweeps.Inc()
	p.lastSweep.Set(float64(unixTime))
}

// PublishError records a failed capture or parse so a stalled sensor is visible
// (sweep_errors_total climbing while last_sweep_timestamp goes stale).
func (p *Publisher) PublishError() {
	p.sweepErr.Inc()
}
