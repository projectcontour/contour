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

package contourconfig_test

import (
	"testing"
	"time"

	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/contourconfig"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"
)

func TestParseTimeoutPolicy(t *testing.T) {
	testCases := map[string]struct {
		config   *contour_api_v1alpha1.TimeoutParameters
		expected contourconfig.Timeouts
		errorMsg string
	}{
		"nil timeout parameters": {
			config: nil,
			expected: contourconfig.Timeouts{
				Request:                       timeout.DefaultSetting(),
				ConnectionIdle:                timeout.DefaultSetting(),
				StreamIdle:                    timeout.DefaultSetting(),
				MaxConnectionDuration:         timeout.DefaultSetting(),
				DelayedClose:                  timeout.DefaultSetting(),
				ConnectionShutdownGracePeriod: timeout.DefaultSetting(),
				ConnectTimeout:                0,
			},
		},
		"timeouts not set": {
			config: &contour_api_v1alpha1.TimeoutParameters{},
			expected: contourconfig.Timeouts{
				Request:                       timeout.DefaultSetting(),
				ConnectionIdle:                timeout.DefaultSetting(),
				StreamIdle:                    timeout.DefaultSetting(),
				MaxConnectionDuration:         timeout.DefaultSetting(),
				DelayedClose:                  timeout.DefaultSetting(),
				ConnectionShutdownGracePeriod: timeout.DefaultSetting(),
				ConnectTimeout:                0,
			},
		},
		"timeouts all set": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				RequestTimeout:                pointer.String("1s"),
				ConnectionIdleTimeout:         pointer.String("2s"),
				StreamIdleTimeout:             pointer.String("3s"),
				MaxConnectionDuration:         pointer.String("infinity"),
				DelayedCloseTimeout:           pointer.String("5s"),
				ConnectionShutdownGracePeriod: pointer.String("6s"),
				ConnectTimeout:                pointer.String("8s"),
			},
			expected: contourconfig.Timeouts{
				Request:                       timeout.DurationSetting(time.Second),
				ConnectionIdle:                timeout.DurationSetting(time.Second * 2),
				StreamIdle:                    timeout.DurationSetting(time.Second * 3),
				MaxConnectionDuration:         timeout.DisabledSetting(),
				DelayedClose:                  timeout.DurationSetting(time.Second * 5),
				ConnectionShutdownGracePeriod: timeout.DurationSetting(time.Second * 6),
				ConnectTimeout:                8 * time.Second,
			},
		},
		"request timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				RequestTimeout: pointer.String("xxx"),
			},
			errorMsg: "failed to parse request timeout",
		},
		"connection idle timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				ConnectionIdleTimeout: pointer.String("a"),
			},
			errorMsg: "failed to parse connection idle timeout",
		},
		"stream idle timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				StreamIdleTimeout: pointer.String("invalid"),
			},
			errorMsg: "failed to parse stream idle timeout",
		},
		"max connection duration invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				MaxConnectionDuration: pointer.String("xxx"),
			},
			errorMsg: "failed to parse max connection duration",
		},
		"delayed close timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				DelayedCloseTimeout: pointer.String("xxx"),
			},
			errorMsg: "failed to parse delayed close timeout",
		},
		"connection shutdown grace period invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				ConnectionShutdownGracePeriod: pointer.String("xxx"),
			},
			errorMsg: "failed to parse connection shutdown grace period",
		},
		"connect timeout invalid": {
			config: &contour_api_v1alpha1.TimeoutParameters{
				ConnectTimeout: pointer.String("infinite"),
			},
			errorMsg: "failed to parse connect timeout",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			parsed, err := contourconfig.ParseTimeoutPolicy(tc.config)
			if len(tc.errorMsg) > 0 {
				require.Error(t, err, "expected error to be returned")
				require.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.Nil(t, err)
				require.Equal(t, tc.expected, parsed)
			}
		})
	}
}
