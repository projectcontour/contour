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

// +build e2e

package infra

import (
	. "github.com/onsi/ginkgo"
	"github.com/projectcontour/contour/test/e2e"
	"github.com/stretchr/testify/require"
)

func testMetrics() {
	Specify("requests to default metrics listener are served", func() {
		t := f.T()

		res, ok := f.HTTP.MetricsRequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/stats",
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}

func testReady() {
	Specify("requests to default ready listener are served", func() {
		t := f.T()

		res, ok := f.HTTP.MetricsRequestUntil(&e2e.HTTPRequestOpts{
			Path:      "/ready",
			Condition: e2e.HasStatusCode(200),
		})
		require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
	})
}
