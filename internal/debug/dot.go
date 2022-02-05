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

package debug

import (
	"fmt"
	"io"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
)

// quick and dirty dot debugging package

type dotWriter struct {
	Builder dag.DagBuilder
}

type pair struct {
	a, b interface{}
}

func (dw *dotWriter) writeDot(w io.Writer) {
	b := dw.Builder
	fmt.Fprintln(w, "digraph DAG {\nrankdir=\"LR\"")

	nodes := map[interface{}]bool{}
	edges := map[pair]bool{}

	// collect nodes and edges
	for _, listener := range b.Build().Listeners {
		nodes[listener] = true

		for _, vhost := range listener.VirtualHosts {
			edges[pair{listener, vhost}] = true
			nodes[vhost] = true

			for _, route := range vhost.Routes {
				edges[pair{vhost, route}] = true
				nodes[route] = true

				clusters := route.Clusters
				if route.MirrorPolicy != nil && route.MirrorPolicy.Cluster != nil {
					clusters = append(clusters, route.MirrorPolicy.Cluster)
				}
				for _, cluster := range clusters {
					edges[pair{route, cluster}] = true
					nodes[cluster] = true

					if service := cluster.Upstream; service != nil {
						edges[pair{cluster, service}] = true
						nodes[service] = true
					}
				}
			}
		}

		for _, vhost := range listener.SecureVirtualHosts {
			edges[pair{listener, vhost}] = true
			nodes[vhost] = true

			for _, route := range vhost.Routes {
				edges[pair{vhost, route}] = true
				nodes[route] = true

				clusters := route.Clusters
				if route.MirrorPolicy != nil && route.MirrorPolicy.Cluster != nil {
					clusters = append(clusters, route.MirrorPolicy.Cluster)
				}
				for _, cluster := range clusters {
					edges[pair{route, cluster}] = true
					nodes[cluster] = true

					if service := cluster.Upstream; service != nil {
						edges[pair{cluster, service}] = true
						nodes[service] = true
					}
				}
			}

			if vhost.TCPProxy != nil {
				edges[pair{vhost, vhost.TCPProxy}] = true
				nodes[vhost.TCPProxy] = true

				for _, cluster := range vhost.TCPProxy.Clusters {
					edges[pair{vhost.TCPProxy, cluster}] = true
					nodes[cluster] = true

					if service := cluster.Upstream; service != nil {
						edges[pair{cluster, service}] = true
						nodes[service] = true
					}
				}
			}

			if vhost.Secret != nil {
				edges[pair{vhost, vhost.Secret}] = true
				nodes[vhost.Secret] = true
			}
		}
	}

	// print nodes
	for node := range nodes {
		switch node := node.(type) {
		case *dag.Listener:
			fmt.Fprintf(w, `"%p" [shape=record, label="{listener|%s:%d}"]`+"\n", node, node.Address, node.Port)
		case *dag.VirtualHost:
			fmt.Fprintf(w, `"%p" [shape=record, label="{http://%s}"]`+"\n", node, node.Name)
		case *dag.SecureVirtualHost:
			fmt.Fprintf(w, `"%p" [shape=record, label="{https://%s}"]`+"\n", node, node.VirtualHost.Name)
		case *dag.Route:
			fmt.Fprintf(w, `"%p" [shape=record, label="{%s}"]`+"\n", node, node.PathMatchCondition.String())
		case *dag.Cluster:
			fmt.Fprintf(w, `"%p" [shape=record, label="{cluster|{%s|weight %d}}"]`+"\n", node, envoy.Clustername(node), node.Weight)
		case *dag.Service:
			fmt.Fprintf(w, `"%p" [shape=record, label="{service|%s/%s:%d}"]`+"\n",
				node, node.Weighted.ServiceNamespace, node.Weighted.ServiceName, node.Weighted.ServicePort.Port)
		case *dag.Secret:
			fmt.Fprintf(w, `"%p" [shape=record, label="{secret|%s/%s}"]`+"\n", node, node.Namespace(), node.Name())
		case *dag.TCPProxy:
			fmt.Fprintf(w, `"%p" [shape=record, label="{tcpproxy}"]`+"\n", node)

		}
	}

	// print edges
	for edge := range edges {
		fmt.Fprintf(w, `"%p" -> "%p"`+"\n", edge.a, edge.b)
	}

	fmt.Fprintln(w, "}")
}
