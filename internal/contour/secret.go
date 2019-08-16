// Copyright © 2018 Heptio
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

	"github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	"github.com/gogo/protobuf/proto"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/envoy"
)

// SecretCache manages the contents of the gRPC SDS cache.
type SecretCache struct {
	mu      sync.Mutex
	values  map[string]*auth.Secret
	waiters []chan int
	last    int
}

// Register registers ch to receive a value when Notify is called.
// The value of last is the count of the times Notify has been called on this Cache.
// It functions of a sequence counter, if the value of last supplied to Register
// is less than the Cache's internal counter, then the caller has missed at least
// one notification and will fire immediately.
//
// Sends by the broadcaster to ch must not block, therefor ch must have a capacity
// of at least 1.
func (c *SecretCache) Register(ch chan int, last int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if last < c.last {
		// notify this channel immediately
		ch <- c.last
		return
	}
	c.waiters = append(c.waiters, ch)
}

// Update replaces the contents of the cache with the supplied map.
func (c *SecretCache) Update(v map[string]*auth.Secret) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.last++
	c.values = v

	for _, ch := range c.waiters {
		ch <- c.last
	}
	c.waiters = c.waiters[:0]
}

// Contents returns a copy of the cache's contents.
func (c *SecretCache) Contents() []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []proto.Message
	for _, v := range c.values {
		values = append(values, v)
	}
	sort.Stable(secretsByName(values))
	return values
}

func (c *SecretCache) Query(names []string) []proto.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var values []proto.Message
	for _, n := range names {
		// we can only return secrets where their value is
		// known. if the secret is not registered in the cache
		// we return nothing.
		if v, ok := c.values[n]; ok {
			values = append(values, v)
		}
	}
	sort.Stable(secretsByName(values))
	return values
}

type secretsByName []proto.Message

func (s secretsByName) Len() int      { return len(s) }
func (s secretsByName) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s secretsByName) Less(i, j int) bool {
	return s[i].(*auth.Secret).Name < s[j].(*auth.Secret).Name
}

func (*SecretCache) TypeURL() string { return cache.SecretType }

type secretVisitor struct {
	secrets map[string]*auth.Secret
}

// visitSecrets produces a map of *auth.Secret
func visitSecrets(root dag.Vertex) map[string]*auth.Secret {
	sv := secretVisitor{
		secrets: make(map[string]*auth.Secret),
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
