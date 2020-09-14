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

import "sort"

// ListenerProcessor adds an HTTP and an HTTPS listener to
// the DAG if there are virtual hosts and secure virtual
// hosts already defined as roots in the DAG.
type ListenerProcessor struct{}

// Run adds HTTP and HTTPS listeners to the DAG if there are
// virtual hosts and secure virtual hosts already defined as
// roots in the DAG.
func (p *ListenerProcessor) Run(dag *DAG, _ *KubernetesCache) {
	p.buildHTTPListener(dag)
	p.buildHTTPSListener(dag)
}

// buildHTTPListener builds a *dag.Listener for the vhosts bound to port 80.
// The list of virtual hosts will attached to the listener will be sorted
// by hostname.
func (p *ListenerProcessor) buildHTTPListener(dag *DAG) {
	var virtualhosts []Vertex
	var remove []Vertex

	for _, root := range dag.roots {
		switch obj := root.(type) {
		case *VirtualHost:
			remove = append(remove, obj)

			if obj.Valid() {
				virtualhosts = append(virtualhosts, obj)
			}
		}
	}

	// Update the DAG's roots to not include virtual hosts.
	for _, r := range remove {
		dag.RemoveRoot(r)
	}

	if len(virtualhosts) == 0 {
		return
	}

	sort.SliceStable(virtualhosts, func(i, j int) bool {
		return virtualhosts[i].(*VirtualHost).Name < virtualhosts[j].(*VirtualHost).Name
	})

	http := &Listener{
		Port:         80,
		VirtualHosts: virtualhosts,
	}

	dag.AddRoot(http)
}

// buildHTTPSListener builds a *dag.Listener for the vhosts bound to port 443.
// The list of virtual hosts will attached to the listener will be sorted
// by hostname.
func (p *ListenerProcessor) buildHTTPSListener(dag *DAG) {
	var virtualhosts []Vertex
	var remove []Vertex

	for _, root := range dag.roots {
		switch obj := root.(type) {
		case *SecureVirtualHost:
			remove = append(remove, obj)

			if obj.Valid() {
				virtualhosts = append(virtualhosts, obj)
			}
		}
	}

	// Update the DAG's roots to not include secure virtual hosts.
	for _, r := range remove {
		dag.RemoveRoot(r)
	}

	if len(virtualhosts) == 0 {
		return
	}

	sort.SliceStable(virtualhosts, func(i, j int) bool {
		return virtualhosts[i].(*SecureVirtualHost).Name < virtualhosts[j].(*SecureVirtualHost).Name
	})

	https := &Listener{
		Port:         443,
		VirtualHosts: virtualhosts,
	}

	dag.AddRoot(https)
}
