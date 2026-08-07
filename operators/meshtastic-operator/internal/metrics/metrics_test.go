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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestRecordPublishesGauges(t *testing.T) {
	ch, tx := 12.5, 3.0
	Record(Sample{
		Namespace: "ns", Name: "n1",
		Ready: true, ConfigInSync: false, ApplyAttempts: 3,
		ChannelUtilization: &ch, AirUtilTx: &tx,
	})
	assert.Equal(t, 1.0, testutil.ToFloat64(ready.WithLabelValues("ns", "n1")))
	assert.Equal(t, 0.0, testutil.ToFloat64(configInSync.WithLabelValues("ns", "n1")))
	assert.Equal(t, 3.0, testutil.ToFloat64(applyAttempts.WithLabelValues("ns", "n1")))
	assert.Equal(t, 12.5, testutil.ToFloat64(channelUtilization.WithLabelValues("ns", "n1")))
	assert.Equal(t, 3.0, testutil.ToFloat64(airUtilTx.WithLabelValues("ns", "n1")))
}

func TestRecordSkipsAbsentAirtime(t *testing.T) {
	// A device that did not report airtime yields no series, so an unknown value
	// is absent rather than a misleading 0.
	before := testutil.CollectAndCount(airUtilTx)
	Record(Sample{Namespace: "ns", Name: "noair", Ready: true})
	assert.Equal(t, before, testutil.CollectAndCount(airUtilTx))
}

func TestForgetDropsSeries(t *testing.T) {
	Record(Sample{Namespace: "ns", Name: "gone", Ready: true})
	before := testutil.CollectAndCount(ready)
	Forget("ns", "gone")
	assert.Equal(t, before-1, testutil.CollectAndCount(ready), "Forget removes exactly the node's series")
}
