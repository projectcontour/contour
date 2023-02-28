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
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/status"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// Processor constructs part of a DAG.
type Processor interface {
	// Run executes the processor.
	Run(dag *DAG, source *KubernetesCache)
}

// ProcessorFunc adapts a function to the Processor interface.
type ProcessorFunc func(*DAG, *KubernetesCache)

func (pf ProcessorFunc) Run(dag *DAG, source *KubernetesCache) {
	if pf != nil {
		pf(dag, source)
	}
}

// Builder builds a DAG.
type Builder struct {
	// Source is the source of Kubernetes objects
	// from which to build a DAG.
	Source KubernetesCache

	// Processors is the ordered list of Processors to
	// use to build the DAG.
	Processors []Processor

	// Metrics contains Prometheus metrics.
	Metrics *metrics.Metrics
}

// Build builds and returns a new DAG by running the
// configured DAG processors, in order.
func (b *Builder) Build() *DAG {

	gatewayNSName := types.NamespacedName{}
	if b.Source.gateway != nil {
		gatewayNSName = k8s.NamespacedNameOf(b.Source.gateway)
	}
	var gatewayController gatewayapi_v1beta1.GatewayController
	if b.Source.gatewayclass != nil {
		gatewayController = b.Source.gatewayclass.Spec.ControllerName
	}

	dag := &DAG{
		StatusCache: status.NewCache(gatewayNSName, gatewayController),
		Listeners:   map[string]*Listener{},
	}

	if b.Metrics != nil {
		t := prometheus.NewTimer(b.Metrics.DAGRebuildSeconds)
		defer t.ObserveDuration()
	}

	for _, p := range b.Processors {
		p.Run(dag, &b.Source)
	}

	// Prune invalid virtual hosts, and Listeners
	// without any valid virtual hosts.
	listeners := map[string]*Listener{}

	for _, listener := range dag.Listeners {
		var vhosts []*VirtualHost
		for _, vh := range listener.VirtualHosts {
			if vh.Valid() {
				vhosts = append(vhosts, vh)
			}
		}
		listener.VirtualHosts = vhosts

		var svhosts []*SecureVirtualHost
		for _, svh := range listener.SecureVirtualHosts {
			if svh.Valid() {
				svhosts = append(svhosts, svh)
			}
		}
		listener.SecureVirtualHosts = svhosts

		if len(listener.VirtualHosts) > 0 || len(listener.SecureVirtualHosts) > 0 {
			sort.SliceStable(listener.VirtualHosts, func(i, j int) bool {
				return listener.VirtualHosts[i].Name < listener.VirtualHosts[j].Name
			})

			sort.SliceStable(listener.SecureVirtualHosts, func(i, j int) bool {
				return listener.SecureVirtualHosts[i].Name < listener.SecureVirtualHosts[j].Name
			})

			listener.vhostsByName = nil
			listener.svhostsByName = nil

			listeners[listener.Name] = listener
		}
	}

	dag.Listeners = listeners

	return dag
}
