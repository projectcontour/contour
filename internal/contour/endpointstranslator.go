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

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_api_v2_endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/golang/protobuf/proto"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/protobuf"
	"github.com/projectcontour/contour/internal/sorter"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8scache "k8s.io/client-go/tools/cache"
)

// A EndpointsTranslator translates Kubernetes Endpoints objects into Envoy
// ClusterLoadAssignment objects.
type EndpointsTranslator struct {
	Cond
	logrus.FieldLogger

	mu      sync.Mutex
	entries map[string]*v2.ClusterLoadAssignment
}

func (e *EndpointsTranslator) OnChange(d *dag.DAG) {
	// TODO(jpeach) Update the internal model to map which
	// services are targets of which cluster load assignments.
}

func (e *EndpointsTranslator) OnAdd(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Endpoints:
		recomputeClusterLoadAssignment(e, nil, obj)
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

		// If there are no endpoints in this object, and the old
		// object also had zero endpoints, ignore this update
		// to avoid sending a noop notification to watchers.
		if len(oldObj.Subsets) == 0 && len(newObj.Subsets) == 0 {
			return
		}

		recomputeClusterLoadAssignment(e, oldObj, newObj)

	default:
		e.Errorf("OnUpdate unexpected type %T: %#v", newObj, newObj)
	}
}

func (e *EndpointsTranslator) OnDelete(obj interface{}) {
	switch obj := obj.(type) {
	case *v1.Endpoints:
		recomputeClusterLoadAssignment(e, obj, nil)
	case k8scache.DeletedFinalStateUnknown:
		e.OnDelete(obj.Obj) // recurse into ourselves with the tombstoned value
	default:
		e.Errorf("OnDelete unexpected type %T: %#v", obj, obj)
	}
}

// Contents returns a copy of the contents of the cache.
func (e *EndpointsTranslator) Contents() []proto.Message {
	e.mu.Lock()
	defer e.mu.Unlock()

	values := make([]*v2.ClusterLoadAssignment, 0, len(e.entries))
	for _, v := range e.entries {
		values = append(values, v)
	}

	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (e *EndpointsTranslator) Query(names []string) []proto.Message {
	e.mu.Lock()
	defer e.mu.Unlock()

	values := make([]*v2.ClusterLoadAssignment, 0, len(names))
	for _, n := range names {
		v, ok := e.entries[n]
		if !ok {
			v = &v2.ClusterLoadAssignment{
				ClusterName: n,
			}
		}
		values = append(values, v)
	}

	sort.Stable(sorter.For(values))
	return protobuf.AsMessages(values)
}

func (*EndpointsTranslator) TypeURL() string { return resource.EndpointType }

// Add adds an entry to the cache. If a ClusterLoadAssignment with the same
// name exists, it is replaced.
func (e *EndpointsTranslator) Add(a *v2.ClusterLoadAssignment) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.entries == nil {
		e.entries = make(map[string]*v2.ClusterLoadAssignment)
	}
	e.entries[a.ClusterName] = a
	e.Notify(a.ClusterName)
}

// Remove removes the named entry from the cache. If the entry
// is not present in the cache, the operation is a no-op.
func (e *EndpointsTranslator) Remove(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.entries, name)
	e.Notify(name)
}

// recomputeClusterLoadAssignment recomputes the EDS cache taking into account old and new endpoints.
func recomputeClusterLoadAssignment(e *EndpointsTranslator, oldep, newep *v1.Endpoints) {
	// Skip computation if either old and new services or
	// endpoints are equal (thus also handling nil).
	if oldep == newep {
		return
	}

	if oldep == nil {
		oldep = &v1.Endpoints{
			ObjectMeta: newep.ObjectMeta,
		}
	}

	if newep == nil {
		newep = &v1.Endpoints{
			ObjectMeta: oldep.ObjectMeta,
		}
	}

	seen := make(map[string]bool)
	// add or update endpoints
	for _, s := range newep.Subsets {
		if len(s.Addresses) < 1 {
			// skip subset without ready addresses.
			continue
		}
		for _, p := range s.Ports {
			if p.Protocol != "TCP" {
				// skip non TCP ports
				continue
			}

			addresses := append([]v1.EndpointAddress{}, s.Addresses...) // shallow copy
			sort.Slice(addresses, func(i, j int) bool { return addresses[i].IP < addresses[j].IP })

			lbendpoints := make([]*envoy_api_v2_endpoint.LbEndpoint, 0, len(addresses))
			for _, a := range addresses {
				addr := envoy.SocketAddress(a.IP, int(p.Port))
				lbendpoints = append(lbendpoints, envoy.LBEndpoint(addr))
			}

			cla := &v2.ClusterLoadAssignment{
				ClusterName: envoy.ClusterLoadAssignmentName(k8s.NamespacedNameOf(newep), p.Name),
				Endpoints: []*envoy_api_v2_endpoint.LocalityLbEndpoints{{
					LbEndpoints: lbendpoints,
				}},
			}
			seen[cla.ClusterName] = true
			e.Add(cla)
		}
	}

	// iterate over the ports in the old spec, remove any were not seen.
	for _, s := range oldep.Subsets {
		if len(s.Addresses) == 0 {
			continue
		}
		for _, p := range s.Ports {
			name := envoy.ClusterLoadAssignmentName(k8s.NamespacedNameOf(newep), p.Name)
			if _, ok := seen[name]; !ok {
				// port is no longer present, remove it.
				e.Remove(name)
			}
		}
	}

}
