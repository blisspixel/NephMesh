# spectrum-exporter package

The receive-only SDR spectrum exporter as a kpt package. It puts sensed spectrum
on the same observability plane as the operator's node health: per-band
occupancy, peak power, peak frequency, and noise floor as Prometheus metrics,
scraped through a ServiceMonitor, with report-only alert rules.

Contents:

- `deployment.yaml`: the hardened exporter Deployment and its ClusterIP metrics
  Service (port 9808).
- `replay-sample.yaml`: a ConfigMap holding a representative `hackrf_sweep`
  capture the default Deployment replays.
- `servicemonitor.yaml`: the Prometheus Operator `ServiceMonitor` that scrapes
  the Service.
- `prometheusrule.yaml`: report-only alerts (no sensor reporting, stale sweeps,
  sweep errors, a band sustained-busy).
- `networkpolicy.yaml`: DNS-only egress, metrics-port-only ingress.

## Simulation-first, receive-only

Like the `mesh-gateway` node, the shipped Deployment runs simulation-first. It
passes `-replay=/etc/nephmesh/sample-sweep.csv`, so it publishes real per-band
metrics from a recorded sweep with no radio attached. This makes the whole
Prometheus wiring provable in a `kind` cluster at `$0` and with no hardware, and
it lets the package render and pass the manifest security gate (no `hostPath`, no
`privileged`, no host namespaces).

The metrics are the same ones the live sensor emits, so a dashboard or alert
built against the replay works unchanged against a real SDR.

## The real-SDR sensor path

Reading a live sweep needs USB access to the SDR on the host. That means a device
mount, which the manifest security gate rightly forbids in a shipped package, so
the live sensor runs outside this hardened Deployment: on the Linux USB host
(or any box with the HackRF or an RTL-SDR attached), run the exporter
directly against the sweep tool.

    spectrum-exporter -bind :9808 -freq-min 902 -freq-max 928 -interval 15s

Point a scrape config or a host-networked ServiceMonitor target at that endpoint.
The same `nephmesh_spectrum_*` metrics and the alert rules here apply. Giving a
pod real device access (a `hostPath` `/dev/bus/usb` mount, or a device plugin) is
a reviewed, site-specific deviation, deliberately not baked into the blueprint.

## Wiring it into your Prometheus

The `ServiceMonitor` and `PrometheusRule` require the Prometheus Operator
(kube-prometheus-stack or bare prometheus-operator). Two install-specific knobs
decide whether Prometheus actually selects them:

- The `release: prometheus` label must match your Prometheus's
  `serviceMonitorSelector` / `ruleSelector`. kube-prometheus-stack selects
  `release: <helm release name>`; adjust the label or drop it if your Prometheus
  selects everything.
- Prometheus must be allowed to discover this namespace
  (`serviceMonitorNamespaceSelector`, or the `...NilUsesHelmValues` defaults).

Both resources are additive: a cluster without the operator ignores them.

## Metrics

All gauges are per band (`ism-433`, `ism-868-eu`, `ism-915-us`), aggregates only
so cardinality stays bounded no matter how wide the sweep:

- `nephmesh_spectrum_occupancy_percent` how busy the band is.
- `nephmesh_spectrum_peak_db`, `nephmesh_spectrum_peak_frequency_hz` the
  strongest bin and where it is.
- `nephmesh_spectrum_noise_floor_db` the estimated per-band noise floor.
- `nephmesh_spectrum_bins` bins observed (0 means the band was not covered).
- `nephmesh_spectrum_sweeps_total`, `nephmesh_spectrum_sweep_errors_total`,
  `nephmesh_spectrum_last_sweep_timestamp_seconds` sensor liveness.

The exporter source is `operators/meshtastic-operator/cmd/spectrum-exporter`; the
per-band reduction is `internal/spectrum` and `internal/specexport`.
