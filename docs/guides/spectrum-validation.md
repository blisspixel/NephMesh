# Spectrum sensing validation runbook

This is the step-by-step for validating NephMesh's receive-only spectrum sensing
against a real SDR (a HackRF One or HackRF Pro, an RTL-SDR, or any SoapySDR
device that produces an rtl_power-format sweep). Everything here is receive-only:
it listens, it never transmits. In our US-scoped, non-lawyer reading, receiving
needs no license, but you are responsible for your own jurisdiction; see the
repository DISCLAIMER.

Until you run this, the sensing path is exercised only against synthetic sweeps
(`internal/spectrum` unit tests and `examples/spectrum-sweep-sample.csv`). This
runbook closes that gap with a real capture.

## Before you plug in: the host

AGENTS.md sets a deliberate policy: USB device work is Linux-hosted. The HackRF
host tools and udev rules are simplest and best-tested on Linux, and the project
keeps device plumbing there on purpose.

- Preferred: attach the SDR to a Linux host (the project's Linux box, a Pi, or
  WSL2 with USB passthrough) and run the capture there.
- If you attach it to the Windows development box instead, that is a conscious
  exception to the policy for a one-off validation. The HackRF tools do run on
  Windows, but treat it as ad hoc, not the supported path, and do not commit
  Windows-specific device tooling on the strength of it.

The analysis half (`nephmeshctl spectrum`) is pure and runs anywhere; only the
capture half needs the radio.

## 1. Confirm the tools and the device

```sh
hackrf_info      # prints board id, serial, firmware; non-zero if no device
```

Install if missing: on Debian/Ubuntu `sudo apt install hackrf`, then ensure the
udev rules (`53-hackrf.rules`) are present so a non-root user can access the
device. An RTL-SDR uses `rtl_power` instead and needs the `dvb_usb_rtl28xxu`
kernel module blacklisted on the host.

## 2. Capture a receive-only sweep

The bundled helper does one pass of the US 915 MHz ISM band and prints CSV:

```sh
sh hack/spectrum-sweep.sh > sweep.csv                 # 902-928 MHz, 1 MHz bins
sh hack/spectrum-sweep.sh 400 960 1000000 > survey.csv # wider ISM survey
```

For an RTL-SDR, produce the same CSV format directly:

```sh
rtl_power -f 902M:928M:1M -1 sweep.csv
```

## 3. Reduce it to per-band occupancy

```sh
go run ./operators/meshtastic-operator/cmd/nephmeshctl spectrum -f sweep.csv -o text
```

Expected shape (numbers depend on your RF environment):

```
ism-433      433.050-434.790 MHz: not covered by this sweep
ism-868-eu   863.000-870.000 MHz: not covered by this sweep
ism-915-us   902.000-928.000 MHz: occupancy 7.7% (2/26 bins), noise -95.4 dB, peak -48.6 dB @ 916.500 MHz
```

JSON (`-o json`) is the machine form for an agent or a downstream exporter.

## 4. What to check

- The parser accepts real `hackrf_sweep` output unchanged (it is rtl_power
  format). If a real capture fails to parse, that is a finding: capture a small
  sample and open the CSV to compare against the format the parser expects.
- The noise floor the tool reports matches the quiet baseline you see in a
  waterfall. If idle bands read as occupied, or a busy band reads as idle, tune
  `-margin-db` (how far above the floor counts as a signal) and
  `-noise-percentile` (what counts as the floor), and record what worked.
- Point it at your own mesh: with a Meshtastic node transmitting on your US
  channel, the `ism-915-us` occupancy and the peak frequency should track its
  activity. This is the "watch your own mesh from the outside" check.

## Scope

This is sensing (is the band busy, and how busy), not classification (busy with
what: my mesh, another network, or a jammer). Classification, direction finding,
and a Prometheus exporter for the per-band gauges are later work; see
`docs/research/spectrum-classification.md` and
`docs/research/sdr-spectrum-sensing.md`. Nothing here transmits, and the transmit
interlock (`hack/check-transmit.sh`) keeps it that way.
