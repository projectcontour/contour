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

//go:build e2e

package infra

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"

	"github.com/projectcontour/contour/test/e2e"
)

func testAdminInterface() {
	Specify("requests to admin listener are served", func() {
		t := f.T()

		cases := map[string]int{
			"/certs":               200,
			"/clusters":            200,
			"/listeners":           200,
			"/config_dump":         200,
			"/memory":              200,
			"/ready":               200,
			"/runtime":             200,
			"/server_info":         200,
			"/stats":               200,
			"/stats/prometheus":    200,
			"/stats/recentlookups": 200,
			"/quitquitquit":        404,
			"/healthcheck/ok":      404,
			"/healthcheck/fail":    404,
		}

		for prefix, code := range cases {
			t.Logf("Querying admin prefix %q", prefix)

			res, ok := f.HTTP.AdminRequestUntil(&e2e.HTTPRequestOpts{
				Path:      prefix,
				Condition: e2e.HasStatusCode(code),
			})
			require.NotNil(t, res, "request never succeeded")
			require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		}
	})
}
