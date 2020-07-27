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
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/projectcontour/contour/internal/assert"
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
			assert.Equal(t, tc.want, Parse(tc.duration), cmp.AllowUnexported(Setting{}))
		})
	}
}

func TestDurationSetting(t *testing.T) {
	assert.Equal(t, 10*time.Second, DurationSetting(10*time.Second).Duration())
}

func TestParseRange(t *testing.T) {
	var got AllowedRange
	var gotErr error

	got, gotErr = ParseRange("1s", "1m")
	assert.Equal(t, got, AllowedRange{min: time.Second, max: time.Minute, maxSet: true}, cmp.AllowUnexported(AllowedRange{}))
	assert.Equal(t, nil, gotErr)

	got, gotErr = ParseRange("0s", "infinity")
	assert.Equal(t, got, AllowedRange{min: 0, max: math.MaxInt64, maxSet: true}, cmp.AllowUnexported(AllowedRange{}))
	assert.Equal(t, nil, gotErr)

	got, gotErr = ParseRange("", "1h")
	assert.Equal(t, got, AllowedRange{min: 0, max: time.Hour, maxSet: true}, cmp.AllowUnexported(AllowedRange{}))
	assert.Equal(t, nil, gotErr)

	got, gotErr = ParseRange("30m", "")
	assert.Equal(t, got, AllowedRange{min: 30 * time.Minute, max: math.MaxInt64, maxSet: true}, cmp.AllowUnexported(AllowedRange{}))
	assert.Equal(t, nil, gotErr)

	got, gotErr = ParseRange("", "")
	assert.Equal(t, got, AllowedRange{min: 0, max: math.MaxInt64, maxSet: true}, cmp.AllowUnexported(AllowedRange{}))
	assert.Equal(t, nil, gotErr)

	got, gotErr = ParseRange("1m", "1s")
	assert.Equal(t, got, AllowedRange{}, cmp.AllowUnexported(AllowedRange{}))
	assert.Equal(t, "min must be less than or equal to max", gotErr.Error())
}

func TestRangeAllows(t *testing.T) {
	// the zero value of AllowedRange should allow anything.
	r := AllowedRange{}
	assert.Equal(t, true, r.Allows(DurationSetting(1)))
	assert.Equal(t, true, r.Allows(DurationSetting(time.Minute)))
	assert.Equal(t, true, r.Allows(DurationSetting(time.Hour)))
	assert.Equal(t, true, r.Allows(DurationSetting(math.MaxInt64)))
	assert.Equal(t, true, r.Allows(DefaultSetting()))
	assert.Equal(t, true, r.Allows(DisabledSetting()))

	// a specific range allows anything within the range (inclusive),
	// plus the "use default" setting.
	r = AllowedRange{min: 0, max: time.Hour, maxSet: true}
	assert.Equal(t, true, r.Allows(DurationSetting(1)))
	assert.Equal(t, true, r.Allows(DurationSetting(time.Minute)))
	assert.Equal(t, true, r.Allows(DurationSetting(time.Hour)))
	assert.Equal(t, false, r.Allows(DurationSetting(time.Hour+time.Second)))
	assert.Equal(t, true, r.Allows(DefaultSetting()))
	assert.Equal(t, false, r.Allows(DisabledSetting()))

	// a range with a max of "infinity" allows anything within the range (inclusive),
	// including a "disabled" setting, plus the "use default" setting.
	r = AllowedRange{min: time.Minute, max: math.MaxInt64, maxSet: true}
	assert.Equal(t, false, r.Allows(DurationSetting(1)))
	assert.Equal(t, false, r.Allows(DurationSetting(time.Minute-time.Second)))
	assert.Equal(t, true, r.Allows(DurationSetting(time.Minute)))
	assert.Equal(t, true, r.Allows(DurationSetting(time.Hour)))
	assert.Equal(t, true, r.Allows(DefaultSetting()))
	assert.Equal(t, true, r.Allows(DisabledSetting()))
}
