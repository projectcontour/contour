// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protobuf

import (
	"math"
	"testing"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestU32Nil(t *testing.T) {
	assert.Equal(t, (*wrapperspb.UInt32Value)(nil), UInt32OrNil(0))
	assert.Equal(t, wrapperspb.UInt32(1), UInt32OrNil(1))
}

func TestU32Default(t *testing.T) {
	assert.Equal(t, wrapperspb.UInt32(99), UInt32OrDefault(0, 99))
	assert.Equal(t, wrapperspb.UInt32(1), UInt32OrDefault(1, 99))
}

func TestSafeIntToUint32(t *testing.T) {
	tests := map[string]struct {
		input    int
		expected uint32
	}{
		"zero": {
			input:    0,
			expected: 0,
		},
		"positive within range": {
			input:    42,
			expected: 42,
		},
		"max uint32": {
			input:    math.MaxUint32,
			expected: math.MaxUint32,
		},
		"negative": {
			input:    -1,
			expected: 0,
		},
		"greater than max uint32": {
			input:    math.MaxUint32 + 1,
			expected: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := SafeIntToUint32(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestAsMessages(t *testing.T) {
	assert.Nil(t, AsMessages([]*envoy_config_cluster_v3.Cluster{}))

	in := []*envoy_config_cluster_v3.Cluster{
		{Name: "cluster-1"},
		{Name: "cluster-2"},
		{Name: "cluster-3"},
		{Name: "cluster-4"},
	}
	out := AsMessages(in)

	require.Len(t, out, len(in))
	for i := range in {
		assert.EqualValues(t, in[i], out[i])
	}
}
