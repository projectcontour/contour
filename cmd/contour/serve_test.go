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

package main

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/projectcontour/contour/internal/timeout"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestGetRequestTimeout(t *testing.T) {
	tests := []struct {
		name string
		ctx  *serveContext
		want timeout.Setting
	}{
		{
			name: "neither field set",
			ctx:  &serveContext{},
			want: timeout.DefaultSetting(),
		},
		{
			name: "only deprecated field set",
			ctx:  &serveContext{RequestTimeoutDeprecated: 7 * time.Second},
			want: timeout.DurationSetting(7 * time.Second),
		},
		{
			name: "only new field set",
			ctx: &serveContext{
				TimeoutConfig: TimeoutConfig{
					RequestTimeout: "70s",
				},
			},
			want: timeout.DurationSetting(70 * time.Second),
		},
		{
			name: "both fields set, new field takes precedence",
			ctx: &serveContext{
				TimeoutConfig: TimeoutConfig{
					RequestTimeout: "70s",
				},
				RequestTimeoutDeprecated: 7 * time.Second,
			},
			want: timeout.DurationSetting(70 * time.Second),
		},
	}

	for _, tc := range tests {
		log := logrus.New()
		log.Out = ioutil.Discard

		assert.Equal(t, tc.want, getRequestTimeout(log, tc.ctx))
	}
}
