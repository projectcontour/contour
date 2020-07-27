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
	"errors"
	"fmt"
	"math"
	"time"
)

// Setting describes a timeout setting that can be exactly one of:
// disable the timeout entirely, use the default, or use a specific
// value. The zero value is a Setting representing "use the default".
type Setting struct {
	val      time.Duration
	disabled bool
}

// IsDisabled returns whether the timeout should be disabled entirely.
func (s Setting) IsDisabled() bool {
	return s.disabled
}

// UseDefault returns whether the default proxy timeout value should be
// used.
func (s Setting) UseDefault() bool {
	return !s.disabled && s.val == 0
}

// Duration returns the explicit timeout value if one exists.
func (s Setting) Duration() time.Duration {
	return s.val
}

// IsWithin returns true if the setting is within the allowed range,
// or false otherwise.
func (s Setting) IsWithin(allowedRange AllowedRange) bool {
	return allowedRange.Allows(s)
}

// DefaultSetting returns a Setting representing "use the default".
func DefaultSetting() Setting {
	return Setting{}
}

// DisabledSetting returns a Setting representing "disable the timeout".
func DisabledSetting() Setting {
	return Setting{disabled: true}
}

// DurationSetting returns a timeout setting with the given duration.
func DurationSetting(duration time.Duration) Setting {
	return Setting{val: duration}
}

// Parse parses string representations of timeout settings that we pass
// in various places in a standard way:
//	- an empty string means "use the default".
//	- any valid representation of "0" means "use the default".
//	- a valid Go duration string is used as the specific timeout value.
//	- any other input means "disable the timeout".
func Parse(timeout string) Setting {
	// An empty string is interpreted as no explicit timeout specified, so
	// use the Envoy default.
	if timeout == "" {
		return DefaultSetting()
	}

	// Interpret "infinity" as a disabled/infinite timeout, which envoy config
	// usually expects as an explicit value of 0.
	if timeout == "infinity" {
		return DisabledSetting()
	}

	d, err := time.ParseDuration(timeout)
	if err != nil {
		// TODO(cmalonty) plumb a logger in here so we can log this error.
		// Assuming infinite duration is going to surprise people less for
		// a not-parseable duration than a implicit 15 second one.
		return DisabledSetting()
	}

	return DurationSetting(d)
}

// AllowedRange defines a range of allowed values for a timeout. The zero
// value of this struct is an AllowedRange that allows any value.
type AllowedRange struct {
	min time.Duration
	max time.Duration

	// This allows us to have a useful zero
	// value for the struct that allows all
	// values.
	maxSet bool
}

// NewAllowedRange creates an AllowedRange with the given min and
// max allowed durations.
func NewAllowedRange(min, max time.Duration) AllowedRange {
	return AllowedRange{
		min:    min,
		max:    max,
		maxSet: true,
	}
}

// ParseRange creates an AllowedRange by parsing the given min and
// max duration strings. An empty string for min or max is interpreted
// as no min/max limit. The string "infinity" is also a valid value.
func ParseRange(min, max string) (AllowedRange, error) {
	parse := func(val string, valIfEmpty time.Duration) (time.Duration, error) {
		if val == "" {
			return valIfEmpty, nil
		}

		if val == "infinity" {
			return math.MaxInt64, nil
		}

		return time.ParseDuration(val)
	}

	minDuration, err := parse(min, 0)
	if err != nil {
		return AllowedRange{}, fmt.Errorf("error parsing min: %v", err)
	}
	maxDuration, err := parse(max, math.MaxInt64)
	if err != nil {
		return AllowedRange{}, fmt.Errorf("error parsing max: %v", err)
	}

	if minDuration > maxDuration {
		return AllowedRange{}, errors.New("min must be less than or equal to max")
	}

	return NewAllowedRange(minDuration, maxDuration), nil
}

// Allows returns true if the provided setting is within the allowed
// range, or false otherwise. A timeout setting of "use default" is
// always considered to be within the range. A timeout setting of
// "disabled" is considered to be within the range if the range's max
// is set to "infinity".
func (ar AllowedRange) Allows(setting Setting) bool {
	// "Use default" is always allowed.
	if setting.UseDefault() {
		return true
	}

	// "Disabled" is only allowed if the range max is
	// unset or set to infinity/the max duration value.
	if setting.IsDisabled() {
		return ar.Max() == math.MaxInt64
	}

	return setting.Duration() >= ar.Min() && setting.Duration() <= ar.Max()
}

// Min returns the allowed range's minimimum duration.
func (ar AllowedRange) Min() time.Duration {
	return ar.min
}

// Max returns the allowed range's maximum duration.
func (ar AllowedRange) Max() time.Duration {
	if !ar.maxSet {
		return math.MaxInt64
	}
	return ar.max
}
