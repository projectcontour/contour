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
// the DAG builder if there are virtual hosts and secure
// virtual hosts already defined in the builder.
type ListenerProcessor struct {
	builder *Builder
}

// Run adds HTTP and HTTPS listeners to the DAG builder
// if there are virtual hosts and secure virtual hosts already
// defined in the builder.
func (p *ListenerProcessor) Run(builder *Builder) {
	p.builder = builder

	// reset the processor when we're done
	defer func() {
		p.builder = nil
	}()

	http := p.buildHTTPListener()
	if len(http.VirtualHosts) > 0 {
		p.builder.listeners = append(p.builder.listeners, http)
	}

	https := p.buildHTTPSListener()
	if len(https.VirtualHosts) > 0 {
		p.builder.listeners = append(p.builder.listeners, https)
	}
}

// buildHTTPListener builds a *dag.Listener for the vhosts bound to port 80.
// The list of virtual hosts will attached to the listener will be sorted
// by hostname.
func (p *ListenerProcessor) buildHTTPListener() *Listener {
	var virtualhosts = make([]Vertex, 0, len(p.builder.virtualhosts))

	for _, vh := range p.builder.virtualhosts {
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
func (p *ListenerProcessor) buildHTTPSListener() *Listener {
	var virtualhosts = make([]Vertex, 0, len(p.builder.securevirtualhosts))
	for _, svh := range p.builder.securevirtualhosts {
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
