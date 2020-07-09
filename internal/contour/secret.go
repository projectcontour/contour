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

package contour

import (
	"sort"
	"sync"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// SecretCache manages the contents of the gRPC SDS cache.
type SecretCache struct {
	mu     sync.Mutex
	values map[string]*envoy_api_v2_auth.Secret
	Cond
}

// Update replaces the contents of the cache with the supplied map.
func (c *SecretCache) Update(v map[string]*envoy_api_v2_auth.Secret) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
	c.Cond.Notify()
}

// Contents returns a copy of the cache's contents.
func (c *SecretCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_api_v2_auth.Secret
	for _, v := range c.values {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (c *SecretCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_api_v2_auth.Secret
	for _, n := range names {
		// we can only return secrets where their value is
		// known. if the secret is not registered in the cache
		// we return nothing.
		if v, ok := c.values[n]; ok {
			values = append(values, v)
		}
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*SecretCache) TypeURL() string { return resource.SecretType }

type secretVisitor struct {
	secrets map[string]*envoy_api_v2_auth.Secret
}

// visitSecrets produces a map of *envoy_api_v2_auth.Secret
func visitSecrets(root dag.Vertex) map[string]*envoy_api_v2_auth.Secret {
	sv := secretVisitor{
		secrets: make(map[string]*envoy_api_v2_auth.Secret),
	}
	sv.visit(root)
	return sv.secrets
}

func (v *secretVisitor) visit(vertex dag.Vertex) {
	switch svh := vertex.(type) {
	case *dag.SecureVirtualHost:
		if svh.Secret != nil {
			name := envoy.Secretname(svh.Secret)
			if _, ok := v.secrets[name]; !ok {
				s := envoy.Secret(svh.Secret)
				v.secrets[s.Name] = s
			}
		}
	default:
		vertex.Visit(v.visit)
	}
}
