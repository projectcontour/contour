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

import "time"

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
