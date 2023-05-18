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

// nolint:revive
const (
	HTTP_LISTENER_NAME  = "ingress_http"
	HTTPS_LISTENER_NAME = "ingress_https"
)

// ListenerProcessor adds an HTTP and an HTTPS listener to
// the DAG.
type ListenerProcessor struct {
	HTTPAddress  string
	HTTPPort     int
	HTTPSAddress string
	HTTPSPort    int
}

// Run adds HTTP and HTTPS listeners to the DAG.
func (p *ListenerProcessor) Run(dag *DAG, cache *KubernetesCache) {
	dag.Listeners[HTTP_LISTENER_NAME] = &Listener{
		Name:            HTTP_LISTENER_NAME,
		Address:         p.HTTPAddress,
		Port:            intOrDefault(p.HTTPPort, 8080),
		RouteConfigName: "ingress_http",
		vhostsByName:    map[string]*VirtualHost{},
	}

	dag.Listeners[HTTPS_LISTENER_NAME] = &Listener{
		Name:                        HTTPS_LISTENER_NAME,
		Address:                     p.HTTPSAddress,
		Port:                        intOrDefault(p.HTTPSPort, 8443),
		RouteConfigName:             "https",
		FallbackCertRouteConfigName: "ingress_fallbackcert",
		svhostsByName:               map[string]*SecureVirtualHost{},
	}
}

func intOrDefault(i, def int) int {
	if i > 0 {
		return i
	}
	return def
}
