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

package timeout

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		duration string
		want     Setting
		wantErr  bool
	}{
		"empty": {
			duration: "",
			want:     DefaultSetting(),
		},
		"0": {
			duration: "0",
			want:     DefaultSetting(),
		},
		"0s": {
			duration: "0s",
			want:     DefaultSetting(),
		},
		"infinity": {
			duration: "infinity",
			want:     DisabledSetting(),
		},
		"infinite": {
			duration: "infinite",
			want:     DisabledSetting(),
		},
		"10 seconds": {
			duration: "10s",
			want:     DurationSetting(10 * time.Second),
		},
		"invalid": {
			duration: "10", // 10 what?
			want:     DefaultSetting(),
			wantErr:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := Parse(tc.duration)
			require.Equal(t, tc.want, got)
			if tc.wantErr {
				require.Error(t, gotErr)
			} else {
				require.NoError(t, gotErr)
			}
		})
	}
}

func TestParseMaxAge(t *testing.T) {
	tests := map[string]struct {
		duration string
		want     Setting
		wantErr  bool
	}{
		"empty": {
			duration: "",
			want:     DefaultSetting(),
		},
		"0": {
			duration: "0",
			want:     DisabledSetting(),
		},
		"0s": {
			duration: "0s",
			want:     DisabledSetting(),
		},
		"10 seconds": {
			duration: "10s",
			want:     DurationSetting(10 * time.Second),
		},
		"invalid": {
			duration: "10", // 10 what?
			want:     DefaultSetting(),
			wantErr:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotErr := ParseMaxAge(tc.duration)
			require.Equal(t, tc.want, got)
			if tc.wantErr {
				require.Error(t, gotErr)
			} else {
				require.NoError(t, gotErr)
			}
		})
	}
}

func TestDurationSetting(t *testing.T) {
	require.Equal(t, 10*time.Second, DurationSetting(10*time.Second).Duration())
}
