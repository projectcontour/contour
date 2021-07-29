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

func testAdminInterface() {
	Specify("requests to admin listener are served", func() {
		t := f.T()

		cases := []string{
			"/certs",
			"/clusters",
			"/listeners",
			"/config_dump",
			"/memory",
			"/ready",
			"/runtime",
			"/server_info",
			"/stats",
			"/stats/prometheus",
			"/stats/recentlookups",
		}

		for _, prefix := range cases {
			t.Logf("Querying admin prefix %q", prefix)

			res, ok := f.HTTP.AdminRequestUntil(&e2e.HTTPRequestOpts{
				Path:      prefix,
				Condition: e2e.HasStatusCode(200),
			})
			require.Truef(t, ok, "expected 200 response code, got %d", res.StatusCode)
		}
	})
}
