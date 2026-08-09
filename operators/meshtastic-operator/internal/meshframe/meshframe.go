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

// Package meshframe parses the clear-text header of a Meshtastic packet as it
// travels over the air. This is the reader behind the out-of-band witness: an SDR
// that demodulates the LoRa PHY hands up the raw packet bytes, and this extracts
// who sent it, to whom, on which channel, and how far it may still hop. The
// header is not encrypted (only the payload is), so a receiver with no channel
// key can still confirm the sender's node id independently of what the node
// itself reports. That is the ground truth a management plane can cross-check a
// radio's self-report against.
//
// The layout matches the Meshtastic firmware PacketHeader (RadioInterface.h): a
// 16-byte header with little-endian 32-bit to, from, and id, a flags byte, a
// channel-hash byte, and the next-hop and relay-node bytes added in firmware 2.3.
// It reads the header only; decrypting the payload needs the channel key and is
// out of scope here.
package meshframe

import (
	"encoding/binary"
	"fmt"
)

// HeaderSize is the on-air Meshtastic packet header length in bytes.
const HeaderSize = 16

// Broadcast is the Meshtastic broadcast address (rendered as ^all).
const Broadcast uint32 = 0xffffffff

// Flag masks from the Meshtastic firmware (RadioInterface.h).
const (
	flagHopLimitMask  = 0x07
	flagWantAckMask   = 0x08
	flagViaMQTTMask   = 0x10
	flagHopStartShift = 5
	flagHopStartMask  = 0xe0
)

// Header is the parsed clear-text Meshtastic packet header.
type Header struct {
	To          uint32 `json:"to"`
	From        uint32 `json:"from"`
	ID          uint32 `json:"id"`
	HopLimit    uint8  `json:"hopLimit"`
	WantAck     bool   `json:"wantAck"`
	ViaMQTT     bool   `json:"viaMqtt"`
	HopStart    uint8  `json:"hopStart"`
	ChannelHash uint8  `json:"channelHash"`
	NextHop     uint8  `json:"nextHop"`
	RelayNode   uint8  `json:"relayNode"`
}

// ParseHeader reads a Meshtastic packet header from the first HeaderSize bytes of
// b (extra bytes, the encrypted payload, are ignored). It errors if b is too
// short to contain a header.
func ParseHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, fmt.Errorf("packet too short: %d bytes, need at least %d", len(b), HeaderSize)
	}
	flags := b[12]
	return Header{
		To:          binary.LittleEndian.Uint32(b[0:4]),
		From:        binary.LittleEndian.Uint32(b[4:8]),
		ID:          binary.LittleEndian.Uint32(b[8:12]),
		HopLimit:    flags & flagHopLimitMask,
		WantAck:     flags&flagWantAckMask != 0,
		ViaMQTT:     flags&flagViaMQTTMask != 0,
		HopStart:    (flags & flagHopStartMask) >> flagHopStartShift,
		ChannelHash: b[13],
		NextHop:     b[14],
		RelayNode:   b[15],
	}, nil
}

// NodeID renders a node number the way Meshtastic does: the broadcast address as
// ^all, and any other address as ! followed by eight lowercase hex digits.
func NodeID(num uint32) string {
	if num == Broadcast {
		return "^all"
	}
	return fmt.Sprintf("!%08x", num)
}

// FromID is the sender's node id in Meshtastic ! notation.
func (h Header) FromID() string { return NodeID(h.From) }

// ToID is the destination's node id in Meshtastic ! notation.
func (h Header) ToID() string { return NodeID(h.To) }
