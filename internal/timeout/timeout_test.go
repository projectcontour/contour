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
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		duration string
		want     Setting
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
		"10 seconds": {
			duration: "10s",
			want:     DurationSetting(10 * time.Second),
		},
		"invalid": {
			duration: "10", // 10 what?
			want:     DisabledSetting(),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := Parse(tc.duration)
			if tc.want != got {
				t.Errorf("Wanted %v, got %v", tc.want, got)
			}
		})
	}
}

func TestWithDuration(t *testing.T) {
	s := DurationSetting(10 * time.Second)
	want := 10 * time.Second
	got := s.Duration()
	if want != got {
		t.Errorf("Wanted %v, got %v", want, got)
	}
}
