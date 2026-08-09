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

Validated end to end on 2026-08-09 against a HackRF (board reported as an
unrecognized id 5 by the 2021.03 tools, i.e. newer than they name) on the USB-C
port of a Jetson Orin Nano (Ubuntu 22.04, arm64), receive-only. The parser
handled real, out-of-frequency-order sweep segments and tens of thousands of bins
with no errors. Findings that shaped this runbook: a single sweep is noisy (the
915 band read anywhere from 30 to 46 percent occupancy with an 8 dB noise-floor
swing across passes), while integrating a few seconds of sweeps settled to a
stable ~32 to 34 percent with a noise floor near -69 dB and a persistent elevated
region around 905 to 918 MHz. There was no DC-spike artifact to mask: modern
`hackrf_sweep` offset-tunes the DC bin away, so each 20 MHz tile center reads as a
dip, not a false signal.

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

If you have a HackRF Pro, the distro package (2021.03) is too old to recognize it
and runs it in a legacy compatibility mode (`hackrf_info` reports an unknown board
id), which also predates the Pro spectrum-inversion fix, so per-frequency
attribution is unreliable until you update. Build the current host tools from
source with the bundled helper, then optionally flash the matching firmware:

```sh
bash hack/update-hackrf.sh          # latest host tools from source
bash hack/update-hackrf.sh --flash  # also flash firmware (a deliberate device write)
```

The aggregate occupancy is robust to the inversion bug (it just reorders bins
within a tile), so an occupancy comparison is valid even before updating; trusting
the peak frequency is not.

## 2. Capture a receive-only sweep

The bundled helper integrates several seconds of sweeps of the US 915 MHz ISM
band (integrating matters, per the findings above) and prints CSV:

```sh
sh hack/spectrum-sweep.sh > sweep.csv                    # 902-928 MHz, 1 MHz bins, 3s
sh hack/spectrum-sweep.sh 400 960 1000000 5 > survey.csv # wider ISM survey, 5s
sh hack/spectrum-sweep.sh 902 928 1000000 0 > one.csv    # a single pass (noisier)
```

You can drive it over SSH from another machine without copying it first:

```sh
ssh user@sensor 'sh -s 902 928 1000000 3' < hack/spectrum-sweep.sh > sweep.csv
```

For an RTL-SDR, produce the same CSV format directly:

```sh
rtl_power -f 902M:928M:1M -1 sweep.csv
```

## 3. Reduce it to per-band occupancy

```sh
go run ./operators/meshtastic-operator/cmd/nephmeshctl spectrum -f sweep.csv -o text
```

Expected shape (numbers depend on your RF environment; this is a real 3-second
integrated capture from the validation host):

```
ism-433      433.050-434.790 MHz: not covered by this sweep
ism-868-eu   863.000-870.000 MHz: not covered by this sweep
ism-915-us   902.000-928.000 MHz: occupancy 32.3% (4936/15288 bins), noise -69.1 dB, peak -52.0 dB @ 906.500 MHz
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
  channel, the peak power and peak frequency should track its activity. This is
  the "watch your own mesh from the outside" check. Validated 2026-08-09: a
  T-Deck flooding its US LongFast channel raised the peak from -40.9 dB idle to
  -17.1 dB at 906.5 MHz (the LongFast channel), 1367 strong hits versus 0 idle.
  Watch peak power (`maxDb` / `peakFreqHz`), not occupancy: a single transmitter
  lights up one channel, so it barely moves the band-wide occupancy percentage
  but spikes the peak by tens of dB. Occupancy answers "how busy is the whole
  band"; peak answers "is a specific transmitter active".
- Integrate, do not trust a single pass. One sweep is noisy; a few seconds
  settles the estimate (the helper defaults to a 3-second integration for this
  reason). If two back-to-back captures disagree by a lot, integrate longer.
- Separate signal from a shaped noise floor. A band reading ~30 percent occupancy
  may be real ambient traffic or just a non-flat instrument noise floor. To tell
  them apart, capture a baseline with the antenna removed (ideally a 50-ohm
  terminator on the input): if occupancy and the peak collapse without the
  antenna, you were seeing real signal; if they persist, you were measuring the
  receiver's own noise-floor shape, and the `-margin-db` / `-noise-percentile`
  thresholds want tuning. Telling "busy with what" apart from "busy" is
  classification, deliberately later work.

## Scope

This is sensing (is the band busy, and how busy), not classification (busy with
what: my mesh, another network, or a jammer). Classification, direction finding,
and a Prometheus exporter for the per-band gauges are later work; see
`docs/research/spectrum-classification.md` and
`docs/research/sdr-spectrum-sensing.md`. Nothing here transmits, and the transmit
interlock (`hack/check-transmit.sh`) keeps it that way.
