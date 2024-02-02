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
	"html"
	"io"

	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/envoy"
)

// quick and dirty dot debugging package

type dotWriter struct {
	Builder DagBuilder
}

// DagBuilder encapsulates only the Build aspect of the dag.Builder
type DagBuilder interface {
	// Build builds and returns a new DAG
	Build() *dag.DAG
}

type pair struct {
	a, b any
}

type (
	nodeCollection map[any]bool
	edgeCollection map[pair]bool
)

func (dw *dotWriter) writeDot(w io.Writer) {
	nodes, edges := collectDag(dw.Builder)

	fmt.Fprintln(w, "digraph DAG {\nrankdir=\"LR\"")

	printNodes(nodes, w)
	printEdges(edges, w)

	fmt.Fprintln(w, "}")
}

func collectDag(b DagBuilder) (nodeCollection, edgeCollection) {
	nodes := map[any]bool{}
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
				for _, mp := range route.MirrorPolicies {
					if mp.Cluster != nil {
						clusters = append(clusters, mp.Cluster)
					}
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
				for _, mp := range route.MirrorPolicies {
					if mp.Cluster != nil {
						clusters = append(clusters, mp.Cluster)
					}
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

		if listener.TCPProxy != nil {
			edges[pair{listener, listener.TCPProxy}] = true
			nodes[listener.TCPProxy] = true
			for _, cluster := range listener.TCPProxy.Clusters {
				edges[pair{listener.TCPProxy, cluster}] = true
				nodes[cluster] = true

				if service := cluster.Upstream; service != nil {
					edges[pair{cluster, service}] = true
					nodes[service] = true
				}
			}
		}
	}

	return nodes, edges
}

func printNodes(nodes nodeCollection, w io.Writer) {
	// print nodes
	for node := range nodes {
		switch node := node.(type) {
		case *dag.Listener:
			fmt.Fprintf(w, `"%p" [shape=record, label="{listener|%s:%d}"]`+"\n", node, html.EscapeString(node.Address), node.Port)
		case *dag.VirtualHost:
			fmt.Fprintf(w, `"%p" [shape=record, label="{http://%s}"]`+"\n", node, html.EscapeString(node.Name))
		case *dag.SecureVirtualHost:
			fmt.Fprintf(w, `"%p" [shape=record, label="{https://%s}"]`+"\n", node, html.EscapeString(node.VirtualHost.Name))
		case *dag.Route:
			fmt.Fprintf(w, `"%p" [shape=record, label="{%s}"]`+"\n", node, html.EscapeString(node.PathMatchCondition.String()))
		case *dag.Cluster:
			fmt.Fprintf(w, `"%p" [shape=record, label="{cluster|{%s|weight %d}}"]`+"\n", node, html.EscapeString(envoy.Clustername(node)), node.Weight)
		case *dag.Service:
			fmt.Fprintf(w, `"%p" [shape=record, label="{service|%s/%s:%d}"]`+"\n",
				node, html.EscapeString(node.Weighted.ServiceNamespace), html.EscapeString(node.Weighted.ServiceName), node.Weighted.ServicePort.Port)
		case *dag.Secret:
			fmt.Fprintf(w, `"%p" [shape=record, label="{secret|%s/%s}"]`+"\n", node, html.EscapeString(node.Namespace()), html.EscapeString(node.Name()))
		case *dag.TCPProxy:
			fmt.Fprintf(w, `"%p" [shape=record, label="{tcpproxy}"]`+"\n", node)

		}
	}
}

func printEdges(edges edgeCollection, w io.Writer) {
	// print edges
	for edge := range edges {
		fmt.Fprintf(w, `"%p" -> "%p"`+"\n", edge.a, edge.b)
	}
}
