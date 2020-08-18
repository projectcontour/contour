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

package dag

import (
	"sort"

	"github.com/projectcontour/contour/internal/k8s"

	"github.com/projectcontour/contour/internal/annotation"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// BuildContext holds the information process from ingress
// resources and is used to build out a new DAG.
type BuildContext struct {
	services map[RouteServiceName]*Service
	secrets  map[types.NamespacedName]*Secret

	virtualhosts       map[string]*VirtualHost
	securevirtualhosts map[string]*SecureVirtualHost

	orphaned map[types.NamespacedName]bool

	StatusWriter
}

func (b *BuildContext) reset() {
	b.services = make(map[RouteServiceName]*Service, len(b.services))
	b.secrets = make(map[types.NamespacedName]*Secret, len(b.secrets))
	b.orphaned = make(map[types.NamespacedName]bool, len(b.orphaned))

	b.virtualhosts = make(map[string]*VirtualHost)
	b.securevirtualhosts = make(map[string]*SecureVirtualHost)

	b.statuses = make(map[types.NamespacedName]Status, len(b.statuses))
}

func (b *BuildContext) addService(svc *v1.Service, port v1.ServicePort) *Service {
	name := k8s.NamespacedNameOf(svc)
	s := &Service{
		Weighted: WeightedService{
			ServiceName:      name.Name,
			ServiceNamespace: name.Namespace,
			ServicePort:      port,
			Weight:           1,
		},
		Protocol:           upstreamProtocol(svc, port),
		MaxConnections:     annotation.MaxConnections(svc),
		MaxPendingRequests: annotation.MaxPendingRequests(svc),
		MaxRequests:        annotation.MaxRequests(svc),
		MaxRetries:         annotation.MaxRetries(svc),
		ExternalName:       externalName(svc),
	}

	b.services[RouteServiceName{
		Name:      name.Name,
		Namespace: name.Namespace,
		Port:      port.Port,
	}] = s

	return s
}

// lookupVirtualHost returns a virtualhost if existing
// or creates a new one before returning.
func (b *BuildContext) lookupVirtualHost(name string) *VirtualHost {
	vh, ok := b.virtualhosts[name]
	if !ok {
		vh := &VirtualHost{
			Name: name,
		}
		b.virtualhosts[vh.Name] = vh
		return vh
	}
	return vh
}

func (b *BuildContext) lookupSecureVirtualHost(name string) *SecureVirtualHost {
	svh, ok := b.securevirtualhosts[name]
	if !ok {
		svh := &SecureVirtualHost{
			VirtualHost: VirtualHost{
				Name: name,
			},
		}
		b.securevirtualhosts[svh.VirtualHost.Name] = svh
		return svh
	}
	return svh
}

// buildHTTPListener builds a *dag.Listener for the vhosts bound to port 80.
// The list of virtual hosts will attached to the listener will be sorted
// by hostname.
func (b *BuildContext) buildHTTPListener() *Listener {
	var virtualhosts = make([]Vertex, 0, len(b.virtualhosts))

	for _, vh := range b.virtualhosts {
		if vh.Valid() {
			virtualhosts = append(virtualhosts, vh)
		}
	}
	sort.SliceStable(virtualhosts, func(i, j int) bool {
		return virtualhosts[i].(*VirtualHost).Name < virtualhosts[j].(*VirtualHost).Name
	})
	return &Listener{
		Port:         80,
		VirtualHosts: virtualhosts,
	}
}

// buildHTTPSListener builds a *dag.Listener for the vhosts bound to port 443.
// The list of virtual hosts will attached to the listener will be sorted
// by hostname.
func (b *BuildContext) buildHTTPSListener() *Listener {
	var virtualhosts = make([]Vertex, 0, len(b.securevirtualhosts))
	for _, svh := range b.securevirtualhosts {
		if svh.Valid() {
			virtualhosts = append(virtualhosts, svh)
		}
	}
	sort.SliceStable(virtualhosts, func(i, j int) bool {
		return virtualhosts[i].(*SecureVirtualHost).Name < virtualhosts[j].(*SecureVirtualHost).Name
	})
	return &Listener{
		Port:         443,
		VirtualHosts: virtualhosts,
	}
}

// setOrphaned records an HTTPProxy resource as orphaned.
func (b *BuildContext) setOrphaned(obj k8s.Object) {
	m := types.NamespacedName{
		Name:      obj.GetObjectMeta().GetName(),
		Namespace: obj.GetObjectMeta().GetNamespace(),
	}
	b.orphaned[m] = true
}

type vhost interface {
	addRoute(*Route)
}

// addRoutes adds all routes to the vhost supplied.
func addRoutes(vhost vhost, routes []*Route) {
	for _, route := range routes {
		vhost.addRoute(route)
	}
}
