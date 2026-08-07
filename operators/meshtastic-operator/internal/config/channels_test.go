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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	meshv1alpha1 "github.com/blisspixel/nephmesh/api/mesh/v1alpha1"
)

func TestPSKHash(t *testing.T) {
	assert.Equal(t, "", PSKHash(nil), "no key hashes to the empty string")
	assert.Equal(t, "", PSKHash([]byte{}), "empty key hashes to the empty string")

	h := PSKHash([]byte{0x01})
	assert.Len(t, h, 64, "sha-256 hex is 64 chars")
	assert.Equal(t, h, PSKHash([]byte{0x01}), "hashing is deterministic")
	assert.NotEqual(t, h, PSKHash([]byte{0x02}), "different keys hash differently")
}

func TestDesiredChannelsSortsAndMapsKeys(t *testing.T) {
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Channels: []meshv1alpha1.ChannelSpec{
			{Index: 2, Name: "ops", UplinkEnabled: true},
			{Index: 0, Name: "primary"},
		},
	}
	hashes := map[int32]string{0: "aaa", 2: "bbb"}

	got := DesiredChannels(spec, hashes)

	assert.Len(t, got, 2)
	assert.Equal(t, int32(0), got[0].Index, "returned sorted by index")
	assert.Equal(t, "primary", got[0].Name)
	assert.Equal(t, "aaa", got[0].PSKHash, "key hash resolved by index")
	assert.Equal(t, int32(2), got[1].Index)
	assert.True(t, got[1].UplinkEnabled)
	assert.Equal(t, "bbb", got[1].PSKHash)
}

func TestDesiredChannelsWithoutKeyHash(t *testing.T) {
	spec := meshv1alpha1.MeshtasticNodeSpec{
		Channels: []meshv1alpha1.ChannelSpec{{Index: 1, Name: "keyless"}},
	}
	got := DesiredChannels(spec, nil)
	assert.Equal(t, "", got[0].PSKHash, "a channel with no resolved key has an empty hash")
}

func TestLiveChannelsParsesAndSkipsBadEntries(t *testing.T) {
	export := map[string]any{
		"channels": []any{
			map[string]any{"index": 2, "name": "ops", "pskHash": "bbb", "uplinkEnabled": true},
			"not-a-map", // skipped
			map[string]any{"index": int64(0), "name": "primary", "pskHash": "aaa"},
			map[string]any{"index": float64(1), "downlinkEnabled": true}, // yaml/json numbers
		},
	}

	got := LiveChannels(export)

	assert.Len(t, got, 3, "the non-map entry is skipped, not fatal")
	assert.Equal(t, int32(0), got[0].Index, "sorted by index")
	assert.Equal(t, "aaa", got[0].PSKHash)
	assert.Equal(t, int32(1), got[1].Index)
	assert.True(t, got[1].DownlinkEnabled)
	assert.Equal(t, int32(2), got[2].Index)
	assert.True(t, got[2].UplinkEnabled)
}

func TestLiveChannelsAbsentOrWrongType(t *testing.T) {
	assert.Nil(t, LiveChannels(map[string]any{}), "no channels key yields nil")
	assert.Nil(t, LiveChannels(map[string]any{"channels": "wrong"}), "a non-list channels value yields nil")
}

func TestChannelDriftDetectsEachField(t *testing.T) {
	desired := []ChannelState{
		{Index: 0, Name: "primary", PSKHash: "aaa"},
		{Index: 1, Name: "ops", PSKHash: "bbb", UplinkEnabled: true, DownlinkEnabled: true},
	}
	live := []ChannelState{
		{Index: 0, Name: "primary", PSKHash: "aaa"},                                                 // matches
		{Index: 1, Name: "operations", PSKHash: "ccc", UplinkEnabled: false, DownlinkEnabled: true}, // name + psk + uplink drift
		{Index: 5, Name: "extra"}, // not declared, ignored
	}

	drift := ChannelDrift(desired, live)

	assert.Equal(t, []string{
		"channel[1].name", "channel[1].psk", "channel[1].uplinkEnabled",
	}, drift)
	assert.False(t, ChannelsConverged(desired, live))
}

func TestChannelDriftMissingChannel(t *testing.T) {
	desired := []ChannelState{{Index: 3, Name: "new"}}
	drift := ChannelDrift(desired, nil)
	assert.Equal(t, []string{"channel[3]"}, drift, "a declared channel absent from the device is drift")
}

func TestChannelsConvergedWhenAllMatch(t *testing.T) {
	desired := []ChannelState{{Index: 0, Name: "primary", PSKHash: "aaa", UplinkEnabled: true}}
	live := []ChannelState{
		{Index: 0, Name: "primary", PSKHash: "aaa", UplinkEnabled: true},
		{Index: 1, Name: "unmanaged"}, // extra live channel does not fight a partial spec
	}
	assert.True(t, ChannelsConverged(desired, live))
	assert.Empty(t, ChannelDrift(desired, live))
}
