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

package v3

import (
	"sort"
	"sync"

	envoy_transport_socket_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/proto"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
)

// SecretCache manages the contents of the gRPC SDS cache.
type SecretCache struct {
	mu           sync.Mutex
	values       map[string]*envoy_transport_socket_tls_v3.Secret
	staticValues map[string]*envoy_transport_socket_tls_v3.Secret
}

func NewSecretsCache(secrets []*envoy_transport_socket_tls_v3.Secret) *SecretCache {
	secretCache := &SecretCache{
		staticValues: map[string]*envoy_transport_socket_tls_v3.Secret{},
	}

	for _, s := range secrets {
		secretCache.staticValues[s.Name] = s
	}
	return secretCache
}

// Update replaces the contents of the cache with the supplied map.
func (c *SecretCache) Update(v map[string]*envoy_transport_socket_tls_v3.Secret) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.values = v
}

// Contents returns a copy of the cache's contents.
func (c *SecretCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []*envoy_transport_socket_tls_v3.Secret
	for _, v := range c.values {
		values = append(values, v)
	}
	for _, v := range c.staticValues {
		values = append(values, v)
	}
	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*SecretCache) TypeURL() string { return resource.SecretType }

func (c *SecretCache) OnChange(root *dag.DAG) {
	secrets := map[string]*envoy_transport_socket_tls_v3.Secret{}

	for _, secret := range root.GetSecrets() {
		name := envoy.Secretname(secret)
		if _, ok := secrets[name]; !ok {
			secrets[name] = envoy_v3.Secret(secret)
		}
	}

	c.Update(secrets)
}
