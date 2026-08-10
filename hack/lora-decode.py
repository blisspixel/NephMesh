#!/usr/bin/env python3
# Copyright 2026 The NephMesh Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Receive-only LoRa decoder for the out-of-band witness.

It demodulates LoRa frames from an SDR (via gr-lora_sdr) and prints each decoded
payload as hex, one per line, which `nephmeshctl decode` reads into a Meshtastic
packet header (who sent it, to whom, on which channel). Nothing here transmits:
the SDR is a receiver only.

It is deliberately portable, not tied to one radio or one mesh: the SDR is a
SoapySDR/osmosdr device string (so an RTL-SDR works as well as a HackRF), and
every LoRa PHY parameter is a flag, defaulting to Meshtastic's US LONG_FAST
channel. Run it with the Python that GNU Radio uses (the installer prints it):

    /usr/bin/python3.10 hack/lora-decode.py --freq 906.875e6 | nephmeshctl decode

Then key a Meshtastic node and watch its sender node id appear, read straight off
the air by a receiver that was never told what the node is.
"""

import argparse
import sys

import pmt
from gnuradio import blocks
from gnuradio import gr
from gnuradio import lora_sdr
from gnuradio.filter import rational_resampler_ccc
import osmosdr


class HexSink(gr.sync_block):
    """A message sink that prints each decoded LoRa payload as one hex line."""

    def __init__(self):
        gr.sync_block.__init__(self, name="hex_sink", in_sig=None, out_sig=None)
        self.message_port_register_in(pmt.intern("in"))
        self.set_msg_handler(pmt.intern("in"), self._handle)

    def _handle(self, msg):
        try:
            vec = pmt.cdr(msg) if pmt.is_pair(msg) else msg
            if pmt.is_u8vector(vec):
                data = bytes(pmt.u8vector_elements(vec))
                sys.stdout.write(data.hex() + "\n")
                sys.stdout.flush()
        except Exception as exc:  # a malformed PDU must not kill the flowgraph
            sys.stderr.write("decode handler error: %s\n" % exc)

    def work(self, input_items, output_items):
        return 0


class LoraDecode(gr.top_block):
    def __init__(self, args):
        gr.top_block.__init__(self, "lora-decode")

        # Bring the SDR rate down to os_factor * bandwidth. Radios like the HackRF
        # cannot sample as low as a LoRa channel is wide, so oversample and
        # decimate to a standard os_factor (4). A large os_factor overwhelms
        # frame_sync (it consumes a couple of oversampled symbols at once), so the
        # decimation keeps it at 4.
        os_factor = max(1, int(args.os))
        target_rate = args.bw * os_factor
        decim = max(1, int(round(args.samp_rate / target_rate)))

        # The source is either the live SDR or a recorded complex-float IQ file
        # (record with --record, then iterate decode params offline with --iq-file
        # without re-transmitting).
        if args.iq_file:
            src = blocks.file_source(gr.sizeof_gr_complex, args.iq_file, False)
        else:
            src = osmosdr.source(args=args.device)
            src.set_sample_rate(args.samp_rate)
            src.set_center_freq(args.freq, 0)
            src.set_gain(args.gain, 0)
            src.set_if_gain(args.if_gain, 0)
            src.set_bb_gain(args.bb_gain, 0)
            src.set_bandwidth(target_rate, 0)

        # The gr-lora_sdr receive chain (see the installed lora_RX.grc example):
        # frame sync, FFT demod, gray mapping, deinterleave, Hamming decode,
        # header decode, dewhiten, CRC verify. The header decoder feeds frame
        # info back to the sync block, and the CRC block emits the payload.
        frame_sync = lora_sdr.frame_sync(
            int(args.freq), int(args.bw), args.sf, args.impl_head,
            args.sync_word, os_factor, args.preamble)
        fft_demod = lora_sdr.fft_demod(args.soft, True)
        gray = lora_sdr.gray_mapping(args.soft)
        deint = lora_sdr.deinterleaver(args.soft)
        hamming = lora_sdr.hamming_dec(args.soft)
        header = lora_sdr.header_decoder(
            args.impl_head, args.cr, args.pay_len, args.has_crc, args.ldro, args.print_header)
        dewhite = lora_sdr.dewhitening()
        crc = lora_sdr.crc_verif(0, False)
        sink = HexSink()

        # The block that feeds frame_sync needs a buffer large enough for its
        # multi-symbol reads (the default is too small at these rates).
        feeder = src
        if decim > 1:
            resamp = rational_resampler_ccc(interpolation=1, decimation=decim)
            self.connect(src, resamp)
            feeder = resamp
        feeder.set_min_output_buffer(int(args.buffer))

        self.connect(feeder, frame_sync, fft_demod, gray, deint, hamming, header)
        self.connect(header, dewhite, crc)
        self.msg_connect(header, "frame_info", frame_sync, "frame_info")
        self.msg_connect(crc, "msg", sink, "in")


def parse_sync_word(text):
    # Accept "0x2b", "43", or a comma list like "0x24,0x44".
    return [int(p, 0) for p in text.split(",") if p.strip()]


def main():
    p = argparse.ArgumentParser(description="Receive-only LoRa/Meshtastic decoder (prints payload hex).")
    p.add_argument("--device", default="hackrf=0", help="SoapySDR/osmosdr device string (e.g. hackrf=0 or rtl=0)")
    p.add_argument("--iq-file", dest="iq_file", default="", help="decode a recorded complex-float IQ file instead of the live SDR")
    p.add_argument("--freq", type=float, default=906.875e6, help="channel center frequency, Hz (default US LONG_FAST)")
    p.add_argument("--samp-rate", dest="samp_rate", type=float, default=2e6, help="SDR sample rate, Hz (decimated to os*bw)")
    p.add_argument("--os", type=int, default=4, help="oversampling factor into the LoRa sync (4 is standard)")
    p.add_argument("--bw", type=int, default=250000, help="LoRa bandwidth, Hz (LONG_FAST=250000)")
    p.add_argument("--sf", type=int, default=11, help="spreading factor (LONG_FAST=11)")
    p.add_argument("--cr", type=int, default=1, help="coding rate 1..4 for 4/5..4/8 (explicit header overrides)")
    p.add_argument("--sync-word", dest="sync_word", type=parse_sync_word, default=[0x2b], help="LoRa sync word (Meshtastic 0x2b)")
    p.add_argument("--preamble", type=int, default=16, help="preamble length in symbols (Meshtastic 16)")
    p.add_argument("--ldro", type=int, default=2, help="low-data-rate optimize: 0 off, 1 on, 2 auto")
    p.add_argument("--pay-len", dest="pay_len", type=int, default=255, help="max payload length (explicit header overrides)")
    p.add_argument("--impl-head", dest="impl_head", action="store_true", help="implicit header (Meshtastic uses explicit)")
    p.add_argument("--no-crc", dest="has_crc", action="store_false", help="payload has no CRC (Meshtastic has one)")
    p.add_argument("--soft", action="store_true", help="soft-decision decoding")
    p.add_argument("--print-header", dest="print_header", action="store_true", help="print the LoRa header to stderr")
    p.add_argument("--buffer", type=float, default=1 << 20, help="minimum source output buffer, items (raise if the scheduler starves)")
    p.add_argument("--gain", type=float, default=32, help="RF/LNA gain, dB")
    p.add_argument("--if-gain", dest="if_gain", type=float, default=24, help="IF gain, dB")
    p.add_argument("--bb-gain", dest="bb_gain", type=float, default=24, help="baseband/VGA gain, dB")
    args = p.parse_args()

    sys.stderr.write(
        "lora-decode: %s at %.3f MHz, SF%d BW%d sync=%s (receive-only). Ctrl-C to stop.\n"
        % (args.device, args.freq / 1e6, args.sf, args.bw, [hex(x) for x in args.sync_word]))
    tb = LoraDecode(args)
    try:
        tb.run()
    except KeyboardInterrupt:
        tb.stop()
        tb.wait()


if __name__ == "__main__":
    main()
