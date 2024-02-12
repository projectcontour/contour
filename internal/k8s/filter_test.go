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
	"github.com/stretchr/testify/require"

	"github.com/projectcontour/contour/internal/fixture"
)

type countHandler struct {
	added   int
	updated int
	deleted int
}

func (t *countHandler) OnAdd(_ any, _ bool) {
	t.added++
}

func (t *countHandler) OnUpdate(_, _ any) {
	t.updated++
}

func (t *countHandler) OnDelete(_ any) {
	t.deleted++
}

func TestNamespaceFilter(t *testing.T) {
	counter := countHandler{}
	filter := NewNamespaceFilter([]string{"ns1", "ns2"}, &counter)

	require.NotNil(t, filter)

	// For each operation, the first call passes an object that
	// doesn't match the filter, so should not update the count.
	// The second call is a match and does update the counter.

	filter.OnAdd(fixture.NewProxy("ns3/proxy"), false)
	assert.Equal(t, 0, counter.added)

	filter.OnAdd(fixture.NewProxy("ns1/proxy"), false)
	assert.Equal(t, 1, counter.added)

	filter.OnUpdate(fixture.NewProxy("ns3/proxy"), fixture.NewProxy("ns3/proxy"))
	assert.Equal(t, 0, counter.updated)

	filter.OnUpdate(fixture.NewProxy("ns2/proxy"), fixture.NewProxy("ns2/proxy"))
	assert.Equal(t, 1, counter.updated)

	filter.OnDelete(fixture.NewProxy("ns3/proxy"))
	assert.Equal(t, 0, counter.deleted)

	filter.OnDelete(fixture.NewProxy("ns1/proxy"))
	assert.Equal(t, 1, counter.deleted)
}
