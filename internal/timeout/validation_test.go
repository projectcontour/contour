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
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKubebuilderValidation verifies that the regex used as a kubebuilder validation
// for timeout strings matches the behavior of Parse().
func TestKubebuilderValidation(t *testing.T) {
	// keep in sync with kubebuilder annotations in apis/projectcontour/v1/httpproxy.go
	regex := regexp.MustCompile(`^(((\d*(\.\d*)?h)|(\d*(\.\d*)?m)|(\d*(\.\d*)?s)|(\d*(\.\d*)?ms)|(\d*(\.\d*)?us)|(\d*(\.\d*)?µs)|(\d*(\.\d*)?ns))+|infinity|infinite)$`)

	for tc, valid := range map[string]bool{
		// valid duration strings across all allowed units
		"1h":    true,
		"1.h":   true,
		"1.27h": true,

		"1m":    true,
		"1.m":   true,
		"1.27m": true,

		"1s":    true,
		"1.s":   true,
		"1.27s": true,

		"1ms":    true,
		"1.ms":   true,
		"1.27ms": true,

		"1us":    true,
		"1.us":   true,
		"1.27us": true,

		"1µs":    true,
		"1.µs":   true,
		"1.27µs": true,

		"1ns":    true,
		"1.ns":   true,
		"1.27ns": true,

		// valid combinations of units
		"1h2.34m1s":                true,
		"1s2.34h1m7.23s0.21us1.ns": true,

		// invalid duration strings
		"abc":      false,
		"1":        false,
		"9,25s":    false,
		"disabled": false,

		// magic strings
		"infinity": true,
		"infinite": true,
	} {
		regexMatches := regex.MatchString(tc)
		_, parseErr := Parse(tc)

		if valid {
			assert.True(t, regexMatches, "input string %q: regex should match but does not", tc)
			require.NoError(t, parseErr, "input string %q", tc)
		} else {
			assert.False(t, regexMatches, "input string %q: regex should not match but does", tc)
			require.Error(t, parseErr, "input string %q", tc)
		}
	}
}
