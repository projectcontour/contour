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
	"fmt"
	"sort"
	"sync"

	envoy_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

type LocalityEndpoints = envoy_endpoint_v3.LocalityLbEndpoints
type LoadBalancingEndpoint = envoy_endpoint_v3.LbEndpoint

// RecalculateEndpoints generates a slice of LoadBalancingEndpoint
// resources by matching the given service port to the given v1.Endpoints.
// ep may be nil, in which case, the result is also nil.
func RecalculateEndpoints(port v1.ServicePort, ep *v1.Endpoints) []*LoadBalancingEndpoint {
	if ep == nil {
		return nil
	}

	var lb []*LoadBalancingEndpoint
	for _, s := range ep.Subsets {
		// Skip subsets without ready addresses.
		if len(s.Addresses) < 1 {
			continue
		}

		for _, p := range s.Ports {
			if port.Protocol != p.Protocol && p.Protocol != v1.ProtocolTCP {
				// NOTE: we only support "TCP", which is the default.
				continue
			}

			// If the port isn't named, it must be the
			// only Service port, so it's a match by
			// definition. Otherwise, only take endpoint
			// ports that match the service port name.
			if port.Name != "" && port.Name != p.Name {
				continue
			}

			// If we matched this port, collect Envoy endpoints for all the ready addresses.
			addresses := append([]v1.EndpointAddress{}, s.Addresses...) // Shallow copy.
			sort.Slice(addresses, func(i, j int) bool { return addresses[i].IP < addresses[j].IP })

			for _, a := range addresses {
				addr := envoy_v3.SocketAddress(a.IP, int(p.Port))
				lb = append(lb, envoy_v3.LBEndpoint(addr))
			}
		}
	}

	return lb
}

// EndpointsCache is a cache of Endpoint and ServiceCluster objects.
type EndpointsCache struct {
	mu sync.Mutex // Protects all fields.

	// Slice of stale clusters. A stale cluster is one that
	// needs to be recalculated. Clusters can be added to the stale
	// slice due to changes in Endpoints or due to a DAG rebuild.
	stale []*dag.ServiceCluster

	// Index of ServiceClusters. ServiceClusters are indexed
	// by the name of their Kubernetes Services. This makes it
	// easy to determine which Endpoints affect which ServiceCluster.
	services map[types.NamespacedName][]*dag.ServiceCluster

	// Cache of endpoints, indexed by name.
	endpoints map[types.NamespacedName]*v1.Endpoints
}

// Recalculate regenerates all the ClusterLoadAssignments from the
// cached Endpoints and stale ServiceClusters. A ClusterLoadAssignment
// will be generated for every stale ServerCluster, however, if there
// are no endpoints for the Services in the ServiceCluster, the
// ClusterLoadAssignment will be empty.
func (c *EndpointsCache) Recalculate() map[string]*envoy_endpoint_v3.ClusterLoadAssignment {
	c.mu.Lock()
	defer c.mu.Unlock()

	assignments := map[string]*envoy_endpoint_v3.ClusterLoadAssignment{}
	for _, cluster := range c.stale {
		// Clusters can be in the stale list multiple times;
		// skip to avoid duplicate recalculations.
		if _, ok := assignments[cluster.ClusterName]; ok {
			continue
		}

		cla := envoy_endpoint_v3.ClusterLoadAssignment{
			ClusterName: cluster.ClusterName,
			Endpoints:   nil,
			Policy:      nil,
		}

		// Look up each service, and if we have endpoints for that service,
		// attach them as a new LocalityEndpoints resource2.
		for _, w := range cluster.Services {
			n := types.NamespacedName{Namespace: w.ServiceNamespace, Name: w.ServiceName}
			if lb := RecalculateEndpoints(w.ServicePort, c.endpoints[n]); lb != nil {
				// Append the new set of endpoints. Users are allowed to set the load
				// balancing weight to 0, which we reflect to Envoy as nil in order to
				// assign no load to that locality.
				cla.Endpoints = append(
					cla.Endpoints,
					&LocalityEndpoints{
						LbEndpoints:         lb,
						LoadBalancingWeight: protobuf.UInt32OrNil(w.Weight),
					},
				)
			}
		}

		assignments[cla.ClusterName] = &cla
	}

	c.stale = nil
	return assignments
}

// SetClusters replaces the cache of ServiceCluster resources. All
// the added clusters will be marked stale.
func (c *EndpointsCache) SetClusters(clusters []*dag.ServiceCluster) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Keep a local index to start with so that errors don't cause
	// partial failure.
	serviceIndex := map[types.NamespacedName][]*dag.ServiceCluster{}

	// Reindex the cluster so that we can find them by service name.
	for _, cluster := range clusters {
		if err := cluster.Validate(); err != nil {
			return fmt.Errorf("invalid ServiceCluster %q: %w", cluster.ClusterName, err)
		}

		// Make sure service clusters with default weights are balanced.
		cluster.Rebalance()

		for _, s := range cluster.Services {
			name := types.NamespacedName{
				Namespace: s.ServiceNamespace,
				Name:      s.ServiceName,
			}

			// Create the slice entry if we have not indexed this service yet.
			entry := serviceIndex[name]
			if entry == nil {
				entry = []*dag.ServiceCluster{}
			}

			serviceIndex[name] = append(entry, cluster)
		}
	}

	c.stale = clusters
	c.services = serviceIndex

	return nil
}

// UpdateEndpoint adds ep to the cache, or replaces it if it is
// already cached. Any ServiceClusters that are backed by a Service
// that ep belongs become stale. Returns a boolean indicating whether
// any ServiceClusters use ep or not.
func (c *EndpointsCache) UpdateEndpoint(ep *v1.Endpoints) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	name := k8s.NamespacedNameOf(ep)
	c.endpoints[name] = ep.DeepCopy()

	// If any service clusters include this endpoint, mark them
	// all as stale.
	if affected := c.services[name]; len(affected) > 0 {
		c.stale = append(c.stale, affected...)
		return true
	}

	return false
}

// DeleteEndpoint deletes ep from the cache. Any ServiceClusters
// that are backed by a Service that ep belongs become stale. Returns
// a boolean indicating whether any ServiceClusters use ep or not.
func (c *EndpointsCache) DeleteEndpoint(ep *v1.Endpoints) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	name := k8s.NamespacedNameOf(ep)
	delete(c.endpoints, name)

	// If any service clusters include this endpoint, mark them
	// all as stale.
	if affected := c.services[name]; len(affected) > 0 {
		c.stale = append(c.stale, affected...)
		return true
	}

	return false
}

// NewEndpointsTranslator allocates a new endpoints translator.
func NewEndpointsTranslator(log logrus.FieldLogger) *EndpointsTranslator {
	return &EndpointsTranslator{
		Cond:        contour.Cond{},
		FieldLogger: log,
		entries:     map[string]*envoy_endpoint_v3.ClusterLoadAssignment{},
		cache: EndpointsCache{
			stale:     nil,
			services:  map[types.NamespacedName][]*dag.ServiceCluster{},
			endpoints: map[types.NamespacedName]*v1.Endpoints{},
		},
	}
}

// A EndpointsTranslator translates Kubernetes Endpoints objects into Envoy
// ClusterLoadAssignment resources.
type EndpointsTranslator struct {
	// Observer notifies when the endpoints cache has been updated.
	Observer contour.Observer

	contour.Cond
	logrus.FieldLogger

	cache EndpointsCache

	mu      sync.Mutex // Protects entries.
	entries map[string]*envoy_endpoint_v3.ClusterLoadAssignment
}

// Merge combines the given entries with the existing entries in the
// EndpointsTranslator. If the same key exists in both maps, an existing entry
// is replaced.
func (e *EndpointsTranslator) Merge(entries map[string]*envoy_endpoint_v3.ClusterLoadAssignment) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for k, v := range entries {
		e.entries[k] = v
	}
}

// OnChange observes DAG rebuild events.
func (e *EndpointsTranslator) OnChange(d *dag.DAG) {
	clusters := []*dag.ServiceCluster{}
	names := map[string]bool{}

	var visitor func(dag.Vertex)
	visitor = func(vertex dag.Vertex) {
		if svc, ok := vertex.(*dag.ServiceCluster); ok {
			if err := svc.Validate(); err != nil {
				e.WithError(err).Errorf("dropping invalid service cluster %q", svc.ClusterName)
			} else if _, ok := names[svc.ClusterName]; ok {
				e.Debugf("dropping service cluster with duplicate name %q", svc.ClusterName)
			} else {
				e.Debugf("added ServiceCluster %q from DAG", svc.ClusterName)
				clusters = append(clusters, svc.DeepCopy())
				names[svc.ClusterName] = true
			}
		}

		vertex.Visit(visitor)
	}

	// Collect all the service clusters from the DAG.
	d.Visit(visitor)

	// Update the cache with the new clusters.
	if err := e.cache.SetClusters(clusters); err != nil {
		e.WithError(err).Error("failed to cache service clusters")
	}

	// After rebuilding the DAG, the service cluster could be
	// completely different. Some could be added, and some could
	// be removed. Since we reset the cluster cache above, all
	// the load assignments will be recalculated and we can just
	// set the entries rather than merging them.
	entries := e.cache.Recalculate()

	// Only update and notify if entries has changed.
	changed := false

	e.mu.Lock()
	if !equal(e.entries, entries) {
		e.entries = entries
		changed = true
	}
	e.mu.Unlock()

	if changed {
		e.Debug("cluster load assignments changed, notifying waiters")
		e.Notify()
	} else {
		e.Debug("cluster load assignments did not change")
	}
}

// equal returns true if a and b are the same length, have the same set
// of keys, and have proto-equivalent values for each key, or false otherwise.
func equal(a, b map[string]*envoy_endpoint_v3.ClusterLoadAssignment) bool {
	if len(a) != len(b) {
		return false
	}

	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}

		if !proto.Equal(a[k], b[k]) {
			return false
		}
	}

	return true
}

func (e *EndpointsTranslator) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Endpoints:
		if !e.cache.UpdateEndpoint(obj) {
			return
		}

		e.WithField("endpoint", k8s.NamespacedNameOf(obj)).Debug("Endpoint is in use by a ServiceCluster, recalculating ClusterLoadAssignments")
		e.Merge(e.cache.Recalculate())
		e.Notify()
		if e.Observer != nil {
			e.Observer.Refresh()
		}
	default:
		e.Errorf("OnAdd unexpected type %T: %#v", obj, obj)
	}
}

func (e *EndpointsTranslator) OnUpdate(oldObj, newObj interface{}) {
	switch newObj := newObj.(type) {
	case *v1.Endpoints:
		oldObj, ok := oldObj.(*v1.Endpoints)
		if !ok {
			e.Errorf("OnUpdate endpoints %#v received invalid oldObj %T; %#v", newObj, oldObj, oldObj)
			return
		}

		// Skip computation if either old and new services or
		// endpoints are equal (thus also handling nil).
		if oldObj == newObj {
			return
		}

		// If there are no endpoints in this object, and the old
		// object also had zero endpoints, ignore this update
		// to avoid sending a noop notification to watchers.
		if len(oldObj.Subsets) == 0 && len(newObj.Subsets) == 0 {
			return
		}

		if !e.cache.UpdateEndpoint(newObj) {
			return
		}

		e.WithField("endpoint", k8s.NamespacedNameOf(newObj)).Debug("Endpoint is in use by a ServiceCluster, recalculating ClusterLoadAssignments")
		e.Merge(e.cache.Recalculate())
		e.Notify()
		if e.Observer != nil {
			e.Observer.Refresh()
		}
	default:
		e.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (e *EndpointsTranslator) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Endpoints:
		if !e.cache.DeleteEndpoint(obj) {
			return
		}

		e.WithField("endpoint", k8s.NamespacedNameOf(obj)).Debug("Endpoint was in use by a ServiceCluster, recalculating ClusterLoadAssignments")
		e.Merge(e.cache.Recalculate())
		e.Notify()
		if e.Observer != nil {
			e.Observer.Refresh()
		}
	case cache.DeletedFinalStateUnknown:
		e.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		e.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

// Contents returns a copy of the contents of the cache.
func (e *EndpointsTranslator) Contents() []proto.Message {
	e.mu.Lock()
	defer e.mu.Unlock()

	values := make([]*envoy_endpoint_v3.ClusterLoadAssignment, 0, len(e.entries))
	for _, v := range e.entries {
		values = append(values, v)
	}

	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (e *EndpointsTranslator) Query(names []string) []proto.Message {
	e.mu.Lock()
	defer e.mu.Unlock()

	values := make([]*envoy_endpoint_v3.ClusterLoadAssignment, 0, len(names))
	for _, n := range names {
		v, ok := e.entries[n]
		if !ok {
			e.Debugf("no cache entry for %q", n)
			v = &envoy_endpoint_v3.ClusterLoadAssignment{
				ClusterName: n,
			}
		}
		values = append(values, v)
	}

	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*EndpointsTranslator) TypeURL() string { return resource.EndpointType }
