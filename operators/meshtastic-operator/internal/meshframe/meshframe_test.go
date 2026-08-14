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

package meshframe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHeaderExtractsSenderAndFields(t *testing.T) {
	// A broadcast packet from synthetic node !01020304, id 0x12345678,
	// hop limit 3, hop start 3, channel hash 8, plus two payload bytes that must
	// be ignored. Not a lab radio.
	pkt := []byte{
		0xff, 0xff, 0xff, 0xff, // to: broadcast
		0x04, 0x03, 0x02, 0x01, // from: 0x01020304 little-endian
		0x78, 0x56, 0x34, 0x12, // id: 0x12345678
		0x63,       // flags: hopLimit 3 | hopStart 3
		0x08,       // channel hash
		0x00, 0x00, // next hop, relay node
		0xde, 0xad, // encrypted payload (ignored)
	}
	h, err := ParseHeader(pkt)
	require.NoError(t, err)
	assert.Equal(t, "!01020304", h.FromID(), "the sender read straight off the air")
	assert.Equal(t, "^all", h.ToID())
	assert.Equal(t, uint32(0x12345678), h.ID)
	assert.EqualValues(t, 3, h.HopLimit)
	assert.EqualValues(t, 3, h.HopStart)
	assert.EqualValues(t, 8, h.ChannelHash)
	assert.False(t, h.WantAck)
	assert.False(t, h.ViaMQTT)
}

func TestParseHeaderFlags(t *testing.T) {
	// wantAck (0x08) and viaMqtt (0x10) set, hop limit 5.
	pkt := make([]byte, HeaderSize)
	pkt[12] = 0x05 | flagWantAckMask | flagViaMQTTMask
	h, err := ParseHeader(pkt)
	require.NoError(t, err)
	assert.EqualValues(t, 5, h.HopLimit)
	assert.True(t, h.WantAck)
	assert.True(t, h.ViaMQTT)
}

func TestParseHeaderTooShort(t *testing.T) {
	_, err := ParseHeader([]byte{0x01, 0x02, 0x03})
	assert.Error(t, err)
}

func TestNodeID(t *testing.T) {
	assert.Equal(t, "!01020304", NodeID(0x01020304))
	assert.Equal(t, "^all", NodeID(Broadcast))
	assert.Equal(t, "!00000001", NodeID(1), "always eight hex digits, zero-padded")
}
