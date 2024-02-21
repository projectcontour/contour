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

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
)

func TestNamespacedNameFrom(t *testing.T) {
	run := func(testName string, got, want types.NamespacedName) {
		t.Helper()
		t.Run(testName, func(t *testing.T) {
			t.Helper()
			assert.Equal(t, want, got)
		})
	}

	run("no namespace",
		NamespacedNameFrom("secret", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "defns",
		},
	)

	run("with namespace",
		NamespacedNameFrom("ns1/secret", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "ns1",
		},
	)

	run("missing namespace",
		NamespacedNameFrom("/secret", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "defns",
		},
	)

	run("missing secret name",
		NamespacedNameFrom("secret/", DefaultNamespace("defns")),
		types.NamespacedName{
			Name:      "",
			Namespace: "secret",
		},
	)

	run("missing default namespace",
		NamespacedNameFrom("secret"),
		types.NamespacedName{
			Name:      "secret",
			Namespace: "default",
		},
	)

	run("empty resource",
		NamespacedNameFrom(""),
		types.NamespacedName{
			Name:      "",
			Namespace: "default",
		},
	)
}
